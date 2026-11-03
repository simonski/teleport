package app_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestProjectRootCreationAndInternalWorktreeStorage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Default", RequestID: "default-root"}).(model.Project)
	if p.Root != "~/code/teleport" || p.Repository != nil {
		t.Fatal("default root or grant changed", p)
	}
	retry := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Default", RequestID: "default-root"}).(model.Project)
	if retry.ID != p.ID {
		t.Fatal("retry created a new project")
	}
	repo := filepath.Join(home, "code", "fixture")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	repoGit(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	repoGit(t, repo, "add", ".")
	repoGit(t, repo, "commit", "-m", "fixture")
	custom := "~/code/fixture"
	p = call(t, svc, token, "saveProject", app.SaveProjectInput{ID: p.ID, ExpectedVersion: p.Version, Root: &custom}).(model.Project)
	if p.Root != custom {
		t.Fatal("root was not saved")
	}
	p = call(t, svc, token, "configureLocalRepository", app.RepositoryInput{ProjectID: p.ID, ExpectedVersion: p.Version}).(model.Project)
	canonicalHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	if p.Repository == nil || p.Repository.Repository != filepath.Join(canonicalHome, "code", "fixture") || p.Root != custom || !strings.HasPrefix(p.Repository.StorageRoot, filepath.Join(canonicalHome, ".teleport", "worktrees")+string(filepath.Separator)) {
		t.Fatal("binding did not use project root and internal storage", p)
	}
	retryBinding := call(t, svc, token, "configureLocalRepository", app.RepositoryInput{ProjectID: p.ID, ExpectedVersion: p.Version}).(model.Project)
	if retryBinding.Version != p.Version || retryBinding.Repository.StorageRoot != p.Repository.StorageRoot {
		t.Fatal("binding retry changed existing storage")
	}
	invalid, _ := json.Marshal(app.CreateProjectInput{Name: "Invalid", Root: "relative/path", RequestID: "invalid-root"})
	if _, err := svc.Call(context.Background(), token, "createProject", invalid); err == nil {
		t.Fatal("relative project root accepted")
	}
	different := "~/elsewhere"
	raw, _ := json.Marshal(app.SaveProjectInput{ID: p.ID, ExpectedVersion: p.Version, Root: &different})
	if _, err := svc.Call(context.Background(), token, "saveProject", raw); err == nil {
		t.Fatal("metadata edit bypassed the binding grant")
	}
}
func TestWelcomeProjectRootIsSavedWithoutGrant(t *testing.T) {
	svc, token := fixture(t)
	draft := call(t, svc, token, "saveWelcomeSetup", app.SaveWelcomeSetupInput{Step: 5, ProjectName: "Welcome root", ProjectKey: "welroot", ProjectRoot: "~/code/welcome", Attention: "quiet"}).(model.WelcomeSetup)
	done := call(t, svc, token, "completeWelcomeSetup", app.CompleteWelcomeSetupInput{ExpectedVersion: draft.Version, Confirm: true}).(model.WelcomeSetup)
	projects := call(t, svc, token, "listProjects", app.ListInput{}).(app.Page[model.Project])
	if len(projects.Items) != 1 || projects.Items[0].Root != "~/code/welcome" || projects.Items[0].Repository != nil || done.ProjectRoot != "~/code/welcome" {
		t.Fatal("welcome root or grant incorrect", done, projects)
	}
}
