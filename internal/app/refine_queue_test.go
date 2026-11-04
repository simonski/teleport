package app_test

import (
	"context"
	"encoding/json"
	"teleport/internal/app"
	"teleport/internal/model"
	"testing"
)

func TestHumanTodoCanQueueAndReturnWithoutHarness(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Queue", RequestID: "queue"}).(model.Project)
	agent := false
	ticket := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Refine this draft", Agent: &agent, RequestID: "draft"}).(model.Ticket)
	usage := call(t, svc, token, "saveAgentUsage", app.AgentUsageInput{ProjectID: p.ID, Name: "Refine", Kind: "todo-refine", View: "Todos", Workflow: ticket.Workflow, Phase: ticket.Phase, Enabled: true, RequestID: "refine-usage"}).(model.AgentUsage)
	queued := call(t, svc, token, "requestRefinement", app.RefinementRequestInput{ID: ticket.ID, ExpectedVersion: ticket.Version, UsageID: usage.ID, UsageVersion: usage.Version}).(model.Ticket)
	if !queued.Agent || queued.Status != "Refine" || queued.Assignee != "" {
		t.Fatal("human draft was not handed to refinement", queued)
	}
	returned := call(t, svc, token, "returnRefinementToTodo", app.VersionInput{ID: queued.ID, ExpectedVersion: queued.Version}).(model.Ticket)
	if returned.Agent || returned.Status != "Todo" || returned.Assignee != "admin" || returned.ID != ticket.ID {
		t.Fatal("queued draft did not return to human Todos", returned)
	}
}

func TestRefiningOwnershipAndRetainedImplementationPreventReturn(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Owned queue", RequestID: "owned"}).(model.Project)
	ticket := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Owned refinement", RequestID: "ticket"}).(model.Ticket)
	ticket = call(t, svc, token, "requestRefinement", app.VersionInput{ID: ticket.ID, ExpectedVersion: ticket.Version}).(model.Ticket)
	for _, ownership := range []string{"active", "unknown", "implementation"} {
		err := db.Update(context.Background(), func(st *model.State) (bool, error) {
			current := st.Tickets[ticket.ID]
			current.Active = ownership == "active"
			st.Tickets[ticket.ID] = current
			st.LocalSessions = map[string]model.LocalSession{}
			if ownership == "unknown" {
				st.LocalSessions["H1"] = model.LocalSession{TicketID: ticket.ID, ProjectID: p.ID, State: "unknown"}
			}
			if ownership == "implementation" {
				prior := current
				prior.Status = "Doing"
				st.History[ticket.ID] = append(st.History[ticket.ID], model.Revision{Snapshot: prior})
			}
			return true, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(app.VersionInput{ID: ticket.ID, ExpectedVersion: ticket.Version})
		if _, err = svc.Call(context.Background(), token, "returnRefinementToTodo", raw); err == nil {
			t.Fatal("return bypassed", ownership)
		}
	}
}
