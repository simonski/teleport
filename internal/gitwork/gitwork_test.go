package gitwork_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"teleport/internal/gitwork"
)

func git(t *testing.T, path string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", path}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@localhost", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@localhost")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
	return string(out)
}
func TestOwnedWorktreeCandidateAndExactFastForward(t *testing.T) {
	ctx := context.Background()
	folder := t.TempDir()
	repo := filepath.Join(folder, "repo")
	os.Mkdir(repo, 0700)
	git(t, repo, "init", "-b", "main")
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("Original\n"), 0600)
	git(t, repo, "add", "README.md")
	git(t, repo, "commit", "-m", "Initial")
	binding, err := gitwork.Inspect(ctx, repo, filepath.Join(folder, "worktrees"))
	if err != nil {
		t.Fatal(err)
	}
	work, err := gitwork.Prepare(ctx, binding, "fixture-owner-1234", 7)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := gitwork.Prepare(ctx, binding, "fixture-owner-1234", 7)
	if err != nil || retry != work {
		t.Fatal("worktree retry did not retain ownership")
	}
	os.WriteFile(filepath.Join(work.Path, "README.md"), []byte("Delivered\n"), 0600)
	git(t, work.Path, "add", "README.md")
	git(t, work.Path, "commit", "-m", "Deliver change")
	candidate, err := gitwork.Snapshot(ctx, binding, work)
	if err != nil {
		t.Fatal(err)
	}
	if !candidate.ChecksPassed || candidate.Commit == work.Base || candidate.Diff == "" {
		t.Fatal("candidate did not contain actual evidence")
	}
	if err = gitwork.Integrate(ctx, binding, work, candidate); err != nil {
		t.Fatal(err)
	}
	if err = gitwork.Integrate(ctx, binding, work, candidate); err != nil {
		t.Fatal("same Git operation did not reconcile idempotently:", err)
	}
	data, _ := os.ReadFile(filepath.Join(repo, "README.md"))
	if string(data) != "Delivered\n" {
		t.Fatal("main was not actually integrated")
	}
}

func TestUnrelatedPathAndStaleMainArePreserved(t *testing.T) {
	ctx := context.Background()
	folder := t.TempDir()
	repo := filepath.Join(folder, "repo")
	os.Mkdir(repo, 0700)
	git(t, repo, "init", "-b", "main")
	os.WriteFile(filepath.Join(repo, "code.txt"), []byte("Original\n"), 0600)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "Initial")
	binding, err := gitwork.Inspect(ctx, repo, filepath.Join(folder, "worktrees"))
	if err != nil {
		t.Fatal(err)
	}
	work, err := gitwork.Prepare(ctx, binding, "fixture-owner-5678", 8)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(work.Path, "code.txt"), []byte("Candidate\n"), 0600)
	git(t, work.Path, "add", ".")
	git(t, work.Path, "commit", "-m", "Candidate")
	candidate, err := gitwork.Snapshot(ctx, binding, work)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "other.txt"), []byte("Other work\n"), 0600)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "Independent main change")
	if err = gitwork.Integrate(ctx, binding, work, candidate); err == nil {
		t.Fatal("stale base integrated")
	}
	data, _ := os.ReadFile(filepath.Join(repo, "other.txt"))
	if string(data) != "Other work\n" {
		t.Fatal("unrelated work was destroyed")
	}
}

func TestIntegrationDoesNotOverwriteIgnoredLocalFiles(t *testing.T) {
	ctx := context.Background()
	folder := t.TempDir()
	repo := filepath.Join(folder, "repo")
	os.Mkdir(repo, 0700)
	git(t, repo, "init", "-b", "main")
	os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("local.env\n"), 0600)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "Ignore local file")
	binding, err := gitwork.Inspect(ctx, repo, filepath.Join(folder, "worktrees"))
	if err != nil {
		t.Fatal(err)
	}
	work, err := gitwork.Prepare(ctx, binding, "fixture-owner-ignore", 10)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(work.Path, "local.env"), []byte("candidate value\n"), 0600)
	git(t, work.Path, "add", "-f", "local.env")
	git(t, work.Path, "commit", "-m", "Add candidate file")
	candidate, err := gitwork.Snapshot(ctx, binding, work)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "local.env"), []byte("unrelated local value\n"), 0600)
	if err = gitwork.Integrate(ctx, binding, work, candidate); err == nil {
		t.Fatal("ignored local file was overwritten")
	}
	data, _ := os.ReadFile(filepath.Join(repo, "local.env"))
	if string(data) != "unrelated local value\n" {
		t.Fatal("unrelated ignored data was changed")
	}
}

func TestIncludedFiltersNeverExecuteDuringCheckout(t *testing.T) {
	ctx := context.Background()
	folder := t.TempDir()
	repo := filepath.Join(folder, "repo")
	os.Mkdir(repo, 0700)
	git(t, repo, "init", "-b", "main")
	os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("*.txt filter=spy\n"), 0600)
	os.WriteFile(filepath.Join(repo, "code.txt"), []byte("Original\n"), 0600)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "Initial")
	script := filepath.Join(folder, "filter.sh")
	marker := filepath.Join(folder, "filter-ran")
	os.WriteFile(script, []byte("#!/bin/sh\nprintf touched > \"$1\"\ncat\n"), 0700)
	quote := func(v string) string { return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'" }
	command := "sh " + quote(script) + " " + quote(marker)
	configuration := filepath.Join(folder, "included.config")
	git(t, repo, "config", "--file", configuration, "filter.spy.smudge", command)
	git(t, repo, "config", "--file", configuration, "filter.spy.required", "true")
	git(t, repo, "config", "include.path", configuration)
	binding, err := gitwork.Inspect(ctx, repo, filepath.Join(folder, "worktrees"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = gitwork.Prepare(ctx, binding, "fixture-filter-owner", 11); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("an included repository filter executed during checkout")
	}
}

func TestWorktreeSpecificFiltersNeverExecuteDuringInspection(t *testing.T) {
	ctx := context.Background()
	folder := t.TempDir()
	repo := filepath.Join(folder, "repo")
	os.Mkdir(repo, 0700)
	git(t, repo, "init", "-b", "main")
	os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("*.txt filter=spy\n"), 0600)
	os.WriteFile(filepath.Join(repo, "code.txt"), []byte("Original\n"), 0600)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "Initial")
	binding, err := gitwork.Inspect(ctx, repo, filepath.Join(folder, "worktrees"))
	if err != nil {
		t.Fatal(err)
	}
	work, err := gitwork.Prepare(ctx, binding, "fixture-filter-specific", 12)
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(folder, "filter.sh")
	marker := filepath.Join(folder, "filter-ran")
	os.WriteFile(script, []byte("#!/bin/sh\nprintf touched > \"$1\"\ncat\n"), 0700)
	quote := func(v string) string { return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'" }
	command := "sh " + quote(script) + " " + quote(marker)
	git(t, repo, "config", "extensions.worktreeConfig", "true")
	git(t, work.Path, "config", "--worktree", "filter.spy.clean", command)
	git(t, work.Path, "config", "--worktree", "filter.spy.required", "true")
	future := time.Now().Add(time.Second)
	os.Chtimes(filepath.Join(work.Path, "code.txt"), future, future)
	gitwork.Snapshot(ctx, binding, work)
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("worktree-specific clean filter executed during inspection")
	}
}

func TestVerifyCheckoutAcceptsCurrentBranchAndRejectsOtherPaths(t *testing.T) {
	folder := t.TempDir()
	repo := filepath.Join(folder, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "file"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "fixture")
	git(t, repo, "checkout", "-b", "feature")
	canonical, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err = gitwork.VerifyCheckout(context.Background(), canonical); err != nil {
		t.Fatal("feature branch checkout refused", err)
	}
	sub := filepath.Join(canonical, "sub")
	if err = os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	if err = gitwork.VerifyCheckout(context.Background(), sub); err == nil {
		t.Fatal("repository subdirectory accepted as root")
	}
	other := filepath.Join(folder, "alias")
	if err = os.Symlink(canonical, other); err != nil {
		t.Fatal(err)
	}
	if err = gitwork.VerifyCheckout(context.Background(), other); err == nil {
		t.Fatal("changed symlink target accepted as exact grant")
	}
}
