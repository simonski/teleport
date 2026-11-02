package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"testing"
	"time"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestHomeUsesLeafHistoryRatherThanLastMutationAndKeepsMissingUsageUnknown(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Overview", RequestID: "overview"}).(model.Project)
	parent := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Kind: "epic", Title: "Epic", RequestID: "epic"}).(model.Ticket)
	ids := []int64{}
	for _, name := range []string{"Done", "Doing", "Todo", "Cancelled", "Archived"} {
		ids = append(ids, call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: name, RequestID: name}).(model.Ticket).ID)
	}
	now := time.Now().UTC()
	start := now.Add(-2 * time.Hour)
	finish := now.Add(-time.Hour)
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		for index, id := range ids {
			ticket := st.Tickets[id]
			ticket.Status = []string{"Done", "Doing", "Todo", "Cancelled", "Done"}[index]
			ticket.Archived = index == 4
			ticket.UpdatedAt = now
			st.Tickets[id] = ticket
			before := ticket
			before.Status = "Doing"
			before.Attention = "Question"
			st.History[id] = []model.Revision{{At: start, Snapshot: before}, {At: finish, Snapshot: ticket}}
		}
		epic := st.Tickets[parent.ID]
		epic.Status = "Done"
		st.Tickets[parent.ID] = epic
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	out := call(t, svc, token, "getHome", app.HomeInput{ProjectID: p.ID, Days: 7}).(app.HomeSummary)
	if out.Total != 3 || out.Completed != 1 || out.Remaining != 2 || out.InProgress != 1 || out.CompletedInPeriod != 1 || len(out.Recent) != 1 || out.Recent[0] != ids[0] {
		t.Fatal("invalid leaf summary", out)
	}
	if math.Abs(out.TicketsPerDay-1.0/7) > 1e-10 || math.Abs(out.TicketsPerHour-1.0/168) > 1e-10 || out.ProgressSeconds < 18000 || out.ProgressSeconds > 18005 || math.Abs(out.AttentionWaitSeconds-14400) > 1e-6 {
		t.Fatal("incorrect retained elapsed/rate basis", out)
	}
	if out.Tokens != nil || out.AttentionSpentSeconds != nil {
		t.Fatal("invented metering", out)
	}
}

func TestHomeFencesProjectRightsAndPeriods(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Private", RequestID: "private"}).(model.Project)
	viewer := "home-viewer"
	hash := sha256.Sum256([]byte(viewer))
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		st.Accounts["viewer"] = model.Account{ID: "viewer", Username: "viewer"}
		st.Sessions[hex.EncodeToString(hash[:])] = model.Session{AccountID: "viewer", Expires: time.Now().Add(time.Hour)}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, input := range []app.HomeInput{{ProjectID: p.ID}, {Days: 1}} {
		raw, _ := json.Marshal(input)
		if _, err := svc.Call(context.Background(), viewer, "getHome", raw); err == nil {
			t.Fatal("invalid scope/period accepted")
		}
	}
	visible := call(t, svc, viewer, "getHome", app.HomeInput{}).(app.HomeSummary)
	if visible.Total != 0 || len(visible.Phases) != 0 {
		t.Fatal("private work leaked", visible)
	}
}
