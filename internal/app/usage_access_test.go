package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestUsageAttributionRespectsMovedContextAndPrivateSessions(t *testing.T) {
	svc, admin, db := fixtureWithStore(t)
	source := call(t, svc, admin, "createProject", app.CreateProjectInput{Name: "Source usage", RequestID: "source"}).(model.Project)
	dest := call(t, svc, admin, "createProject", app.CreateProjectInput{Name: "Private destination", RequestID: "dest"}).(model.Project)
	ticket := call(t, svc, admin, "createTicket", app.CreateTicketInput{ProjectID: dest.ID, Title: "Private destination ticket", RequestID: "ticket"}).(model.Ticket)
	token := "usage-viewer-token"
	hash := sha256.Sum256([]byte(token))
	now := time.Now().UTC()
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		st.Accounts["viewer"] = model.Account{ID: "viewer", Username: "viewer"}
		st.Sessions[hex.EncodeToString(hash[:])] = model.Session{AccountID: "viewer", Expires: now.Add(time.Hour)}
		p := st.Projects[source.ID]
		p.Members["viewer"] = "Contributor"
		st.Projects[source.ID] = p
		st.LocalSessions = map[string]model.LocalSession{
			"H1": {ID: "H1", Number: 1, Creator: "viewer", ProjectID: source.ID, Harness: "codex-pty", State: "exited", CreatedAt: now, Spellcheck: &model.SpellcheckRun{State: "completed"}, Attribution: &model.ExecutionAttribution{ProjectID: source.ID, TicketID: ticket.ID, EpicID: dest.UnassignedEpic, GoalID: dest.UnassignedGoal, HierarchyStatus: "complete", CallerKind: "human", CallerID: "viewer", InitiatorID: "viewer"}},
			"H2": {ID: "H2", Number: 2, Creator: "admin", ProjectID: source.ID, Harness: "codex-pty", State: "exited", CreatedAt: now},
			"H3": {ID: "H3", Number: 3, Creator: "admin", ProjectID: dest.ID, Harness: "codex-pty", State: "exited", CreatedAt: now},
		}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	first := call(t, svc, token, "listUsageBreakdown", app.UsageBreakdownInput{ProjectID: source.ID, GroupBy: "epic", Window: "all", Limit: 1}).(app.UsageBreakdownPage)
	if len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatalf("missing bounded page: %+v", first)
	}
	second := call(t, svc, token, "listUsageBreakdown", app.UsageBreakdownInput{ProjectID: source.ID, GroupBy: "epic", Window: "all", Cursor: first.NextCursor, Limit: 1}).(app.UsageBreakdownPage)
	if len(second.Items) != 1 || second.NextCursor != "" || first.Items[0].Key == second.Items[0].Key {
		t.Fatalf("incorrect cursor: %+v %+v", first, second)
	}
	labels := map[string]bool{first.Items[0].Label: true, second.Items[0].Label: true}
	if !labels["Restricted or moved context"] || !labels["Private sessions"] {
		t.Fatalf("private context leaked: %+v", labels)
	}
	at := call(t, svc, token, "getUsageAttribution", app.SessionIDInput{ID: "H1"}).(model.ExecutionAttribution)
	if at.TicketID != 0 || at.EpicID != 0 || at.GoalID != 0 || at.HierarchyStatus != "unavailable" {
		t.Fatalf("moved ticket disclosed: %+v", at)
	}
	if _, err := svc.Call(context.Background(), token, "getUsageAttribution", []byte(`{"id":"H2"}`)); err == nil {
		t.Fatal("private session detail exposed")
	}
}
