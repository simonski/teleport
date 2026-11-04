package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestWelcomeProgressIsPrivateVersionedAndStartsNothing(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	initial := call(t, svc, token, "getWelcomeSetup", struct{}{}).(app.WelcomeSetupResult)
	if initial.Setup.Version != 0 || initial.Setup.Completed || initial.CanBindRepository {
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
	if w.Repository != "" || w.StorageRoot != "" {
		t.Fatal("legacy paths retained in setup", w)
	}
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

func TestDefaultHarnessRemainsEditableAfterWelcome(t *testing.T) {
	svc, token, _ := fixtureWithStore(t)
	initial := call(t, svc, token, "getWelcomeSetup", struct{}{}).(app.WelcomeSetupResult)
	if len(initial.Harnesses) != 3 || initial.Setup.Harness != "codex" {
		t.Fatal("default harness choices missing", initial)
	}
	in := app.SaveWelcomeSetupInput{Step: 5, ProjectName: "Defaults", ProjectKey: "defaults", Harness: "claude", Attention: "questions"}
	draft := call(t, svc, token, "saveWelcomeSetup", in).(model.WelcomeSetup)
	finished := call(t, svc, token, "completeWelcomeSetup", app.CompleteWelcomeSetupInput{ExpectedVersion: draft.Version, Confirm: true}).(model.WelcomeSetup)
	updated := call(t, svc, token, "setDefaultHarness", app.DefaultHarnessInput{ExpectedVersion: finished.Version, Harness: "copilot"}).(model.WelcomeSetup)
	if !updated.Completed || updated.Harness != "copilot" || updated.ProjectID != finished.ProjectID || updated.Version != finished.Version+1 {
		t.Fatal("default change damaged completed setup", updated)
	}
	raw, _ := json.Marshal(app.DefaultHarnessInput{ExpectedVersion: finished.Version, Harness: "codex"})
	if _, err := svc.Call(context.Background(), token, "setDefaultHarness", raw); err == nil || app.ErrorResponse(err).Code != "welcome_conflict" {
		t.Fatal("stale default change was accepted", err)
	}
}

func TestWelcomeCreatesOnlyAtFinalCompletionAndRechecksName(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	in := app.SaveWelcomeSetupInput{Step: 5, ProjectName: "Teleport", ProjectKey: "tele", Harness: "codex", Attention: "questions"}
	w := call(t, svc, token, "saveWelcomeSetup", in).(model.WelcomeSetup)
	st, _ := db.Read(context.Background())
	if len(st.Projects) != 0 {
		t.Fatal("draft published a project")
	}
	call(t, svc, token, "createProject", app.CreateProjectInput{Name: " TELEPORT ", Key: "taken", RequestID: "concurrent"})
	raw, _ := json.Marshal(app.CompleteWelcomeSetupInput{ExpectedVersion: w.Version, Confirm: true})
	if _, err := svc.Call(context.Background(), token, "completeWelcomeSetup", raw); err == nil || app.ErrorResponse(err).Code != "project_name_exists" {
		t.Fatal("concurrent name collision not rejected", err)
	}
	st, _ = db.Read(context.Background())
	if len(st.Projects) != 1 || st.WelcomeSetups["admin"].Completed {
		t.Fatal("failed completion had partial effects")
	}
	in.ExpectedVersion, in.ProjectName, in.ProjectKey = w.Version, "New project", "newp"
	w = call(t, svc, token, "saveWelcomeSetup", in).(model.WelcomeSetup)
	done := call(t, svc, token, "completeWelcomeSetup", app.CompleteWelcomeSetupInput{ExpectedVersion: w.Version, Confirm: true}).(model.WelcomeSetup)
	retried := call(t, svc, token, "completeWelcomeSetup", app.CompleteWelcomeSetupInput{ExpectedVersion: w.Version, Confirm: true}).(model.WelcomeSetup)
	st, _ = db.Read(context.Background())
	if done != retried || !done.Completed || len(st.Projects) != 2 || st.Projects[done.ProjectID].Repository != nil {
		t.Fatal("final creation was not atomic/retry-safe", done, retried)
	}
}

func TestProjectNameAvailabilityPrefixesAndConcurrentCreation(t *testing.T) {
	svc, token, _ := fixtureWithStore(t)
	for _, in := range []app.CreateProjectInput{{Name: "Teleport", Key: "tele", RequestID: "one"}, {Name: "Other", Key: "tel1", RequestID: "two"}} {
		call(t, svc, token, "createProject", in)
	}
	for _, tc := range []struct {
		name, key, suggestion string
		available             bool
	}{
		{"  TELEPORT ", "new", "tel2", false}, {"Television", "new", "tel2", true}, {"Tiny name", "new", "tiny", true}, {"🚀", "new", "proj", true},
	} {
		r := call(t, svc, token, "checkProjectAvailability", app.ProjectAvailabilityInput{Name: tc.name, Key: tc.key}).(app.ProjectAvailability)
		if r.NameAvailable != tc.available || r.SuggestedKey != tc.suggestion || !r.KeyAvailable {
			t.Fatal(tc, r)
		}
	}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for _, key := range []string{"race1", "race2"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			raw, _ := json.Marshal(app.CreateProjectInput{Name: "Race project", Key: key, RequestID: key})
			_, err := svc.Call(context.Background(), token, "createProject", raw)
			errors <- err
		}(key)
	}
	wg.Wait()
	close(errors)
	successes, conflicts := 0, 0
	for err := range errors {
		if err == nil {
			successes++
		} else if app.ErrorResponse(err).Code == "project_name_exists" {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("duplicate concurrent names", successes, conflicts)
	}
}
