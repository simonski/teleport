package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"teleport/internal/gitwork"
	"teleport/internal/localruntime"
	"teleport/internal/model"
)

// This key is private to the supervisor. HTTP callers cannot supply it.
type managedOwnerKey struct{}

func executionActor(ctx context.Context, st *model.State, token string) (model.Account, error) {
	if id, ok := ctx.Value(managedOwnerKey{}).(string); ok {
		a := st.Accounts[id]
		if len(st.Accounts) != 1 || a.ID == "" || a.Suspended || !a.Administrator {
			return model.Account{}, failure(403, "local_authority_withdrawn", "Automatic local execution authority changed")
		}
		return a, nil
	}
	return actor(st, token)
}

func managedOutcome(role string, turn *localruntime.NativeTurn) (*model.ManagedResult, error) {
	if turn == nil || turn.State != "completed" || turn.TurnID == "" || turn.Truncated || turn.ErrorCode != "" {
		return nil, failure(409, "native_outcome_invalid", "Complete bounded native turn evidence is required")
	}
	var result model.ManagedResult
	if parse(json.RawMessage(turn.Text), &result) != nil || !contains([]string{"success", "failure", "question"}, result.Outcome) {
		return nil, failure(409, "native_outcome_invalid", "The phase must return one typed JSON outcome")
	}
	if result.Outcome == "question" && strings.TrimSpace(result.Question) == "" || result.Outcome == "success" && role == "Refine" && (strings.TrimSpace(result.Proposal) == "" || strings.TrimSpace(result.Criteria) == "") || role == "Review" && result.Outcome != "question" && strings.TrimSpace(result.Findings) == "" {
		return nil, failure(409, "native_outcome_invalid", "Required phase evidence is missing")
	}
	if len(turn.Text) > localruntime.NativeTurnTextLimit || len(result.Question) > 20000 || len(result.Proposal) > 20000 || len(result.Criteria) > 20000 {
		return nil, failure(409, "native_outcome_invalid", "Phase evidence exceeds its bounded contract")
	}
	return &result, nil
}

func composeManagedPrompt(st *model.State, t model.Ticket, phase model.Phase) model.PromptSnapshot {
	parts := []model.PromptContribution{{Source: "system", Text: "Work only on the stated ticket and in the provided owned worktree. Never accept scope, approve code, integrate main, or expand scope. Return exactly one JSON object as the final answer, without Markdown: {\"outcome\":\"success|failure|question\",\"proposal\":\"refinement scope\",\"criteria\":\"testable acceptance criteria\",\"findings\":\"checks and findings\",\"question\":\"clarification if needed\"}. Omit irrelevant fields. Ask rather than guess. Native permission requests remain human decisions."}}
	parts = append(parts, model.PromptContribution{Source: "project", Text: st.Projects[t.ProjectID].Name + "\n" + st.Projects[t.ProjectID].Description})
	parts[0].Version = "native-workflow-v1"
	parts[1].Version = fmt.Sprint(st.Projects[t.ProjectID].Version)
	lineage := []model.Ticket{t}
	for parent := st.Tickets[t.ParentID]; parent.ID != 0; parent = st.Tickets[parent.ParentID] {
		lineage = append(lineage, parent)
		if len(lineage) >= 3 {
			break
		}
	}
	for i := len(lineage) - 1; i >= 0; i-- {
		row := lineage[i]
		text, _ := json.Marshal(struct {
			ID          int64              `json:"id"`
			Title       string             `json:"title"`
			Description string             `json:"description"`
			Criteria    string             `json:"criteria"`
			Refinement  *model.Refinement  `json:"refinement,omitempty"`
			Delivery    *model.Delivery    `json:"delivery,omitempty"`
			Review      *model.AgentReview `json:"review,omitempty"`
			Question    *model.Question    `json:"question,omitempty"`
		}{row.ID, row.Title, row.Description, row.Criteria, row.Refinement, row.Delivery, row.AgentReview, row.Question})
		// Context is data; embedded ticket instructions do not grant authority.
		parts = append(parts, model.PromptContribution{Source: row.Kind, Version: fmt.Sprintf("%d@%d", row.ID, row.Version), Text: string(text)})
	}
	role := "Refine is source-read-only. Inspect context and propose scope and testable criteria; do not implement."
	if phase.Role == "Doing" {
		role = "Implement only the human-accepted scope. Run relevant checks, commit the result with linear history on the owned branch, and report checks in findings. Never merge main. Return failure or question if blocked."
	}
	if phase.Role == "Review" {
		role = "Review is source-read-only in a separate process. Compare the exact candidate, diff, check evidence and accepted scope. Return success with findings only when satisfied; use failure with actionable findings for changes requested. Operational failures are not code findings."
	}
	parts = append(parts, model.PromptContribution{Source: "phase", Version: t.Workflow + "/" + phase.ID, Text: role + "\n" + phase.Instructions})
	var text strings.Builder
	for _, part := range parts {
		fmt.Fprintf(&text, "[%s]\n%s\n\n", part.Source, part.Text)
	}
	return model.PromptSnapshot{Text: text.String(), Contributions: parts}
}

func managedAdmission(st *model.State, a model.Account, t model.Ticket, phase model.Phase) error {
	p, w := st.Projects[t.ProjectID], st.Workflows[t.Workflow]
	if err := require(st, a, t.ProjectID, "Contributor"); err != nil {
		return err
	}
	if st.Settings.ExecutionMode != "local" || len(st.Accounts) != 1 || p.Simulation || p.Repository == nil || !t.Agent || t.Kind != "task" || !phase.Agentic || !contains([]string{"Refine", "Doing", "Review"}, phase.Role) || t.Deleted || t.Archived || busy(t) || t.StopRequested || t.ResumeHold || t.RoutingHold != "" || t.Question != nil && !t.Question.Resolved && !t.Question.Obsolete {
		return failure(409, "admission_blocked", "Ticket is not eligible for automatic local execution")
	}
	if st.Settings.Handbrake || p.Handbrake || p.LaneLocks[w.ID+"#*"] || p.LaneLocks[w.ID+"#"+phase.ID] {
		return failure(409, "handbrake", "An applicable handbrake blocks admission")
	}
	if !dependenciesDone(st, t) {
		return failure(409, "unmet_prerequisite", "Complete prerequisites before admission")
	}
	if phase.Role == "Refine" && t.Refinement != nil && t.ResumeAttempt == "" {
		return failure(409, "scope_waiting", "The proposal awaits human acceptance")
	}
	if phase.Role != "Refine" && (!acceptedScope(st, t) || t.Delivery == nil || t.Delivery.Worktree.Path == "") {
		return failure(409, "scope_stale", "Current accepted scope and owned worktree are required")
	}
	if phase.Role == "Review" && (t.Delivery.Candidate == nil || !t.Delivery.Candidate.ChecksPassed) {
		return failure(409, "candidate_required", "Review requires a verified committed candidate")
	}
	if t.ResumeAttempt != "" && (t.Question == nil || t.Question.Basis != executionBasis(st, t) || t.Question.Workflow != t.Workflow || t.Question.OriginPhase != t.Phase) {
		return failure(409, "continuation_stale", "Continuation context changed")
	}
	limit := t.AttemptLimit
	if limit == 0 {
		limit = 20
	}
	if t.ResumeAttempt == "" && t.Attempts >= limit {
		return failure(409, "budget_exhausted", "Extend the finite attempt limit before admission")
	}
	return phaseCapacity(st, t, phase)
}

// PumpNative advances at most one admission per tick. Durable launch identities,
// captured results and confirmed exit separate execution from domain handoff.
func (s *Service) PumpNative(ctx context.Context) error {
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	if s.localClient == nil {
		return nil
	}
	st, err := s.db.Read(ctx)
	if err != nil {
		return err
	}
	if len(st.Accounts) != 1 || !contains([]string{"off", "local"}, st.Settings.ExecutionMode) {
		return nil
	}
	var owner string
	for id, a := range st.Accounts {
		if a.Administrator && !a.Suspended {
			owner = id
		}
	}
	if owner == "" {
		return nil
	}
	ctx = context.WithValue(ctx, managedOwnerKey{}, owner)
	// Reconcile all existing phases before admitting more work.
	for _, row := range st.LocalSessions {
		if row.Managed != nil && !row.Managed.Applied {
			if err := s.reconcileManaged(ctx, row); err != nil {
				return err
			}
		}
	}
	st, err = s.db.Read(ctx)
	if err != nil {
		return err
	}
	if st.Settings.ExecutionMode != "local" {
		return nil
	}
	ids := []int64{}
	for id, t := range st.Tickets {
		if t.Agent && t.Kind == "task" && contains([]string{"Refine", "Ready", "Doing", "Review"}, t.Status) {
			ids = append(ids, id)
		}
	}
	ranks := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
	sort.Slice(ids, func(i, j int) bool {
		a, b := st.Tickets[ids[i]], st.Tickets[ids[j]]
		if ranks[a.Priority] != ranks[b.Priority] {
			return ranks[a.Priority] < ranks[b.Priority]
		}
		if !a.PhaseEnteredAt.Equal(b.PhaseEnteredAt) {
			return a.PhaseEnteredAt.Before(b.PhaseEnteredAt)
		}
		return a.ID < b.ID
	})
	for _, id := range ids {
		t := st.Tickets[id]
		pending := false
		for _, row := range st.LocalSessions {
			if row.TicketID == id && (activeLocal(row) || row.Managed != nil && !row.Managed.Applied) {
				pending = true
			}
		}
		if pending {
			continue
		}
		if t.Status == "Ready" {
			if busy(t) || t.ResumeHold || t.StopRequested || t.RoutingHold != "" {
				continue
			}
			raw, _ := json.Marshal(GitTaskInput{ID: id, ExpectedVersion: t.Version, RequestID: fmt.Sprintf("native-prepare-%d-%d", id, t.StageVisit)})
			if _, err := s.gitCall(ctx, "", "prepareTaskWorktree", raw); err != nil {
				continue
			}
			return nil
		}
		phase, ok := phaseByID(st.Workflows[t.Workflow], t.Phase)
		if !ok || managedAdmission(st, st.Accounts[owner], t, phase) != nil {
			continue
		}
		_, err = s.openManagedLocal(ctx, OpenLocalSessionInput{ProjectID: t.ProjectID, TicketID: t.ID, ExpectedVersion: t.Version, Harness: "codex", RequestID: fmt.Sprintf("native-%d-%d-%d", t.ID, t.StageVisit, t.Attempts)})
		return err
	}
	return nil
}

func (s *Service) openManagedLocal(ctx context.Context, in OpenLocalSessionInput) (any, error) {
	s.localMu.Lock()
	defer s.localMu.Unlock()
	return s.openLocal(ctx, "", "startTicketSession", in)
}

func (s *Service) reconcileManaged(ctx context.Context, row model.LocalSession) error {
	if row.State == "preparing" {
		st, err := s.db.Read(ctx)
		if err != nil {
			return err
		}
		if st.Settings.ExecutionMode != "local" {
			return nil
		}
		_, err = s.openManagedLocal(ctx, OpenLocalSessionInput{ProjectID: row.ProjectID, TicketID: row.TicketID, ExpectedVersion: row.Managed.RequestVersion, Harness: row.Harness, RequestID: row.Managed.RequestID})
		return err
	}
	if row.Managed.Result == nil && row.Managed.Error == "" && s.localClient != nil {
		out, err := s.localClient.Call(ctx, daemonRequest(row, "nativeTurn"))
		if err != nil {
			return nil
		}
		if activeLocal(row) && out.NativeFailure == nil && (out.Turn == nil || out.Turn.State == "inProgress") {
			return nil
		}
		result, resultErr := managedOutcome(row.Managed.Role, out.Turn)
		if out.NativeFailure != nil {
			resultErr = failure(409, "native_protocol_failure", "Native protocol failed")
		}
		err = s.db.Update(ctx, func(st *model.State) (bool, error) {
			current := st.LocalSessions[row.ID]
			if current.Managed == nil || current.Managed.Applied || current.Managed.Result != nil || current.Managed.Error != "" {
				return false, nil
			}
			if resultErr != nil {
				current.Managed.Error = "Native phase evidence is invalid or unavailable; inspect the retained terminal and explicitly resume"
			} else {
				current.Managed.Result = result
				current.Managed.TurnID = out.Turn.TurnID
			}
			st.LocalSessions[row.ID] = current
			row = current
			return true, nil
		})
		if err != nil {
			return err
		}
	}
	if activeLocal(row) {
		if row.Managed.Result != nil || row.Managed.Error != "" {
			_, _ = s.localClient.Call(ctx, daemonRequest(row, "stop"))
		}
		return nil
	}
	// Snapshot and verify outside a transaction, then compare the exact frozen basis.
	var observed *model.GitCandidate
	if row.Managed.Role == "Review" && row.Managed.Result != nil {
		st, err := s.db.Read(ctx)
		if err != nil {
			return err
		}
		p := st.Projects[row.ProjectID]
		if p.Repository != nil {
			c, err := gitwork.Snapshot(ctx, gitwork.Binding{Repository: p.Repository.Repository, StorageRoot: p.Repository.StorageRoot}, ownedWorktree(row.Worktree))
			if err == nil {
				candidate := modelCandidate(c)
				observed = &candidate
			}
		}
	}
	if row.Managed.Role == "Doing" && row.Managed.Result != nil && row.Managed.Result.Outcome == "success" && row.Managed.Error == "" {
		st, err := s.db.Read(ctx)
		if err != nil {
			return err
		}
		t := st.Tickets[row.TicketID]
		if t.Delivery != nil && t.Delivery.Operation != nil && t.Delivery.Operation.RequestID == "native-delivery-"+row.ID && t.Delivery.Operation.State == "complete" {
			return s.finishManaged(ctx, row, false)
		}
		if t.Workflow == row.Managed.Workflow && t.Phase == row.Managed.Phase && t.StageVisit == row.Managed.Visit && executionBasis(st, t) == row.Managed.Basis && !t.StopRequested && !t.ResumeHold {
			raw, _ := json.Marshal(GitTaskInput{ID: t.ID, ExpectedVersion: t.Version, RequestID: "native-delivery-" + row.ID, ReportedChecks: row.Managed.Result.Findings})
			if _, err = s.gitCall(ctx, "", "recordTaskDelivery", raw); err != nil {
				return s.failManaged(ctx, row, "Candidate capture failed; reconcile retained work before explicit resume")
			}
			return s.finishManaged(ctx, row, false)
		}
	}
	return s.db.Update(ctx, func(st *model.State) (bool, error) {
		current := st.LocalSessions[row.ID]
		if current.Managed == nil || current.Managed.Applied {
			return false, nil
		}
		t := st.Tickets[row.TicketID]
		m := current.Managed
		a, err := executionActor(ctx, st, "")
		if err != nil {
			return false, err
		}
		valid := require(st, a, t.ProjectID, "Contributor") == nil && t.Workflow == m.Workflow && t.Phase == m.Phase && t.StageVisit == m.Visit && executionBasis(st, t) == m.Basis && !t.StopRequested && !t.ResumeHold && !t.Deleted && !t.Archived && !busy(t)
		if p := st.Projects[t.ProjectID]; p.Repository == nil || p.Repository.Version != row.BindingVersion {
			valid = false
		}
		if !valid || m.Error != "" || m.Result == nil {
			if t.Status != "Done" && t.Status != "Cancelled" && !busy(t) {
				t.ResumeHold = true
				t.Attention = m.Error
				if t.Attention == "" {
					t.Attention = "Native phase authority or outcome changed; inspect retained work before explicit resume"
				}
				saveTicket(st, &t, a, "nativePhaseHeld")
			}
			m.Applied = true
			st.LocalSessions[row.ID] = current
			return true, nil
		}
		r := m.Result
		now := time.Now().UTC()
		if m.Role == "Review" && (observed == nil || t.Delivery == nil || t.Delivery.Candidate == nil || observed.Fingerprint != m.Candidate || t.Delivery.Candidate.Fingerprint != m.Candidate) {
			t.ResumeHold = true
			t.Attention = "Review candidate or check evidence changed; obtain a fresh review"
		} else {
			if r.Outcome == "question" {
				t.Question = &model.Question{ID: randomID(), Text: r.Question, OriginPhase: t.Phase, OriginAttempt: m.Attempt, Workflow: t.Workflow, Basis: m.Basis, AskedAt: now, OriginWaitingSince: t.PhaseEnteredAt}
				t.Attention = "Question requires an answer"
			}
			if r.Outcome == "success" && m.Role == "Refine" {
				t.Refinement = &model.Refinement{ID: randomID(), Text: r.Proposal, Criteria: r.Criteria, Basis: m.Basis}
				t.Attention = "Scope acceptance required"
			}
			if m.Role == "Review" && r.Outcome != "question" {
				t.AgentReview = &model.AgentReview{ID: randomID(), Outcome: r.Outcome, Findings: r.Findings, Candidate: observed.Commit, Basis: observed.Fingerprint, At: now}
			}
			if _, err := applyRoute(&t, st.Workflows[t.Workflow], r.Outcome, nil); err != nil {
				t.ResumeHold = true
				t.Attention = "Native outcome has no unambiguous configured route"
			}
			if t.Status == "Approval" {
				t.Attention = "Human approval required for the reviewed candidate"
			}
			if t.Status == "Clarify" {
				t.Attention = "Verification requests human clarification: " + r.Findings
			}
			t.StageEvents = append(t.StageEvents, model.StageEvent{AttemptID: m.Attempt, Phase: m.Phase, Outcome: r.Outcome, Actor: "agent:" + row.ID, At: now, Continues: m.Continues})
		}
		t.Active, t.Claim, t.Execution = false, "", ""
		saveTicket(st, &t, model.Account{ID: "agent:" + row.ID}, "nativePhaseOutcome")
		reconcile(st, t.ParentID, a)
		m.Applied = true
		st.LocalSessions[row.ID] = current
		return true, nil
	})
}

func (s *Service) finishManaged(ctx context.Context, row model.LocalSession, hold bool) error {
	return s.db.Update(ctx, func(st *model.State) (bool, error) {
		current := st.LocalSessions[row.ID]
		if current.Managed == nil || current.Managed.Applied {
			return false, nil
		}
		current.Managed.Applied = true
		st.LocalSessions[row.ID] = current
		if !hold {
			t := st.Tickets[row.TicketID]
			t.StageEvents = append(t.StageEvents, model.StageEvent{AttemptID: row.Managed.Attempt, Phase: row.Managed.Phase, Outcome: "success", Actor: "agent:" + row.ID, At: time.Now().UTC(), Continues: row.Managed.Continues})
			saveTicket(st, &t, model.Account{ID: "agent:" + row.ID}, "nativeImplementationDelivered")
		}
		return true, nil
	})
}
func (s *Service) failManaged(ctx context.Context, row model.LocalSession, message string) error {
	if err := s.db.Update(ctx, func(st *model.State) (bool, error) {
		t := st.Tickets[row.TicketID]
		t.ResumeHold = true
		t.Attention = message
		saveTicket(st, &t, model.Account{ID: "native-supervisor"}, "nativePhaseHeld")
		return true, nil
	}); err != nil {
		return err
	}
	return s.finishManaged(ctx, row, true)
}
