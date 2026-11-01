package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestWelcomeProgressIsPrivateVersionedAndStartsNothing(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	initial := call(t, svc, token, "getWelcomeSetup", struct{}{}).(app.WelcomeSetupResult)
	if initial.Setup.Version != 0 || initial.Setup.Completed || !initial.CanBindRepository {
		t.Fatal("invalid fresh setup", initial)
	}
	in := app.SaveWelcomeSetupInput{Step: 1, ProjectName: "Private setup", ProjectKey: "private-setup", Harness: "codex", Attention: "quiet"}
	saved := call(t, svc, token, "saveWelcomeSetup", in).(model.WelcomeSetup)
	if saved.ID == "" || saved.Version != 1 {
		t.Fatal("draft identity missing", saved)
	}
	denied := func(op string, input any, code string) {
		t.Helper()
		raw, _ := json.Marshal(input)
		_, err := svc.Call(context.Background(), token, op, raw)
		if err == nil || app.ErrorResponse(err).Code != code {
			t.Fatalf("%s: expected %s; received %v", op, code, err)
		}
	}
	denied("saveWelcomeSetup", in, "welcome_conflict")
	in.ExpectedVersion = saved.Version
	in.Repository = strings.Repeat("x", 4097)
	denied("saveWelcomeSetup", in, "invalid_setup")
	in.Repository = ""
	denied("completeWelcomeSetup", app.CompleteWelcomeSetupInput{ExpectedVersion: 1, Confirm: true}, "welcome_conflict")
	st, err := db.Read(context.Background())
	if err != nil || len(st.Projects) != 0 || len(st.Tickets) != 0 || st.Settings.Agents || st.Settings.ExecutionMode != "" {
		t.Fatal("draft produced execution/domain effects", err)
	}
	second := "welcome-second-session"
	hash := sha256.Sum256([]byte(second))
	if err = db.Update(context.Background(), func(st *model.State) (bool, error) {
		st.Accounts["second"] = model.Account{ID: "second", Username: "second", Administrator: true}
		st.Sessions[hex.EncodeToString(hash[:])] = model.Session{AccountID: "second", Expires: time.Now().Add(time.Hour)}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	other := call(t, svc, second, "getWelcomeSetup", struct{}{}).(app.WelcomeSetupResult)
	if other.Setup.ProjectName != "" || other.Setup.ID != "" || other.CanBindRepository {
		t.Fatal("another administrator received a private draft or an isolation claim", other)
	}
	// A new API service resumes durable progress without creating another draft.
	resumed := call(t, app.New(db), token, "getWelcomeSetup", struct{}{}).(app.WelcomeSetupResult)
	if resumed.Setup != saved {
		t.Fatal("restart lost setup progress")
	}
}

func TestWelcomeCompletionRequiresCurrentBasisAndIsRetrySafe(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Welcome project", RequestID: "welcome-project"}).(model.Project)
	in := app.SaveWelcomeSetupInput{Step: 5, ProjectID: p.ID, Repository: "/not/granted", StorageRoot: "/not/granted-worktrees", Harness: "codex", Attention: "questions"}
	w := call(t, svc, token, "saveWelcomeSetup", in).(model.WelcomeSetup)
	raw, _ := json.Marshal(app.CompleteWelcomeSetupInput{ExpectedVersion: w.Version, Confirm: true})
	if _, err := svc.Call(context.Background(), token, "completeWelcomeSetup", raw); err == nil || app.ErrorResponse(err).Code != "binding_required" {
		t.Fatal("ungranted filesystem basis completed", err)
	}
	in.ExpectedVersion, in.Repository, in.StorageRoot = w.Version, "", ""
	w = call(t, svc, token, "saveWelcomeSetup", in).(model.WelcomeSetup)
	finish := app.CompleteWelcomeSetupInput{ExpectedVersion: w.Version, Confirm: true}
	done := call(t, svc, token, "completeWelcomeSetup", finish).(model.WelcomeSetup)
	replayed := call(t, svc, token, "completeWelcomeSetup", finish).(model.WelcomeSetup)
	if !done.Completed || done != replayed {
		t.Fatal("completion retry mutated progress", done, replayed)
	}
	st, err := db.Read(context.Background())
	if err != nil || len(st.Projects) != 1 || st.Settings.Agents || st.Projects[p.ID].Repository != nil {
		t.Fatal("completion created an implicit grant/start", err)
	}
}
