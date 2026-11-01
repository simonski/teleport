package app_test

import (
	"context"
	"teleport/internal/app"
	"teleport/internal/model"
	"testing"
)

func TestBoardLockAndCountsPreserveProjectPoolsAndUnknownReservations(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Board", RequestID: "board"}).(model.Project)
	p = call(t, svc, token, "setLaneCapacity", app.CapacityInput{ProjectID: p.ID, ExpectedVersion: p.Version, RefineWorkers: 2, DoingWorkers: 3}).(model.Project)
	p = call(t, svc, token, "setHandbrake", app.HandbrakeInput{ProjectID: p.ID, ExpectedVersion: p.Version, WorkflowKey: "agent@1", Engaged: true}).(model.Project)
	if p.Handbrake || !p.LaneLocks["agent#*"] || p.LaneLocks["human#*"] {
		t.Fatal("Board lock changed another scope", p)
	}
	ticket := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Unresolved reservation", RequestID: "t"}).(model.Ticket)
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		ticket.Phase, ticket.Status, ticket.Claim = "Doing", "Doing", "unknown-owner"
		st.Tickets[ticket.ID] = ticket
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	result := call(t, svc, token, "getBoardExecution", app.BoardExecutionInput{ProjectID: p.ID, WorkflowKey: "agent@1"}).(app.BoardExecution)
	if !result.Braked {
		t.Fatal("Board lock missing")
	}
	for _, phase := range result.Phases {
		if phase.PhaseID == "Doing" && (phase.Active != 0 || phase.Reserved != 1 || phase.Unresolved != 1 || phase.Capacity != 3) {
			t.Fatal("unknown reservation became active or lost capacity", phase)
		}
	}
}
