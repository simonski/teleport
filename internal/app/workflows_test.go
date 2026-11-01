package app_test

import (
	"context"
	"encoding/json"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestWorkflowCyclesPublicationAndPinnedVersions(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Flows", "key": "flows", "requestId": "p"}).(model.Project)
	before := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "title": "Existing pin", "agent": false, "requestId": "before"}).(model.Ticket)
	draft := call(t, svc, token, "copyWorkflow", map[string]any{"sourceKey": "human@1", "projectId": p.ID, "title": "Human with a return", "requestId": "flow"}).(model.Workflow)
	draft.Rules = append(draft.Rules, model.Rule{ID: "retry", Source: "Doing", Outcome: "retry", Target: "Doing"})
	draft = call(t, svc, token, "saveWorkflow", app.SaveWorkflowInput{Definition: draft, ExpectedRevision: draft.DraftRevision}).(model.Workflow)
	preview := call(t, svc, token, "previewWorkflowOutcome", app.OutcomePreviewInput{WorkflowKey: draft.ID + "@draft", Source: "Doing", Outcome: "retry"}).(app.RoutePreview)
	if preview.Target != "Doing" || preview.NeedsAttempt {
		t.Fatal("manual self retry classified incorrectly")
	}
	p = call(t, svc, token, "activateWorkflowVersion", app.ActivateWorkflowInput{WorkflowKey: draft.ID + "@draft", ExpectedRevision: draft.DraftRevision, ProjectID: p.ID, ExpectedProjectVersion: p.Version}).(model.Project)
	after := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "title": "New pin", "agent": false, "requestId": "after"}).(model.Ticket)
	if before.Workflow == after.Workflow || before.Workflow != "human@1" || after.Workflow != p.HumanWorkflow {
		t.Fatal("activation reinterpreted an old ticket or missed new work")
	}
	input := app.SaveWorkflowInput{Definition: draft, ExpectedRevision: draft.DraftRevision}
	input.Definition.Rules = append(input.Definition.Rules, model.Rule{ID: "ambiguous", Source: "Todo", Outcome: "start", Target: "Done"})
	body, _ := json.Marshal(input)
	if _, err := svc.Call(context.Background(), token, "saveWorkflow", body); err == nil {
		t.Fatal("ambiguous rules accepted")
	}
}

func TestWorkflowRulesPreserveHumanGatesAndRejectAmbiguity(t *testing.T) {
	for _, workflow := range model.SeedWorkflows() {
		if err := app.ValidateWorkflow(workflow, true); err != nil {
			t.Fatal(err)
		}
	}
	agent := model.SeedWorkflows()["agent@1"]
	agent.Rules = append(agent.Rules, model.Rule{ID: "skip-approval", Source: "Doing", Outcome: "approve", Target: "Done"})
	if err := app.ValidateWorkflow(agent, false); err == nil {
		t.Fatal("agent approval bypass accepted")
	}
	human := model.SeedWorkflows()["human@1"]
	human.Rules = append(human.Rules, model.Rule{ID: "ambiguous", Source: "Todo", Outcome: "start", Target: "Done"})
	if err := app.ValidateWorkflow(human, false); err == nil {
		t.Fatal("overlapping same-priority outcomes accepted")
	}
}
