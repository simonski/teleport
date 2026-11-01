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

func TestOwnedPreparationHonorsPinnedLaneQuota(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Paused implementation", RequestID: "p"}).(model.Project)
	folder := t.TempDir()
	repo := filepath.Join(folder, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	repoGit(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "fixture.txt"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	repoGit(t, repo, "add", ".")
	repoGit(t, repo, "commit", "-m", "Seed")
	p = call(t, svc, token, "configureLocalRepository", app.RepositoryInput{ProjectID: p.ID, ExpectedVersion: p.Version, Repository: repo, StorageRoot: filepath.Join(folder, "worktrees")}).(model.Project)
	draft := call(t, svc, token, "copyWorkflow", app.CopyWorkflowInput{SourceKey: "agent@1", ProjectID: p.ID, Title: "Paused lane", RequestID: "w"}).(model.Workflow)
	zero := 0
	for i := range draft.Phases {
		if draft.Phases[i].Role == "Doing" {
			draft.Phases[i].Workers = &zero
		}
	}
	draft = call(t, svc, token, "saveWorkflow", app.SaveWorkflowInput{Definition: draft, ExpectedRevision: draft.DraftRevision}).(model.Workflow)
	call(t, svc, token, "activateWorkflowVersion", app.ActivateWorkflowInput{WorkflowKey: draft.ID + "@draft", ExpectedRevision: draft.DraftRevision, ProjectID: p.ID, ExpectedProjectVersion: p.Version})
	ticket := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Wait before preparation", RequestID: "t"}).(model.Ticket)
	settings := call(t, svc, token, "setExecutionMode", app.ExecutionModeInput{Mode: "demo"}).(model.Settings)
	ticket = call(t, svc, token, "requestRefinement", app.VersionInput{ID: ticket.ID, ExpectedVersion: ticket.Version}).(model.Ticket)
	result := call(t, svc, token, "runDemoStage", app.DemoStageInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "scope"}).(app.DemoStageResult)
	ticket = call(t, svc, token, "acceptRefinement", app.AcceptScopeInput{ID: ticket.ID, ExpectedVersion: result.Ticket.Version, ProposalID: result.Ticket.Refinement.ID}).(model.Ticket)
	call(t, svc, token, "setExecutionMode", app.ExecutionModeInput{Mode: "off", ExpectedVersion: settings.Version})
	body, _ := json.Marshal(app.GitTaskInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "blocked"})
	_, err := svc.Call(context.Background(), token, "prepareTaskWorktree", body)
	if err == nil || app.ErrorResponse(err).Code != "lane_capacity_disabled" {
		t.Fatal("zero lane quota prepared real work", err)
	}
	after := call(t, svc, token, "getTicket", app.IDInput{ID: ticket.ID}).(model.Ticket)
	if after.Version != ticket.Version || after.Attempts != ticket.Attempts || after.Delivery != nil || after.Execution != "" {
		t.Fatal("blocked preparation changed work, ownership or allowance")
	}
}

func TestWorkflowLaneMetadataValidationAndPinnedAdmission(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Lane limits", RequestID: "p"}).(model.Project)
	draft := call(t, svc, token, "copyWorkflow", app.CopyWorkflowInput{SourceKey: "agent@1", ProjectID: p.ID, Title: "Capped refinement", RequestID: "w"}).(model.Workflow)
	zero := 0
	for i := range draft.Phases {
		if draft.Phases[i].Role == "Refine" {
			draft.Phases[i].Description = "Clarify scope before implementation."
			draft.Phases[i].Workers = &zero
		}
	}
	draft = call(t, svc, token, "saveWorkflow", app.SaveWorkflowInput{Definition: draft, ExpectedRevision: draft.DraftRevision}).(model.Workflow)
	p = call(t, svc, token, "activateWorkflowVersion", app.ActivateWorkflowInput{WorkflowKey: draft.ID + "@draft", ExpectedRevision: draft.DraftRevision, ProjectID: p.ID, ExpectedProjectVersion: p.Version}).(model.Project)
	ticket := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Do not spend an attempt", RequestID: "t"}).(model.Ticket)
	call(t, svc, token, "setExecutionMode", app.ExecutionModeInput{Mode: "demo"})
	ticket = call(t, svc, token, "requestRefinement", map[string]any{"id": ticket.ID, "expectedVersion": ticket.Version}).(model.Ticket)
	body, _ := json.Marshal(app.DemoStageInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "blocked"})
	_, err := svc.Call(context.Background(), token, "runDemoStage", body)
	if err == nil || app.ErrorResponse(err).Code != "lane_capacity_disabled" {
		t.Fatal("zero lane quota admitted work", err)
	}
	unchanged := call(t, svc, token, "getTicket", map[string]any{"id": ticket.ID}).(model.Ticket)
	if unchanged.Version != ticket.Version || unchanged.Attempts != 0 || unchanged.Execution != "" {
		t.Fatal("blocked admission consumed an attempt or reservation")
	}
	version := strings.Split(ticket.Workflow, "@")[1]
	if version != "1" || ticket.Workflow != p.AgentWorkflow {
		t.Fatal("published pin lost")
	}
	saved := call(t, svc, token, "getWorkflow", app.WorkflowKeyInput{WorkflowKey: ticket.Workflow}).(model.Workflow)
	for _, phase := range saved.Phases {
		if phase.Role == "Refine" && (phase.Description == "" || phase.Workers == nil || *phase.Workers != 0) {
			t.Fatal("lane fields lost during publication")
		}
	}
	for _, quota := range []int{-1, 21} {
		bad := model.SeedWorkflows()["agent@1"]
		bad.Phases[1].Workers = &quota
		if app.ValidateWorkflow(bad, false) == nil {
			t.Fatal("invalid quota accepted", quota)
		}
	}
	manual := model.SeedWorkflows()["human@1"]
	manual.Phases[0].Workers = &zero
	if app.ValidateWorkflow(manual, false) == nil {
		t.Fatal("manual worker settings accepted")
	}
	for _, description := range []string{"  ", strings.Repeat("é", 10001)} {
		bad := model.SeedWorkflows()["agent@1"]
		bad.Phases[1].Description = description
		if app.ValidateWorkflow(bad, false) == nil {
			t.Fatal("invalid description accepted")
		}
	}
}
