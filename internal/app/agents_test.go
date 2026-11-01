package app_test

import (
	"context"
	"encoding/json"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestDemoRefinementQuestionsAndHumanApprovalBoundary(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Demo", "key": "demo", "requestId": "p"}).(model.Project)
	ticket := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "title": "Build a flight case", "requestId": "t"}).(model.Ticket)
	call(t, svc, token, "setExecutionMode", map[string]any{"mode": "demo"})
	ticket = call(t, svc, token, "requestRefinement", map[string]any{"id": ticket.ID, "expectedVersion": ticket.Version}).(model.Ticket)
	question := call(t, svc, token, "runDemoStage", app.DemoStageInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "question", Outcome: "question", Question: "Which material?"}).(app.DemoStageResult)
	if question.Ticket.Attempts != 1 || question.Ticket.Question == nil || question.Ticket.Active || question.Ticket.Claim != "" {
		t.Fatal("question did not safely release demo capacity")
	}
	ticket = call(t, svc, token, "answerQuestion", app.AnswerInput{ID: ticket.ID, ExpectedVersion: question.Ticket.Version, QuestionID: question.Ticket.Question.ID, Answer: "Aluminium"}).(model.Ticket)
	result := call(t, svc, token, "runDemoStage", app.DemoStageInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "refine"}).(app.DemoStageResult)
	ticket = result.Ticket
	if ticket.Attempts != 1 || ticket.Refinement == nil {
		t.Fatal("clarification continuation consumed another attempt or lost scope")
	}
	ticket = call(t, svc, token, "acceptRefinement", app.AcceptScopeInput{ID: ticket.ID, ExpectedVersion: ticket.Version, ProposalID: ticket.Refinement.ID}).(model.Ticket)
	for range 2 {
		if err := svc.PumpDemo(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	ticket = call(t, svc, token, "getTicket", map[string]any{"id": ticket.ID}).(model.Ticket)
	if ticket.Status != "Approval" || ticket.Attempts != 3 || ticket.AgentReview == nil || !ticket.AgentReview.Simulated {
		t.Fatalf("invalid demo delivery state: %+v", ticket)
	}
	body, _ := json.Marshal(map[string]any{"id": ticket.ID, "expectedVersion": ticket.Version, "confirm": true})
	if _, err := svc.Call(context.Background(), token, "approveTicket", body); err == nil {
		t.Fatal("simulation evidence approved real code")
	}
}

func TestDemoRetriesPreserveFiniteAttemptsAndDoNotReplay(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Limits", "key": "limits", "requestId": "p"}).(model.Project)
	ticket := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "title": "Bounded work", "requestId": "t"}).(model.Ticket)
	call(t, svc, token, "setExecutionMode", app.ExecutionModeInput{Mode: "demo"})
	ticket = call(t, svc, token, "requestRefinement", map[string]any{"id": ticket.ID, "expectedVersion": ticket.Version}).(model.Ticket)
	input := app.DemoStageInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "failure-1", Outcome: "failure"}
	failed := call(t, svc, token, "runDemoStage", input).(app.DemoStageResult)
	replayed := call(t, svc, token, "runDemoStage", input).(app.DemoStageResult)
	if failed.Ticket.Status != "Todo" || replayed.Ticket.Attempts != 1 || replayed.Ticket.Version != failed.Ticket.Version {
		t.Fatal("failure replay executed again")
	}
	ticket = call(t, svc, token, "setExecutionBudget", app.BudgetInput{ID: ticket.ID, ExpectedVersion: failed.Ticket.Version, Limit: 1}).(model.Ticket)
	ticket = call(t, svc, token, "requestRefinement", map[string]any{"id": ticket.ID, "expectedVersion": ticket.Version}).(model.Ticket)
	blocked := call(t, svc, token, "runDemoStage", app.DemoStageInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "blocked"}).(app.DemoStageResult)
	if blocked.BlockedReason != "Budget exhausted" || blocked.Ticket.Attempts != 1 || !blocked.Ticket.ResumeHold {
		t.Fatal("retry reset or exceeded the finite allowance")
	}
}

func TestEpicPlanningSurvivesCompletedChildrenAndClosesWithoutImplementation(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Planning", "key": "planning", "requestId": "p"}).(model.Project)
	epic := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "kind": "epic", "title": "Ship the cases", "requestId": "e"}).(model.Ticket)
	child := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "parentId": epic.ID, "title": "Inspect case", "agent": false, "requestId": "c"}).(model.Ticket)
	call(t, svc, token, "setExecutionMode", app.ExecutionModeInput{Mode: "demo"})
	epic = call(t, svc, token, "requestRefinement", map[string]any{"id": epic.ID, "expectedVersion": epic.Version}).(model.Ticket)
	call(t, svc, token, "transitionTicket", app.TransitionInput{ID: child.ID, ExpectedVersion: child.Version, Target: "Done", Confirm: true})
	epic = call(t, svc, token, "getTicket", map[string]any{"id": epic.ID}).(model.Ticket)
	if epic.Progress != 100 || epic.Status != "Refine" || !epic.Planning {
		t.Fatal("child rollup ended planning")
	}
	result := call(t, svc, token, "runDemoStage", app.DemoStageInput{ID: epic.ID, ExpectedVersion: epic.Version, RequestID: "scope"}).(app.DemoStageResult)
	epic = call(t, svc, token, "acceptRefinement", app.AcceptScopeInput{ID: epic.ID, ExpectedVersion: result.Ticket.Version, ProposalID: result.Ticket.Refinement.ID, Decompose: true}).(model.Ticket)
	if err := svc.PumpDemo(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := svc.PumpDemo(context.Background()); err != nil {
		t.Fatal(err)
	}
	epic = call(t, svc, token, "getTicket", map[string]any{"id": epic.ID}).(model.Ticket)
	if epic.Planning || epic.Attempts != 2 || epic.Status == "Done" {
		t.Fatal("closed planning reentered implementation or completed unfinished children")
	}
}

func TestDeclaredHaltDoesNotAutomaticallyRetryDecomposition(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Halt", "key": "halt", "requestId": "p"}).(model.Project)
	epic := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "kind": "epic", "title": "Plan shipment", "requestId": "e"}).(model.Ticket)
	call(t, svc, token, "setExecutionMode", app.ExecutionModeInput{Mode: "demo"})
	epic = call(t, svc, token, "requestRefinement", app.VersionInput{ID: epic.ID, ExpectedVersion: epic.Version}).(model.Ticket)
	result := call(t, svc, token, "runDemoStage", app.DemoStageInput{ID: epic.ID, ExpectedVersion: epic.Version, RequestID: "scope"}).(app.DemoStageResult)
	epic = call(t, svc, token, "acceptRefinement", app.AcceptScopeInput{ID: epic.ID, ExpectedVersion: result.Ticket.Version, ProposalID: result.Ticket.Refinement.ID, Decompose: true}).(model.Ticket)
	failed := call(t, svc, token, "runDemoStage", app.DemoStageInput{ID: epic.ID, ExpectedVersion: epic.Version, RequestID: "failed-plan", Outcome: "failure"}).(app.DemoStageResult)
	for range 3 {
		if err := svc.PumpDemo(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	current := call(t, svc, token, "getTicket", app.IDInput{ID: epic.ID}).(model.Ticket)
	if current.Attempts != failed.Ticket.Attempts || !current.Planning {
		t.Fatal("halted decomposition retried automatically")
	}
}

func TestMetadataEditsDoNotResetPhaseWaitingOrder(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Queue", "key": "queue", "requestId": "p"}).(model.Project)
	first := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "title": "First waiting", "requestId": "first"}).(model.Ticket)
	second := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "title": "Second waiting", "requestId": "second"}).(model.Ticket)
	call(t, svc, token, "setExecutionMode", app.ExecutionModeInput{Mode: "demo"})
	first = call(t, svc, token, "requestRefinement", app.VersionInput{ID: first.ID, ExpectedVersion: first.Version}).(model.Ticket)
	call(t, svc, token, "requestRefinement", app.VersionInput{ID: second.ID, ExpectedVersion: second.Version})
	call(t, svc, token, "updateTicket", map[string]any{"id": first.ID, "expectedVersion": first.Version, "labels": []string{"label-only"}})
	if err := svc.PumpDemo(context.Background()); err != nil {
		t.Fatal(err)
	}
	first = call(t, svc, token, "getTicket", app.IDInput{ID: first.ID}).(model.Ticket)
	if first.Refinement == nil {
		t.Fatal("metadata edit moved the older waiting phase behind newer work")
	}
}
