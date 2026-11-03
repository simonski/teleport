package app

import (
	"encoding/json"
	"testing"

	"teleport/internal/model"
)

func TestProjectAgentLimitCountsOwnedReservationsAndUnknownSessions(t *testing.T) {
	maximum := 2
	st := &model.State{Projects: map[int64]model.Project{1: {ID: 1, MaxAgents: &maximum}}, Tickets: map[int64]model.Ticket{}, LocalSessions: map[string]model.LocalSession{}}
	st.Tickets[7] = model.Ticket{ID: 7, ProjectID: 1, Agent: true, Active: true, Execution: "local:H1"}
	st.LocalSessions["H1"] = model.LocalSession{ID: "H1", ProjectID: 1, TicketID: 7, State: "unknown"}
	if err := projectAgentCapacity(st, 1); err != nil {
		t.Fatal("ticket and its session were counted twice", err)
	}
	st.LocalSessions["H2"] = model.LocalSession{ID: "H2", ProjectID: 1, State: "stopping"}
	if err := projectAgentCapacity(st, 1); err == nil || ErrorResponse(err).Code != "project_agent_limit" {
		t.Fatal("uncertain session did not reserve capacity")
	}
	row := st.LocalSessions["H2"]
	row.State = "exited"
	st.LocalSessions["H2"] = row
	if err := projectAgentCapacity(st, 1); err != nil {
		t.Fatal(err)
	}
	maximum = 0
	if err := projectAgentCapacity(st, 1); err == nil {
		t.Fatal("zero maximum did not block admission")
	}
}

func TestProjectAgentSettingsRequireMaintainerAndPreserveOmittedFields(t *testing.T) {
	maximum := 3
	allowed := []string{"codex"}
	st := &model.State{Projects: map[int64]model.Project{1: {ID: 1, Version: 1, MaxAgents: &maximum, AllowedHarnesses: &allowed, Members: map[string]string{"maintainer": "Maintainer", "viewer": "Viewer"}}}}
	apply := func(actor string, input CapacityInput) (model.Project, error) {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		value, _, err := agentOperations(st, model.Account{ID: actor}, "setLaneCapacity", raw)
		if err != nil {
			return model.Project{}, err
		}
		return value.(model.Project), nil
	}
	in := CapacityInput{ProjectID: 1, ExpectedVersion: 1, RefineWorkers: 1, DoingWorkers: 2}
	if _, err := apply("viewer", in); err == nil {
		t.Fatal("viewer changed project capacity")
	}
	p, err := apply("maintainer", in)
	if err != nil || p.MaxAgents == nil || *p.MaxAgents != 3 || !projectAllowsHarness(p, "codex") || projectAllowsHarness(p, "fixture") {
		t.Fatalf("omitted settings changed: %+v %v", p, err)
	}
	in.ExpectedVersion = p.Version
	in.MaxAgents = &maximum
	in.ClearMaxAgents = true
	if _, err := apply("maintainer", in); err == nil {
		t.Fatal("conflicting maximum accepted")
	}
	in.ClearMaxAgents = false
	invalid := []string{"codex", "codex"}
	in.AllowedHarnesses = &invalid
	if _, err := apply("maintainer", in); err == nil {
		t.Fatal("duplicate harness accepted")
	}
	invalid = []string{"unknown"}
	if _, err := apply("maintainer", in); err == nil {
		t.Fatal("unsupported harness accepted")
	}
	in.AllowedHarnesses = nil
	in.MaxAgents = nil
	in.ClearMaxAgents = true
	in.ClearHarnessPolicy = true
	p, err = apply("maintainer", in)
	if err != nil || p.MaxAgents != nil || p.AllowedHarnesses != nil {
		t.Fatalf("clearing failed: %+v %v", p, err)
	}
	if _, err := apply("maintainer", in); err == nil {
		t.Fatal("stale project version accepted")
	}
}

func TestManagedAdmissionRejectsDisallowedHarness(t *testing.T) {
	allowed := []string{}
	st := &model.State{Projects: map[int64]model.Project{1: {ID: 1, AllowedHarnesses: &allowed, Members: map[string]string{"owner": "Maintainer"}}}}
	err := managedAdmission(st, model.Account{ID: "owner"}, model.Ticket{ProjectID: 1}, model.Phase{})
	if err == nil || ErrorResponse(err).Code != "harness_not_permitted" {
		t.Fatalf("disallowed harness was not blocked: %v", err)
	}
}

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
