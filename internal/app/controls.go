package app

import (
	"encoding/json"
	"reflect"
	"sort"

	"teleport/internal/model"
)

type VersionInput struct {
	ID              int64  `json:"id"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason,omitempty"`
}
type RefinementRequestInput struct {
	ID              int64  `json:"id"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason,omitempty"`
	UsageID         string `json:"usageId,omitempty"`
	UsageVersion    int64  `json:"usageVersion,omitempty"`
}
type VisibilityInput struct {
	ID              int64 `json:"id"`
	ExpectedVersion int64 `json:"expectedVersion"`
	Archived        *bool `json:"archived,omitempty"`
	Deleted         *bool `json:"deleted,omitempty"`
}
type DependenciesInput struct {
	ID              int64   `json:"id"`
	ExpectedVersion int64   `json:"expectedVersion"`
	Dependencies    []int64 `json:"dependencies"`
}
type TransferInput struct {
	ID                   int64  `json:"id"`
	ExpectedVersion      int64  `json:"expectedVersion"`
	DestinationProjectID int64  `json:"destinationProjectId"`
	ParentID             int64  `json:"parentId,omitempty"`
	ExpectedRevision     *int64 `json:"expectedRevision,omitempty"`
}
type TransferPreviewInput struct {
	ID                   int64 `json:"id"`
	DestinationProjectID int64 `json:"destinationProjectId"`
}
type TransferPreview struct {
	RootID         int64  `json:"rootId"`
	RootTitle      string `json:"rootTitle"`
	RootVersion    int64  `json:"rootVersion"`
	Revision       int64  `json:"revision"`
	Total          int    `json:"total"`
	Stories        int    `json:"stories"`
	Hidden         int    `json:"hidden"`
	IncludesParent bool   `json:"includesParent"`
}
type MoveInput struct {
	ID              int64 `json:"id"`
	ExpectedVersion int64 `json:"expectedVersion"`
	ParentID        int64 `json:"parentId"`
}
type PasswordInput struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func busy(t model.Ticket) bool {
	return t.Active || t.Claim != "" || t.Execution != "" || t.Delivery != nil && t.Delivery.Operation != nil && t.Delivery.Operation.State != "complete"
}
func subtree(st *model.State, id int64) []int64 {
	ids := []int64{id}
	for i := 0; i < len(ids); i++ {
		for _, t := range st.Tickets {
			if t.ParentID == ids[i] {
				ids = append(ids, t.ID)
			}
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
func effectiveDependencies(st *model.State, t model.Ticket) []int64 {
	out := append([]int64{}, t.Dependencies...)
	if t.Kind == "task" {
		out = append(out, st.Tickets[t.ParentID].Dependencies...)
	}
	return out
}
func dependenciesDone(st *model.State, t model.Ticket) bool {
	for _, id := range effectiveDependencies(st, t) {
		if st.Tickets[id].Status != "Done" {
			return false
		}
	}
	return true
}
func validateDependencies(st *model.State) error {
	graph := map[int64][]int64{}
	for id, t := range st.Tickets {
		for _, dep := range effectiveDependencies(st, t) {
			other, ok := st.Tickets[dep]
			if !ok || other.ProjectID != t.ProjectID || other.Kind == "goal" {
				return failure(422, "invalid_dependency", "Prerequisites must remain same-project tasks or epics")
			}
			graph[id] = append(graph[id], dep)
		}
		if t.Kind != "task" {
			for _, child := range st.Tickets {
				if child.ParentID == id && !child.Deleted {
					graph[id] = append(graph[id], child.ID)
				}
			}
		}
	}
	visiting, visited := map[int64]bool{}, map[int64]bool{}
	var visit func(int64) bool
	visit = func(id int64) bool {
		if visiting[id] {
			return false
		}
		if visited[id] {
			return true
		}
		visiting[id] = true
		for _, next := range graph[id] {
			if !visit(next) {
				return false
			}
		}
		delete(visiting, id)
		visited[id] = true
		return true
	}
	for id := range st.Tickets {
		if !visit(id) {
			return failure(409, "dependency_cycle", "Prerequisites and hierarchy must not form a cycle")
		}
	}
	return nil
}
func reconcileDependencyStops(st *model.State, a model.Account) {
	for _, t := range st.Tickets {
		if t.Status == "Done" || t.Status == "Cancelled" || dependenciesDone(st, t) {
			continue
		}
		if busy(t) && !t.StopRequested {
			t.StopRequested = true
			saveTicket(st, &t, a, "dependencyStopRequested")
		}
	}
}

func controlOperations(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	switch operation {
	case "setDependencies":
		var in DependenciesInput
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
		if t.Status != "Todo" || busy(t) || t.Kind == "goal" {
			return nil, false, failure(409, "dependencies_locked", "Edit prerequisites only on unclaimed Todo tasks or epics")
		}
		seen := map[int64]bool{}
		for _, id := range in.Dependencies {
			dep, ok := st.Tickets[id]
			if seen[id] || id == t.ID || !ok || dep.ProjectID != t.ProjectID || dep.Kind == "goal" || dep.Status == "Cancelled" && !containsID(t.Dependencies, id) {
				return nil, false, failure(422, "invalid_dependency", "Choose unique same-project non-cancelled tasks or epics")
			}
			seen[id] = true
		}
		if len(in.Dependencies) > 200 {
			return nil, false, failure(422, "invalid_dependency", "At most 200 direct prerequisites")
		}
		if in.Dependencies == nil {
			in.Dependencies = []int64{}
		}
		if reflect.DeepEqual(in.Dependencies, t.Dependencies) {
			return t, false, nil
		}
		t.Dependencies = in.Dependencies
		st.Tickets[t.ID] = t
		if err = validateDependencies(st); err != nil {
			return nil, false, err
		}
		saveTicket(st, &t, a, operation)
		reconcileDependencyStops(st, a)
		return t, true, nil
	case "cancelTicket":
		var in VersionInput
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
		if t.Protected {
			return nil, false, failure(403, "protected_record", "Unassigned defaults cannot be cancelled")
		}
		if t.Status == "Cancelled" {
			return t, false, nil
		}
		if t.Status == "Done" {
			return nil, false, failure(409, "already_done", "Completed work must be explicitly reopened")
		}
		if len(in.Reason) > 20000 {
			return nil, false, failure(422, "invalid_reason", "Reason allows at most 20,000 bytes")
		}
		batch := randomID()
		for _, id := range subtree(st, t.ID) {
			item := st.Tickets[id]
			if item.Status == "Done" || item.Status == "Cancelled" {
				continue
			}
			item.Status, item.Phase = "Cancelled", "Cancelled"
			item.CancellationBatch, item.CancellationReason = batch, in.Reason
			if busy(item) {
				item.StopRequested = true
			} else {
				item.Active = false
			}
			saveTicket(st, &item, a, operation)
		}
		reconcile(st, t.ParentID, a)
		reconcileDependencyStops(st, a)
		return st.Tickets[t.ID], true, nil
	case "reopenTicket":
		var in VersionInput
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
		if t.Agent || t.Kind != "task" {
			if err = require(st, a, t.ProjectID, "Maintainer"); err != nil {
				return nil, false, err
			}
		}
		if t.Protected || !contains([]string{"Done", "Cancelled"}, t.Status) {
			return nil, false, failure(409, "invalid_reopen", "Select ordinary terminal work to reopen")
		}
		if busy(t) || hasContinuation(t) || proposalOwnsTicket(st, t.ID) {
			return nil, false, failure(409, "execution_busy", "Confirm prior execution exit before reopening")
		}
		if t.Agent {
			phase, ok := phaseForRole(st.Workflows[t.Workflow], "Todo")
			if !ok {
				return nil, false, failure(409, "workflow_mapping", "Pinned workflow needs one exact Todo destination")
			}
			t.Phase = phase.ID
		} else {
			t.Phase = "Todo"
		}
		t.Status, t.Progress, t.Active = "Todo", 0, false
		t.ResumeHold = t.Agent
		t.Planning = false
		t.ResumeAttempt = ""
		t.Refinement = nil
		t.AgentReview = nil
		if t.Question != nil {
			t.Question.Obsolete = true
		}
		saveTicket(st, &t, a, operation)
		if t.Kind != "task" {
			reconcile(st, t.ID, a)
		}
		reconcile(st, t.ParentID, a)
		reconcileDependencyStops(st, a)
		return st.Tickets[t.ID], true, nil
	case "setTicketVisibility":
		var in VisibilityInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		t, err := ticketFor(st, a, in.ID, "Maintainer")
		if err != nil {
			return nil, false, err
		}
		if err = expected(t, in.ExpectedVersion); err != nil {
			return nil, false, err
		}
		if t.Protected {
			return nil, false, failure(403, "protected_record", "Unassigned defaults cannot be hidden")
		}
		if busy(t) || hasContinuation(t) || proposalOwnsTicket(st, t.ID) {
			return nil, false, failure(409, "execution_busy", "Stop and confirm own execution exit before changing visibility")
		}
		old := t
		if in.Archived != nil {
			t.Archived = *in.Archived
		}
		if in.Deleted != nil {
			t.Deleted = *in.Deleted
		}
		if old.Archived && !t.Archived || old.Deleted && !t.Deleted {
			t.ResumeHold = t.Agent
		}
		if reflect.DeepEqual(old, t) {
			return t, false, nil
		}
		saveTicket(st, &t, a, operation)
		reconcile(st, t.ParentID, a)
		if err = validateDependencies(st); err != nil {
			return nil, false, err
		}
		reconcileDependencyStops(st, a)
		return t, true, nil
	case "previewTicketTransfer":
		var in TransferPreviewInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		t, err := ticketFor(st, a, in.ID, "Maintainer")
		if err != nil {
			return nil, false, err
		}
		if err = require(st, a, in.DestinationProjectID, "Maintainer"); err != nil {
			return nil, false, err
		}
		if t.ProjectID == in.DestinationProjectID {
			return nil, false, failure(422, "same_project", "Choose another project")
		}
		root := t
		if t.Kind == "task" {
			if parent, ok := st.Tickets[t.ParentID]; ok && !parent.Protected {
				root = parent
			}
		}
		if root.Protected {
			return nil, false, failure(403, "protected_record", "Protected defaults cannot be transferred")
		}
		ids := subtree(st, root.ID)
		out := TransferPreview{RootID: root.ID, RootTitle: root.Title, RootVersion: root.Version, Revision: st.Revision, Total: len(ids), IncludesParent: root.ID != t.ID}
		for _, id := range ids {
			item := st.Tickets[id]
			if item.Kind == "task" {
				out.Stories++
			}
			if item.Archived || item.Deleted {
				out.Hidden++
			}
		}
		return out, false, nil
	case "transferTicket":
		var in TransferInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		t, err := ticketFor(st, a, in.ID, "Maintainer")
		if err != nil {
			return nil, false, err
		}
		if err = require(st, a, in.DestinationProjectID, "Maintainer"); err != nil {
			return nil, false, err
		}
		if in.ExpectedRevision != nil && *in.ExpectedRevision != st.Revision {
			return nil, false, failure(409, "version_conflict", "The workspace changed; review the transfer again")
		}
		if err = expected(t, in.ExpectedVersion); err != nil {
			return nil, false, err
		}
		if t.ProjectID == in.DestinationProjectID {
			return nil, false, failure(422, "same_project", "Use moveTicket within one project")
		}
		p := st.Projects[in.DestinationProjectID]
		if st.Projects[t.ProjectID].Simulation != p.Simulation {
			return nil, false, failure(409, "simulation_boundary", "Simulation work cannot transfer between simulated and real projects")
		}
		ids := subtree(st, t.ID)
		for _, id := range ids {
			item := st.Tickets[id]
			if item.Protected || busy(item) || hasContinuation(item) || item.Delivery != nil || proposalOwnsTicket(st, item.ID) {
				return nil, false, failure(409, "transfer_blocked", "Protected defaults or pending execution block the whole transfer")
			}
			if item.Assignee != "" && role(st, st.Accounts[item.Assignee], p.ID) == "" {
				return nil, false, failure(409, "destination_assignment", "Resolve the human assignee's destination access before transfer")
			}
		}
		parent := in.ParentID
		if parent == 0 {
			if t.Kind == "task" {
				parent = p.UnassignedEpic
			}
			if t.Kind == "epic" {
				parent = p.UnassignedGoal
			}
		}
		oldParent := t.ParentID
		t.ProjectID, t.ParentID = p.ID, parent
		if err = validParent(st, t); err != nil {
			return nil, false, err
		}
		for _, id := range ids {
			item := st.Tickets[id]
			item.ProjectID = p.ID
			if id == t.ID {
				item.ParentID = parent
			}
			saveTicket(st, &item, a, operation)
		}
		if err = validateDependencies(st); err != nil {
			return nil, false, err
		}
		reconcile(st, oldParent, a)
		reconcile(st, parent, a)
		return st.Tickets[t.ID], true, nil
	case "moveTicket":
		var in MoveInput
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
		if t.Protected || busy(t) {
			return nil, false, failure(409, "move_blocked", "Protected defaults or execution block movement")
		}
		if t.ParentID == in.ParentID {
			return t, false, nil
		}
		oldParent := t.ParentID
		t.ParentID = in.ParentID
		if err = validParent(st, t); err != nil {
			return nil, false, err
		}
		st.Tickets[t.ID] = t
		if err = validateDependencies(st); err != nil {
			return nil, false, err
		}
		saveTicket(st, &t, a, operation)
		reconcile(st, oldParent, a)
		reconcile(st, t.ParentID, a)
		reconcileDependencyStops(st, a)
		return t, true, nil
	case "changePassword":
		var in PasswordInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		if !passwordMatches(a.PasswordHash, in.CurrentPassword) {
			return nil, false, failure(403, "verification_failed", "Current password is incorrect")
		}
		if len(in.NewPassword) < 12 || len(in.NewPassword) > 1024 {
			return nil, false, failure(422, "invalid_password", "Use a password between 12 and 1,024 bytes")
		}
		a.PasswordHash = hashPassword(in.NewPassword)
		st.Accounts[a.ID] = a
		for key, session := range st.Sessions {
			if session.AccountID == a.ID {
				delete(st.Sessions, key)
			}
		}
		return map[string]bool{"signedOut": true}, true, nil
	}
	return workflowOperations(st, a, operation, raw)
}
func containsID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
func phaseForRole(w model.Workflow, role string) (model.Phase, bool) {
	var found model.Phase
	count := 0
	for _, p := range w.Phases {
		if p.Role == role {
			found = p
			count++
		}
	}
	return found, count == 1
}
