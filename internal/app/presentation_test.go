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
	if p.DefaultView != "Backlog" || len(p.Recent) != 1 || p.Recent[0].View != "Backlog" || p.Style != "instrument" || p.Mode != "dark" {
		t.Fatal("appearance replaced navigation")
	}
	body, _ := json.Marshal(app.AppearanceInput{ExpectedVersion: p.Version - 1, Style: "paper", Mode: "light"})
	if _, err := svc.Call(context.Background(), token, "setAppearance", body); err == nil {
		t.Fatal("stale preference update succeeded")
	}
}

func TestGraphProjectionLayoutsAreIndependent(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Projection", RequestID: "projection"}).(model.Project)
	ticket := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Card", RequestID: "card"}).(model.Ticket)
	id := app.GraphTicketID(ticket.ID)
	three := model.Point3{X: 30, Y: 40, Z: 50}
	two := model.Point3{X: 300, Y: 400}
	call(t, svc, token, "saveGraphLayout", app.GraphLayoutInput{ProjectID: p.ID, Positions: map[string]model.Point3{id: three}})
	saved := call(t, svc, token, "saveGraphLayout", app.GraphLayoutInput{Projection: "2d", ProjectID: p.ID, Positions: map[string]model.Point3{id: two}}).(model.GraphLayout)
	for _, projection := range []string{"", "3d", "2d"} {
		g := call(t, svc, token, "getGraph", app.GraphInput{ProjectID: p.ID, Projection: projection}).(app.Graph)
		expected := three
		if projection == "2d" {
			expected = two
		}
		if g.Layout.Version != 1 || g.Layout.Positions[id] != expected {
			t.Fatalf("projection %q lost its own layout: %+v", projection, g.Layout)
		}
	}
	for _, denied := range []struct {
		op   string
		in   any
		code string
	}{
		{"getGraph", app.GraphInput{Projection: "flat"}, "invalid_graph_projection"},
		{"saveGraphLayout", app.GraphLayoutInput{Projection: "flat"}, "invalid_graph_projection"},
		{"saveGraphLayout", app.GraphLayoutInput{Projection: "2d", ProjectID: p.ID, ExpectedVersion: saved.Version, Positions: map[string]model.Point3{id: three}}, "invalid_coordinate"},
		{"saveGraphLayout", app.GraphLayoutInput{Projection: "2d", ProjectID: p.ID, Positions: map[string]model.Point3{id: two}}, "layout_conflict"},
	} {
		body, _ := json.Marshal(denied.in)
		if _, err := svc.Call(context.Background(), token, denied.op, body); err == nil || app.ErrorResponse(err).Code != denied.code {
			t.Fatalf("%s should fail with %s: %v", denied.op, denied.code, err)
		}
	}
	current := call(t, svc, token, "getTicket", app.IDInput{ID: ticket.ID}).(model.Ticket)
	if current.Version != ticket.Version || current.ParentID != ticket.ParentID {
		t.Fatal("projection layout changed work")
	}
}

func TestStudioStyleRetainsSavedNavigationAcrossReload(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	position := "left"
	p := call(t, svc, token, "saveViewPreferences", app.ViewPreferencesInput{DefaultView: "Graph", NavigationPosition: &position}).(model.Preferences)
	p = call(t, svc, token, "setAppearance", app.AppearanceInput{ExpectedVersion: p.Version, Style: "studio", Mode: "dark"}).(model.Preferences)
	loaded := call(t, app.New(db), token, "getViewPreferences", struct{}{}).(model.Preferences)
	if loaded.Style != "studio" || loaded.Mode != "dark" || loaded.NavigationPosition != "left" || loaded.DefaultView != "Graph" || loaded.Version != p.Version {
		t.Fatal("Studio changed or lost saved preferences", loaded)
	}
}
