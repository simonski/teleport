package app

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"teleport/internal/model"
)

type GraphInput struct {
	Projection string `json:"projection,omitempty"`
	ProjectID  int64  `json:"projectId,omitempty"`
	Query      string `json:"query,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Agent      *bool  `json:"agent,omitempty"`
	MaxNodes   int    `json:"maxNodes,omitempty"`
}
type GraphNode struct {
	ID        string       `json:"id"`
	ProjectID int64        `json:"projectId"`
	TicketID  int64        `json:"ticketId,omitempty"`
	Title     string       `json:"title"`
	Kind      string       `json:"kind"`
	Status    string       `json:"status"`
	Progress  int          `json:"progress"`
	Context   bool         `json:"context"`
	Position  model.Point3 `json:"position"`
}
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
}
type Graph struct {
	Nodes   []GraphNode       `json:"nodes"`
	Edges   []GraphEdge       `json:"edges"`
	Layout  model.GraphLayout `json:"layout"`
	CanSave bool              `json:"canSave"`
}
type GraphLayoutInput struct {
	Projection      string                  `json:"projection,omitempty"`
	ProjectID       int64                   `json:"projectId,omitempty"`
	ExpectedVersion int64                   `json:"expectedVersion"`
	Positions       map[string]model.Point3 `json:"positions"`
}
type AppearanceInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Style           string `json:"style"`
	Mode            string `json:"mode"`
}
type ViewPreferencesInput struct {
	ExpectedVersion     int64                 `json:"expectedVersion"`
	DefaultView         string                `json:"defaultView"`
	DefaultProject      int64                 `json:"defaultProject"`
	Recent              []model.RecentContext `json:"recent"`
	NavigationOrder     *[]string             `json:"navigationOrder,omitempty"`
	NavigationPosition  *string               `json:"navigationPosition,omitempty"`
	NavigationCollapsed *bool                 `json:"navigationCollapsed,omitempty"`
}
type AppearanceStyles struct {
	Styles []string `json:"styles"`
	Modes  []string `json:"modes"`
}

func GraphTicketID(id int64) string  { return fmt.Sprintf("ticket:%d", id) }
func graphProjectID(id int64) string { return fmt.Sprintf("project:%d", id) }
func graphScope(a model.Account, project int64, projection string) string {
	suffix := ""
	if projection == "2d" {
		suffix = ":2d"
	}
	if project != 0 {
		return graphProjectID(project) + suffix
	}
	return "account:" + a.ID + suffix
}
func graphNodeProject(st *model.State, key string) (int64, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 2 {
		return 0, failure(422, "invalid_node", "Use a current graph node identity")
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id < 1 || parts[1] != strconv.FormatInt(id, 10) {
		return 0, failure(422, "invalid_node", "Use a positive graph identity")
	}
	switch parts[0] {
	case "project":
		if _, ok := st.Projects[id]; ok {
			return id, nil
		}
	case "ticket":
		if t, ok := st.Tickets[id]; ok {
			return t.ProjectID, nil
		}
	}
	return 0, failure(404, "not_found", "Graph node not found")
}
func readGraph(st *model.State, a model.Account, in GraphInput) (Graph, error) {
	if !contains([]string{"", "2d", "3d"}, in.Projection) {
		return Graph{}, failure(422, "invalid_graph_projection", "Choose 2d or 3d")
	}
	if in.MaxNodes == 0 {
		in.MaxNodes = 1000
	}
	if in.MaxNodes < 1 || in.MaxNodes > 2000 || in.Kind != "" && !contains([]string{"goal", "epic", "task"}, in.Kind) {
		return Graph{}, failure(422, "invalid_graph_filter", "Choose a ticket type and 1–2,000 nodes")
	}
	if in.ProjectID != 0 {
		if err := require(st, a, in.ProjectID, "Viewer"); err != nil {
			return Graph{}, err
		}
	}
	selected := map[int64]bool{}
	context := map[int64]bool{}
	projects := map[int64]bool{}
	for _, t := range st.Tickets {
		if role(st, a, t.ProjectID) == "" || in.ProjectID != 0 && t.ProjectID != in.ProjectID || t.Archived || t.Deleted || in.Kind != "" && t.Kind != in.Kind || in.Agent != nil && t.Agent != *in.Agent {
			continue
		}
		matches := true
		for _, token := range strings.Fields(strings.ToLower(in.Query)) {
			if !strings.Contains(strings.ToLower(t.Title+" "+t.Description+" "+strings.Join(t.Labels, " ")), token) {
				matches = false
				break
			}
		}
		if matches {
			selected[t.ID] = true
		}
	}
	var addContext func(int64)
	addContext = func(id int64) {
		if id == 0 || selected[id] || context[id] {
			return
		}
		t, ok := st.Tickets[id]
		if !ok || role(st, a, t.ProjectID) == "" || in.ProjectID != 0 && t.ProjectID != in.ProjectID {
			return
		}
		context[id] = true
		addContext(t.ParentID)
	}
	for id := range selected {
		t := st.Tickets[id]
		addContext(t.ParentID)
		for _, dep := range effectiveDependencies(st, t) {
			addContext(dep)
		}
	}
	for id := range context {
		selected[id] = true
	}
	ids := []int64{}
	for id := range selected {
		ids = append(ids, id)
		projects[st.Tickets[id].ProjectID] = true
	}
	sort.Slice(ids, func(x, y int) bool { return ids[x] < ids[y] })
	// Empty unfiltered projects remain navigable; filtered graphs only need context projects.
	if in.Query == "" && in.Kind == "" && in.Agent == nil {
		for id := range st.Projects {
			if role(st, a, id) != "" && (in.ProjectID == 0 || id == in.ProjectID) {
				projects[id] = true
			}
		}
	}
	if len(ids)+len(projects) > in.MaxNodes {
		return Graph{}, failure(422, "graph_too_large", "Narrow project/type/search before loading more than the selected node limit")
	}
	layout := st.GraphLayouts[graphScope(a, in.ProjectID, in.Projection)]
	g := Graph{Nodes: []GraphNode{}, Edges: []GraphEdge{}, Layout: model.GraphLayout{Version: layout.Version, Positions: map[string]model.Point3{}}, CanSave: in.ProjectID == 0 || require(st, a, in.ProjectID, "Contributor") == nil}
	projectIDs := []int64{}
	for id := range projects {
		projectIDs = append(projectIDs, id)
	}
	sort.Slice(projectIDs, func(x, y int) bool { return projectIDs[x] < projectIDs[y] })
	for n, id := range projectIDs {
		p := st.Projects[id]
		g.Nodes = append(g.Nodes, GraphNode{ID: graphProjectID(id), ProjectID: id, Title: p.Name, Kind: "project", Context: in.Query != "" || in.Kind != "" || in.Agent != nil, Position: model.Point3{X: float64(n) * 900, Y: 380}})
	}
	for _, id := range ids {
		t := st.Tickets[id]
		level := map[string]float64{"goal": 180, "epic": -20, "task": -240}[t.Kind]
		key := GraphTicketID(id)
		g.Nodes = append(g.Nodes, GraphNode{ID: key, ProjectID: t.ProjectID, TicketID: id, Title: t.Title, Kind: t.Kind, Status: t.Status, Progress: t.Progress, Context: context[id], Position: model.Point3{X: float64((id-1)%6)*160 - 400, Y: level, Z: float64((id-1)/6%8)*140 - 140}})
		parent := graphProjectID(t.ProjectID)
		if t.ParentID != 0 && selected[t.ParentID] {
			parent = GraphTicketID(t.ParentID)
		}
		g.Edges = append(g.Edges, GraphEdge{Source: parent, Target: key, Kind: "hierarchy"})
		for _, dep := range effectiveDependencies(st, t) {
			if selected[dep] {
				g.Edges = append(g.Edges, GraphEdge{Source: GraphTicketID(dep), Target: key, Kind: "dependency"})
			}
		}
	}
	for n, node := range g.Nodes {
		if point, ok := layout.Positions[node.ID]; ok {
			g.Nodes[n].Position = point
			g.Layout.Positions[node.ID] = point
		}
	}
	return g, nil
}
func personalPreferences(st *model.State, a model.Account) model.Preferences {
	p, ok := st.Preferences[a.ID]
	if !ok {
		p = model.Preferences{Style: "paper", Mode: "light", DefaultView: "Home", Recent: []model.RecentContext{}}
	}
	if p.DefaultView == "Settings" {
		p.DefaultView = "Home"
	}
	p.DefaultView = canonicalView(p.DefaultView)
	if p.DefaultView == "Agents" && !a.Administrator {
		p.DefaultView = "Home"
	}
	p.NavigationOrder = normalizeNavigationOrder(p.NavigationOrder)
	if !contains([]string{"top", "left", "right"}, p.NavigationPosition) {
		p.NavigationPosition = "top"
	}
	if p.DefaultProject != 0 && role(st, a, p.DefaultProject) == "" {
		p.DefaultProject = 0
	}
	recent := []model.RecentContext{}
	for _, c := range p.Recent {
		c.View = canonicalView(c.View)
		if validRecent(st, a, c) == nil {
			recent = append(recent, c)
		}
	}
	p.Recent = recent
	return p
}
func validRecent(st *model.State, a model.Account, c model.RecentContext) error {
	if !contains([]string{"Home", "Backlog", "Board", "Todos", "Calendar", "Graph", "Workflows", "Agents", "Cost"}, canonicalView(c.View)) || canonicalView(c.View) == "Agents" && !a.Administrator {
		return failure(422, "invalid_view", "Choose a shipped workspace view")
	}
	if c.ProjectID != 0 {
		if err := require(st, a, c.ProjectID, "Viewer"); err != nil {
			return err
		}
	}
	if c.TicketID != 0 {
		t, err := ticketFor(st, a, c.TicketID, "Viewer")
		if err != nil {
			return err
		}
		if c.ProjectID != 0 && t.ProjectID != c.ProjectID {
			return failure(422, "invalid_context", "Ticket and project context differ")
		}
	}
	return nil
}
func presentationOperations(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	if st.GraphLayouts == nil {
		st.GraphLayouts = map[string]model.GraphLayout{}
	}
	if st.Preferences == nil {
		st.Preferences = map[string]model.Preferences{}
	}
	switch operation {
	case "getBacklogOrder", "saveBacklogOrder":
		return backlogOperations(st, a, operation, raw)
	case "getGraph":
		var in GraphInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		g, err := readGraph(st, a, in)
		return g, false, err
	case "saveGraphLayout":
		var in GraphLayoutInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		if !contains([]string{"", "2d", "3d"}, in.Projection) {
			return nil, false, failure(422, "invalid_graph_projection", "Choose 2d or 3d")
		}
		if in.ProjectID != 0 {
			if err := require(st, a, in.ProjectID, "Contributor"); err != nil {
				return nil, false, err
			}
		}
		key := graphScope(a, in.ProjectID, in.Projection)
		layout := st.GraphLayouts[key]
		if layout.Version != in.ExpectedVersion {
			return nil, false, failure(409, "layout_conflict", "Graph layout changed; reload before saving")
		}
		if len(in.Positions) > 2000 {
			return nil, false, failure(422, "layout_too_large", "Save at most 2,000 nodes per patch")
		}
		if layout.Positions == nil {
			layout.Positions = map[string]model.Point3{}
		}
		changed := false
		for id, point := range in.Positions {
			if in.Projection == "2d" && point.Z != 0 {
				return nil, false, failure(422, "invalid_coordinate", "2D coordinates require zero depth")
			}
			project, err := graphNodeProject(st, id)
			if err != nil {
				return nil, false, err
			}
			if err = require(st, a, project, "Viewer"); err != nil {
				return nil, false, err
			}
			if in.ProjectID != 0 && project != in.ProjectID {
				return nil, false, failure(403, "wrong_scope", "Node belongs to a different project")
			}
			for _, v := range []float64{point.X, point.Y, point.Z} {
				if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 100000 {
					return nil, false, failure(422, "invalid_coordinate", "Use finite coordinates within ±100,000")
				}
			}
			if previous, ok := layout.Positions[id]; !ok || previous != point {
				layout.Positions[id] = point
				changed = true
			}
		}
		if changed {
			layout.Version++
			st.GraphLayouts[key] = layout
		}
		visible := model.GraphLayout{Version: layout.Version, Positions: map[string]model.Point3{}}
		for id, point := range layout.Positions {
			project, err := graphNodeProject(st, id)
			if err == nil && require(st, a, project, "Viewer") == nil && (in.ProjectID == 0 || project == in.ProjectID) {
				visible.Positions[id] = point
			}
		}
		return visible, changed, nil
	case "getAppearanceStyles":
		if err := parse(raw, &struct{}{}); err != nil {
			return nil, false, err
		}
		return AppearanceStyles{Styles: []string{"paper", "instrument", "studio"}, Modes: []string{"light", "dark"}}, false, nil
	case "getViewPreferences":
		if err := parse(raw, &struct{}{}); err != nil {
			return nil, false, err
		}
		return personalPreferences(st, a), false, nil
	case "setAppearance", "saveViewPreferences":
		p := personalPreferences(st, a)
		old := p
		if operation == "setAppearance" {
			var in AppearanceInput
			if err := parse(raw, &in); err != nil {
				return nil, false, err
			}
			if in.ExpectedVersion != p.Version {
				return nil, false, failure(409, "preference_conflict", "Account preferences changed; refresh before saving")
			}
			if !contains([]string{"paper", "instrument", "studio"}, in.Style) || !contains([]string{"light", "dark"}, in.Mode) {
				return nil, false, failure(422, "invalid_appearance", "Choose a bundled style and mode")
			}
			p.Style, p.Mode = in.Style, in.Mode
		} else {
			var in ViewPreferencesInput
			if err := parse(raw, &in); err != nil {
				return nil, false, err
			}
			if in.ExpectedVersion != p.Version {
				return nil, false, failure(409, "preference_conflict", "Account preferences changed; refresh before saving")
			}
			if len(in.Recent) > 20 {
				return nil, false, failure(422, "too_many_contexts", "Retain at most 20 recent contexts")
			}
			in.DefaultView = canonicalView(in.DefaultView)
			for index := range in.Recent {
				in.Recent[index].View = canonicalView(in.Recent[index].View)
			}
			if err := validRecent(st, a, model.RecentContext{View: in.DefaultView, ProjectID: in.DefaultProject}); err != nil {
				return nil, false, err
			}
			for _, c := range in.Recent {
				if err := validRecent(st, a, c); err != nil {
					return nil, false, err
				}
			}
			p.DefaultView, p.DefaultProject, p.Recent = in.DefaultView, in.DefaultProject, append([]model.RecentContext{}, in.Recent...)
			if in.NavigationOrder != nil {
				if !validNavigationOrder(*in.NavigationOrder) {
					return nil, false, failure(422, "invalid_navigation", "Provide each canonical view exactly once")
				}
				p.NavigationOrder = normalizeNavigationOrder(*in.NavigationOrder)
			}
			if in.NavigationPosition != nil {
				if !contains([]string{"top", "left", "right"}, *in.NavigationPosition) {
					return nil, false, failure(422, "invalid_navigation", "Choose top, left or right navigation")
				}
				p.NavigationPosition = *in.NavigationPosition
			}
			if in.NavigationCollapsed != nil {
				p.NavigationCollapsed = *in.NavigationCollapsed
			}
		}
		if reflect.DeepEqual(old, p) {
			return p, false, nil
		}
		p.Version++
		st.Preferences[a.ID] = p
		return p, true, nil
	}
	return capabilityOperations(st, a, operation, raw)
}
