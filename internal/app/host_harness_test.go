package app_test

import (
	"context"
	"encoding/json"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestHostHarnessDiscoveryRequiresHumanAdministratorAndStopConfirmation(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	listing := call(t, svc, token, "listHostHarnesses", struct{}{}).(app.HostHarnessList)
	if listing.Items == nil || listing.Scope == "" {
		t.Fatal("missing host scope")
	}
	raw, _ := json.Marshal(app.HostHarnessStopInput{PID: 1, Identity: "fixture"})
	if _, err := svc.Call(context.Background(), token, "stopHostHarness", raw); err == nil || app.ErrorResponse(err).Code != "confirmation_required" {
		t.Fatal("unconfirmed stop accepted", err)
	}
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		a := st.Accounts["admin"]
		a.Administrator = false
		st.Accounts["admin"] = a
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Call(context.Background(), token, "listHostHarnesses", []byte(`{}`)); err == nil || app.ErrorResponse(err).Code != "forbidden" {
		t.Fatal("non-administrator discovered host processes", err)
	}
}
func TestUnknownSessionStopDoesNotClaimSignalOrChangeOwnership(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	svc.ConfigureLocalRuntime(t.TempDir(), "/missing/tk", true)
	project := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Unknown console", RequestID: "unknown-console"}).(model.Project)
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		st.LocalSessions = map[string]model.LocalSession{"H1": {ID: "H1", Creator: "admin", ProjectID: project.ID, State: "unknown"}}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(app.SessionStop{ID: "H1", Confirm: true})
	_, err := svc.Call(context.Background(), token, "stopSession", raw)
	if err == nil || app.ErrorResponse(err).Code != "session_ownership_unknown" {
		t.Fatal("unresolved ownership did not produce honest stop result", err)
	}
	state, err := db.Read(context.Background())
	if err != nil || state.LocalSessions["H1"].State != "unknown" || !state.LocalSessions["H1"].StopRequested {
		t.Fatal("unknown session lost its durable stop intent", err)
	}
}

func TestSessionProcessIDVisibleAndRetained(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	project := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "PID console", RequestID: "pid-console"}).(model.Project)
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		if st.LocalSessions == nil {
			st.LocalSessions = map[string]model.LocalSession{}
		}
		st.LocalSessions["H1"] = model.LocalSession{ID: "H1", Number: 1, Creator: "admin", ProjectID: project.ID, State: "exited", ProcessID: 12345}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	page := call(t, svc, token, "listSessions", app.ListInput{}).(app.LocalSessionPage)
	if len(page.Items) != 1 || page.Items[0].ProcessID != 12345 {
		t.Fatal("retained PID missing from session listing", page)
	}
}

func TestRemoveStaleConsoleReference(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	project := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Stale console", RequestID: "stale-console"}).(model.Project)
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		st.LocalSessions = map[string]model.LocalSession{
			"H3": {ID: "H3", Number: 3, Creator: "admin", ProjectID: project.ID, State: "unknown", LaunchID: "launch", Generation: "generation", StopRequested: true},
			"H4": {ID: "H4", Number: 4, Creator: "admin", ProjectID: project.ID, State: "running", LaunchID: "other", Generation: "generation"},
			"H5": {ID: "H5", Number: 5, Creator: "admin", ProjectID: project.ID, TicketID: 1, State: "unknown"},
		}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	page := call(t, svc, token, "listSessions", app.ListInput{}).(app.LocalSessionPage)
	var ref string
	for _, row := range page.Items {
		if row.ID == "H3" {
			ref = row.Reference
		}
		if row.ID == "H4" || row.ID == "H5" {
			if row.Reference != "" {
				t.Fatal("ineligible reference exposed")
			}
		}
	}
	if ref == "" {
		t.Fatal("missing exact reference")
	}
	invoke := func(in app.RemoveSessionReferenceInput) error {
		raw, _ := json.Marshal(in)
		_, err := svc.Call(context.Background(), token, "removeSessionReference", raw)
		return err
	}
	if err := invoke(app.RemoveSessionReferenceInput{ID: "H3", ExpectedReference: ref}); err == nil {
		t.Fatal("unconfirmed removal accepted")
	}
	if err := invoke(app.RemoveSessionReferenceInput{ID: "H3", ExpectedReference: "stale", Confirm: true}); err == nil {
		t.Fatal("stale removal accepted")
	}
	if err := invoke(app.RemoveSessionReferenceInput{ID: "H3", ExpectedReference: ref, Confirm: true}); err != nil {
		t.Fatal(err)
	}
	if err := invoke(app.RemoveSessionReferenceInput{ID: "H3", ExpectedReference: ref, Confirm: true}); err != nil {
		t.Fatal("retry failed", err)
	}
	page = call(t, svc, token, "listSessions", app.ListInput{}).(app.LocalSessionPage)
	for _, row := range page.Items {
		if row.ID == "H3" {
			t.Fatal("removed reference remains visible")
		}
	}
	page = call(t, svc, token, "listSessions", app.ListInput{IncludeHidden: true}).(app.LocalSessionPage)
	visibleInRecovery := false
	for _, row := range page.Items {
		if row.ID == "H3" && row.ReferenceRemoved && row.State == "unknown" {
			visibleInRecovery = true
		}
	}
	if !visibleInRecovery {
		t.Fatal("removed unresolved session missing from recovery listing")
	}
	if err := invoke(app.RemoveSessionReferenceInput{ID: "H3", ExpectedReference: "stale", Confirm: true, ReleaseCapacity: true}); err == nil {
		t.Fatal("stale release accepted")
	}
	if err := invoke(app.RemoveSessionReferenceInput{ID: "H3", ExpectedReference: ref, Confirm: true, ReleaseCapacity: true}); err != nil {
		t.Fatal(err)
	}
	state, err := db.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	row := state.LocalSessions["H3"]
	if row.State != "unknown" || row.ReferenceRemovedAt == nil || row.ReferenceRemovedBy != "admin" || row.CapacityReleasedAt == nil || row.CapacityReleasedBy != "admin" || !row.StopRequested || row.CredentialHash != "" || row.ExitedAt != nil {
		t.Fatal("quarantine or audit lost", row)
	}
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		row := st.LocalSessions["H3"]
		row.State = "running"
		st.LocalSessions["H3"] = row
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	page = call(t, svc, token, "listSessions", app.ListInput{}).(app.LocalSessionPage)
	found := false
	for _, row := range page.Items {
		if row.ID == "H3" {
			found = true
		}
	}
	if !found {
		t.Fatal("reconciled live session must reappear")
	}
}
