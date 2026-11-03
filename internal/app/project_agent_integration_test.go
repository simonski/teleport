package app_test

import (
	"context"
	"encoding/json"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestProjectAgentLimitBlocksDemoAdmissionUntilUnknownSessionExits(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Capacity fixture", "key": "capacity", "requestId": "project"}).(model.Project)
	maximum := 1
	p = call(t, svc, token, "setLaneCapacity", app.CapacityInput{ProjectID: p.ID, ExpectedVersion: p.Version, RefineWorkers: 1, DoingWorkers: 1, MaxAgents: &maximum}).(model.Project)
	work := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "title": "Pending work", "requestId": "work"}).(model.Ticket)
	call(t, svc, token, "setExecutionMode", app.ExecutionModeInput{Mode: "demo"})
	work = call(t, svc, token, "requestRefinement", app.VersionInput{ID: work.ID, ExpectedVersion: work.Version}).(model.Ticket)
	if err := db.Update(context.Background(), func(state *model.State) (bool, error) {
		if state.LocalSessions == nil {
			state.LocalSessions = map[string]model.LocalSession{}
		}
		state.LocalSessions["fixture"] = model.LocalSession{ID: "fixture", ProjectID: p.ID, State: "unknown"}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	input := app.DemoStageInput{ID: work.ID, ExpectedVersion: work.Version, RequestID: "attempt"}
	raw, _ := json.Marshal(input)
	if _, err := svc.Call(context.Background(), token, "runDemoStage", raw); err == nil || app.ErrorResponse(err).Code != "project_agent_limit" {
		t.Fatalf("unknown session did not block admission: %v", err)
	}
	if err := db.Update(context.Background(), func(state *model.State) (bool, error) {
		row := state.LocalSessions["fixture"]
		row.State = "exited"
		state.LocalSessions["fixture"] = row
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Call(context.Background(), token, "runDemoStage", raw); err != nil {
		t.Fatalf("admission remained blocked after exit: %v", err)
	}
}
