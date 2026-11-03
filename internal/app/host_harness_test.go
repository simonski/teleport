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
