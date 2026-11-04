package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"teleport/internal/app"
	"teleport/internal/model"
	"testing"
	"time"
)

func TestTicketProposalAcceptanceCreatesWorkOnce(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Proposal", RequestID: "project"}).(model.Project)
	agent := false
	source := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Source", Agent: &agent, RequestID: "source"}).(model.Ticket)
	proposal := call(t, svc, token, "submitTicketProposal", app.TicketProposalInput{TicketID: source.ID, ExpectedVersion: source.Version, RequestID: "proposal", Title: "Breakdown", Items: []model.TicketProposalItem{{Key: "e", Kind: "epic", Title: "Epic"}, {Key: "t", ParentKey: "e", Kind: "task", Title: "Task"}}}).(model.TicketProposal)
	if !call(t, svc, token, "getTicket", app.IDInput{ID: source.ID}).(model.Ticket).ProposalAttention {
		t.Fatal("proposal attention missing from ticket")
	}
	listed := call(t, svc, token, "listTickets", app.ListInput{ProjectID: p.ID}).(app.Page[model.Ticket])
	for _, ticket := range listed.Items {
		if ticket.ID == source.ID && !ticket.ProposalAttention {
			t.Fatal("proposal attention missing from listing")
		}
	}
	in := app.TicketProposalAcceptInput{TicketID: source.ID, ProposalID: proposal.ID, ExpectedVersion: proposal.Version, ExpectedTicketVersion: source.Version}
	accepted := call(t, svc, token, "acceptTicketProposal", in).(model.TicketProposal)
	again := call(t, svc, token, "acceptTicketProposal", in).(model.TicketProposal)
	if call(t, svc, token, "getTicket", app.IDInput{ID: source.ID}).(model.Ticket).ProposalAttention {
		t.Fatal("accepted proposal still needs attention")
	}
	if len(accepted.CreatedIDs) != 2 || len(again.CreatedIDs) != 2 || accepted.CreatedIDs[0] != again.CreatedIDs[0] {
		t.Fatal("acceptance was not idempotent")
	}
	child := call(t, svc, token, "getTicket", app.IDInput{ID: accepted.CreatedIDs[1]}).(model.Ticket)
	if child.ParentID != accepted.CreatedIDs[0] {
		t.Fatal("proposed hierarchy was not retained")
	}
}

func TestTicketProposalDiscardResetsReviewAndRetainsHistory(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	project := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Discard proposals", RequestID: "project"}).(model.Project)
	agent := false
	ticket := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: project.ID, Title: "Source", Agent: &agent, RequestID: "source"}).(model.Ticket)
	first := call(t, svc, token, "submitTicketProposal", app.TicketProposalInput{TicketID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "first", Title: "First", Items: []model.TicketProposalItem{{Key: "e", Kind: "epic", Title: "Epic"}}}).(model.TicketProposal)
	feedback := call(t, svc, token, "commentTicketProposal", app.TicketProposalFeedbackInput{TicketID: ticket.ID, ProposalID: first.ID, ExpectedVersion: first.Version, RequestID: "feedback", Text: "Try again"}).(model.TicketProposal)
	second := call(t, svc, token, "submitTicketProposal", app.TicketProposalInput{TicketID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "second", PreviousID: first.ID, Title: "Second", Items: []model.TicketProposalItem{{Key: "e", Kind: "epic", Title: "Epic"}}}).(model.TicketProposal)
	in := app.TicketProposalDiscardInput{TicketID: ticket.ID, ExpectedTicketVersion: ticket.Version, LatestID: second.ID, LatestVersion: second.Version, RequestID: "discard"}
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		if st.LocalSessions == nil {
			st.LocalSessions = map[string]model.LocalSession{}
		}
		st.LocalSessions["H99"] = model.LocalSession{ID: "H99", State: "unknown", ProposalRun: &model.ProposalRun{TicketID: ticket.ID}}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(in)
	if _, err := svc.Call(context.Background(), token, "discardTicketProposals", body); err == nil {
		t.Fatal("unresolved run did not block discard")
	}
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		row := st.LocalSessions["H99"]
		row.State = "exited"
		st.LocalSessions["H99"] = row
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	result := call(t, svc, token, "discardTicketProposals", in).(app.TicketProposalDiscardResult)
	if result.Discarded != 2 || call(t, svc, token, "discardTicketProposals", in).(app.TicketProposalDiscardResult) != result {
		t.Fatal("discard did not atomically retain its retry result")
	}
	rows := call(t, svc, token, "listTicketProposals", map[string]any{"ticketId": ticket.ID}).(app.TicketProposalList).Items
	if len(rows) != 2 || rows[0].DiscardedBy == "" || rows[1].DiscardedBy == "" || len(rows[0].Feedback) != 1 || feedback.Feedback[0].Text != "Try again" {
		t.Fatal("discard lost history")
	}
	if call(t, svc, token, "getTicket", app.IDInput{ID: ticket.ID}).(model.Ticket).ProposalAttention {
		t.Fatal("discarded proposal still requests attention")
	}
	accept, _ := json.Marshal(app.TicketProposalAcceptInput{TicketID: ticket.ID, ProposalID: second.ID, ExpectedVersion: second.Version, ExpectedTicketVersion: ticket.Version})
	if _, err := svc.Call(context.Background(), token, "acceptTicketProposal", accept); err == nil {
		t.Fatal("discarded proposal accepted")
	}
	revision, _ := json.Marshal(app.TicketProposalInput{TicketID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "revision", PreviousID: second.ID, Title: "Revision", Items: []model.TicketProposalItem{{Key: "e", Kind: "epic", Title: "Epic"}}})
	if _, err := svc.Call(context.Background(), token, "submitTicketProposal", revision); err == nil {
		t.Fatal("discarded proposal used as revision basis")
	}
	fresh := call(t, svc, token, "submitTicketProposal", app.TicketProposalInput{TicketID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "fresh", Title: "Fresh", Items: []model.TicketProposalItem{{Key: "e", Kind: "epic", Title: "Fresh epic"}}}).(model.TicketProposal)
	if fresh.PreviousID != "" || !call(t, svc, token, "getTicket", app.IDInput{ID: ticket.ID}).(model.Ticket).ProposalAttention {
		t.Fatal("fresh proposal did not reset review")
	}
	stale := in
	stale.RequestID = "stale-discard"
	body, _ = json.Marshal(stale)
	if _, err := svc.Call(context.Background(), token, "discardTicketProposals", body); err == nil {
		t.Fatal("stale review discarded fresh proposal")
	}
	approved := call(t, svc, token, "acceptTicketProposal", app.TicketProposalAcceptInput{TicketID: ticket.ID, ProposalID: fresh.ID, ExpectedVersion: fresh.Version, ExpectedTicketVersion: ticket.Version}).(model.TicketProposal)
	pending := call(t, svc, token, "submitTicketProposal", app.TicketProposalInput{TicketID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "another", Title: "Another", Items: []model.TicketProposalItem{{Key: "e", Kind: "epic", Title: "Other"}}}).(model.TicketProposal)
	call(t, svc, token, "discardTicketProposals", app.TicketProposalDiscardInput{TicketID: ticket.ID, ExpectedTicketVersion: ticket.Version, LatestID: pending.ID, LatestVersion: pending.Version, RequestID: "discard-again"})
	if len(approved.CreatedIDs) != 1 || call(t, svc, token, "getTicket", app.IDInput{ID: approved.CreatedIDs[0]}).(model.Ticket).ID != approved.CreatedIDs[0] {
		t.Fatal("discard removed accepted work")
	}
}

func TestProposalCredentialCannotCreateOrApproveTickets(t *testing.T) {
	svc, human, db := fixtureWithStore(t)
	p := call(t, svc, human, "createProject", app.CreateProjectInput{Name: "Scoped proposal", RequestID: "project"}).(model.Project)
	agent := false
	ticket := call(t, svc, human, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Source", Agent: &agent, RequestID: "ticket"}).(model.Ticket)
	credential := "proposal-fixture-token"
	hash := sha256.Sum256([]byte(credential))
	reader := "proposal-reader-token"
	readerHash := sha256.Sum256([]byte(reader))
	err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		project := st.Projects[p.ID]
		project.Repository = &model.RepositoryBinding{Version: 1}
		st.Projects[p.ID] = project
		definition := model.AgentDefinition{ID: "agent:planner", Version: 1, Harness: "codex"}
		st.AgentDefinitions = map[string]model.AgentDefinition{definition.ID: definition}
		st.AgentUsages = map[string]model.AgentUsage{"usage:planner": {ID: "usage:planner", ProjectID: p.ID, AgentID: definition.ID, Kind: "todo-proposal", Enabled: true, Version: 1}}
		if st.LocalSessions == nil {
			st.LocalSessions = map[string]model.LocalSession{}
		}
		st.LocalSessions["H42"] = model.LocalSession{ID: "H42", Creator: "admin", ProjectID: p.ID, State: "running", BindingVersion: 1, CredentialHash: hex.EncodeToString(hash[:]), ReadCredentialHash: hex.EncodeToString(readerHash[:]), CredentialExpires: time.Now().Add(time.Hour), Persona: &definition, ProposalRun: &model.ProposalRun{TicketID: ticket.ID, TicketVersion: ticket.Version, UsageID: "usage:planner", UsageVersion: 1}}
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	denied := func(operation string, input any) {
		data, _ := json.Marshal(input)
		if _, e := svc.Call(context.Background(), credential, operation, data); e == nil {
			t.Fatalf("proposal credential was allowed %s", operation)
		}
	}
	denied("createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Unauthorized", RequestID: "bad"})
	denied("acceptTicketProposal", app.TicketProposalAcceptInput{TicketID: ticket.ID})
	data, _ := json.Marshal(app.TicketProposalInput{TicketID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "proposal", Title: "Work", Items: []model.TicketProposalItem{{Key: "e", Kind: "epic", Title: "Epic"}}})
	if _, err = svc.Call(context.Background(), reader, "submitTicketProposal", data); err == nil {
		t.Fatal("read-only harness credential submitted a proposal")
	}
	if _, err = svc.Call(context.Background(), reader, "reportProposalRunUsage", json.RawMessage(`{}`)); err == nil {
		t.Fatal("read-only harness credential reported usage")
	}
	if _, err = svc.Call(context.Background(), reader, "getTicket", json.RawMessage(fmt.Sprintf(`{"id":%d}`, ticket.ID))); err != nil {
		t.Fatalf("read-only harness cannot inspect ticket: %v", err)
	}
	if _, err = svc.Call(context.Background(), credential, "submitTicketProposal", data); err != nil {
		t.Fatalf("scoped proposal submission: %v", err)
	}
}

func TestTicketProposalRejectsSupersededAndStaleAcceptance(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Review guards", RequestID: "project"}).(model.Project)
	agent := false
	source := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Source", Agent: &agent, RequestID: "source"}).(model.Ticket)
	first := call(t, svc, token, "submitTicketProposal", app.TicketProposalInput{TicketID: source.ID, ExpectedVersion: source.Version, RequestID: "first", Title: "First", Items: []model.TicketProposalItem{{Key: "e", Kind: "epic", Title: "Epic"}}}).(model.TicketProposal)
	second := call(t, svc, token, "submitTicketProposal", app.TicketProposalInput{TicketID: source.ID, ExpectedVersion: source.Version, RequestID: "second", PreviousID: first.ID, Title: "Second", Items: []model.TicketProposalItem{{Key: "e", Kind: "epic", Title: "Epic"}}}).(model.TicketProposal)
	tryAccept := func(proposal model.TicketProposal) error {
		raw, _ := json.Marshal(app.TicketProposalAcceptInput{TicketID: source.ID, ProposalID: proposal.ID, ExpectedVersion: proposal.Version, ExpectedTicketVersion: source.Version})
		_, err := svc.Call(context.Background(), token, "acceptTicketProposal", raw)
		return err
	}
	if tryAccept(first) == nil {
		t.Fatal("superseded proposal was accepted")
	}
	call(t, svc, token, "updateTicket", map[string]any{"id": source.ID, "expectedVersion": source.Version, "title": "Changed"})
	if tryAccept(second) == nil {
		t.Fatal("stale source was accepted")
	}
	list := call(t, svc, token, "listTicketProposals", map[string]any{"ticketId": source.ID}).(app.TicketProposalList)
	for _, proposal := range list.Items {
		if proposal.AcceptedBy != "" || len(proposal.CreatedIDs) != 0 {
			t.Fatal("failed acceptance created work")
		}
	}
}

func TestTicketProposalAcceptanceRollsBackAllCreatedWorkOnFailure(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	project := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Atomic proposal", RequestID: "project"}).(model.Project)
	agent := false
	source := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: project.ID, Title: "Source", Agent: &agent, RequestID: "source"}).(model.Ticket)
	p := call(t, svc, token, "submitTicketProposal", app.TicketProposalInput{TicketID: source.ID, ExpectedVersion: source.Version, RequestID: "proposal", Title: "Plan", Items: []model.TicketProposalItem{{Key: "e", Kind: "epic", Title: "Epic"}, {Key: "t", ParentKey: "e", Kind: "task", Title: "Task"}}}).(model.TicketProposal)
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		row := st.TicketProposals[p.ID]
		row.Items[1].ParentKey = "missing"
		st.TicketProposals[p.ID] = row
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := db.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(app.TicketProposalAcceptInput{TicketID: source.ID, ProposalID: p.ID, ExpectedVersion: p.Version, ExpectedTicketVersion: source.Version})
	if _, err = svc.Call(context.Background(), token, "acceptTicketProposal", body); err == nil {
		t.Fatal("invalid proposal accepted")
	}
	after, err := db.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Tickets) != len(before.Tickets) || after.TicketProposals[p.ID].AcceptedBy != "" {
		t.Fatal("failed acceptance committed partial work")
	}
}
