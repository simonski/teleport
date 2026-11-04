package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"teleport/internal/model"
)

func TestTicketProposalPreservesFeedbackAndChecksSource(t *testing.T) {
	st, actor, ticket, _ := managedFixture()
	ticket.Version = 1
	st.Tickets[ticket.ID] = ticket
	st.Receipts = map[string]model.Receipt{}
	input := json.RawMessage(`{"ticketId":3,"expectedVersion":1,"requestId":"proposal-1","title":"Breakdown","items":[{"key":"epic","kind":"epic","title":"First epic"}]}`)
	result, _, err := ticketProposalOperations(st, actor, "submitTicketProposal", input)
	if err != nil {
		t.Fatal(err)
	}
	proposal := result.(model.TicketProposal)
	feedback, _ := json.Marshal(TicketProposalFeedbackInput{TicketID: ticket.ID, ProposalID: proposal.ID, ExpectedVersion: proposal.Version, RequestID: "feedback-1", Text: "Split the epic"})
	if _, _, err = ticketProposalOperations(st, actor, "commentTicketProposal", feedback); err != nil {
		t.Fatal(err)
	}
	if len(st.TicketProposals[proposal.ID].Feedback) != 1 {
		t.Fatal("feedback was not retained")
	}
	input = json.RawMessage(`{"ticketId":3,"expectedVersion":2,"requestId":"stale","title":"Breakdown","items":[{"key":"epic","kind":"epic","title":"First epic"}]}`)
	if _, _, err = ticketProposalOperations(st, actor, "submitTicketProposal", input); err == nil {
		t.Fatal("stale source accepted")
	}
}

func TestProposalSessionCanSubmitOnlyItsOwnCurrentTicket(t *testing.T) {
	st, owner, ticket, _ := managedFixture()
	ticket.Version = 1
	st.Tickets[ticket.ID] = ticket
	st.Receipts = map[string]model.Receipt{}
	agent := model.AgentDefinition{ID: "agent:planner", Version: 2}
	st.AgentDefinitions = map[string]model.AgentDefinition{agent.ID: agent}
	st.AgentUsages = map[string]model.AgentUsage{"usage:planner": {ID: "usage:planner", ProjectID: ticket.ProjectID, AgentID: agent.ID, Kind: "todo-proposal", Enabled: true, Version: 1}}
	st.LocalSessions["H1"] = model.LocalSession{ID: "H1", Creator: owner.ID, ProjectID: ticket.ProjectID, State: "running", Persona: &agent, ProposalRun: &model.ProposalRun{TicketID: ticket.ID, TicketVersion: ticket.Version, UsageID: "usage:planner", UsageVersion: 1}}
	service := model.Account{ID: "agent:H1", ServiceSessionID: "H1", ServiceProjectID: ticket.ProjectID}
	input := json.RawMessage(`{"ticketId":3,"expectedVersion":1,"requestId":"agent-proposal","title":"Breakdown","items":[{"key":"e","kind":"epic","title":"Epic"}]}`)
	if _, _, err := ticketProposalOperations(st, service, "submitTicketProposal", input); err != nil {
		t.Fatal(err)
	}
	if st.LocalSessions["H1"].ProposalRun.ProposalID == "" {
		t.Fatal("run did not retain its proposal")
	}
	if _, changed, err := ticketProposalOperations(st, service, "submitTicketProposal", input); err != nil || changed {
		t.Fatal("retry did not return original proposal", err)
	}
	if _, _, err := ticketProposalOperations(st, service, "acceptTicketProposal", json.RawMessage(`{}`)); err == nil {
		t.Fatal("agent could approve its proposal")
	}
	other := json.RawMessage(`{"ticketId":3,"expectedVersion":1,"requestId":"second","title":"Breakdown","items":[{"key":"e","kind":"epic","title":"Epic"}]}`)
	if _, _, err := ticketProposalOperations(st, service, "submitTicketProposal", other); err == nil {
		t.Fatal("agent submitted a second proposal")
	}
}

func TestProposalAdmissionCountsUnresolvedPlanningReservations(t *testing.T) {
	st, _, ticket, _ := managedFixture()
	if err := proposalAdmission(st, ticket); err != nil {
		t.Fatal(err)
	}
	st.LocalSessions["H1"] = model.LocalSession{ID: "H1", ProjectID: ticket.ProjectID, State: "unknown", ProposalRun: &model.ProposalRun{TicketID: ticket.ID}}
	if err := proposalAdmission(st, ticket); err == nil {
		t.Fatal("unresolved planning did not block replacement")
	}
	row := st.LocalSessions["H1"]
	now := time.Now()
	row.CapacityReleasedAt = &now
	st.LocalSessions["H1"] = row
	if err := proposalAdmission(st, ticket); err == nil {
		t.Fatal("released capacity must not release unresolved ticket ownership")
	}
	row.CapacityReleasedAt = nil
	st.LocalSessions["H1"] = row
	other := ticket
	other.ID = 4
	st.Tickets[other.ID] = other
	if err := proposalAdmission(st, other); err == nil {
		t.Fatal("unresolved planning did not reserve Refine capacity")
	}
}

func TestTicketProposalRejectsFeedbackOnSupersededRound(t *testing.T) {
	st, actor, ticket, _ := managedFixture()
	ticket.Version = 1
	st.Tickets[ticket.ID] = ticket
	st.Receipts = map[string]model.Receipt{}
	first := json.RawMessage(`{"ticketId":3,"expectedVersion":1,"requestId":"first","title":"First","items":[{"key":"e","kind":"epic","title":"Epic"}]}`)
	result, _, err := ticketProposalOperations(st, actor, "submitTicketProposal", first)
	if err != nil {
		t.Fatal(err)
	}
	p := result.(model.TicketProposal)
	second, _ := json.Marshal(TicketProposalInput{TicketID: ticket.ID, ExpectedVersion: 1, RequestID: "second", PreviousID: p.ID, Title: "Second", Items: []model.TicketProposalItem{{Key: "e", Kind: "epic", Title: "Epic"}}})
	if _, _, err := ticketProposalOperations(st, actor, "submitTicketProposal", second); err != nil {
		t.Fatal(err)
	}
	feedback, _ := json.Marshal(TicketProposalFeedbackInput{TicketID: ticket.ID, ProposalID: p.ID, ExpectedVersion: p.Version, RequestID: "feedback", Text: "Too late"})
	if _, _, err := ticketProposalOperations(st, actor, "commentTicketProposal", feedback); err == nil {
		t.Fatal("accepted feedback on superseded round")
	}
}

func TestTicketProposalStorageBound(t *testing.T) {
	st := &model.State{TicketProposals: map[string]model.TicketProposal{}}
	p := model.TicketProposal{ID: "p", TicketID: 1, ProjectID: 1, Title: strings.Repeat("x", 256*1024)}
	if proposalStorageWithinLimits(st, p, nil) {
		t.Fatal("oversized proposal accepted")
	}
}

func TestProposalRequiresSuccessfulOwnedExit(t *testing.T) {
	zero, one := 0, 1
	p := model.TicketProposal{ID: "proposal:one", SessionID: "H1"}
	st := &model.State{LocalSessions: map[string]model.LocalSession{"H1": {ID: "H1", State: "running", ProposalRun: &model.ProposalRun{ProposalID: p.ID}}}}
	if proposalRunReady(st, p) {
		t.Fatal("live run accepted as complete")
	}
	row := st.LocalSessions["H1"]
	row.State = "exited"
	row.ExitCode = &one
	st.LocalSessions[row.ID] = row
	if proposalRunReady(st, p) {
		t.Fatal("failed run accepted as complete")
	}
	row.ExitCode = &zero
	row.StopRequested = true
	st.LocalSessions[row.ID] = row
	if proposalRunReady(st, p) {
		t.Fatal("stopped run accepted as complete")
	}
	row.StopRequested = false
	st.LocalSessions[row.ID] = row
	if !proposalRunReady(st, p) {
		t.Fatal("successful owned exit not accepted")
	}
}

func TestProposalAttentionTracksLatestReviewableRound(t *testing.T) {
	st, _, ticket, _ := managedFixture()
	now := time.Now().UTC()
	p := model.TicketProposal{ID: "p1", TicketID: ticket.ID, ProjectID: ticket.ProjectID, At: now}
	st.TicketProposals = map[string]model.TicketProposal{p.ID: p}
	if !proposalAttention(st)[ticket.ID] {
		t.Fatal("reviewable proposal needs attention")
	}
	newer := model.TicketProposal{ID: "p2", TicketID: ticket.ID, ProjectID: ticket.ProjectID, At: now.Add(time.Second), SessionID: "H1"}
	st.TicketProposals[newer.ID] = newer
	if proposalAttention(st)[ticket.ID] {
		t.Fatal("unfinished latest round must not be offered for review")
	}
	zero := 0
	st.LocalSessions["H1"] = model.LocalSession{ID: "H1", State: "exited", ExitCode: &zero, ProposalRun: &model.ProposalRun{ProposalID: newer.ID}}
	if !proposalAttention(st)[ticket.ID] {
		t.Fatal("successful latest round needs attention")
	}
	newer.AcceptedBy = "human"
	st.TicketProposals[newer.ID] = newer
	if proposalAttention(st)[ticket.ID] {
		t.Fatal("accepted proposal needs no attention")
	}
	newer.AcceptedBy = ""
	newer.ProjectID++
	st.TicketProposals[newer.ID] = newer
	if !proposalAttention(st)[ticket.ID] {
		t.Fatal("foreign project proposal affected ticket")
	}
}
