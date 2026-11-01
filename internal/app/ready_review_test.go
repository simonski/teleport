package app_test

import (
	"context"
	"encoding/json"
	"strings"
	"teleport/internal/app"
	"teleport/internal/model"
	"testing"
)

func TestReadyFinalReviewHoldsImplementationUntilExactHumanConfirmation(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Ready review", RequestID: "p"}).(model.Project)
	w := call(t, svc, token, "copyWorkflow", app.CopyWorkflowInput{SourceKey: "agent@1", ProjectID: p.ID, Title: "Reviewed Ready", RequestID: "w"}).(model.Workflow)
	for i := range w.Phases {
		if w.Phases[i].Role == "Ready" {
			w.Phases[i].RequireReadyReview = true
		}
	}
	w = call(t, svc, token, "saveWorkflow", app.SaveWorkflowInput{Definition: w, ExpectedRevision: w.DraftRevision}).(model.Workflow)
	call(t, svc, token, "activateWorkflowVersion", app.ActivateWorkflowInput{WorkflowKey: w.ID + "@draft", ExpectedRevision: w.DraftRevision, ProjectID: p.ID, ExpectedProjectVersion: p.Version})
	call(t, svc, token, "setExecutionMode", app.ExecutionModeInput{Mode: "demo"})
	ticket := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Review before implementation", RequestID: "t"}).(model.Ticket)
	ticket = call(t, svc, token, "requestRefinement", app.VersionInput{ID: ticket.ID, ExpectedVersion: ticket.Version}).(model.Ticket)
	result := call(t, svc, token, "runDemoStage", app.DemoStageInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "scope"}).(app.DemoStageResult)
	ticket = call(t, svc, token, "acceptRefinement", app.AcceptScopeInput{ID: ticket.ID, ExpectedVersion: result.Ticket.Version, ProposalID: result.Ticket.Refinement.ID}).(model.Ticket)
	if !strings.Contains(ticket.Attention, "Final human review") {
		t.Fatal("Ready review is missing attention", ticket.Attention)
	}
	raw, _ := json.Marshal(app.DemoStageInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "held"})
	if _, err := svc.Call(context.Background(), token, "runDemoStage", raw); err == nil {
		t.Fatal("scope acceptance bypassed Ready final review")
	}
	basis := call(t, svc, token, "getTicketAgreements", app.IDInput{ID: ticket.ID}).(app.AgreementBasis)
	ticket = call(t, svc, token, "markReady", app.MarkReadyInput{ID: ticket.ID, ExpectedVersion: ticket.Version, Basis: basis.CurrentBasis, ReadyBasis: basis.ReadyBasis, Confirm: true}).(model.Ticket)
	ticket = call(t, svc, token, "updateTicket", map[string]any{"id": ticket.ID, "expectedVersion": ticket.Version, "priority": "high"}).(model.Ticket)
	if ticket.Attention == "" {
		t.Fatal("a changed reviewed ticket lost attention")
	}
	raw, _ = json.Marshal(app.DemoStageInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "changed"})
	if _, err := svc.Call(context.Background(), token, "runDemoStage", raw); err == nil {
		t.Fatal("stale Ready review authorized implementation")
	}
	basis = call(t, svc, token, "getTicketAgreements", app.IDInput{ID: ticket.ID}).(app.AgreementBasis)
	ticket = call(t, svc, token, "markReady", app.MarkReadyInput{ID: ticket.ID, ExpectedVersion: ticket.Version, Basis: basis.CurrentBasis, ReadyBasis: basis.ReadyBasis, Confirm: true}).(model.Ticket)
	result = call(t, svc, token, "runDemoStage", app.DemoStageInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "reviewed"}).(app.DemoStageResult)
	if result.Ticket.Status != "Review" {
		t.Fatal("reviewed ticket did not enter implementation")
	}
}
