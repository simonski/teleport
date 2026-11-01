package app_test

import (
	"context"
	"encoding/json"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
	"teleport/internal/teststore"
)

func TestDemoInitializationIsProjectScopedAndIdempotent(t *testing.T) {
	db := teststore.Open(t)
	ctx := context.Background()
	st, _, err := app.InitializeWithOptions(ctx, db, app.InitializeOptions{Demo: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Projects) != 1 || len(st.Ideas) < 3 || st.Settings.Agents {
		t.Fatal("demo changed global execution or lacks sample work")
	}
	kinds, phases := map[string]int{}, map[string]int{}
	for _, p := range st.Projects {
		if !p.Simulation || p.DemoSeed != "petstore-v1" || p.Repository != nil {
			t.Fatal("unsafe demo project", p)
		}
	}
	for _, ticket := range st.Tickets {
		kinds[ticket.Kind]++
		phases[ticket.Status]++
		if ticket.Agent && ticket.Status == "Done" {
			t.Fatal("seed fabricated agent completion")
		}
	}
	if kinds["goal"] < 3 || kinds["epic"] < 5 || kinds["task"] < 12 || len(phases) < 7 {
		t.Fatal("insufficient sample variety", kinds, phases)
	}
	again, _, err := app.InitializeWithOptions(ctx, db, app.InitializeOptions{Demo: true})
	if err != nil || again.InstallationID != st.InstallationID || again.Revision != st.Revision || len(again.Tickets) != len(st.Tickets) {
		t.Fatal("demo retry rewrote state", err)
	}
}

func TestSimulationAdvancesWithGlobalAgentsOffAndCannotBindRepository(t *testing.T) {
	db := teststore.Open(t)
	ctx := context.Background()
	st, _, err := app.InitializeWithOptions(ctx, db, app.InitializeOptions{Demo: true})
	if err != nil {
		t.Fatal(err)
	}
	svc := app.New(db)
	login, err := svc.Call(ctx, "", "login", []byte(`{"username":"admin","password":"password"}`))
	if err != nil {
		t.Fatal(err)
	}
	token := login.(app.LoginResult).Token
	ideas := call(t, svc, token, "listIdeas", app.ListIdeasInput{ProjectID: 1}).(app.Page[model.Idea])
	if len(ideas.Items) != 4 {
		t.Fatal("seeded Ideas are not discoverable through the API", len(ideas.Items))
	}
	var ready model.Ticket
	for _, ticket := range st.Tickets {
		if ticket.Status == "Ready" {
			ready = ticket
			break
		}
	}
	raw, _ := json.Marshal(app.DemoStageInput{ID: ready.ID, ExpectedVersion: ready.Version, RequestID: "simulate"})
	result, err := svc.Call(ctx, token, "runDemoStage", raw)
	if err != nil {
		t.Fatal("simulation depends on globally enabling agents", err)
	}
	if result.(app.DemoStageResult).Ticket.Status != "Review" {
		t.Fatal("simulation did not advance")
	}
	raw, _ = json.Marshal(app.RepositoryInput{ProjectID: ready.ProjectID, ExpectedVersion: 1, Repository: "/not/a/repo", StorageRoot: "/not/storage"})
	_, err = svc.Call(ctx, token, "configureLocalRepository", raw)
	if err == nil || err.(*app.Error).Code != "simulation_only" {
		t.Fatal("simulation attempted real repository configuration", err)
	}
}

func TestDemoCollisionRollsBackAndExistingEditsSurvive(t *testing.T) {
	db := teststore.Open(t)
	ctx := context.Background()
	if _, err := app.Initialize(ctx, db); err != nil {
		t.Fatal(err)
	}
	svc := app.New(db)
	login, err := svc.Call(ctx, "", "login", []byte(`{"username":"admin","password":"password"}`))
	if err != nil {
		t.Fatal(err)
	}
	token := login.(app.LoginResult).Token
	call(t, svc, token, "createProject", app.CreateProjectInput{Name: "PetStore", Key: "real", RequestID: "real-project"})
	before, _ := db.Read(ctx)
	if _, _, err := app.InitializeWithOptions(ctx, db, app.InitializeOptions{Demo: true}); err == nil {
		t.Fatal("demo overwrote a name collision")
	}
	after, _ := db.Read(ctx)
	if before.Revision != after.Revision || len(after.Projects) != 1 || len(after.Ideas) != 0 {
		t.Fatal("collision made partial changes")
	}
}

func TestSeededSimulationCannotLaunchOrTransferToRealProject(t *testing.T) {
	db := teststore.Open(t)
	ctx := context.Background()
	st, _, err := app.InitializeWithOptions(ctx, db, app.InitializeOptions{Demo: true})
	if err != nil {
		t.Fatal(err)
	}
	svc := app.New(db)
	svc.ConfigureLocalRuntime(t.TempDir(), "/missing/tk", true)
	login, err := svc.Call(ctx, "", "login", []byte(`{"username":"admin","password":"password"}`))
	if err != nil {
		t.Fatal(err)
	}
	token := login.(app.LoginResult).Token
	var p model.Project
	for _, project := range st.Projects {
		p = project
	}
	_, err = svc.Call(ctx, token, "openConsoleSession", []byte(`{"projectId":1,"harness":"fixture","requestId":"no-launch"}`))
	if err == nil || err.(*app.Error).Code != "simulation_only" {
		t.Fatal("simulation admitted a native process", err)
	}
	real := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Real", RequestID: "real-project"}).(model.Project)
	var human model.Ticket
	for _, ticket := range st.Tickets {
		if ticket.Kind == "task" && !ticket.Agent && ticket.Status == "Todo" {
			human = ticket
			break
		}
	}
	raw, _ := json.Marshal(app.TransferInput{ID: human.ID, ExpectedVersion: human.Version, DestinationProjectID: real.ID})
	if _, err = svc.Call(ctx, token, "transferTicket", raw); err == nil {
		t.Fatal("synthetic work crossed into a real project")
	}
	call(t, svc, token, "saveProject", app.SaveProjectInput{ID: p.ID, ExpectedVersion: p.Version, Description: ptrString("Human-edited demo description")})
	before, _ := db.Read(ctx)
	after, _, err := app.InitializeWithOptions(ctx, db, app.InitializeOptions{Demo: true})
	if err != nil || after.Revision != before.Revision || after.Projects[p.ID].Description != "Human-edited demo description" || after.Projects[real.ID].Simulation {
		t.Fatal("retry rewrote demo edits or changed another project", err)
	}
}

func ptrString(value string) *string { return &value }
