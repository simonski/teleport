package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"teleport/internal/gitwork"
	"teleport/internal/model"
)

type RepositoryInput struct {
	ProjectID       int64  `json:"projectId"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Repository      string `json:"repository"`
	StorageRoot     string `json:"storageRoot"`
}
type GitTaskInput struct {
	ID              int64  `json:"id"`
	ExpectedVersion int64  `json:"expectedVersion"`
	RequestID       string `json:"requestId"`
	ReportedChecks  string `json:"reportedChecks,omitempty"`
}

func ownedWorktree(w model.Worktree) gitwork.Worktree {
	return gitwork.Worktree{Owner: w.Owner, TicketID: w.TicketID, Path: w.Path, Branch: w.Branch, Base: w.Base, ConsoleID: w.ConsoleID}
}
func modelWorktree(w gitwork.Worktree) model.Worktree {
	return model.Worktree{Owner: w.Owner, TicketID: w.TicketID, Path: w.Path, Branch: w.Branch, Base: w.Base, ConsoleID: w.ConsoleID}
}
func gitCandidate(c model.GitCandidate) gitwork.Candidate {
	return gitwork.Candidate{Base: c.Base, Commit: c.Commit, Diff: c.Diff, ChecksPassed: c.ChecksPassed, CheckOutput: c.CheckOutput, Fingerprint: c.Fingerprint}
}
func modelCandidate(c gitwork.Candidate) model.GitCandidate {
	return model.GitCandidate{Base: c.Base, Commit: c.Commit, Diff: c.Diff, ChecksPassed: c.ChecksPassed, CheckOutput: c.CheckOutput, Fingerprint: c.Fingerprint}
}

// gitCall reserves short SQL intents, performs Git outside write transactions, then reconciles.
func (s *Service) gitCall(ctx context.Context, token, operation string, raw json.RawMessage) (any, error) {
	s.gitMu.Lock()
	defer s.gitMu.Unlock()
	st, readErr := s.db.Read(ctx)
	if readErr != nil {
		return nil, readErr
	}
	if a, err := actor(st, token); err != nil {
		return nil, err
	} else if a.ServiceSessionID != "" {
		return nil, failure(403, "service_scope", "Managed context credentials cannot deliver or approve code")
	}
	if a, err := actor(st, token); err != nil {
		return nil, err
	} else if err = checkScriptDispatch(ctx, st, &a, operation, raw); err != nil {
		return nil, err
	}
	if operation == "configureLocalRepository" {
		return s.configureRepository(ctx, token, raw)
	}
	if operation == "approveTicket" {
		return s.approveGit(ctx, token, raw)
	}
	if operation == "reconcileGitOperation" {
		return s.reconcileGit(ctx, token, raw)
	}
	var in GitTaskInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	var reserved model.Ticket
	var binding model.RepositoryBinding
	var replay []byte
	err := s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, err := actor(st, token)
		if err != nil {
			return false, err
		}
		collections(st)
		t, err := ticketFor(st, a, in.ID, "Contributor")
		if err != nil {
			return false, err
		}
		if len(st.Accounts) > 1 {
			return false, failure(409, "isolation_required", "Real execution-host operations remain single-user until project isolation is verified")
		}
		if st.Settings.Agents || st.Settings.ExecutionMode == "demo" {
			return false, failure(409, "demo_active", "Disable demo execution before preparing real delivery")
		}
		p := st.Projects[t.ProjectID]
		if p.Simulation {
			return false, failure(409, "simulation_only", "Simulation projects cannot prepare real delivery")
		}
		if p.Repository == nil {
			return false, failure(409, "binding_required", "An administrator must grant this project's exact local repository and storage")
		}
		binding = *p.Repository
		if err := otherGitPending(st, t); err != nil {
			return false, err
		}
		key, fingerprint, previous, err := receipt(st, a, operation, in.RequestID, in)
		_ = key
		if err != nil {
			return false, err
		}
		if previous != nil {
			replay = previous
			return false, nil
		}
		if t.Delivery != nil && t.Delivery.Operation != nil && t.Delivery.Operation.State != "complete" {
			op := t.Delivery.Operation
			if op.Kind != operation || op.RequestID != in.RequestID || op.Fingerprint != fingerprint || op.Actor != a.ID {
				return false, failure(409, "git_pending", "Reconcile the exact pending Git operation before replacement")
			}
			if t.Delivery.BindingVersion != binding.Version {
				return false, failure(409, "binding_changed", "Pending Git operation belongs to a different binding")
			}
			if !op.CommandExited {
				return false, failure(409, "ownership_unknown", "Prior Git command exit is unverified; replacement execution is blocked")
			}
			if t.Status == "Cancelled" || t.StopRequested || !acceptedScope(st, t) {
				return false, failure(409, "git_reconciliation_required", "Use read-only Git reconciliation; cancelled or changed work cannot execute again")
			}
			op.ControllerID = s.controllerID
			op.CommandExited = false
			op.State = "reserved"
			saveTicket(st, &t, a, "gitRetryReserved")
			reserved = t
			return true, nil
		}
		if err = expected(t, in.ExpectedVersion); err != nil {
			return false, err
		}
		if !t.Agent || t.Kind != "task" || t.Archived || t.Deleted || t.Status == "Done" || t.Status == "Cancelled" || busy(t) || hasContinuation(t) || t.ResumeHold || !acceptedScope(st, t) || !dependenciesDone(st, t) {
			return false, failure(409, "delivery_blocked", "Real delivery requires visible stopped agent work with current accepted scope and completed prerequisites")
		}
		if operation == "prepareTaskWorktree" {
			if err := requireReadyReview(st, t); err != nil {
				return false, err
			}
			if t.Delivery != nil && t.Delivery.Worktree.Path != "" {
				reserved = t
				return false, nil
			}
			if t.Status != "Ready" {
				return false, failure(409, "readiness_required", "Prepare retained work only after accepted scope reaches Ready")
			}
			if st.Settings.Handbrake || p.Handbrake || p.LaneLocks[st.Workflows[t.Workflow].ID+"#*"] {
				return false, failure(409, "handbrake", "Release applicable admissions handbrakes")
			}
			if p.CapacityConfigured && p.DoingWorkers == 0 {
				return false, failure(409, "capacity_disabled", "Doing capacity is zero")
			}
			limit := t.AttemptLimit
			if limit == 0 {
				limit = 20
			}
			if t.Attempts >= limit {
				return false, failure(409, "budget_exhausted", "Extend the finite attempt allowance before implementation admission")
			}
			route, err := ResolveOutcome(st.Workflows[t.Workflow], t.Phase, "start", nil)
			if err != nil {
				return false, err
			}
			target, ok := phaseByID(st.Workflows[t.Workflow], route.Target)
			if !ok || target.Role != "Doing" || p.LaneLocks[st.Workflows[t.Workflow].ID+"#"+target.ID] {
				return false, failure(409, "phase_gate", "Implementation destination is missing or locked")
			}
			if err := phaseCapacity(st, t, target); err != nil {
				return false, err
			}
			if err = enterPhase(&t, st.Workflows[t.Workflow], target.ID); err != nil {
				return false, err
			}
			t.Attempts++
			t.Delivery = &model.Delivery{BindingVersion: binding.Version, ScopeBasis: executionBasis(st, t), SourcePhase: t.Phase, Source: "external", Worktree: model.Worktree{Owner: randomID()[:24], TicketID: t.ID}}
		} else {
			if t.Delivery == nil || t.Delivery.Worktree.Path == "" || !contains([]string{"Doing", "Review", "Approval"}, t.Status) || t.Delivery.BindingVersion != binding.Version || t.Delivery.ScopeBasis != executionBasis(st, t) {
				return false, failure(409, "worktree_required", "Use the matching retained worktree and current accepted basis")
			}
			if len(in.ReportedChecks) > 20000 {
				return false, failure(422, "invalid_report", "External check reports allow at most 20,000 bytes")
			}
		}
		t.Delivery.Operation = &model.GitOperation{ID: randomID(), Kind: operation, RequestID: in.RequestID, Fingerprint: fingerprint, Actor: a.ID, State: "reserved", ControllerID: s.controllerID}
		t.Execution = "git:" + t.Delivery.Operation.ID
		t.Attention = "Git operation pending reconciliation"
		saveTicket(st, &t, a, operation+"Reserved")
		reserved = t
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if replay != nil {
		var saved model.Ticket
		json.Unmarshal(replay, &saved)
		return saved, nil
	}
	if reserved.Delivery.Operation == nil || reserved.Delivery.Operation.State == "complete" {
		return reserved, nil
	}
	controller := gitwork.Binding{Repository: binding.Repository, StorageRoot: binding.StorageRoot}
	runCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	work := ownedWorktree(reserved.Delivery.Worktree)
	var candidate gitwork.Candidate
	if operation == "prepareTaskWorktree" {
		work, err = gitwork.Prepare(runCtx, controller, work.Owner, in.ID)
	} else {
		candidate, err = gitwork.Snapshot(runCtx, controller, work)
	}
	finalCtx, finalCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finalCancel()
	gitErr := err
	if err = s.recordGitExit(finalCtx, reserved.ID, reserved.Delivery.Operation.ID); err != nil {
		return nil, err
	}
	err = s.db.Update(finalCtx, func(st *model.State) (bool, error) {
		a, err := actor(st, token)
		if err != nil {
			return false, err
		}
		t, err := ticketFor(st, a, in.ID, "Contributor")
		if err != nil {
			return false, err
		}
		p := st.Projects[t.ProjectID]
		if t.Delivery == nil || t.Delivery.Operation == nil || t.Delivery.Operation.ID != reserved.Delivery.Operation.ID || t.Execution != "git:"+reserved.Delivery.Operation.ID || p.Repository == nil || p.Repository.Version != binding.Version || t.Status == "Cancelled" || t.StopRequested {
			return false, failure(409, "git_reconciliation_required", "Git facts changed or cancellation won; preserve retained work and reconcile this exact operation")
		}
		if gitErr != nil {
			t.Delivery.Operation.State = "needs_reconciliation"
			t.Attention = "Git operation needs reconciliation; retained work is preserved"
			saveTicket(st, &t, a, "gitOperationUncertain")
			return true, nil
		}
		if !acceptedScope(st, t) || t.Delivery.ScopeBasis != executionBasis(st, t) {
			return false, failure(409, "scope_stale", "Delivery scope changed during Git processing")
		}
		t.Delivery.Worktree = modelWorktree(work)
		if operation == "recordTaskDelivery" {
			captured := modelCandidate(candidate)
			t.Delivery.Candidate = &captured
			t.Delivery.ReportedChecks = in.ReportedChecks
			t.AgentReview = nil
			route, err := ResolveOutcome(st.Workflows[t.Workflow], t.Delivery.SourcePhase, "success", nil)
			if err != nil {
				return false, err
			}
			destination, ok := phaseByID(st.Workflows[t.Workflow], route.Target)
			if !ok || !contains([]string{"Review", "Approval"}, destination.Role) {
				return false, failure(409, "delivery_route", "The declared implementation handoff needs Review or human Approval")
			}
			if err = enterPhase(&t, st.Workflows[t.Workflow], route.Target); err != nil {
				return false, err
			}
			t.Attention = "Real candidate awaits configured review and human approval"
		} else {
			t.Attention = "Owned worktree prepared; no coding process was launched"
		}
		t.Execution = ""
		t.Delivery.Operation.State = "complete"
		saveTicket(st, &t, a, operation)
		remember(st, a.ID+":"+operation+":"+in.RequestID, t.Delivery.Operation.Fingerprint, t)
		reserved = t
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if gitErr != nil {
		slog.Error("Git controller operation requires reconciliation", "operation", operation, "ticket", in.ID, "error", gitErr)
		return nil, failure(409, "git_reconciliation_required", "Git processing failed; retry the exact request to reconcile retained work")
	}
	return reserved, nil
}

func (s *Service) configureRepository(ctx context.Context, token string, raw json.RawMessage) (any, error) {
	var in RepositoryInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	snapshot, err := s.db.Read(ctx)
	if err != nil {
		return nil, err
	}
	a, err := actor(snapshot, token)
	if err != nil {
		return nil, err
	}
	if !a.Administrator {
		return nil, failure(403, "forbidden", "Only an installation administrator may grant local filesystem bindings")
	}
	if _, ok := snapshot.Projects[in.ProjectID]; !ok {
		return nil, failure(404, "not_found", "Project not found")
	}
	if snapshot.Projects[in.ProjectID].Simulation {
		return nil, failure(409, "simulation_only", "Simulation projects cannot grant repositories or run real harnesses")
	}
	binding, err := gitwork.Inspect(ctx, in.Repository, in.StorageRoot)
	if err != nil {
		return nil, failure(422, "invalid_repository", "Choose the exact committed main checkout and separate absolute storage root")
	}
	var result model.Project
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, err := actor(st, token)
		if err != nil {
			return false, err
		}
		if !a.Administrator {
			return false, failure(403, "forbidden", "Administrator rights changed")
		}
		p, ok := st.Projects[in.ProjectID]
		if !ok {
			return false, failure(404, "not_found", "Project not found")
		}
		if p.Simulation {
			return false, failure(409, "simulation_only", "Simulation projects cannot grant repositories or run real harnesses")
		}
		if p.Version != in.ExpectedVersion {
			return false, failure(409, "version_conflict", "Project changed")
		}
		if p.Repository != nil && p.Repository.Repository == binding.Repository && p.Repository.StorageRoot == binding.StorageRoot {
			result = p
			return false, nil
		}
		for _, t := range st.Tickets {
			if t.ProjectID == p.ID && (busy(t) || t.Delivery != nil) {
				return false, failure(409, "migration_required", "Existing execution/artifacts require a separate stopped host migration")
			}
		}
		for _, session := range st.LocalSessions {
			if session.ProjectID == p.ID && (activeLocal(session) || session.Worktree.Path != "") {
				return false, failure(409, "migration_required", "Retained harness worktrees and unresolved sessions require explicit host migration")
			}
		}
		version := int64(1)
		if p.Repository != nil {
			version = p.Repository.Version + 1
		}
		p.Repository = &model.RepositoryBinding{Repository: binding.Repository, StorageRoot: binding.StorageRoot, Version: version}
		p.Version++
		st.Projects[p.ID] = p
		reconcileReadyAttention(st, a)
		result = p
		return true, nil
	})
	return result, err
}

func (s *Service) approveGit(ctx context.Context, token string, raw json.RawMessage) (any, error) {
	var in ApprovalInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	var reserved model.Ticket
	var binding model.RepositoryBinding
	completedRetry := false
	err := s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, err := actor(st, token)
		if err != nil {
			return false, err
		}
		t, err := ticketFor(st, a, in.ID, "Maintainer")
		if err != nil {
			return false, err
		}
		if t.Delivery == nil || t.Delivery.Candidate == nil {
			return false, failure(409, "delivery_evidence_required", "Demo evidence cannot approve real code; a verified Git candidate is required")
		}
		if len(st.Accounts) > 1 {
			return false, failure(409, "isolation_required", "Real host operations require verified project isolation before multi-user activation")
		}
		if op := t.Delivery.Operation; t.Status == "Done" && op != nil && op.Kind == "approveTicket" && op.State == "complete" && op.Actor == a.ID && op.BasisVersion == in.ExpectedVersion && in.Confirm {
			reserved = t
			completedRetry = true
			return false, nil
		}
		if t.Status != "Approval" || t.Kind != "task" || !in.Confirm || !acceptedScope(st, t) || !dependenciesDone(st, t) || t.Archived || t.Deleted || t.ResumeHold || t.StopRequested {
			return false, failure(409, "approval_blocked", "Confirm the current unblocked scoped Approval candidate")
		}
		d := t.Delivery
		if err := otherGitPending(st, t); err != nil {
			return false, err
		}
		p := st.Projects[t.ProjectID]
		if p.Repository == nil || d.BindingVersion != p.Repository.Version || d.ScopeBasis != executionBasis(st, t) || !d.Candidate.ChecksPassed {
			return false, failure(409, "candidate_stale", "Host, scope or required checks changed")
		}
		for _, phase := range st.Workflows[t.Workflow].Phases {
			if phase.Role == "Review" {
				review := t.AgentReview
				if review == nil || review.Simulated || review.Outcome != "success" || review.Candidate != d.Candidate.Commit || review.Basis != d.Candidate.Fingerprint {
					return false, failure(409, "review_required", "Configured real agent review must match this exact candidate and check basis")
				}
			}
		}
		if busy(t) {
			if d.Operation == nil || d.Operation.Kind != "approveTicket" || d.Operation.Actor != a.ID {
				return false, failure(409, "git_pending", "Another operation requires reconciliation")
			}
			if !d.Operation.CommandExited {
				return false, failure(409, "ownership_unknown", "Prior controller exit is unverified; no replacement command may start")
			}
			d.Operation.ControllerID = s.controllerID
			d.Operation.CommandExited = false
		} else {
			if err = expected(t, in.ExpectedVersion); err != nil {
				return false, err
			}
			d.Operation = &model.GitOperation{ID: randomID(), Kind: "approveTicket", Actor: a.ID, State: "reserved", ControllerID: s.controllerID, BasisVersion: in.ExpectedVersion}
			t.Execution = "git:" + d.Operation.ID
			t.Attention = "Human approval integration pending"
			saveTicket(st, &t, a, "approvalReserved")
		}
		reserved = t
		binding = *p.Repository
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if completedRetry {
		return reserved, nil
	}
	controller := gitwork.Binding{Repository: binding.Repository, StorageRoot: binding.StorageRoot}
	runCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	gitErr := gitwork.Integrate(runCtx, controller, ownedWorktree(reserved.Delivery.Worktree), gitCandidate(*reserved.Delivery.Candidate))
	finalCtx, finalCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finalCancel()
	if err = s.recordGitExit(finalCtx, reserved.ID, reserved.Delivery.Operation.ID); err != nil {
		return nil, err
	}
	err = s.db.Update(finalCtx, func(st *model.State) (bool, error) {
		a, err := actor(st, token)
		if err != nil {
			return false, err
		}
		t, err := ticketFor(st, a, in.ID, "Maintainer")
		if err != nil {
			return false, err
		}
		if t.Delivery == nil || t.Delivery.Operation == nil || t.Delivery.Operation.ID != reserved.Delivery.Operation.ID || t.Status != "Approval" || t.StopRequested || !acceptedScope(st, t) || !dependenciesDone(st, t) {
			return false, failure(409, "git_reconciliation_required", "Integration needs reconciliation against the current authority and task basis")
		}
		if gitErr != nil {
			t.Delivery.Operation.State = "needs_reconciliation"
			t.Attention = "Integration blocked; retained branch and checks are preserved"
			saveTicket(st, &t, a, "integrationBlocked")
			return true, nil
		}
		if p := st.Projects[t.ProjectID]; p.Repository == nil || p.Repository.Version != binding.Version {
			return false, failure(409, "binding_changed", "Binding changed during integration")
		}
		if _, err = applyRoute(&t, st.Workflows[t.Workflow], "approve", nil); err != nil {
			return false, err
		}
		if t.Status != "Done" {
			return false, failure(409, "approval_route", "Approval must complete the exact declared Done phase")
		}
		t.Progress = 100
		t.Execution = ""
		t.Attention = ""
		t.Delivery.Operation.State = "complete"
		t.Agreements = append(t.Agreements, completionBasis(st, t, a, t.Refinement.Criteria, in.ExpectedVersion))
		saveTicket(st, &t, a, "approveTicket")
		reconcile(st, t.ParentID, a)
		reserved = t
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if gitErr != nil {
		slog.Error("Git integration blocked", "ticket", in.ID, "error", gitErr)
		return nil, failure(409, "integration_blocked", "Main or candidate changed; preserve the branch and obtain fresh verified delivery/review")
	}
	return reserved, nil
}

func otherGitPending(st *model.State, t model.Ticket) error {
	for _, other := range st.Tickets {
		if other.ID != t.ID && other.ProjectID == t.ProjectID && other.Delivery != nil && other.Delivery.Operation != nil && other.Delivery.Operation.State != "complete" {
			return failure(409, "project_git_pending", "Reconcile the project's previous Git operation before another may start")
		}
	}
	return nil
}
func (s *Service) recordGitExit(ctx context.Context, ticketID int64, operationID string) error {
	return s.db.Update(ctx, func(st *model.State) (bool, error) {
		t, ok := st.Tickets[ticketID]
		if !ok || t.Delivery == nil || t.Delivery.Operation == nil || t.Delivery.Operation.ID != operationID || t.Delivery.Operation.ControllerID != s.controllerID {
			return false, failure(409, "ownership_changed", "Controller operation identity changed")
		}
		t.Delivery.Operation.CommandExited = true
		saveTicket(st, &t, model.Account{ID: "controller:local"}, "gitCommandExitConfirmed")
		return true, nil
	})
}

func (s *Service) reconcileGit(ctx context.Context, token string, raw json.RawMessage) (any, error) {
	var in VersionInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	snapshot, err := s.db.Read(ctx)
	if err != nil {
		return nil, err
	}
	a, err := actor(snapshot, token)
	if err != nil {
		return nil, err
	}
	t, err := ticketFor(snapshot, a, in.ID, "Maintainer")
	if err != nil {
		return nil, err
	}
	if err = expected(t, in.ExpectedVersion); err != nil {
		return nil, err
	}
	if t.Delivery == nil || t.Delivery.Operation == nil || t.Delivery.Operation.State == "complete" {
		return t, nil
	}
	if !t.Delivery.Operation.CommandExited {
		return nil, failure(409, "ownership_unknown", "Controller exit has no proof; live-host recovery is required before any replacement")
	}
	p := snapshot.Projects[t.ProjectID]
	if p.Repository == nil || p.Repository.Version != t.Delivery.BindingVersion {
		return nil, failure(409, "binding_changed", "Use verified host migration for changed repository bindings")
	}
	binding := gitwork.Binding{Repository: p.Repository.Repository, StorageRoot: p.Repository.StorageRoot}
	work, err := gitwork.Recover(ctx, binding, t.Delivery.Worktree.Owner, t.ID)
	if err != nil {
		return nil, failure(409, "git_reconciliation_required", "Retained Git facts remain incomplete; no files were discarded")
	}
	operationID := t.Delivery.Operation.ID
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, err := actor(st, token)
		if err != nil {
			return false, err
		}
		current, err := ticketFor(st, a, t.ID, "Maintainer")
		if err != nil {
			return false, err
		}
		if err = expected(current, in.ExpectedVersion); err != nil {
			return false, err
		}
		if project := st.Projects[current.ProjectID]; project.Repository == nil || project.Repository.Version != t.Delivery.BindingVersion {
			return false, failure(409, "binding_changed", "Binding changed during reconciliation")
		}
		if current.Delivery == nil || current.Delivery.Operation == nil || current.Delivery.Operation.ID != operationID || !current.Delivery.Operation.CommandExited {
			return false, failure(409, "ownership_changed", "Reconciliation basis changed")
		}
		current.Delivery.Worktree = modelWorktree(work)
		current.Delivery.Operation.State = "complete"
		current.Execution = ""
		current.Attention = "Git facts reconciled; new delivery/approval still requires explicit current validation"
		saveTicket(st, &current, a, "reconcileGitOperation")
		t = current
		return true, nil
	})
	return t, err
}
