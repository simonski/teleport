package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"teleport/internal/model"
)

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor"`
}
type ListInput struct {
	ProjectID     int64  `json:"projectId,omitempty"`
	Cursor        string `json:"cursor,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Agent         *bool  `json:"agent,omitempty"`
	Status        string `json:"status,omitempty"`
	Query         string `json:"query,omitempty"`
	IncludeHidden bool   `json:"includeHidden,omitempty"`
}
type IDInput struct {
	ID     int64  `json:"id"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type CreateProjectInput struct {
	Name      string `json:"name"`
	Key       string `json:"key"`
	RequestID string `json:"requestId"`
}
type CreateTicketInput struct {
	PhaseID     string `json:"phaseId,omitempty"`
	ProjectID   int64  `json:"projectId"`
	ParentID    int64  `json:"parentId,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Criteria    string `json:"criteria,omitempty"`
	Agent       *bool  `json:"agent,omitempty"`
	Priority    string `json:"priority,omitempty"`
	RequestID   string `json:"requestId"`
}
type UpdateTicketInput struct {
	ID              int64     `json:"id"`
	ExpectedVersion int64     `json:"expectedVersion"`
	Title           *string   `json:"title,omitempty"`
	Description     *string   `json:"description,omitempty"`
	Criteria        *string   `json:"criteria,omitempty"`
	Priority        *string   `json:"priority,omitempty"`
	Labels          *[]string `json:"labels,omitempty"`
	Start           *string   `json:"start,omitempty"`
	ETA             *string   `json:"eta,omitempty"`
	Due             *string   `json:"due,omitempty"`
	Progress        *int      `json:"progress,omitempty"`
}
type TransitionInput struct {
	Outcome         string            `json:"outcome,omitempty"`
	Context         map[string]string `json:"context,omitempty"`
	ID              int64             `json:"id"`
	ExpectedVersion int64             `json:"expectedVersion"`
	Target          string            `json:"target"`
	Confirm         bool              `json:"confirm,omitempty"`
}

func collections(st *model.State) {
	if st.Projects == nil {
		st.Projects = map[int64]model.Project{}
	}
	if st.Tickets == nil {
		st.Tickets = map[int64]model.Ticket{}
	}
	if st.History == nil {
		st.History = map[int64][]model.Revision{}
	}
	if st.Receipts == nil {
		st.Receipts = map[string]model.Receipt{}
	}
}
func role(st *model.State, a model.Account, projectID int64) string {
	if a.ServiceSessionID != "" {
		if a.ServiceProjectID == projectID {
			return "Viewer"
		}
		return ""
	}
	p, ok := st.Projects[projectID]
	if !ok {
		return ""
	}
	if a.Administrator {
		return "Maintainer"
	}
	return p.Members[a.ID]
}
func require(st *model.State, a model.Account, projectID int64, minimum string) error {
	r := role(st, a, projectID)
	rank := map[string]int{"Viewer": 1, "Contributor": 2, "Maintainer": 3}
	if rank[r] < rank[minimum] {
		return failure(403, "forbidden", "Current project role does not permit this operation")
	}
	return nil
}
func ticketFor(st *model.State, a model.Account, id int64, minimum string) (model.Ticket, error) {
	t, ok := st.Tickets[id]
	if !ok {
		return t, failure(404, "not_found", "Ticket not found")
	}
	if err := require(st, a, t.ProjectID, minimum); err != nil {
		return model.Ticket{}, err
	}
	return t, nil
}
func expected(t model.Ticket, version int64) error {
	if version < 1 || version != t.Version {
		return failure(409, "version_conflict", "Ticket changed; refresh before retrying")
	}
	return nil
}
func saveTicket(st *model.State, t *model.Ticket, a model.Account, action string) {
	t.Version++
	t.UpdatedAt = time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = t.UpdatedAt
	}
	if t.PhaseEnteredAt.IsZero() {
		t.PhaseEnteredAt = t.CreatedAt
	}
	st.Tickets[t.ID] = *t
	st.History[t.ID] = append(st.History[t.ID], model.Revision{Version: t.Version, Actor: a.ID, Action: action, At: t.UpdatedAt, Snapshot: *t, TransactionID: st.Revision + 1, ScriptRunID: a.ServiceRunID, ScriptCallID: a.ServiceCallID})
}
func allocate(st *model.State) int64 { st.NextTicketID++; return st.NextTicketID }
func newTicket(st *model.State, p model.Project, a model.Account, kind, title string, agent, protected bool) model.Ticket {
	workflow := p.HumanWorkflow
	if agent {
		workflow = p.AgentWorkflow
	}
	t := model.Ticket{ID: allocate(st), ProjectID: p.ID, Kind: kind, Title: title, Agent: agent, Protected: protected, Workflow: workflow, Status: "Todo", Phase: "Todo", Creator: a.ID, Priority: "medium", Labels: []string{}, Dependencies: []int64{}, Agreements: []model.Agreement{}}
	if agent {
		t.AttemptLimit = 20
	}
	t.StageVisit = 1
	if initial, ok := phaseForRole(st.Workflows[workflow], "Todo"); ok {
		t.Phase = initial.ID
	}
	if !agent && kind == "task" {
		t.Assignee = a.ID
	}
	return t
}
func validContent(t model.Ticket) error {
	if strings.TrimSpace(t.Title) == "" || len(t.Title) > 20000 || len(t.Description) > 20000 || len(t.Criteria) > 20000 {
		return failure(422, "invalid_content", "Title is required; content fields allow at most 20,000 bytes")
	}
	if !contains([]string{"critical", "high", "medium", "low"}, t.Priority) {
		return failure(422, "invalid_priority", "Choose critical, high, medium or low priority")
	}
	for _, date := range []string{t.Start, t.ETA, t.Due} {
		if date != "" {
			if parsed, err := time.Parse("2006-01-02", date); err != nil || parsed.Year() < 1 {
				return failure(422, "invalid_date", "Use YYYY-MM-DD dates")
			}
		}
	}
	if t.Start != "" && t.ETA != "" && t.ETA < t.Start {
		return failure(422, "invalid_date", "ETA cannot precede start")
	}
	if len(t.Labels) > 100 {
		return failure(422, "invalid_labels", "At most 100 labels are supported")
	}
	return nil
}
func contains(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}
func pageBounds(cursor string, limit int) (int64, int, error) {
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return 0, 0, failure(422, "invalid_limit", "Limit must be between 1 and 200")
	}
	var after int64
	if cursor != "" {
		b, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return 0, 0, failure(400, "invalid_cursor", "Invalid cursor")
		}
		after, err = strconv.ParseInt(string(b), 10, 64)
		if err != nil || after < 0 {
			return 0, 0, failure(400, "invalid_cursor", "Invalid cursor")
		}
	}
	return after, limit, nil
}
func cursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}
func receipt(st *model.State, a model.Account, operation, key string, input any) (string, string, []byte, error) {
	if strings.TrimSpace(key) == "" || len(key) > 100 {
		return "", "", nil, failure(400, "request_id_required", "Provide a stable requestId (1–100 bytes) for safe creation retries")
	}
	encoded, _ := json.Marshal(input)
	fingerprint := digest(string(encoded))
	identity := a.ID + ":" + operation + ":" + key
	if saved, ok := st.Receipts[identity]; ok {
		if saved.Fingerprint != fingerprint {
			return "", "", nil, failure(409, "idempotency_conflict", "requestId was already used with different input")
		}
		return identity, fingerprint, saved.Result, nil
	}
	return identity, fingerprint, nil, nil
}
func remember(st *model.State, key, fingerprint string, result any) {
	b, _ := json.Marshal(result)
	st.Receipts[key] = model.Receipt{Fingerprint: fingerprint, Result: b}
}
func reconcile(st *model.State, parent int64, a model.Account) {
	for parent != 0 {
		p, ok := st.Tickets[parent]
		if !ok {
			return
		}
		if p.Status == "Cancelled" {
			parent = p.ParentID
			continue
		}
		total, done := 0, 0
		for _, child := range st.Tickets {
			if child.ParentID == p.ID && !child.Deleted {
				total++
				if child.Status == "Done" {
					done++
				}
			}
		}
		progress := 0
		if total > 0 {
			progress = (100*done + total/2) / total
		}
		status := "Todo"
		if progress > 0 {
			status = "Doing"
		}
		if total > 0 && total == done && !p.Planning {
			status = "Done"
		}
		phaseID := p.Phase
		if p.Planning {
			status = p.Status
		} else if phase, ok := phaseForRole(st.Workflows[p.Workflow], status); ok {
			phaseID = phase.ID
		}
		if p.Progress != progress || p.Status != status {
			if status == "Done" && p.Status != "Done" {
				agreement := completionBasis(st, p, model.Account{ID: "server:derived"}, p.Criteria, p.Version)
				agreement.Kind = "DerivedDone"
				agreement.ChildVersions = map[int64]int64{}
				for _, child := range st.Tickets {
					if child.ParentID == p.ID && !child.Deleted {
						agreement.ChildVersions[child.ID] = child.Version
					}
				}
				p.Agreements = append(p.Agreements, agreement)
			}
			p.Progress, p.Status, p.Phase = progress, status, phaseID
			saveTicket(st, &p, a, "reconcileHierarchy")
		}
		parent = p.ParentID
	}
}

func projectOperations(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	switch operation {
	case "createProject":
		var in CreateProjectInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Key == "" {
			in.Key = strings.ToLower(strings.ReplaceAll(in.Name, " ", "-"))
		}
		if len(in.Name) == 0 || len(in.Name) > 200 || !regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,79}$`).MatchString(in.Key) {
			return nil, false, failure(422, "invalid_project", "Provide a name and a lowercase project key")
		}
		key, fingerprint, previous, err := receipt(st, a, operation, in.RequestID, in)
		if err != nil {
			return nil, false, err
		}
		if previous != nil {
			var p model.Project
			json.Unmarshal(previous, &p)
			if err = require(st, a, p.ID, "Viewer"); err != nil {
				return nil, false, err
			}
			return p, false, nil
		}
		for _, p := range st.Projects {
			if sameProjectName(p.Name, in.Name) {
				return nil, false, failure(409, "project_name_exists", "Project name already exists")
			}
			if p.Key == in.Key {
				return nil, false, failure(409, "project_exists", "Project key already exists")
			}
		}
		st.NextProjectID++
		p := model.Project{ID: st.NextProjectID, Version: 1, Key: in.Key, Name: in.Name, Creator: a.ID, Members: map[string]string{a.ID: "Maintainer"}, HumanWorkflow: "human@1", AgentWorkflow: "agent@1"}
		goal := newTicket(st, p, a, "goal", "Unassigned", false, true)
		epic := newTicket(st, p, a, "epic", "Unassigned", false, true)
		epic.ParentID = goal.ID
		p.UnassignedGoal, p.UnassignedEpic = goal.ID, epic.ID
		st.Projects[p.ID] = p
		saveTicket(st, &goal, a, operation)
		saveTicket(st, &epic, a, operation)
		remember(st, key, fingerprint, p)
		return p, true, nil
	case "listProjects":
		var in ListInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		after, limit, err := pageBounds(in.Cursor, in.Limit)
		if err != nil {
			return nil, false, err
		}
		ids := []int64{}
		for id, p := range st.Projects {
			if id > after && role(st, a, id) != "" && strings.Contains(strings.ToLower(p.Name), strings.ToLower(in.Query)) {
				ids = append(ids, id)
			}
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		result := Page[model.Project]{Items: []model.Project{}}
		for i, id := range ids {
			if i == limit {
				result.NextCursor = cursor(ids[i-1])
				break
			}
			result.Items = append(result.Items, st.Projects[id])
		}
		return result, false, nil
	case "createTicket":
		var in CreateTicketInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		if err := require(st, a, in.ProjectID, "Contributor"); err != nil {
			return nil, false, err
		}
		p := st.Projects[in.ProjectID]
		if in.Kind == "" {
			in.Kind = "task"
		}
		if !contains([]string{"goal", "epic", "task"}, in.Kind) {
			return nil, false, failure(422, "invalid_kind", "Choose goal, epic or task")
		}
		agent := true
		if in.Agent != nil {
			agent = *in.Agent
		}
		t := newTicket(st, p, a, in.Kind, strings.TrimSpace(in.Title), agent, false)
		initial, valid := phaseForRole(st.Workflows[t.Workflow], "Todo")
		if in.PhaseID != "" {
			initial, valid = phaseByID(st.Workflows[t.Workflow], in.PhaseID)
			valid = valid && initial.Role == "Todo"
		}
		if !valid {
			return nil, false, failure(409, "workflow_mapping", "Select an exact Todo phase for this workflow")
		}
		t.Phase = initial.ID
		t.Description, t.Criteria = in.Description, in.Criteria
		if in.Priority != "" {
			t.Priority = in.Priority
		}
		t.ParentID = in.ParentID
		if t.ParentID == 0 {
			if t.Kind == "task" {
				t.ParentID = p.UnassignedEpic
			}
			if t.Kind == "epic" {
				t.ParentID = p.UnassignedGoal
			}
		}
		if err := validParent(st, t); err != nil {
			return nil, false, err
		}
		if err := validContent(t); err != nil {
			return nil, false, err
		}
		key, fingerprint, previous, err := receipt(st, a, operation, in.RequestID, in)
		if err != nil {
			return nil, false, err
		}
		if previous != nil {
			var saved model.Ticket
			json.Unmarshal(previous, &saved)
			st.NextTicketID--
			if _, err = ticketFor(st, a, saved.ID, "Viewer"); err != nil {
				return nil, false, err
			}
			return saved, false, nil
		}
		saveTicket(st, &t, a, operation)
		reconcile(st, t.ParentID, a)
		remember(st, key, fingerprint, t)
		return t, true, nil
	case "getTicket":
		var in IDInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		t, err := ticketFor(st, a, in.ID, "Viewer")
		return t, false, err
	case "listTickets":
		var in ListInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		if in.ProjectID != 0 {
			if err := require(st, a, in.ProjectID, "Viewer"); err != nil {
				return nil, false, err
			}
		}
		after, limit, err := pageBounds(in.Cursor, in.Limit)
		if err != nil {
			return nil, false, err
		}
		ids := []int64{}
		for id, t := range st.Tickets {
			if id <= after || role(st, a, t.ProjectID) == "" || in.ProjectID != 0 && t.ProjectID != in.ProjectID || !in.IncludeHidden && (t.Archived || t.Deleted) || in.Kind != "" && t.Kind != in.Kind || in.Agent != nil && t.Agent != *in.Agent || in.Status != "" && t.Status != in.Status {
				continue
			}
			if !textMatches(t.Title+" "+t.Description+" "+strings.Join(t.Labels, " ")+" "+t.Kind+" "+t.Priority+" "+strconv.FormatInt(t.ID, 10), in.Query) {
				continue
			}
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		result := Page[model.Ticket]{Items: []model.Ticket{}}
		for i, id := range ids {
			if i == limit {
				result.NextCursor = cursor(ids[i-1])
				break
			}
			result.Items = append(result.Items, st.Tickets[id])
		}
		return result, false, nil
	case "listTicketHistory":
		var in IDInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		if _, err := ticketFor(st, a, in.ID, "Viewer"); err != nil {
			return nil, false, err
		}
		after, limit, err := pageBounds(in.Cursor, in.Limit)
		if err != nil {
			return nil, false, err
		}
		result := Page[model.Revision]{Items: []model.Revision{}}
		for _, v := range st.History[in.ID] {
			if v.Version <= after {
				continue
			}
			if len(result.Items) == limit {
				result.NextCursor = cursor(result.Items[len(result.Items)-1].Version)
				break
			}
			result.Items = append(result.Items, v)
		}
		return result, false, nil
	case "updateTicket":
		var in UpdateTicketInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		t, err := ticketFor(st, a, in.ID, "Contributor")
		if err != nil {
			return nil, false, err
		}
		if err = expected(t, in.ExpectedVersion); err != nil {
			return nil, false, err
		}
		if busy(t) {
			return nil, false, failure(409, "execution_busy", "Wait for the reserved operation before editing this ticket")
		}
		old := t
		for _, field := range []struct {
			input   *string
			target  *string
			content bool
		}{{in.Title, &t.Title, true}, {in.Description, &t.Description, true}, {in.Criteria, &t.Criteria, true}, {in.Priority, &t.Priority, false}, {in.Start, &t.Start, false}, {in.ETA, &t.ETA, false}, {in.Due, &t.Due, false}} {
			if field.input != nil && *field.target != *field.input {
				if t.Protected && field.target == &t.Title {
					return nil, false, failure(403, "protected_record", "Unassigned defaults are protected")
				}
				if field.content && old.Status != "Todo" {
					return nil, false, failure(409, "content_locked", "Return to Todo before editing original content")
				}
				*field.target = *field.input
			}
		}
		if in.Labels != nil {
			labels := []string{}
			for _, l := range *in.Labels {
				l = strings.TrimSpace(l)
				if l != "" && !contains(labels, l) {
					labels = append(labels, l)
				}
			}
			t.Labels = labels
		}
		if in.Progress != nil {
			if t.Kind != "task" {
				return nil, false, failure(409, "derived_progress", "Container progress is derived")
			}
			if *in.Progress < 0 || *in.Progress > 100 {
				return nil, false, failure(422, "invalid_progress", "Progress must be 0–100")
			}
			if !t.Agent {
				return nil, false, failure(409, "use_transition", "Use an explicit human transition with its agreement")
			}
			t.Progress = *in.Progress
		}
		if err = validContent(t); err != nil {
			return nil, false, err
		}
		if reflect.DeepEqual(old, t) {
			return t, false, nil
		}
		saveTicket(st, &t, a, operation)
		return t, true, nil
	case "transitionTicket":
		var in TransitionInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		t, err := ticketFor(st, a, in.ID, "Contributor")
		if err != nil {
			return nil, false, err
		}
		if err = expected(t, in.ExpectedVersion); err != nil {
			return nil, false, err
		}
		if t.Kind != "task" {
			return nil, false, failure(409, "derived_status", "Container completion is derived from children")
		}
		if t.Agent {
			return nil, false, failure(409, "agent_gate", "Agent phases require scoped outcome and human acceptance actions")
		}
		workflow := st.Workflows[t.Workflow]
		target, found := phaseByID(workflow, in.Target)
		if !found {
			target, found = phaseForRole(workflow, in.Target)
		}
		if !found {
			return nil, false, failure(409, "workflow_mapping", "Choose an exact valid destination phase")
		}
		if target.ID == t.Phase {
			return t, false, nil
		}
		outcome := in.Outcome
		if outcome == "" {
			matches := 0
			for _, r := range workflow.Rules {
				if r.Source == t.Phase && r.Target == target.ID && len(r.Conditions) == 0 {
					matches++
					outcome = r.Outcome
				}
			}
			if matches != 1 {
				return nil, false, failure(409, "explicit_outcome_required", "Select the exact declared outcome for this destination")
			}
		}
		route, routeErr := ResolveOutcome(workflow, t.Phase, outcome, in.Context)
		if routeErr != nil {
			return nil, false, routeErr
		}
		if route.Target != target.ID {
			return nil, false, failure(409, "invalid_transition", "No declared workflow route to this phase")
		}
		if (target.Role == "Done" || target.Role == "Doing") && !dependenciesDone(st, t) {
			return nil, false, failure(409, "unmet_prerequisite", "Every prerequisite must be Done")
		}
		if target.Role == "Done" {
			if !in.Confirm {
				return nil, false, failure(409, "agreement_required", "Explicitly confirm the current completion criteria")
			}
			t.Agreements = append(t.Agreements, completionBasis(st, t, a, t.Criteria, t.Version))
			t.Progress = 100
		} else if target.Role == "Doing" {
			if t.Progress == 0 || t.Progress == 100 {
				t.Progress = 1
			}
		} else {
			t.Progress = 0
		}
		t.Status, t.Phase = target.Role, target.ID
		saveTicket(st, &t, a, operation)
		reconcile(st, t.ParentID, a)
		reconcileDependencyStops(st, a)
		return t, true, nil
	}
	return controlOperations(st, a, operation, raw)
}
func validParent(st *model.State, t model.Ticket) error {
	if t.Kind == "goal" {
		if t.ParentID != 0 {
			return failure(422, "invalid_parent", "Goals belong to root")
		}
		return nil
	}
	parent, ok := st.Tickets[t.ParentID]
	expectedKind := "goal"
	if t.Kind == "task" {
		expectedKind = "epic"
	}
	if !ok || parent.ProjectID != t.ProjectID || parent.Kind != expectedKind || parent.Archived || parent.Deleted {
		return failure(422, "invalid_parent", fmt.Sprintf("Choose a visible %s in the same project", expectedKind))
	}
	return nil
}
