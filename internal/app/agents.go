package app

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"teleport/internal/model"
)

type ExecutionModeInput struct {
	Mode            string `json:"mode"`
	ExpectedVersion int64  `json:"expectedVersion"`
}
type DemoStageInput struct {
	ID              int64  `json:"id"`
	ExpectedVersion int64  `json:"expectedVersion"`
	RequestID       string `json:"requestId"`
	Outcome         string `json:"outcome,omitempty"`
	Question        string `json:"question,omitempty"`
}
type DemoStageResult struct {
	Ticket        model.Ticket `json:"ticket"`
	Simulated     bool         `json:"simulated"`
	AttemptID     string       `json:"attemptId,omitempty"`
	BlockedReason string       `json:"blockedReason,omitempty"`
}
type AcceptScopeInput struct {
	ID              int64  `json:"id"`
	ExpectedVersion int64  `json:"expectedVersion"`
	ProposalID      string `json:"proposalId"`
	Decompose       bool   `json:"decompose,omitempty"`
}
type AnswerInput struct {
	ID              int64  `json:"id"`
	ExpectedVersion int64  `json:"expectedVersion"`
	QuestionID      string `json:"questionId"`
	Answer          string `json:"answer"`
}
type ApprovalInput struct {
	ID              int64  `json:"id"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Confirm         bool   `json:"confirm"`
	RevisionKind    string `json:"revisionKind,omitempty"`
	Feedback        string `json:"feedback,omitempty"`
}
type BudgetInput struct {
	ID              int64 `json:"id"`
	ExpectedVersion int64 `json:"expectedVersion"`
	Limit           int   `json:"limit"`
	Resume          bool  `json:"resume,omitempty"`
}
type CapacityInput struct {
	ProjectID       int64 `json:"projectId"`
	ExpectedVersion int64 `json:"expectedVersion"`
	RefineWorkers   int   `json:"refineWorkers"`
	DoingWorkers    int   `json:"doingWorkers"`
}
type HandbrakeInput struct {
	ProjectID       int64  `json:"projectId,omitempty"`
	ExpectedVersion int64  `json:"expectedVersion"`
	WorkflowKey     string `json:"workflowKey,omitempty"`
	PhaseID         string `json:"phaseId,omitempty"`
	Engaged         bool   `json:"engaged"`
}

func scopeBasis(t model.Ticket) string {
	b, _ := json.Marshal(struct {
		ProjectID, ParentID                    int64
		Title, Description, Criteria, Workflow string
		Dependencies                           []int64
	}{t.ProjectID, t.ParentID, t.Title, t.Description, t.Criteria, t.Workflow, t.Dependencies})
	return digest(string(b))
}
func hasContinuation(t model.Ticket) bool {
	return !t.ResumeHold && (t.ResumeAttempt != "" || t.Question != nil && !t.Question.Resolved && !t.Question.Obsolete)
}
func executionBasis(st *model.State, t model.Ticket) string {
	basis := scopeBasis(t)
	if repository := st.Projects[t.ProjectID].Repository; repository != nil {
		encoded, _ := json.Marshal(struct {
			TaskBasis         string
			RepositoryVersion int64
		}{basis, repository.Version})
		return digest(string(encoded))
	}
	return basis
}
func enterPhase(t *model.Ticket, w model.Workflow, target string) error {
	p, ok := phaseByID(w, target)
	if !ok {
		return failure(409, "workflow_mapping", "Declared destination is unavailable")
	}
	t.Phase, t.Status = target, p.Role
	t.StageVisit++
	t.PhaseEnteredAt = time.Now().UTC()
	t.RoutingHold = ""
	return nil
}
func applyRoute(t *model.Ticket, w model.Workflow, outcome string, context map[string]string) (RoutePreview, error) {
	route, err := ResolveOutcome(w, t.Phase, outcome, context)
	if err != nil {
		return route, err
	}
	if route.Target != "" {
		err = enterPhase(t, w, route.Target)
	} else if route.Action != "" {
		t.RoutingHold = route.Action
	}
	return route, err
}
func acceptedScope(st *model.State, t model.Ticket) bool {
	if t.Refinement == nil || t.Refinement.AcceptedBy == "" {
		return false
	}
	basis := t.Refinement.AcceptedBasis
	if basis == "" {
		basis = t.Refinement.Basis
	}
	return basis == executionBasis(st, t)
}
func demoAdmission(st *model.State, t model.Ticket, p model.Phase) error {
	if t.Delivery != nil {
		return failure(409, "real_delivery", "Real retained work cannot be executed by the demo adapter")
	}
	if t.RoutingHold != "" {
		return failure(409, "routing_hold", "A declared workflow wait/halt requires its explicit continuation")
	}
	if t.Kind != "task" && !t.Planning {
		return failure(409, "planning_closed", "A container's delivery rollup is not an executable implementation phase")
	}
	if !st.Projects[t.ProjectID].Simulation && (st.Settings.ExecutionMode != "demo" || !st.Settings.Agents) {
		return failure(409, "execution_disabled", "Enable explicit demo execution first")
	}
	if !t.Agent || t.Archived || t.Deleted || t.Status == "Done" || t.Status == "Cancelled" || t.ResumeHold || t.StopRequested || busy(t) || t.Question != nil && !t.Question.Resolved && !t.Question.Obsolete {
		return failure(409, "admission_blocked", "Work is held, hidden, terminal, questioned or already owned")
	}
	project := st.Projects[t.ProjectID]
	w := st.Workflows[t.Workflow]
	if st.Settings.Handbrake || project.Handbrake || project.LaneLocks[w.ID+"#*"] || project.LaneLocks[w.ID+"#"+t.Phase] {
		return failure(409, "handbrake", "An applicable handbrake blocks admission")
	}
	if !dependenciesDone(st, t) {
		return failure(409, "unmet_prerequisite", "Every prerequisite must be Done")
	}
	if contains([]string{"Doing", "Rebase", "Review", "Decompose"}, p.Role) && !acceptedScope(st, t) {
		return failure(409, "scope_stale", "Current human-accepted scope is required")
	}
	if err := phaseCapacity(st, t, p); err != nil {
		return err
	}
	if p.Role == "Refine" && t.Refinement != nil && t.ResumeAttempt == "" {
		return failure(409, "scope_waiting", "The proposal is awaiting human acceptance")
	}
	return nil
}

// demoStep runs a deterministic in-process adapter. No OS process, model or code write is implied.
func demoStep(st *model.State, a model.Account, in DemoStageInput, internal bool) (DemoStageResult, bool, error) {
	t, err := ticketFor(st, a, in.ID, "Contributor")
	if internal {
		t = st.Tickets[in.ID]
		err = nil
	}
	if err != nil {
		return DemoStageResult{}, false, err
	}
	key, fingerprint, previous, err := receipt(st, a, "runDemoStage", in.RequestID, in)
	if err != nil {
		return DemoStageResult{}, false, err
	}
	if previous != nil {
		var result DemoStageResult
		json.Unmarshal(previous, &result)
		return result, false, nil
	}
	if err = expected(t, in.ExpectedVersion); err != nil {
		return DemoStageResult{}, false, err
	}
	if t.RoutingHold != "" {
		return DemoStageResult{}, false, failure(409, "routing_hold", "A declared workflow wait/halt is still active")
	}
	w := st.Workflows[t.Workflow]
	phase, ok := phaseByID(w, t.Phase)
	if !ok {
		return DemoStageResult{}, false, failure(409, "workflow_mapping", "Pinned phase is unavailable")
	}
	if phase.Role == "Ready" {
		if err := requireReadyReview(st, t); err != nil {
			return DemoStageResult{}, false, err
		}
		route, err := ResolveOutcome(w, t.Phase, "start", nil)
		if err != nil {
			return DemoStageResult{}, false, err
		}
		if route.Action != "" {
			t.RoutingHold = route.Action
			t.Attention = "Workflow " + route.Action
			saveTicket(st, &t, a, "routingHold")
			return DemoStageResult{Ticket: t, Simulated: true, BlockedReason: t.Attention}, true, nil
		}
		target, ok := phaseByID(w, route.Target)
		if !ok || target.Role != "Doing" {
			return DemoStageResult{}, false, failure(409, "phase_gate", "Ready must admit to an implementation role")
		}
		if err = demoAdmission(st, t, target); err != nil {
			return DemoStageResult{}, false, err
		}
		if st.Projects[t.ProjectID].LaneLocks[w.ID+"#"+target.ID] {
			return DemoStageResult{}, false, failure(409, "handbrake", "Destination phase is locked")
		}
		if err = enterPhase(&t, w, target.ID); err != nil {
			return DemoStageResult{}, false, err
		}
		phase = target
	}
	if !phase.Agentic {
		return DemoStageResult{}, false, failure(409, "manual_phase", "This phase requires a human action")
	}
	if err = demoAdmission(st, t, phase); err != nil {
		return DemoStageResult{}, false, err
	}
	outcome := in.Outcome
	if outcome == "" {
		outcome = "success"
	}
	if !contains([]string{"success", "failure", "question"}, outcome) {
		return DemoStageResult{}, false, failure(422, "invalid_outcome", "Demo outcomes are success, failure or question")
	}
	if in.Outcome == "question" && (strings.TrimSpace(in.Question) == "" || len(in.Question) > 20000) {
		return DemoStageResult{}, false, failure(422, "invalid_question", "Provide a question up to 20,000 bytes")
	}
	limit := t.AttemptLimit
	if limit == 0 {
		limit = 20
		t.AttemptLimit = limit
	}
	attempt := t.ResumeAttempt
	continues := attempt != ""
	if continues {
		if t.Question == nil || t.Question.Basis != executionBasis(st, t) || t.Question.Workflow != t.Workflow || t.Question.OriginPhase != t.Phase {
			return DemoStageResult{}, false, failure(409, "continuation_stale", "Continuation basis changed; explicit replanning is required")
		}
	} else {
		if t.Attempts >= limit {
			t.Attention = "Budget exhausted"
			t.ResumeHold = true
			saveTicket(st, &t, a, "budgetExhausted")
			return DemoStageResult{Ticket: t, Simulated: true, BlockedReason: t.Attention}, true, nil
		}
		t.Attempts++
		attempt = randomID()
	}
	t.ResumeAttempt = ""
	t.Attention = ""
	now := time.Now().UTC()
	serviceActor := model.Account{ID: "demo:" + attempt}
	source := t.Phase
	if outcome == "question" {
		t.Question = &model.Question{ID: randomID(), Text: in.Question, OriginPhase: source, OriginAttempt: attempt, Workflow: t.Workflow, Basis: executionBasis(st, t), AskedAt: now, OriginWaitingSince: t.PhaseEnteredAt}
		route, err := applyRoute(&t, w, outcome, nil)
		if err != nil {
			return DemoStageResult{}, false, err
		}
		if route.Target != "" && t.Status != "Clarify" && t.Status != "Todo" {
			return DemoStageResult{}, false, failure(409, "question_route", "Structured questions may wait or enter manual Clarify/Todo")
		}
		t.Attention = "Question requires an answer"
	} else {
		if phase.Role == "Refine" && outcome == "success" {
			criteria := t.Criteria
			if strings.TrimSpace(criteria) == "" {
				criteria = "Deliver the requested work with recorded checks."
			}
			t.Refinement = &model.Refinement{ID: randomID(), Text: "Demo proposal: " + t.Title, Criteria: criteria, Basis: executionBasis(st, t), Simulated: true}
			t.Attention = "Scope acceptance required"
		}
		if phase.Role == "Review" {
			t.AgentReview = &model.AgentReview{ID: randomID(), Outcome: outcome, Findings: "Deterministic demo review; no real code or checks were inspected.", Simulated: true, At: now}
		}
		if _, err = applyRoute(&t, w, outcome, nil); err != nil {
			return DemoStageResult{}, false, err
		}
		if t.RoutingHold != "" && t.Attention == "" {
			t.Attention = "Workflow " + t.RoutingHold
		}
		if t.Status == "Approval" {
			t.Attention = "Demo result: real delivery evidence is unavailable"
		}
		if phase.Role == "Decompose" && outcome == "success" {
			child := newTicket(st, st.Projects[t.ProjectID], serviceActor, "task", "Demo plan: "+t.Title, true, false)
			child.ParentID = t.ID
			saveTicket(st, &child, serviceActor, "demoPlan")
			t.Planning = false
		}
	}
	t.Active = false
	t.Claim = ""
	t.Execution = ""
	t.StageEvents = append(t.StageEvents, model.StageEvent{AttemptID: attempt, Phase: source, Outcome: outcome, Actor: serviceActor.ID, At: now, Continues: continues, Simulated: true})
	saveTicket(st, &t, serviceActor, "demoStageOutcome")
	if !t.Planning && t.Kind != "task" {
		reconcile(st, t.ID, serviceActor)
		t = st.Tickets[t.ID]
	}
	reconcile(st, t.ParentID, serviceActor)
	result := DemoStageResult{Ticket: t, Simulated: true, AttemptID: attempt}
	remember(st, key, fingerprint, result)
	return result, true, nil
}

func (s *Service) PumpDemo(ctx context.Context) error {
	return s.db.Update(ctx, func(st *model.State) (bool, error) {
		attentionChanged := reconcileReadyAttention(st, model.Account{ID: "ready-review-supervisor"})
		collections(st)
		ids := []int64{}
		for id, t := range st.Tickets {
			if !st.Projects[t.ProjectID].Simulation && (st.Settings.ExecutionMode != "demo" || !st.Settings.Agents) {
				continue
			}
			if st.Projects[t.ProjectID].Simulation && time.Since(t.PhaseEnteredAt) < time.Second {
				continue
			}
			if !t.Agent || t.Kind != "task" && !t.Planning || t.Status == "Done" || t.Status == "Cancelled" || t.Status == "Todo" || t.Status == "Approval" || t.Status == "Clarify" {
				continue
			}
			ids = append(ids, id)
		}
		ranks := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		sort.Slice(ids, func(i, j int) bool {
			a, b := st.Tickets[ids[i]], st.Tickets[ids[j]]
			if ranks[a.Priority] != ranks[b.Priority] {
				return ranks[a.Priority] < ranks[b.Priority]
			}
			aw, bw := a.PhaseEnteredAt, b.PhaseEnteredAt
			if aw.IsZero() {
				aw = a.CreatedAt
			}
			if bw.IsZero() {
				bw = b.CreatedAt
			}
			if aw.Equal(bw) {
				return a.ID < b.ID
			}
			return aw.Before(bw)
		})
		for _, id := range ids {
			t := st.Tickets[id]
			in := DemoStageInput{ID: id, ExpectedVersion: t.Version, RequestID: "pump-" + t.Workflow + "-" + randomID()[:12]}
			encoded, err := json.Marshal(st)
			if err != nil {
				return false, err
			}
			var candidate model.State
			if err = json.Unmarshal(encoded, &candidate); err != nil {
				return false, err
			}
			_, changed, err := demoStep(&candidate, model.Account{ID: "project-demo"}, in, true)
			if err == nil && changed {
				*st = candidate
				return true, nil
			}
		}
		return attentionChanged, nil
	})
}

func agentOperations(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	switch operation {
	case "setExecutionMode":
		var in ExecutionModeInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		if !a.Administrator {
			return nil, false, failure(403, "forbidden", "Installation execution configuration requires administrator rights")
		}
		if in.ExpectedVersion != st.Settings.Version {
			return nil, false, failure(409, "version_conflict", "Execution settings changed")
		}
		if !contains([]string{"off", "demo"}, in.Mode) {
			return nil, false, failure(422, "unsupported_adapter", "Only off and deterministic demo modes have verification evidence")
		}
		if st.Settings.ExecutionMode == in.Mode {
			return st.Settings, false, nil
		}
		st.Settings.ExecutionMode = in.Mode
		st.Settings.Agents = in.Mode == "demo"
		st.Settings.Version++
		return st.Settings, true, nil
	case "requestRefinement":
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
		if !t.Agent {
			if t.Kind != "task" || t.Status != "Todo" || !refinementQueueAvailable(st, t) {
				return nil, false, failure(409, "refinement_blocked", "Send an unclaimed human Todo draft without retained implementation to refinement")
			}
			t.Agent, t.Assignee, t.Workflow = true, "", st.Projects[t.ProjectID].AgentWorkflow
			initial, ok := phaseForRole(st.Workflows[t.Workflow], "Todo")
			if !ok {
				return nil, false, failure(409, "workflow_mapping", "The project agent workflow needs one Todo phase")
			}
			t.Phase = initial.ID
		}
		if !t.Agent || t.Status != "Todo" || t.Kind == "goal" || busy(t) || t.Archived || t.Deleted {
			return nil, false, failure(409, "refinement_blocked", "Request refinement on an ordinary visible unclaimed agent Todo task/epic")
		}
		if _, err = applyRoute(&t, st.Workflows[t.Workflow], "requestRefinement", nil); err != nil {
			return nil, false, err
		}
		if t.Status != "Refine" {
			return nil, false, failure(409, "phase_gate", "Refinement request must enter a Refine role")
		}
		if t.Kind == "epic" {
			t.Planning = true
		}
		t.Refinement = nil
		t.AgentReview = nil
		t.Question = nil
		t.ResumeAttempt = ""
		t.ResumeHold = false
		t.StopRequested = false
		t.Attention = ""
		saveTicket(st, &t, a, operation)
		reconcile(st, t.ParentID, a)
		reconcileDependencyStops(st, a)
		return t, true, nil
	case "returnRefinementToTodo":
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
		if a.ServiceSessionID != "" || a.ServiceRunID != "" || !t.Agent || t.Kind != "task" || t.Status != "Refine" || !refinementQueueAvailable(st, t) {
			return nil, false, failure(409, "refinement_owned", "Only an unowned queued Refine task can return to human Todo")
		}
		workflow := st.Projects[t.ProjectID].HumanWorkflow
		initial, ok := phaseForRole(st.Workflows[workflow], "Todo")
		if !ok {
			return nil, false, failure(409, "workflow_mapping", "The project human workflow needs one Todo phase")
		}
		t.Agent, t.Assignee, t.Workflow = false, a.ID, workflow
		if err = enterPhase(&t, st.Workflows[workflow], initial.ID); err != nil {
			return nil, false, err
		}
		t.Refinement, t.ResumeAttempt, t.Attention = nil, "", ""
		t.ResumeHold, t.StopRequested = false, false
		if t.Question != nil {
			t.Question.Obsolete = true
		}
		saveTicket(st, &t, a, operation)
		reconcile(st, t.ParentID, a)
		return t, true, nil
	case "runDemoStage":
		var in DemoStageInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		return demoStep(st, a, in, false)
	case "acceptRefinement":
		var in AcceptScopeInput
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
		if t.Status != "Refine" || busy(t) || t.Question != nil && !t.Question.Resolved && !t.Question.Obsolete || t.Refinement == nil || t.Refinement.ID != in.ProposalID || t.Refinement.Basis != executionBasis(st, t) {
			return nil, false, failure(409, "scope_stale", "Accept the exact current unblocked refinement proposal")
		}
		previousParent := t.ParentID
		outcome := "acceptScope"
		if in.Decompose {
			outcome = "acceptPlan"
		}
		if t.Kind == "epic" && !in.Decompose {
			return nil, false, failure(409, "planning_required", "Epics require accepted decomposition")
		}
		if _, err = applyRoute(&t, st.Workflows[t.Workflow], outcome, nil); err != nil {
			return nil, false, err
		}
		if in.Decompose {
			if t.Status != "Decompose" {
				return nil, false, failure(409, "phase_gate", "Plan acceptance must enter Decompose")
			}
			if t.Kind == "task" {
				t.Kind = "epic"
				t.ParentID = st.Tickets[t.ParentID].ParentID
			}
			t.Planning = true
		} else if t.Status != "Ready" {
			return nil, false, failure(409, "phase_gate", "Scope acceptance must enter Ready")
		}
		t.Refinement.AcceptedBy = a.ID
		t.Refinement.AcceptedAt = time.Now().UTC()
		t.Refinement.AcceptedBasis = executionBasis(st, t)
		t.Attention = ""
		agreement := completionBasis(st, t, a, t.Refinement.Criteria, t.Version)
		agreement.Kind = "Ready"
		if in.Decompose {
			agreement.Kind = "Plan"
		}
		t.Agreements = append(t.Agreements, agreement)
		saveTicket(st, &t, a, operation)
		if previousParent != t.ParentID {
			reconcile(st, previousParent, a)
		}
		reconcile(st, t.ParentID, a)
		return t, true, nil
	case "answerQuestion":
		var in AnswerInput
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
		q := t.Question
		if q == nil || q.ID != in.QuestionID || q.Resolved || q.Obsolete || t.Status == "Done" || t.Status == "Cancelled" || busy(t) || strings.TrimSpace(in.Answer) == "" || len(in.Answer) > 20000 {
			return nil, false, failure(409, "question_unavailable", "Answer an exact current question after confirmed prior exit")
		}
		q.Answer, q.Resolved, q.AnsweredAt = in.Answer, true, time.Now().UTC()
		target := q.OriginPhase
		if t.Status == "Clarify" {
			route, err := ResolveOutcome(st.Workflows[t.Workflow], t.Phase, "answered", map[string]string{"questionOrigin": q.OriginPhase})
			if err != nil {
				return nil, false, err
			}
			target = route.Target
		}
		if t.Workflow != q.Workflow || executionBasis(st, t) != q.Basis {
			t.Attention = "Answer saved; continuation basis changed"
		} else {
			if err = enterPhase(&t, st.Workflows[t.Workflow], target); err != nil {
				return nil, false, err
			}
			if target == q.OriginPhase {
				t.ResumeAttempt = q.OriginAttempt
				if !q.OriginWaitingSince.IsZero() {
					t.PhaseEnteredAt = q.OriginWaitingSince
				}
			}
			t.Attention = ""
		}
		saveTicket(st, &t, a, operation)
		return t, true, nil
	case "approveTicket":
		var in ApprovalInput
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
		return nil, false, failure(409, "delivery_evidence_required", "Demo evidence cannot approve real code; verified candidate, review, checks and Git integration are required")
	case "rejectTicket":
		var in ApprovalInput
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
		if t.Status != "Approval" || busy(t) || !contains([]string{"implementation", "scope"}, in.RevisionKind) || len(in.Feedback) > 20000 {
			return nil, false, failure(409, "invalid_revision", "Select an exact human Approval revision kind")
		}
		outcome := "revise"
		if in.RevisionKind == "scope" {
			outcome = "rescope"
		}
		if _, err = applyRoute(&t, st.Workflows[t.Workflow], outcome, nil); err != nil {
			return nil, false, err
		}
		if in.RevisionKind == "scope" {
			t.Refinement = nil
			t.ResumeHold = true
		} else if !acceptedScope(st, t) {
			return nil, false, failure(409, "scope_stale", "Readiness must be re-agreed before implementation revision")
		}
		t.Attention = "Human feedback: " + in.Feedback
		saveTicket(st, &t, a, operation)
		return t, true, nil
	case "setExecutionBudget":
		var in BudgetInput
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
		if in.Limit < 1 || in.Limit > 1000 || !t.Agent {
			return nil, false, failure(422, "invalid_budget", "Agent attempt limits must be integers from 1 to 1,000")
		}
		t.AttemptLimit = in.Limit
		if in.Resume {
			if busy(t) {
				return nil, false, failure(409, "execution_busy", "Prior execution must have exited")
			}
			t.ResumeHold = false
			t.Attention = ""
		}
		saveTicket(st, &t, a, operation)
		return t, true, nil
	case "pauseTicket", "resumeTicket":
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
		if operation == "resumeTicket" {
			if busy(t) || t.Status == "Done" || t.Status == "Cancelled" {
				return nil, false, failure(409, "resume_blocked", "Confirm prior exit and keep terminal work explicitly closed")
			}
			t.ResumeHold = false
			t.StopRequested = false
			t.RoutingHold = ""
		} else {
			t.ResumeHold = true
			if busy(t) {
				t.StopRequested = true
			}
		}
		saveTicket(st, &t, a, operation)
		return t, true, nil
	case "setLaneCapacity":
		var in CapacityInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		if err := require(st, a, in.ProjectID, "Maintainer"); err != nil {
			return nil, false, err
		}
		p := st.Projects[in.ProjectID]
		if p.Version != in.ExpectedVersion {
			return nil, false, failure(409, "version_conflict", "Project configuration changed")
		}
		if in.RefineWorkers < 0 || in.RefineWorkers > 20 || in.DoingWorkers < 0 || in.DoingWorkers > 20 {
			return nil, false, failure(422, "invalid_capacity", "Worker counts must be 0–20")
		}
		p.RefineWorkers, p.DoingWorkers, p.CapacityConfigured = in.RefineWorkers, in.DoingWorkers, true
		p.Version++
		st.Projects[p.ID] = p
		return p, true, nil
	case "setHandbrake":
		var in HandbrakeInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		if in.ProjectID == 0 {
			if !a.Administrator {
				return nil, false, failure(403, "forbidden", "System handbrake requires administrator rights")
			}
			if st.Settings.Version != in.ExpectedVersion {
				return nil, false, failure(409, "version_conflict", "Settings changed")
			}
			st.Settings.Handbrake = in.Engaged
			st.Settings.Version++
			return st.Settings, true, nil
		}
		if err := require(st, a, in.ProjectID, "Maintainer"); err != nil {
			return nil, false, err
		}
		p := st.Projects[in.ProjectID]
		if p.Version != in.ExpectedVersion {
			return nil, false, failure(409, "version_conflict", "Project changed")
		}
		if in.PhaseID != "" || in.WorkflowKey != "" {
			w, err := workflowRead(st, a, in.WorkflowKey)
			if err != nil {
				return nil, false, err
			}
			if w.ProjectID != 0 && w.ProjectID != p.ID {
				return nil, false, failure(422, "invalid_board_workflow", "The workflow belongs to another project")
			}
			if _, ok := phaseByID(w, in.PhaseID); in.PhaseID != "" && !ok {
				return nil, false, failure(422, "invalid_phase", "Phase is missing")
			}
			if p.LaneLocks == nil {
				p.LaneLocks = map[string]bool{}
			}
			phase := in.PhaseID
			if phase == "" {
				phase = "*"
			}
			p.LaneLocks[w.ID+"#"+phase] = in.Engaged
		} else {
			p.Handbrake = in.Engaged
		}
		p.Version++
		st.Projects[p.ID] = p
		return p, true, nil
	}
	return agreementOperations(st, a, operation, raw)
}
