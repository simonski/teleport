package app_test

import (
	"context"
	"encoding/json"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
	"teleport/internal/teststore"
)

// Existing installations may retain simulation projects after sample seeding is removed.
func TestExistingSimulationCannotLaunchBindOrTransferToRealProject(t *testing.T) {
	db := teststore.Open(t)
	ctx := context.Background()
	if _, err := app.Initialize(ctx, db); err != nil {
		t.Fatal(err)
	}
	svc := app.New(db)
	svc.ConfigureLocalRuntime(t.TempDir(), "/missing/tk", true)
	login, err := svc.Call(ctx, "", "login", []byte(`{"username":"admin","password":"password"}`))
	if err != nil {
		t.Fatal(err)
	}
	token := login.(app.LoginResult).Token
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Simulation", RequestID: "sim-project"}).(model.Project)
	human := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Example task", RequestID: "sim-task"}).(model.Ticket)
	if err := db.Update(ctx, func(st *model.State) (bool, error) {
		p := st.Projects[p.ID]
		p.Simulation = true
		st.Projects[p.ID] = p
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"projectId": p.ID, "harness": "fixture", "requestId": "no-launch"})
	if _, err = svc.Call(ctx, token, "openConsoleSession", raw); err == nil || err.(*app.Error).Code != "simulation_only" {
		t.Fatal("simulation admitted a native process", err)
	}
	raw, _ = json.Marshal(app.RepositoryInput{ProjectID: p.ID, ExpectedVersion: p.Version, Repository: "/not/a/repo", StorageRoot: "/not/storage"})
	if _, err = svc.Call(ctx, token, "configureLocalRepository", raw); err == nil || err.(*app.Error).Code != "simulation_only" {
		t.Fatal("simulation admitted a repository", err)
	}
	real := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Real", RequestID: "real-project"}).(model.Project)
	raw, _ = json.Marshal(app.TransferInput{ID: human.ID, ExpectedVersion: human.Version, DestinationProjectID: real.ID})
	if _, err = svc.Call(ctx, token, "transferTicket", raw); err == nil {
		t.Fatal("simulation work crossed into real project")
	}
	before, _ := db.Read(ctx)
	after, _, err := app.InitializeWithOptions(ctx, db, app.InitializeOptions{})
	if err != nil || after.Revision != before.Revision || !after.Projects[p.ID].Simulation {
		t.Fatal("initialization rewrote existing simulation", err)
	}
}
