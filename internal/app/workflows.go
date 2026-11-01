package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"teleport/internal/model"
)

type WorkflowKeyInput struct {
	WorkflowKey string `json:"workflowKey"`
	TicketID    int64  `json:"ticketId,omitempty"`
}
type WorkflowListInput struct {
	ProjectID int64  `json:"projectId,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}
type CopyWorkflowInput struct {
	SourceKey string `json:"sourceKey"`
	ProjectID int64  `json:"projectId"`
	Title     string `json:"title"`
	RequestID string `json:"requestId"`
}
type SaveWorkflowInput struct {
	Definition       model.Workflow `json:"definition"`
	ExpectedRevision int64          `json:"expectedRevision"`
}
type DeleteWorkflowInput struct {
	WorkflowKey      string `json:"workflowKey"`
	ExpectedRevision int64  `json:"expectedRevision"`
}
type ActivateWorkflowInput struct {
	WorkflowKey            string `json:"workflowKey"`
	ExpectedRevision       int64  `json:"expectedRevision"`
	ProjectID              int64  `json:"projectId"`
	ExpectedProjectVersion int64  `json:"expectedProjectVersion"`
}
type OutcomePreviewInput struct {
	WorkflowKey string            `json:"workflowKey"`
	Source      string            `json:"source"`
	Outcome     string            `json:"outcome"`
	Context     map[string]string `json:"context,omitempty"`
}
type RoutePreview struct {
	RuleID        string   `json:"ruleId"`
	Target        string   `json:"target,omitempty"`
	Action        string   `json:"action,omitempty"`
	NeedsAttempt  bool     `json:"needsAttempt"`
	RequiredGates []string `json:"requiredGates"`
}
type MigrationRecord struct {
	ID              int64 `json:"id"`
	ExpectedVersion int64 `json:"expectedVersion"`
}
type MigrationInput struct {
	SourceKey string            `json:"sourceKey"`
	TargetKey string            `json:"targetKey"`
	Tickets   []MigrationRecord `json:"tickets"`
	Mapping   map[string]string `json:"mapping"`
}
type MigrationResult struct {
	Items   []model.Ticket `json:"items"`
	Preview bool           `json:"preview"`
}

func phaseByID(w model.Workflow, id string) (model.Phase, bool) {
	for _, p := range w.Phases {
		if p.ID == id {
			return p, true
		}
	}
	return model.Phase{}, false
}
func workflowKey(w model.Workflow) string {
	if !w.Published {
		return w.ID + "@draft"
	}
	return fmt.Sprintf("%s@%d", w.ID, w.Version)
}
func workflowRead(st *model.State, a model.Account, key string) (model.Workflow, error) {
	w, ok := st.Workflows[key]
	if !ok {
		return w, failure(404, "not_found", "Workflow not found")
	}
	if w.ProjectID != 0 {
		if err := require(st, a, w.ProjectID, "Viewer"); err != nil {
			return model.Workflow{}, err
		}
	} else if !w.Published && !a.Administrator {
		return model.Workflow{}, failure(403, "forbidden", "Installation drafts require administrator rights")
	}
	return w, nil
}
func workflowWrite(st *model.State, a model.Account, w model.Workflow) error {
	if w.ProjectID == 0 {
		if !a.Administrator {
			return failure(403, "forbidden", "Installation workflow edits require administrator rights")
		}
		return nil
	}
	return require(st, a, w.ProjectID, "Maintainer")
}
func overlap(a, b map[string]string) bool {
	for key, value := range a {
		if other, ok := b[key]; ok && other != value {
			return false
		}
	}
	return true
}

func ValidateWorkflow(w model.Workflow, complete bool) error {
	if strings.TrimSpace(w.Title) == "" || len(w.Title) > 200 || !contains([]string{"human", "agent"}, w.Family) || len(w.Phases) < 1 || len(w.Phases) > 100 || len(w.Rules) > 1000 {
		return failure(422, "invalid_workflow", "Provide a title, family, 1–100 phases and at most 1,000 rules")
	}
	roles := []string{"Todo", "Refine", "Decompose", "Ready", "Doing", "Rebase", "Review", "Clarify", "Approval", "Done", "Cancelled"}
	phases := map[string]model.Phase{}
	counts := map[string]int{}
	for _, p := range w.Phases {
		validID := contains(roles, p.ID) || regexp.MustCompile(`^lane:[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}$`).MatchString(p.ID)
		if _, exists := phases[p.ID]; exists || !validID || !contains(roles, p.Role) || strings.TrimSpace(p.Title) == "" || len(p.Title) > 20000 || strings.TrimSpace(p.Instructions) == "" || len(p.Instructions) > 20000 {
			return failure(422, "invalid_phase", "Phases need unique stable IDs, known roles, titles and instructions")
		}
		if p.Agentic && (w.Family == "human" || contains([]string{"Todo", "Ready", "Clarify", "Approval", "Done", "Cancelled"}, p.Role)) {
			return failure(422, "phase_gate", "This semantic role must remain manual")
		}
		if w.Family == "human" && !contains([]string{"Todo", "Doing", "Done", "Clarify", "Cancelled"}, p.Role) {
			return failure(422, "phase_gate", "Human workflows use manual human roles")
		}
		phases[p.ID] = p
		counts[p.Role]++
	}
	seen := map[string]bool{}
	for i, r := range w.Rules {
		source, ok := phases[r.Source]
		if !ok || r.ID == "" || len(r.ID) > 100 || seen[r.ID] || !regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`).MatchString(r.Outcome) || r.Priority < 0 || r.Priority > 10000 {
			return failure(422, "invalid_rule", "Rules need unique IDs, exact sources, typed outcomes and priority 0–10,000")
		}
		seen[r.ID] = true
		if (r.Target == "") == (r.Action == "") {
			return failure(422, "invalid_rule", "Choose one destination or wait/halt action")
		}
		if r.Action != "" && !contains([]string{"wait", "halt"}, r.Action) {
			return failure(422, "invalid_rule", "Supported actions are wait and halt")
		}
		if r.Target != "" {
			target, ok := phases[r.Target]
			if !ok {
				return failure(422, "invalid_rule", "Rule destination is missing")
			}
			if w.Family == "agent" && target.Role == "Done" && (source.Role != "Approval" || r.Outcome != "approve") {
				return failure(422, "approval_gate", "Agent Done requires human Approval and approve outcome")
			}
			if w.Family == "agent" && source.Role == "Refine" && contains([]string{"Ready", "Decompose"}, target.Role) && !contains([]string{"acceptScope", "acceptPlan"}, r.Outcome) {
				return failure(422, "scope_gate", "Refinement requires explicit human scope acceptance")
			}
		}
		for key, value := range r.Conditions {
			if !contains([]string{"reason", "revisionKind", "questionOrigin", "result"}, key) || len(value) > 200 {
				return failure(422, "invalid_condition", "Use bounded declarative conditions")
			}
		}
		for _, other := range w.Rules[:i] {
			if other.Source == r.Source && other.Outcome == r.Outcome && other.Priority == r.Priority && overlap(other.Conditions, r.Conditions) {
				return failure(422, "ambiguous_rule", "Overlapping rules need distinct priorities or disjoint conditions")
			}
		}
	}
	for id, point := range w.Layout {
		if _, ok := phases[id]; !ok || point.X < -10000 || point.X > 10000 || point.Y < -10000 || point.Y > 10000 {
			return failure(422, "invalid_layout", "Layout must reference valid phases within finite bounds")
		}
	}
	if complete {
		for _, p := range w.Phases {
			requiredOutcomes := []string{}
			if w.Family == "agent" {
				switch p.Role {
				case "Todo":
					requiredOutcomes = []string{"requestRefinement"}
				case "Refine":
					requiredOutcomes = []string{"acceptScope"}
				case "Ready":
					requiredOutcomes = []string{"start"}
				case "Approval":
					requiredOutcomes = []string{"approve", "revise", "rescope"}
				}
			}
			if p.Role == "Clarify" {
				requiredOutcomes = append(requiredOutcomes, "answered")
			}
			for _, outcome := range requiredOutcomes {
				found := false
				for _, r := range w.Rules {
					if r.Source == p.ID && r.Outcome == outcome && len(r.Conditions) == 0 {
						found = true
					}
				}
				if !found {
					return failure(422, "incomplete_routing", "A required human action needs a fallback route")
				}
			}
			if p.Role == "Todo" {
				queue := []string{p.ID}
				seen := map[string]bool{}
				reachesDone := false
				for len(queue) > 0 {
					current := queue[0]
					queue = queue[1:]
					if seen[current] {
						continue
					}
					seen[current] = true
					if phases[current].Role == "Done" {
						reachesDone = true
						break
					}
					for _, r := range w.Rules {
						if r.Source == current && r.Target != "" {
							queue = append(queue, r.Target)
						}
					}
				}
				if !reachesDone {
					return failure(422, "incomplete_routing", "Every Todo entry needs a declared path to Done")
				}
			}
		}
		required := []string{"Todo", "Done"}
		if w.Family == "agent" {
			required = append(required, "Refine", "Ready", "Doing", "Approval")
		}
		for _, role := range required {
			if counts[role] == 0 {
				return failure(422, "incomplete_workflow", "A required semantic gate is missing")
			}
		}
		for _, p := range w.Phases {
			if !p.Agentic {
				continue
			}
			for _, outcome := range []string{"success", "failure", "question"} {
				fallback := false
				for _, r := range w.Rules {
					if r.Source == p.ID && r.Outcome == outcome && len(r.Conditions) == 0 {
						fallback = true
					}
				}
				if !fallback {
					return failure(422, "incomplete_routing", "Every agentic phase requires fallback success, failure and question routes")
				}
			}
		}
	}
	return nil
}
func ResolveOutcome(w model.Workflow, source, outcome string, context map[string]string) (RoutePreview, error) {
	if _, ok := phaseByID(w, source); !ok {
		return RoutePreview{}, failure(422, "invalid_phase", "Source phase is missing")
	}
	matches := []model.Rule{}
	for _, r := range w.Rules {
		if r.Source != source || r.Outcome != outcome {
			continue
		}
		match := true
		for key, value := range r.Conditions {
			if context[key] != value {
				match = false
				break
			}
		}
		if match {
			matches = append(matches, r)
		}
	}
	if len(matches) == 0 {
		return RoutePreview{}, failure(409, "routing_unmatched", "No declared outcome rule matches; attention is required")
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Priority < matches[j].Priority })
	if len(matches) > 1 && matches[0].Priority == matches[1].Priority {
		return RoutePreview{}, failure(409, "routing_ambiguous", "More than one effective rule matches")
	}
	r := matches[0]
	p := RoutePreview{RuleID: r.ID, Target: r.Target, Action: r.Action, RequiredGates: []string{"current rights and versions"}}
	if target, ok := phaseByID(w, r.Target); ok {
		p.NeedsAttempt = target.Agentic
		if outcome == "answered" && context["questionOrigin"] == target.ID {
			p.NeedsAttempt = false
		}
		if target.Agentic {
			p.RequiredGates = append(p.RequiredGates, "prior exit, finite attempts, capacity, dependencies and execution policy")
		}
		if target.Role == "Ready" || target.Role == "Decompose" {
			p.RequiredGates = append(p.RequiredGates, "current human-accepted scope")
		}
		if target.Role == "Done" && w.Family == "agent" {
			p.RequiredGates = append(p.RequiredGates, "exact-candidate human approval and verified Git integration")
		}
	}
	return p, nil
}

func workflowOperations(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	switch operation {
	case "listWorkflows":
		var in WorkflowListInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		_, limit, err := pageBounds("", in.Limit)
		if err != nil {
			return nil, false, err
		}
		if in.ProjectID != 0 {
			if err = require(st, a, in.ProjectID, "Viewer"); err != nil {
				return nil, false, err
			}
		}
		keys := []string{}
		for key, w := range st.Workflows {
			if key <= in.Cursor || in.ProjectID != 0 && w.ProjectID != 0 && w.ProjectID != in.ProjectID {
				continue
			}
			if _, err := workflowRead(st, a, key); err == nil {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		page := Page[model.Workflow]{Items: []model.Workflow{}}
		for i, key := range keys {
			if i == limit {
				page.NextCursor = keys[i-1]
				break
			}
			page.Items = append(page.Items, st.Workflows[key])
		}
		return page, false, nil
	case "getWorkflow":
		var in WorkflowKeyInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		if in.TicketID != 0 {
			t, err := ticketFor(st, a, in.TicketID, "Viewer")
			if err != nil {
				return nil, false, err
			}
			if t.Workflow != in.WorkflowKey {
				return nil, false, failure(403, "forbidden", "Ticket context grants only its pinned definition")
			}
			w, ok := st.Workflows[in.WorkflowKey]
			if !ok {
				return nil, false, failure(404, "not_found", "Pinned workflow unavailable")
			}
			return w, false, nil
		}
		w, err := workflowRead(st, a, in.WorkflowKey)
		return w, false, err
	case "copyWorkflow":
		var in CopyWorkflowInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		source, err := workflowRead(st, a, in.SourceKey)
		if err != nil {
			return nil, false, err
		}
		candidate := source
		candidate.ProjectID = in.ProjectID
		if err = workflowWrite(st, a, candidate); err != nil {
			return nil, false, err
		}
		key, fingerprint, previous, err := receipt(st, a, operation, in.RequestID, in)
		if err != nil {
			return nil, false, err
		}
		if previous != nil {
			var saved model.Workflow
			json.Unmarshal(previous, &saved)
			return saved, false, nil
		}
		candidate.ID = "flow-" + randomID()[:12]
		candidate.Title = in.Title
		candidate.Published = false
		candidate.Version = 1
		candidate.DraftRevision = 1
		if err = ValidateWorkflow(candidate, false); err != nil {
			return nil, false, err
		}
		st.Workflows[workflowKey(candidate)] = candidate
		remember(st, key, fingerprint, candidate)
		return candidate, true, nil
	case "draftWorkflow":
		var in WorkflowKeyInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		source, err := workflowRead(st, a, in.WorkflowKey)
		if err != nil {
			return nil, false, err
		}
		if err = workflowWrite(st, a, source); err != nil {
			return nil, false, err
		}
		if draft, ok := st.Workflows[source.ID+"@draft"]; ok {
			return draft, false, nil
		}
		latest := source.Version
		for _, existing := range st.Workflows {
			if existing.ID == source.ID && existing.Version > latest {
				latest = existing.Version
			}
		}
		source.Published = false
		source.Version = latest + 1
		source.DraftRevision = 1
		st.Workflows[workflowKey(source)] = source
		return source, true, nil
	case "saveWorkflow":
		var in SaveWorkflowInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		old, err := workflowRead(st, a, in.Definition.ID+"@draft")
		if err != nil {
			return nil, false, err
		}
		if err = workflowWrite(st, a, old); err != nil {
			return nil, false, err
		}
		if old.DraftRevision != in.ExpectedRevision || in.Definition.Published || in.Definition.ProjectID != old.ProjectID || in.Definition.Version != old.Version || in.Definition.Family != old.Family {
			return nil, false, failure(409, "workflow_conflict", "Refresh draft; identity, family and ownership are immutable")
		}
		candidate := in.Definition
		candidate.DraftRevision = old.DraftRevision
		if reflect.DeepEqual(candidate, old) {
			return old, false, nil
		}
		candidate.DraftRevision = old.DraftRevision + 1
		if err = ValidateWorkflow(candidate, false); err != nil {
			return nil, false, err
		}
		st.Workflows[workflowKey(candidate)] = candidate
		return candidate, true, nil
	case "deleteWorkflow":
		var in DeleteWorkflowInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		w, err := workflowRead(st, a, in.WorkflowKey)
		if err != nil {
			return nil, false, err
		}
		if err = workflowWrite(st, a, w); err != nil {
			return nil, false, err
		}
		if w.Published || w.DraftRevision != in.ExpectedRevision {
			return nil, false, failure(409, "workflow_conflict", "Only an exact current draft may be deleted; published history remains intact")
		}
		delete(st.Workflows, in.WorkflowKey)
		return map[string]bool{"deleted": true}, true, nil
	case "previewWorkflowOutcome":
		var in OutcomePreviewInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		w, err := workflowRead(st, a, in.WorkflowKey)
		if err != nil {
			return nil, false, err
		}
		preview, err := ResolveOutcome(w, in.Source, in.Outcome, in.Context)
		return preview, false, err
	case "activateWorkflowVersion":
		var in ActivateWorkflowInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		w, err := workflowRead(st, a, in.WorkflowKey)
		if err != nil {
			return nil, false, err
		}
		if err = require(st, a, in.ProjectID, "Maintainer"); err != nil {
			return nil, false, err
		}
		if w.ProjectID != 0 && w.ProjectID != in.ProjectID {
			return nil, false, failure(403, "forbidden", "Workflow belongs to another project")
		}
		p := st.Projects[in.ProjectID]
		if p.Version != in.ExpectedProjectVersion {
			return nil, false, failure(409, "version_conflict", "Project settings changed")
		}
		if w.Published && ((w.Family == "human" && p.HumanWorkflow == workflowKey(w)) || (w.Family == "agent" && p.AgentWorkflow == workflowKey(w))) {
			return p, false, nil
		}
		if !w.Published {
			if err = workflowWrite(st, a, w); err != nil {
				return nil, false, err
			}
			if w.DraftRevision != in.ExpectedRevision {
				return nil, false, failure(409, "workflow_conflict", "Draft changed")
			}
			if err = ValidateWorkflow(w, true); err != nil {
				return nil, false, err
			}
			w.Published = true
			w.DraftRevision = 0
			if _, exists := st.Workflows[workflowKey(w)]; exists {
				return nil, false, failure(409, "workflow_conflict", "Published versions are immutable")
			}
			st.Workflows[workflowKey(w)] = w
			delete(st.Workflows, w.ID+"@draft")
		}
		if w.Family == "human" {
			p.HumanWorkflow = workflowKey(w)
		} else {
			p.AgentWorkflow = workflowKey(w)
		}
		p.Version++
		st.Projects[p.ID] = p
		return p, true, nil
	case "previewWorkflowMigration", "migrateWorkflow":
		var in MigrationInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		source, err := workflowRead(st, a, in.SourceKey)
		if err != nil {
			return nil, false, err
		}
		target, err := workflowRead(st, a, in.TargetKey)
		if err != nil {
			return nil, false, err
		}
		if !source.Published || !target.Published || source.Family != target.Family || len(in.Tickets) == 0 || len(in.Tickets) > 200 {
			return nil, false, failure(422, "invalid_migration", "Select 1–200 records and published same-family workflows")
		}
		result := MigrationResult{Items: []model.Ticket{}, Preview: operation == "previewWorkflowMigration"}
		seen := map[int64]bool{}
		for _, record := range in.Tickets {
			t, err := ticketFor(st, a, record.ID, "Maintainer")
			if err != nil {
				return nil, false, err
			}
			if err = expected(t, record.ExpectedVersion); err != nil {
				return nil, false, err
			}
			if seen[t.ID] || t.Workflow != in.SourceKey || busy(t) || hasContinuation(t) {
				return nil, false, failure(409, "migration_blocked", "Select unique stopped/unclaimed source-pinned work")
			}
			seen[t.ID] = true
			if target.ProjectID != 0 && target.ProjectID != t.ProjectID {
				return nil, false, failure(403, "forbidden", "Target belongs to another project")
			}
			old, ok := phaseByID(source, t.Phase)
			next, found := phaseByID(target, in.Mapping[t.Phase])
			if !ok || !found || old.Role != next.Role {
				return nil, false, failure(422, "invalid_mapping", "Every selected phase needs an exact same-role target")
			}
			t.Workflow, t.Phase = in.TargetKey, next.ID
			t.ResumeHold = t.Agent
			result.Items = append(result.Items, t)
		}
		if result.Preview {
			return result, false, nil
		}
		for i := range result.Items {
			saveTicket(st, &result.Items[i], a, operation)
		}
		return result, true, nil
	}
	return agentOperations(st, a, operation, raw)
}
