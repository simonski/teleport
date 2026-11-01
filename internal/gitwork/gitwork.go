// Package gitwork performs bounded controller-owned local Git operations, never shell commands.
package gitwork

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type Binding struct {
	Repository  string `json:"repository"`
	StorageRoot string `json:"storageRoot"`
}
type Worktree struct {
	ConsoleID string `json:"consoleId,omitempty"`
	Owner     string `json:"owner"`
	TicketID  int64  `json:"ticketId"`
	Path      string `json:"path"`
	Branch    string `json:"branch"`
	Base      string `json:"base"`
}
type Candidate struct {
	Base         string `json:"base"`
	Commit       string `json:"commit"`
	Diff         string `json:"diff"`
	ChecksPassed bool   `json:"checksPassed"`
	CheckOutput  string `json:"checkOutput"`
	Fingerprint  string `json:"fingerprint"`
}
type bounded struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	overflow bool
}

func (b *bounded) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	left := (256 << 10) - b.buffer.Len()
	if len(p) > left {
		b.overflow = true
		b.buffer.Write(p[:left])
	} else {
		b.buffer.Write(p)
	}
	return len(p), nil
}

func rawGit(ctx context.Context, dir string, configuration, args []string) (string, error) {
	prefix := []string{"--no-pager", "-C", dir, "-c", "core.hooksPath=" + os.DevNull, "-c", "core.fsmonitor=false", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "protocol.file.allow=never"}
	prefix = append(prefix, configuration...)
	cmd := exec.CommandContext(ctx, "git", append(prefix, args...)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "GIT_AUTHOR_NAME=teleport", "GIT_AUTHOR_EMAIL=teleport@localhost", "GIT_COMMITTER_NAME=teleport", "GIT_COMMITTER_EMAIL=teleport@localhost", "LC_ALL=C"}
	output := &bounded{}
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	text := output.buffer.String()
	if output.overflow {
		return "", errors.New("Git evidence exceeds the 256 KiB capture limit")
	}
	if err != nil {
		return text, fmt.Errorf("Git operation failed: %w: %s", err, strings.TrimSpace(text))
	}
	return text, nil
}
func git(ctx context.Context, dir string, args ...string) (string, error) {
	keys, err := rawGit(ctx, dir, nil, []string{"config", "--includes", "--name-only", "--get-regexp", `^filter\..*\.(smudge|clean|process|required)$`})
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return "", err
		}
	}
	configuration := []string{}
	for _, key := range strings.Fields(keys) {
		value := ""
		if strings.HasSuffix(key, ".required") {
			value = "false"
		}
		configuration = append(configuration, "-c", key+"="+value)
	}
	return rawGit(ctx, dir, configuration, args)
}
func potentialPath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("repository and storage paths must be absolute")
	}
	clean := filepath.Clean(path)
	if real, err := filepath.EvalSymlinks(clean); err == nil {
		return real, nil
	}
	parent := filepath.Dir(clean)
	if parent == clean {
		return "", errors.New("path cannot be resolved")
	}
	real, err := potentialPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(real, filepath.Base(clean)), nil
}
func contained(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func Inspect(ctx context.Context, repository, storage string) (Binding, error) {
	repo, err := potentialPath(repository)
	if err != nil {
		return Binding{}, err
	}
	root, err := potentialPath(storage)
	if err != nil {
		return Binding{}, err
	}
	if contained(repo, root) {
		return Binding{}, errors.New("worktree storage must be outside the main checkout")
	}
	top, err := git(ctx, repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return Binding{}, err
	}
	actual, err := potentialPath(strings.TrimSpace(top))
	if err != nil || actual != repo {
		return Binding{}, errors.New("grant the exact repository root, not an inferred parent")
	}
	branch, err := git(ctx, repo, "symbolic-ref", "--short", "HEAD")
	if err != nil || strings.TrimSpace(branch) != "main" {
		return Binding{}, errors.New("the authoritative checkout must have main checked out")
	}
	if _, err = git(ctx, repo, "rev-parse", "--verify", "refs/heads/main"); err != nil {
		return Binding{}, err
	}
	files, err := git(ctx, repo, "ls-tree", "--name-only", "HEAD")
	if err != nil || strings.TrimSpace(files) == "" {
		return Binding{}, errors.New("the repository needs a committed codebase")
	}
	return Binding{repo, root}, nil
}
func validateBinding(ctx context.Context, b Binding) error {
	current, err := Inspect(ctx, b.Repository, b.StorageRoot)
	if err != nil {
		return err
	}
	if current != b {
		return errors.New("binding paths changed after authorization")
	}
	return nil
}
func manifestPath(b Binding, w Worktree) string {
	if w.ConsoleID != "" {
		return filepath.Join(b.StorageRoot, ".ownership", w.Owner+"-console-"+w.ConsoleID+".json")
	}
	return filepath.Join(b.StorageRoot, ".ownership", w.Owner+"-"+strconv.FormatInt(w.TicketID, 10)+".json")
}
func validateWork(ctx context.Context, b Binding, w Worktree) error {
	if err := validateBinding(ctx, b); err != nil {
		return err
	}
	validIdentity := w.TicketID > 0 && w.ConsoleID == "" || w.TicketID == 0 && regexp.MustCompile(`^H[1-9][0-9]{0,15}$`).MatchString(w.ConsoleID)
	if !regexp.MustCompile(`^[a-z0-9-]{8,80}$`).MatchString(w.Owner) || !validIdentity || !contained(b.StorageRoot, w.Path) {
		return errors.New("invalid worktree identity")
	}
	actualPath, err := potentialPath(w.Path)
	if err != nil || actualPath != w.Path {
		return errors.New("worktree path changed after authorization")
	}
	manifest, err := potentialPath(manifestPath(b, w))
	if err != nil || !contained(b.StorageRoot, manifest) {
		return errors.New("ownership manifest escaped storage")
	}
	data, err := os.ReadFile(manifestPath(b, w))
	if err != nil {
		return errors.New("owned worktree manifest unavailable")
	}
	var saved Worktree
	if json.Unmarshal(data, &saved) != nil || saved != w {
		return errors.New("worktree manifest does not match the reserved operation")
	}
	branch, err := git(ctx, w.Path, "symbolic-ref", "--short", "HEAD")
	if err != nil || strings.TrimSpace(branch) != w.Branch {
		return errors.New("owned branch changed")
	}
	common, err := git(ctx, w.Path, "rev-parse", "--git-common-dir")
	if err != nil {
		return err
	}
	commonPath := strings.TrimSpace(common)
	if !filepath.IsAbs(commonPath) {
		commonPath = filepath.Join(w.Path, commonPath)
	}
	commonPath, err = potentialPath(commonPath)
	if err != nil {
		return err
	}
	repoCommon, err := git(ctx, b.Repository, "rev-parse", "--git-common-dir")
	if err != nil {
		return err
	}
	rootCommon := strings.TrimSpace(repoCommon)
	if !filepath.IsAbs(rootCommon) {
		rootCommon = filepath.Join(b.Repository, rootCommon)
	}
	rootCommon, err = potentialPath(rootCommon)
	if err != nil || commonPath != rootCommon {
		return errors.New("worktree belongs to another repository")
	}
	return nil
}
func Prepare(ctx context.Context, b Binding, owner string, ticketID int64) (Worktree, error) {
	return prepare(ctx, b, owner, ticketID, "")
}
func PrepareConsole(ctx context.Context, b Binding, owner, consoleID string) (Worktree, error) {
	return prepare(ctx, b, owner, 0, consoleID)
}
func Verify(ctx context.Context, b Binding, w Worktree) error { return validateWork(ctx, b, w) }
func prepare(ctx context.Context, b Binding, owner string, ticketID int64, consoleID string) (Worktree, error) {
	if err := validateBinding(ctx, b); err != nil {
		return Worktree{}, err
	}
	validIdentity := ticketID > 0 && consoleID == "" || ticketID == 0 && regexp.MustCompile(`^H[1-9][0-9]{0,15}$`).MatchString(consoleID)
	if !regexp.MustCompile(`^[a-z0-9-]{8,80}$`).MatchString(owner) || !validIdentity {
		return Worktree{}, errors.New("invalid owner or ticket identity")
	}
	name := owner + "-" + strconv.FormatInt(ticketID, 10)
	w := Worktree{Owner: owner, TicketID: ticketID, Path: filepath.Join(b.StorageRoot, "tk-"+name), Branch: "tk/" + owner + "/ticket-" + strconv.FormatInt(ticketID, 10)}
	if consoleID != "" {
		w.ConsoleID = consoleID
		w.Path = filepath.Join(b.StorageRoot, "tk-"+owner+"-console-"+consoleID)
		w.Branch = "tk/" + owner + "/console-" + consoleID
	}
	file := manifestPath(b, w)
	resolved, err := potentialPath(file)
	if err != nil || !contained(b.StorageRoot, resolved) {
		return Worktree{}, errors.New("ownership manifest escaped storage")
	}
	if data, err := os.ReadFile(file); err == nil {
		var saved Worktree
		if json.Unmarshal(data, &saved) != nil || saved.Owner != owner || saved.TicketID != ticketID || saved.ConsoleID != consoleID || saved.Path != w.Path || saved.Branch != w.Branch {
			return Worktree{}, errors.New("existing ownership manifest conflicts")
		}
		w = saved
		if _, err = os.Stat(w.Path); err == nil {
			if err = validateWork(ctx, b, w); err != nil {
				return Worktree{}, err
			}
			return w, nil
		}
	}
	if w.Base == "" {
		base, err := git(ctx, b.Repository, "rev-parse", "refs/heads/main")
		if err != nil {
			return Worktree{}, err
		}
		w.Base = strings.TrimSpace(base)
		if _, err = os.Stat(w.Path); !os.IsNotExist(err) {
			return Worktree{}, errors.New("refusing an existing unowned path")
		}
		if err = os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			return Worktree{}, err
		}
		encoded, _ := json.Marshal(w)
		f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return Worktree{}, err
		}
		_, writeErr := f.Write(encoded)
		closeErr := f.Close()
		if writeErr != nil {
			return Worktree{}, writeErr
		}
		if closeErr != nil {
			return Worktree{}, closeErr
		}
	}
	if _, err := git(ctx, b.Repository, "worktree", "add", "--no-checkout", "-b", w.Branch, "--", w.Path, w.Base); err != nil {
		return Worktree{}, err
	}
	// Resolve effective configuration in the new worktree/branch before its first checkout.
	if _, err := git(ctx, w.Path, "reset", "--hard", w.Base); err != nil {
		return Worktree{}, err
	}
	if err := validateWork(ctx, b, w); err != nil {
		return Worktree{}, err
	}
	return w, nil
}

// Recover reads existing facts only. It never creates a branch, checks out files or asserts process exit.
func Recover(ctx context.Context, b Binding, owner string, ticketID int64) (Worktree, error) {
	if err := validateBinding(ctx, b); err != nil {
		return Worktree{}, err
	}
	if !regexp.MustCompile(`^[a-z0-9-]{8,80}$`).MatchString(owner) || ticketID < 1 {
		return Worktree{}, errors.New("invalid identity")
	}
	w := Worktree{Owner: owner, TicketID: ticketID}
	path, err := potentialPath(manifestPath(b, w))
	if err != nil || !contained(b.StorageRoot, path) {
		return Worktree{}, errors.New("manifest escaped storage")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Worktree{}, err
	}
	if err = json.Unmarshal(data, &w); err != nil {
		return Worktree{}, err
	}
	if w.Owner != owner || w.TicketID != ticketID {
		return Worktree{}, errors.New("ownership mismatch")
	}
	if err = validateWork(ctx, b, w); err != nil {
		return Worktree{}, err
	}
	return w, nil
}
func Snapshot(ctx context.Context, b Binding, w Worktree) (Candidate, error) {
	if err := validateWork(ctx, b, w); err != nil {
		return Candidate{}, err
	}
	status, err := git(ctx, w.Path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return Candidate{}, err
	}
	if strings.TrimSpace(status) != "" {
		return Candidate{}, errors.New("commit or resolve retained work before submitting a candidate")
	}
	head, err := git(ctx, w.Path, "rev-parse", "HEAD")
	if err != nil {
		return Candidate{}, err
	}
	head = strings.TrimSpace(head)
	if head == w.Base {
		return Candidate{}, errors.New("no committed delivery exists on the owned branch")
	}
	if _, err = git(ctx, w.Path, "merge-base", "--is-ancestor", w.Base, head); err != nil {
		return Candidate{}, errors.New("candidate is not based on its pinned main commit")
	}
	merges, err := git(ctx, w.Path, "rev-list", "--min-parents=2", w.Base+".."+head)
	if err != nil || strings.TrimSpace(merges) != "" {
		return Candidate{}, errors.New("candidate history must remain linear")
	}
	diff, err := git(ctx, w.Path, "diff", "--no-ext-diff", "--no-textconv", "--binary", w.Base, head, "--")
	if err != nil {
		return Candidate{}, err
	}
	checks, checkErr := git(ctx, w.Path, "diff", "--check", "--no-ext-diff", "--no-textconv", w.Base, head, "--")
	c := Candidate{Base: w.Base, Commit: head, Diff: diff, ChecksPassed: checkErr == nil, CheckOutput: strings.TrimSpace(checks)}
	if checkErr == nil {
		c.CheckOutput = "git diff --check passed"
	}
	encoded, _ := json.Marshal(c)
	hash := sha256.Sum256(encoded)
	c.Fingerprint = hex.EncodeToString(hash[:])
	return c, nil
}
func Integrate(ctx context.Context, b Binding, w Worktree, c Candidate) error {
	current, err := Snapshot(ctx, b, w)
	if err != nil {
		return err
	}
	if !c.ChecksPassed || current.Fingerprint != c.Fingerprint {
		return errors.New("candidate or verification basis changed after review")
	}
	main, err := git(ctx, b.Repository, "rev-parse", "refs/heads/main")
	if err != nil {
		return err
	}
	main = strings.TrimSpace(main)
	if main == c.Commit {
		return nil
	}
	if main != c.Base {
		return errors.New("main advanced; rebase and obtain fresh review before approval")
	}
	if _, err = git(ctx, b.Repository, "diff", "--quiet", "--no-ext-diff", "--no-textconv", "--"); err != nil {
		return errors.New("main has tracked changes; preserve them before integration")
	}
	if _, err = git(ctx, b.Repository, "diff", "--cached", "--quiet", "--no-ext-diff", "--no-textconv", "HEAD", "--"); err != nil {
		return errors.New("main has staged changes; preserve them before integration")
	}
	if _, err = git(ctx, b.Repository, "merge", "--ff-only", "--no-overwrite-ignore", "--no-edit", "--no-stat", c.Commit); err != nil {
		return err
	}
	actual, err := git(ctx, b.Repository, "rev-parse", "refs/heads/main")
	if err != nil || strings.TrimSpace(actual) != c.Commit {
		return errors.New("integration outcome needs reconciliation")
	}
	return nil
}
