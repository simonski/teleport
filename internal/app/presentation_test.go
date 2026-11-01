package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestGraphContextLayoutVersionsAndTicketIndependence(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Graph", "key": "graph", "requestId": "p"}).(model.Project)
	ticket := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "A matching task", RequestID: "t"}).(model.Ticket)
	graph := call(t, svc, token, "getGraph", app.GraphInput{ProjectID: p.ID, Query: "matching"}).(app.Graph)
	if len(graph.Nodes) != 4 {
		t.Fatalf("required project/ancestors absent: %d", len(graph.Nodes))
	}
	seen := map[string]bool{}
	for _, node := range graph.Nodes {
		if seen[node.ID] {
			t.Fatal("duplicate entity")
		}
		seen[node.ID] = true
		if node.TicketID != ticket.ID && !node.Context {
			t.Fatal("filtered ancestor lacks context flag")
		}
	}
	key := app.GraphTicketID(ticket.ID)
	input := app.GraphLayoutInput{ProjectID: p.ID, ExpectedVersion: graph.Layout.Version, Positions: map[string]model.Point3{key: {X: 34, Y: 56, Z: 78}}}
	saved := call(t, svc, token, "saveGraphLayout", input).(model.GraphLayout)
	if saved.Version != 1 {
		t.Fatal("layout version missing")
	}
	body, _ := json.Marshal(input)
	if _, err := svc.Call(context.Background(), token, "saveGraphLayout", body); err == nil {
		t.Fatal("stale layout overwrote arrangement")
	}
	retained := call(t, svc, token, "getTicket", app.IDInput{ID: ticket.ID}).(model.Ticket)
	if retained.Version != ticket.Version || retained.ParentID != ticket.ParentID {
		t.Fatal("layout changed ticket history or hierarchy")
	}
	all := call(t, svc, token, "getGraph", app.GraphInput{}).(app.Graph)
	if all.Layout.Version != 0 {
		t.Fatal("shared coordinates became the All projects arrangement")
	}
	graph = call(t, svc, token, "getGraph", app.GraphInput{ProjectID: p.ID}).(app.Graph)
	if graph.Layout.Positions[key].Z != 78 {
		t.Fatal("shared layout was not retained")
	}
}

func TestGraphViewerCannotPublishAndPreferencesRevalidateAccess(t *testing.T) {
	svc, admin, db := fixtureWithStore(t)
	p := call(t, svc, admin, "createProject", app.CreateProjectInput{Name: "Visible", RequestID: "p"}).(model.Project)
	other := call(t, svc, admin, "createProject", app.CreateProjectInput{Name: "Foreign", RequestID: "other"}).(model.Project)
	viewer := "graph-viewer"
	hash := sha256.Sum256([]byte(viewer))
	setRole := func(role string) {
		t.Helper()
		if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
			st.Accounts["viewer"] = model.Account{ID: "viewer", Username: "viewer"}
			st.Sessions[hex.EncodeToString(hash[:])] = model.Session{AccountID: "viewer", Expires: time.Now().Add(time.Hour)}
			project := st.Projects[p.ID]
			if role == "" {
				delete(project.Members, "viewer")
			} else {
				project.Members["viewer"] = role
			}
			st.Projects[p.ID] = project
			return true, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	setRole("Viewer")
	denied := func(op string, input any) {
		t.Helper()
		body, _ := json.Marshal(input)
		if _, err := svc.Call(context.Background(), viewer, op, body); err == nil {
			t.Fatal("unauthorized operation", op)
		}
	}
	denied("saveGraphLayout", app.GraphLayoutInput{ProjectID: p.ID, Positions: map[string]model.Point3{app.GraphTicketID(p.UnassignedEpic): {X: 1}}})
	private := call(t, svc, viewer, "saveGraphLayout", app.GraphLayoutInput{Positions: map[string]model.Point3{app.GraphTicketID(p.UnassignedEpic): {X: 1}}}).(model.GraphLayout)
	denied("saveGraphLayout", app.GraphLayoutInput{ExpectedVersion: private.Version, Positions: map[string]model.Point3{app.GraphTicketID(other.UnassignedEpic): {X: 2}}})
	prefs := call(t, svc, viewer, "setAppearance", app.AppearanceInput{Style: "instrument", Mode: "dark"}).(model.Preferences)
	call(t, svc, viewer, "saveViewPreferences", app.ViewPreferencesInput{ExpectedVersion: prefs.Version, DefaultView: "Board", DefaultProject: p.ID, Recent: []model.RecentContext{{View: "Board", ProjectID: p.ID}}})
	setRole("")
	graph := call(t, svc, viewer, "getGraph", app.GraphInput{}).(app.Graph)
	if len(graph.Nodes) != 0 || len(graph.Layout.Positions) != 0 {
		t.Fatal("retained private layout leaked revoked project context")
	}
	prefs = call(t, svc, viewer, "getViewPreferences", struct{}{}).(model.Preferences)
	if prefs.DefaultProject != 0 || len(prefs.Recent) != 0 || prefs.Style != "instrument" {
		t.Fatal("preferences disclosed revoked context or lost personal appearance")
	}
}

func TestPersonalAppearanceOptimisticUpdatesPreserveNavigation(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "getViewPreferences", struct{}{}).(model.Preferences)
	p = call(t, svc, token, "saveViewPreferences", app.ViewPreferencesInput{ExpectedVersion: p.Version, DefaultView: "Ideas", Recent: []model.RecentContext{{View: "Ideas"}}}).(model.Preferences)
	p = call(t, svc, token, "setAppearance", app.AppearanceInput{ExpectedVersion: p.Version, Style: "instrument", Mode: "dark"}).(model.Preferences)
	if p.DefaultView != "Ideas" || len(p.Recent) != 1 || p.Style != "instrument" || p.Mode != "dark" {
		t.Fatal("appearance replaced navigation")
	}
	body, _ := json.Marshal(app.AppearanceInput{ExpectedVersion: p.Version - 1, Style: "paper", Mode: "light"})
	if _, err := svc.Call(context.Background(), token, "setAppearance", body); err == nil {
		t.Fatal("stale preference update succeeded")
	}
}
