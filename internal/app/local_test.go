package app_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestRetainedConsolePreventsSilentRepositoryRebinding(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Console binding", RequestID: "p"}).(model.Project)
	folder := t.TempDir()
	repo := filepath.Join(folder, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	repoGit(t, repo, "init", "-b", "main")
	os.WriteFile(filepath.Join(repo, "fixture.txt"), []byte("fixture"), 0600)
	repoGit(t, repo, "add", ".")
	repoGit(t, repo, "commit", "-m", "Seed")
	p = call(t, svc, token, "configureLocalRepository", app.RepositoryInput{ProjectID: p.ID, ExpectedVersion: p.Version, Repository: repo, StorageRoot: filepath.Join(folder, "worktrees")}).(model.Project)
	for _, state := range []string{"running", "unknown", "exited"} {
		if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
			st.LocalSessions = map[string]model.LocalSession{"H1": {ID: "H1", Creator: "admin", ProjectID: p.ID, State: state, Worktree: model.Worktree{Owner: "retained-owner", Path: filepath.Join(folder, "retained-console")}, BindingVersion: p.Repository.Version}}
			return true, nil
		}); err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(app.RepositoryInput{ProjectID: p.ID, ExpectedVersion: p.Version, Repository: repo, StorageRoot: filepath.Join(folder, "replacement-worktrees")})
		if _, err := svc.Call(context.Background(), token, "configureLocalRepository", body); err == nil || app.ErrorResponse(err).Code != "migration_required" {
			t.Fatal("console artifacts were silently rebound", state, err)
		}
	}
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		st.LocalSessions = map[string]model.LocalSession{"H2": {ID: "H2", Creator: "admin", ProjectID: p.ID, State: "exited", Worktree: model.Worktree{Path: p.Repository.Repository}, BindingVersion: p.Repository.Version}}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	result := call(t, svc, token, "configureLocalRepository", app.RepositoryInput{ProjectID: p.ID, ExpectedVersion: p.Version, Repository: repo, StorageRoot: filepath.Join(folder, "replacement-worktrees")}).(model.Project)
	if result.Repository.Version != p.Repository.Version+1 {
		t.Fatal("completed root console blocked rebinding")
	}

}
