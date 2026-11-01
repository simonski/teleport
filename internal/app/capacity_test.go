package app

import (
	"testing"

	"teleport/internal/model"
)

func TestLaneQuotaSharesProjectPoolAndRetainsUnknownOwnership(t *testing.T) {
	one, twenty := 1, 20
	phase := model.Phase{ID: "lane:build", Role: "Doing", Agentic: true, Workers: &one}
	st := &model.State{
		Projects: map[int64]model.Project{1: {ID: 1, CapacityConfigured: true, DoingWorkers: 2, RefineWorkers: 1}},
		Workflows: map[string]model.Workflow{
			"w@1": {ID: "w", Phases: []model.Phase{phase, {ID: "Refine", Role: "Refine"}}},
			"w@2": {ID: "w", Phases: []model.Phase{phase}},
			"v@1": {ID: "v", Phases: []model.Phase{phase}},
		},
		Tickets: map[int64]model.Ticket{}, LocalSessions: map[string]model.LocalSession{},
	}
	candidate := model.Ticket{ID: 1, ProjectID: 1, Workflow: "w@2", Phase: phase.ID}
	check := func(want string) {
		t.Helper()
		err := phaseCapacity(st, candidate, phase)
		if want == "" && err != nil || want != "" && (err == nil || ErrorResponse(err).Code != want) {
			t.Fatalf("want %q, got %v", want, err)
		}
	}
	check("")
	st.Tickets[2] = model.Ticket{ID: 2, ProjectID: 1, Workflow: "w@1", Phase: phase.ID, Claim: "reserved"}
	check("lane_capacity_full") // Same stable lane across immutable versions.
	phase.Workers = &twenty
	check("")
	st.Tickets[3] = model.Ticket{ID: 3, ProjectID: 1, Workflow: "v@1", Phase: phase.ID, Execution: "local:H3"}
	st.LocalSessions["H3"] = model.LocalSession{ID: "H3", ProjectID: 1, TicketID: 3, State: "unknown"}
	check("capacity_full") // A high lane quota cannot multiply the project pool.
	delete(st.Tickets, 2)
	check("") // The matching ticket/session reservation is counted once.
	st.Tickets[4] = model.Ticket{ID: 4, ProjectID: 1, Workflow: "w@1", Phase: "Refine", Active: true}
	check("") // Refine capacity is independent.
	st.Tickets[3] = model.Ticket{ID: 3, ProjectID: 1, Workflow: "w@1", Phase: "Cancelled", Delivery: &model.Delivery{SourcePhase: phase.ID}}
	phase.Workers = &one
	check("lane_capacity_full") // A stop/unknown native instance keeps its source lane.
	row := st.LocalSessions["H3"]
	row.State = "exited"
	st.LocalSessions["H3"] = row
	check("")
	zero := 0
	phase.Workers = &zero
	check("lane_capacity_disabled")
	phase.Workers = nil
	check("") // Legacy pins inherit the shared pool.
	project := st.Projects[1]
	project.DoingWorkers = 0
	st.Projects[1] = project
	check("capacity_disabled")
}
