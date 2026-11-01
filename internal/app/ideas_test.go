package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestIdeaProposalAcceptanceIsAtomicAndRetainsIdentity(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Ideas", "requestId": "p"}).(model.Project)
	idea := call(t, svc, token, "saveIdea", app.SaveIdeaInput{Title: "A better boarding process", Text: "Keep the source request", RequestID: "idea"}).(model.Idea)
	agent := false
	proposal := call(t, svc, token, "submitIdeaProposal", app.ProposeIdeaInput{ID: idea.ID, ExpectedVersion: idea.Version, RequestID: "proposal", Targets: []model.IdeaTarget{{Create: &model.IdeaCreate{ProjectID: p.ID, Title: "Prepare boarding cards", Agent: &agent}}}}).(model.IdeaProposal)
	input := app.AcceptIdeaInput{ID: idea.ID, ExpectedVersion: idea.Version, ProposalID: proposal.ID}
	// A public receipt chosen from a known proposal ID must never become
	// acceptance's internal creation identity.
	preexisting := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Prepare boarding cards", Agent: &agent, RequestID: proposal.ID + ":0"}).(model.Ticket)
	call(t, svc, token, "transitionTicket", app.TransitionInput{ID: preexisting.ID, ExpectedVersion: preexisting.Version, Target: "Done", Confirm: true})
	accepted := call(t, svc, token, "acceptIdeaProposal", input).(model.IdeaAcceptance)
	repeated := call(t, svc, token, "acceptIdeaProposal", input).(model.IdeaAcceptance)
	if len(accepted.TicketIDs) != 1 || accepted.TicketIDs[0] != repeated.TicketIDs[0] {
		t.Fatal("acceptance duplicated work")
	}
	if accepted.TicketIDs[0] == preexisting.ID {
		t.Fatal("acceptance reused an attacker-selected public creation receipt")
	}
	ticket := call(t, svc, token, "getTicket", app.IDInput{ID: accepted.TicketIDs[0]}).(model.Ticket)
	if ticket.Status != "Todo" || ticket.Active {
		t.Fatal("idea acceptance authorized execution")
	}
	detail := call(t, svc, token, "getIdea", app.IdeaInput{ID: idea.ID}).(app.IdeaDetail)
	if detail.Idea.ID != idea.ID || detail.Idea.Text != idea.Text || detail.Idea.Archived || detail.Idea.AcceptedCount != 1 || len(detail.Links) != 1 {
		t.Fatal("acceptance rewrote or hid source idea")
	}
	links := call(t, svc, token, "getTicketIdeas", app.IDInput{ID: ticket.ID}).(app.Page[model.IdeaLink]).Items
	if len(links) != 1 || links[0].IdeaID != idea.ID || links[0].IdeaVersion != idea.Version {
		t.Fatal("bidirectional versioned provenance missing")
	}
	idea = detail.Idea
	proposal = call(t, svc, token, "submitIdeaProposal", app.ProposeIdeaInput{ID: idea.ID, ExpectedVersion: idea.Version, RequestID: "invalid-batch", Targets: []model.IdeaTarget{{Create: &model.IdeaCreate{ProjectID: p.ID, Title: "Must roll back"}}, {Create: &model.IdeaCreate{ProjectID: p.ID, ParentID: 999999, Title: "Invalid parent"}}}}).(model.IdeaProposal)
	before := call(t, svc, token, "listTickets", app.ListInput{ProjectID: p.ID}).(app.Page[model.Ticket])
	raw, _ := json.Marshal(app.AcceptIdeaInput{ID: idea.ID, ExpectedVersion: idea.Version, ProposalID: proposal.ID})
	if _, err := svc.Call(context.Background(), token, "acceptIdeaProposal", raw); err == nil {
		t.Fatal("invalid batch accepted")
	}
	after := call(t, svc, token, "listTickets", app.ListInput{ProjectID: p.ID}).(app.Page[model.Ticket])
	if len(before.Items) != len(after.Items) {
		t.Fatal("invalid acceptance created partial work")
	}
}

func TestIdeaAccessPlacementAndStaleProposalGuards(t *testing.T) {
	svc, admin, db := fixtureWithStore(t)
	p := call(t, svc, admin, "createProject", map[string]any{"name": "Ideas access", "key": "ideas-access", "requestId": "p"}).(model.Project)
	member := "idea-test-session"
	sum := sha256.Sum256([]byte(member))
	setRole := func(r string) {
		t.Helper()
		if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
			st.Accounts["member"] = model.Account{ID: "member", Username: "member"}
			st.Sessions[hex.EncodeToString(sum[:])] = model.Session{AccountID: "member", Expires: time.Now().Add(time.Hour)}
			project := st.Projects[p.ID]
			project.Members["member"] = r
			st.Projects[p.ID] = project
			return true, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	setRole("Contributor")
	idea := call(t, svc, admin, "saveIdea", app.SaveIdeaInput{Title: "Private source", Text: "Do not expose", RequestID: "private"}).(model.Idea)
	denied := func(token, op string, input any) {
		t.Helper()
		body, _ := json.Marshal(input)
		if _, err := svc.Call(context.Background(), token, op, body); err == nil {
			t.Fatalf("%s unexpectedly allowed", op)
		}
	}
	denied(member, "getIdea", app.IdeaInput{ID: idea.ID})
	denied(member, "getIdeaHistory", app.IdeaInput{ID: idea.ID})
	page := call(t, svc, member, "listIdeas", app.ListIdeasInput{}).(app.Page[model.Idea])
	if len(page.Items) != 0 {
		t.Fatal("private inbox leaked into list")
	}
	proposal := call(t, svc, admin, "submitIdeaProposal", app.ProposeIdeaInput{ID: idea.ID, ExpectedVersion: idea.Version, RequestID: "private-proposal", Targets: []model.IdeaTarget{{Create: &model.IdeaCreate{ProjectID: p.ID, Title: "Public draft"}}}}).(model.IdeaProposal)
	accepted := call(t, svc, admin, "acceptIdeaProposal", app.AcceptIdeaInput{ID: idea.ID, ExpectedVersion: idea.Version, ProposalID: proposal.ID}).(model.IdeaAcceptance)
	links := call(t, svc, member, "getTicketIdeas", app.IDInput{ID: accepted.TicketIDs[0]}).(app.Page[model.IdeaLink]).Items
	if len(links) != 0 {
		t.Fatal("ticket navigation disclosed private source")
	}
	idea = call(t, svc, admin, "getIdea", app.IdeaInput{ID: idea.ID}).(app.IdeaDetail).Idea
	denied(admin, "setIdeaPlacement", app.IdeaControlInput{ID: idea.ID, ExpectedVersion: idea.Version, ProjectID: p.ID})
	preview := call(t, svc, admin, "previewIdeaPlacement", app.IdeaControlInput{ID: idea.ID, ExpectedVersion: idea.Version, ProjectID: p.ID}).(app.IdeaPlacementPreview)
	if preview.Warning == "" {
		t.Fatal("publication warning missing")
	}
	idea = call(t, svc, admin, "setIdeaPlacement", app.IdeaControlInput{ID: idea.ID, ExpectedVersion: idea.Version, ProjectID: p.ID, Confirm: true}).(model.Idea)
	call(t, svc, member, "getIdeaHistory", app.IdeaInput{ID: idea.ID})
	denied(member, "setIdeaVisibility", app.IdeaControlInput{ID: idea.ID, ExpectedVersion: idea.Version, Archived: ptr(true)})
	setRole("Viewer")
	denied(member, "saveIdea", app.SaveIdeaInput{ID: idea.ID, ExpectedVersion: idea.Version, Title: "Unauthorized rewrite"})
	setRole("Contributor")
	proposal = call(t, svc, member, "submitIdeaProposal", app.ProposeIdeaInput{ID: idea.ID, ExpectedVersion: idea.Version, RequestID: "stale-proposal", Targets: []model.IdeaTarget{{Create: &model.IdeaCreate{ProjectID: p.ID, Title: "Stale draft"}}}}).(model.IdeaProposal)
	idea = call(t, svc, member, "saveIdea", app.SaveIdeaInput{ID: idea.ID, ExpectedVersion: idea.Version, Title: "Changed source", Text: idea.Text}).(model.Idea)
	denied(member, "acceptIdeaProposal", app.AcceptIdeaInput{ID: idea.ID, ExpectedVersion: idea.Version, ProposalID: proposal.ID})
	idea = call(t, svc, admin, "setIdeaVisibility", app.IdeaControlInput{ID: idea.ID, ExpectedVersion: idea.Version, Archived: ptr(true)}).(model.Idea)
	denied(admin, "acceptIdeaProposal", app.AcceptIdeaInput{ID: idea.ID, ExpectedVersion: idea.Version, ProposalID: proposal.ID})
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		i := st.Ideas[idea.ID]
		i.CurationState = "unknown"
		st.Ideas[i.ID] = i
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	denied(admin, "setIdeaPlacement", app.IdeaControlInput{ID: idea.ID, ExpectedVersion: idea.Version, Confirm: true})
}

func ptr[T any](v T) *T { return &v }

func TestIdeaReadsRemainBoundedWithRetainedContinuation(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Bounded Ideas", "key": "bounded-ideas", "requestId": "p"}).(model.Project)
	i := call(t, svc, token, "saveIdea", app.SaveIdeaInput{Title: "Retained thought", RequestID: "i"}).(model.Idea)
	ticket := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Existing work", RequestID: "t"}).(model.Ticket)
	first := ""
	for n := range 23 {
		proposal := call(t, svc, token, "submitIdeaProposal", app.ProposeIdeaInput{ID: i.ID, ExpectedVersion: i.Version, RequestID: fmt.Sprintf("p%d", n), Targets: []model.IdeaTarget{{ExistingID: ticket.ID, ExistingVersion: ticket.Version}}}).(model.IdeaProposal)
		if n == 0 {
			first = proposal.ID
		}
		if n < 3 {
			call(t, svc, token, "acceptIdeaProposal", app.AcceptIdeaInput{ID: i.ID, ExpectedVersion: i.Version, ProposalID: proposal.ID})
			i = call(t, svc, token, "getIdea", app.IdeaInput{ID: i.ID}).(app.IdeaDetail).Idea
		}
	}
	detail := call(t, svc, token, "getIdea", app.IdeaInput{ID: i.ID}).(app.IdeaDetail)
	if len(detail.Proposals) != 20 || !detail.HasOlder {
		t.Fatal("detail response is unbounded")
	}
	old := call(t, svc, token, "getIdeaProposal", app.IdeaProposalInput{ID: i.ID, ProposalID: first}).(app.IdeaProposalDetail)
	if old.Acceptance == nil || old.Proposal.ID != first {
		t.Fatal("older proposal/acceptance became inaccessible")
	}
	page := call(t, svc, token, "getTicketIdeas", app.IDInput{ID: ticket.ID, Limit: 1}).(app.Page[model.IdeaLink])
	if len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatal("provenance response is unbounded")
	}
	next := call(t, svc, token, "getTicketIdeas", app.IDInput{ID: ticket.ID, Limit: 1, Cursor: page.NextCursor}).(app.Page[model.IdeaLink])
	if len(next.Items) != 1 || next.Items[0].ProposalID == page.Items[0].ProposalID {
		t.Fatal("provenance continuation repeats or loses links")
	}
}
