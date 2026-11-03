package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"teleport/internal/gitwork"
	"teleport/internal/localruntime"
	"teleport/internal/model"
)

type LocalSessionInfo struct {
	Activity  string     `json:"activity,omitempty"`
	ID        string     `json:"id"`
	Creator   string     `json:"creator"`
	ProjectID int64      `json:"projectId"`
	TicketID  int64      `json:"ticketId"`
	Harness   string     `json:"harness"`
	State     string     `json:"state"`
	Attention string     `json:"attention,omitempty"`
	ErrorCode string     `json:"errorCode,omitempty"`
	Directory string     `json:"directory"`
	CreatedAt time.Time  `json:"createdAt"`
	ExitedAt  *time.Time `json:"exitedAt,omitempty"`
	ExitCode  *int       `json:"exitCode,omitempty"`
}
type LocalSessionPage struct {
	Items      []LocalSessionInfo `json:"items"`
	NextCursor string             `json:"nextCursor"`
}
type LocalHarnesses struct {
	Mode      string          `json:"mode"`
	Available []HarnessChoice `json:"available"`
}
type OpenLocalSessionInput struct {
	ProjectID       int64  `json:"projectId"`
	TicketID        int64  `json:"ticketId,omitempty"`
	ExpectedVersion int64  `json:"expectedVersion,omitempty"`
	Harness         string `json:"harness"`
	RequestID       string `json:"requestId"`
}
type SessionIDInput struct {
	ID string `json:"id"`
}
type AttachSessionInput struct {
	ID     string `json:"id"`
	Cursor int64  `json:"cursor,omitempty"`
}
type SessionView struct {
	Session LocalSessionInfo `json:"session"`
	Data    []byte           `json:"data,omitempty"`
	Cursor  int64            `json:"cursor"`
	Next    int64            `json:"next"`
	Gap     bool             `json:"gap"`
	Cols    uint16           `json:"cols"`
	Rows    uint16           `json:"rows"`
}
type SessionInput struct {
	ID        string `json:"id"`
	RequestID string `json:"requestId"`
	Data      []byte `json:"data"`
}
type SessionResize struct {
	ID   string `json:"id"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}
type SessionStop struct {
	ID      string `json:"id"`
	Confirm bool   `json:"confirm"`
}
type SessionIntervention struct {
	SessionID string    `json:"sessionId"`
	RequestID string    `json:"requestId"`
	Actor     string    `json:"actor"`
	Sequence  int64     `json:"sequence"`
	State     string    `json:"state"`
	Written   int       `json:"written"`
	At        time.Time `json:"at"`
}
type InterventionList struct {
	ID     string `json:"id"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type InterventionPage struct {
	Items      []SessionIntervention `json:"items"`
	NextCursor string                `json:"nextCursor"`
}

func (s *Service) ConfigureLocalRuntime(root, executable string, fixture bool, endpoint ...string) {
	s.localClient = localruntime.New(root, executable)
	s.fixtureHarness = fixture
	if len(endpoint) > 0 {
		s.localEndpoint = endpoint[0]
	}
}
func localOperation(op string) bool {
	return contains([]string{"getLocalHarnesses", "listSessions", "openConsoleSession", "startTicketSession", "attachSession", "sendSessionInput", "resizeSession", "stopSession", "shutdownLocalRuntime", "listSessionInterventions", "getSessionRequests", "getSessionTurn", "respondToAgent"}, op)
}
func localInfo(row model.LocalSession) LocalSessionInfo {
	return LocalSessionInfo{ID: row.ID, Creator: row.Creator, ProjectID: row.ProjectID, TicketID: row.TicketID, Harness: row.Harness, State: row.State, Attention: row.Attention, ErrorCode: row.ErrorCode, Directory: row.Worktree.Path, CreatedAt: row.CreatedAt, ExitedAt: row.ExitedAt, ExitCode: row.ExitCode}
}
func localAllowed(st *model.State, a model.Account, row model.LocalSession) error {
	if row.Creator != a.ID {
		return failure(403, "private_session", "Only the initiating account may access this terminal")
	}
	if err := require(st, a, row.ProjectID, "Contributor"); err != nil {
		return err
	}
	return nil
}
func activeLocal(row model.LocalSession) bool { return row.State != "exited" && row.State != "failed" }
func (s *Service) localSnapshot(ctx context.Context, token, id string) (*model.State, model.Account, model.LocalSession, error) {
	st, err := s.db.Read(ctx)
	if err != nil {
		return nil, model.Account{}, model.LocalSession{}, err
	}
	a, err := executionActor(ctx, st, token)
	if err != nil {
		return nil, a, model.LocalSession{}, err
	}
	if a.ServiceSessionID != "" {
		return nil, a, model.LocalSession{}, failure(403, "service_scope", "Managed context credentials cannot control terminals")
	}
	if id == "" {
		return st, a, model.LocalSession{}, nil
	}
	row, ok := st.LocalSessions[id]
	if !ok {
		return nil, a, row, failure(404, "not_found", "Session not found")
	}
	if err = localAllowed(st, a, row); err != nil {
		return nil, a, row, err
	}
	return st, a, row, nil
}
func daemonRequest(row model.LocalSession, op string) localruntime.Request {
	return localruntime.Request{Operation: op, ID: row.ID, LaunchID: row.LaunchID, Generation: row.Generation}
}
func (s *Service) localCall(ctx context.Context, token, operation string, raw json.RawMessage) (any, error) {
	s.localMu.Lock()
	defer s.localMu.Unlock()
	st, a, _, err := s.localSnapshot(ctx, token, "")
	if err != nil {
		return nil, err
	}
	if operation == "getLocalHarnesses" {
		if err = parse(raw, &struct{}{}); err != nil {
			return nil, err
		}
		items := []HarnessChoice{}
		if _, available := nativeExecutable(ctx); available {
			items = append(items, HarnessChoice{ID: "codex", Title: "Codex · native local", Available: true})
		}
		if s.fixtureHarness && localruntime.Supported() {
			items = append(items, HarnessChoice{ID: "fixture", Title: "Terminal fixture · no model", Available: true})
		}
		return LocalHarnesses{Mode: "local · native permissions", Available: items}, nil
	}
	if operation == "listSessions" {
		var in ListInput
		if err = parse(raw, &in); err != nil {
			return nil, err
		}
		after, limit, err := pageBounds(in.Cursor, in.Limit)
		if err != nil {
			return nil, err
		}
		rows := []model.LocalSession{}
		for _, row := range st.LocalSessions {
			if row.Number > after && (in.ProjectID == 0 || row.ProjectID == in.ProjectID) && localAllowed(st, a, row) == nil {
				rows = append(rows, row)
			}
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Number < rows[j].Number })
		out := LocalSessionPage{Items: []LocalSessionInfo{}}
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		for _, row := range rows {
			if len(out.Items) == limit {
				out.NextCursor = cursorNumber(rows[limit-1].Number)
				break
			}
			info := localInfo(row)
			if activeLocal(row) && row.State == "running" && s.localClient != nil {
				info.Activity = "unknown"
				if row.Harness == "codex" {
					if observed, callErr := s.localClient.Call(probe, daemonRequest(row, "nativeTurn")); callErr == nil {
						info.Activity = "idle"
						if observed.Turn != nil {
							if observed.Turn.State == "inProgress" {
								info.Activity = "active"
							} else if observed.Turn.State == "unknown" {
								info.Activity = "unknown"
							}
						}
					}
				}
			}
			out.Items = append(out.Items, info)
		}
		current, currentActor, _, readErr := s.localSnapshot(ctx, token, "")
		if readErr != nil {
			return nil, readErr
		}
		filtered := []LocalSessionInfo{}
		for _, info := range out.Items {
			row, exists := current.LocalSessions[info.ID]
			if exists && row.LaunchID == st.LocalSessions[info.ID].LaunchID && row.Generation == st.LocalSessions[info.ID].Generation && localAllowed(current, currentActor, row) == nil {
				fresh := localInfo(row)
				if row.State == "running" && !row.StopRequested {
					fresh.Activity = info.Activity
				}
				filtered = append(filtered, fresh)
			}
		}
		out.Items = filtered
		return out, nil
	}
	if s.localClient == nil || !localruntime.Supported() {
		return nil, failure(409, "local_runtime_unavailable", "Local terminals require configured macOS/Linux runtime support")
	}
	if operation == "getSessionRequests" || operation == "getSessionTurn" || operation == "respondToAgent" {
		return s.nativeRequestCall(ctx, token, operation, raw)
	}
	if operation == "openConsoleSession" || operation == "startTicketSession" {
		var in OpenLocalSessionInput
		if err = parse(raw, &in); err != nil {
			return nil, err
		}
		return s.openLocal(ctx, token, operation, in)
	}
	if operation == "shutdownLocalRuntime" {
		var in struct {
			Confirm bool `json:"confirm"`
		}
		if err = parse(raw, &in); err != nil {
			return nil, err
		}
		if !a.Administrator || !in.Confirm {
			return nil, failure(403, "confirmation_required", "An administrator must explicitly confirm runtime shutdown")
		}
		if _, err = s.localClient.Call(ctx, localruntime.Request{Operation: "shutdown"}); err != nil {
			return nil, failure(409, "runtime_shutdown_blocked", "Runtime must be reachable with no active or unknown sessions")
		}
		return map[string]bool{"shutdown": true}, nil
	}
	var id string
	var attach AttachSessionInput
	var input SessionInput
	var resize SessionResize
	var stop SessionStop
	var audit InterventionList
	switch operation {
	case "attachSession":
		err = parse(raw, &attach)
		id = attach.ID
	case "sendSessionInput":
		err = parse(raw, &input)
		id = input.ID
	case "resizeSession":
		err = parse(raw, &resize)
		id = resize.ID
	case "stopSession":
		err = parse(raw, &stop)
		id = stop.ID
	case "listSessionInterventions":
		err = parse(raw, &audit)
		id = audit.ID
	}
	if err != nil {
		return nil, err
	}
	st, a, row, err := s.localSnapshot(ctx, token, id)
	if err != nil {
		return nil, err
	}
	if operation == "listSessionInterventions" {
		after, limit, err := pageBounds(audit.Cursor, audit.Limit)
		if err != nil {
			return nil, err
		}
		items := []model.LocalInput{}
		for _, item := range st.LocalInputs {
			if item.SessionID == row.ID && item.Sequence > after {
				items = append(items, item)
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Sequence < items[j].Sequence })
		out := InterventionPage{Items: []SessionIntervention{}}
		for _, item := range items {
			if len(out.Items) == limit {
				out.NextCursor = cursorNumber(items[limit-1].Sequence)
				break
			}
			out.Items = append(out.Items, intervention(item))
		}
		return out, nil
	}
	if operation == "attachSession" {
		request := daemonRequest(row, "read")
		request.Cursor = attach.Cursor
		out, err := s.localClient.Call(ctx, request)
		if err != nil {
			return nil, failure(503, "session_unavailable", "The existing terminal is unavailable; no replacement was started")
		}
		if _, _, _, err = s.localSnapshot(ctx, token, id); err != nil {
			return nil, err
		}
		return SessionView{Session: localInfo(row), Data: out.Data, Cursor: out.Cursor, Next: out.Next, Gap: out.Gap, Cols: out.Status.Cols, Rows: out.Status.Rows}, nil
	}
	if len(st.Accounts) > 1 {
		return nil, failure(409, "isolation_required", "Native-local input remains single-user until project isolation is verified")
	}
	if operation == "sendSessionInput" {
		return s.sendLocalInput(ctx, token, input)
	}
	if operation == "resizeSession" {
		request := daemonRequest(row, "resize")
		request.Cols, request.Rows = resize.Cols, resize.Rows
		if _, err = s.localClient.Call(ctx, request); err != nil {
			return nil, failure(409, "resize_unavailable", "Resize requires the same live owned terminal and a valid size")
		}
		return map[string]bool{"resized": true}, nil
	}
	if !stop.Confirm {
		return nil, failure(422, "confirmation_required", "Explicitly confirm stopping this harness")
	}
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, err := actor(st, token)
		if err != nil {
			return false, err
		}
		row := st.LocalSessions[id]
		if err = localAllowed(st, a, row); err != nil {
			return false, err
		}
		if !activeLocal(row) {
			return false, nil
		}
		row.State = "stopping"
		row.StopRequested = true
		st.LocalSessions[id] = row
		if row.TicketID != 0 {
			t := st.Tickets[row.TicketID]
			t.StopRequested, t.ResumeHold = true, true
			saveTicket(st, &t, a, "localStopRequested")
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	out, err := s.localClient.Call(ctx, daemonRequest(row, "stop"))
	if err != nil {
		return nil, failure(409, "owned_stop_pending", "Stop is recorded; owned exit is not yet confirmed")
	}
	if out.Status != nil {
		row.State = out.Status.State
	}
	return localInfo(row), nil
}
func cursorNumber(number int64) string { return cursor(number) }
func intervention(item model.LocalInput) SessionIntervention {
	return SessionIntervention{SessionID: item.SessionID, RequestID: item.RequestID, Actor: item.Actor, Sequence: item.Sequence, State: item.State, Written: item.Written, At: item.At}
}
func (s *Service) openLocal(ctx context.Context, token, operation string, in OpenLocalSessionInput) (any, error) {
	_, managed := ctx.Value(managedOwnerKey{}).(string)
	nativePath := ""
	if in.Harness == "codex" {
		var available bool
		nativePath, available = nativeExecutable(ctx)
		if !available {
			return nil, failure(409, "adapter_unavailable", "Native Codex requires the inspected 0.166.0 version on macOS/Linux")
		}
	} else if !s.fixtureHarness || in.Harness != "fixture" {
		return nil, failure(409, "adapter_unavailable", "Select an available supported harness")
	}
	if len(in.RequestID) < 1 || len(in.RequestID) > 100 || strings.ContainsRune(in.RequestID, 0) {
		return nil, failure(422, "invalid_request_id", "Use a stable request ID of 1–100 bytes")
	}
	if operation == "openConsoleSession" && in.TicketID != 0 || operation == "startTicketSession" && in.TicketID == 0 {
		return nil, failure(422, "invalid_session_scope", "Select either a free console or its exact ticket")
	}
	st, a, _, err := s.localSnapshot(ctx, token, "")
	if err != nil {
		return nil, err
	}
	if err = require(st, a, in.ProjectID, "Contributor"); err != nil {
		return nil, err
	}
	if len(st.Accounts) > 1 || st.Settings.Agents {
		return nil, failure(409, "local_mode_required", "Use single-user local mode with demo execution off")
	}
	p := st.Projects[in.ProjectID]
	if !projectAllowsHarness(p, in.Harness) {
		return nil, failure(403, "harness_not_permitted", "This harness is not permitted for the project")
	}
	if p.Simulation {
		return nil, failure(409, "simulation_only", "Simulation projects cannot run real harnesses")
	}
	if p.Repository == nil {
		return nil, failure(409, "binding_required", "Grant this project's local repository in Settings")
	}
	if err = s.localClient.Ensure(ctx); err != nil {
		return nil, failure(503, "runtime_unavailable", "The owned local daemon could not be reached or started")
	}
	ping, err := s.localClient.Call(ctx, localruntime.Request{Operation: "ping"})
	if err != nil {
		return nil, err
	}
	fingerprint := digest(fmt.Sprintf("%s:%d:%d:%d:%s", operation, in.ProjectID, in.TicketID, in.ExpectedVersion, in.Harness))
	var row model.LocalSession
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, err := executionActor(ctx, st, token)
		if err != nil {
			return false, err
		}
		if err = require(st, a, in.ProjectID, "Contributor"); err != nil {
			return false, err
		}
		if st.Projects[in.ProjectID].Simulation {
			return false, failure(409, "simulation_only", "Simulation projects cannot run real harnesses")
		}
		if st.LocalSessions == nil {
			st.LocalSessions = map[string]model.LocalSession{}
		}
		key := a.ID + ":" + operation + ":" + in.RequestID
		if receipt, ok := st.Receipts[key]; ok {
			if receipt.Fingerprint != fingerprint {
				return false, failure(409, "request_conflict", "Session request identity was reused with different fields")
			}
			var id string
			json.Unmarshal(receipt.Result, &id)
			row = st.LocalSessions[id]
			return false, localAllowed(st, a, row)
		}
		if len(st.Accounts) > 1 || st.Settings.Agents || st.Settings.Handbrake || st.Projects[in.ProjectID].Handbrake {
			return false, failure(409, "admission_blocked", "Local session admission is paused")
		}
		if !projectAllowsHarness(st.Projects[in.ProjectID], in.Harness) {
			return false, failure(403, "harness_not_permitted", "This harness is not permitted for the project")
		}
		if err := projectAgentCapacity(st, in.ProjectID); err != nil {
			return false, err
		}
		for _, other := range st.LocalSessions {
			if activeLocal(other) && (in.TicketID != 0 && other.TicketID == in.TicketID || in.TicketID == 0 && other.Creator == a.ID && other.TicketID == 0) {
				return false, failure(409, "session_exists", "Reconnect the existing running or unresolved harness")
			}
		}
		if st.NextLocalSessionID >= 1000 {
			return false, failure(409, "runtime_capacity_exhausted", "Retained session limit requires explicit maintenance")
		}
		st.NextLocalSessionID++
		row = model.LocalSession{ID: "H" + strconv.FormatInt(st.NextLocalSessionID, 10), Number: st.NextLocalSessionID, Creator: a.ID, ProjectID: in.ProjectID, TicketID: in.TicketID, Harness: in.Harness, LaunchID: randomID(), Generation: ping.Generation, Fingerprint: fingerprint, State: "preparing", BindingVersion: st.Projects[in.ProjectID].Repository.Version, CreatedAt: time.Now().UTC()}
		if in.TicketID != 0 {
			t, err := ticketFor(st, a, in.TicketID, "Contributor")
			if err != nil {
				return false, err
			}
			if err = expected(t, in.ExpectedVersion); err != nil {
				return false, err
			}
			if managed {
				phase, ok := phaseByID(st.Workflows[t.Workflow], t.Phase)
				if !ok {
					return false, failure(409, "phase_gate", "Pinned phase is unavailable")
				}
				if err := managedAdmission(st, a, t, phase); err != nil {
					return false, err
				}
				attempt := t.ResumeAttempt
				continues := attempt != ""
				if !continues {
					attempt = randomID()
					t.Attempts++
				}
				t.ResumeAttempt = ""
				row.Managed = &model.ManagedLaunch{RequestID: in.RequestID, RequestVersion: in.ExpectedVersion, Workflow: t.Workflow, Phase: t.Phase, Role: phase.Role, Visit: t.StageVisit, Basis: executionBasis(st, t), Attempt: attempt, Continues: continues, Prompt: composeManagedPrompt(st, t, phase)}
				if len(row.Managed.Prompt.Text) > 128<<10 {
					return false, failure(409, "prompt_too_large", "Ticket context exceeds the bounded launch prompt")
				}
				if t.Delivery != nil {
					row.Worktree = t.Delivery.Worktree
				}
				if phase.Role == "Review" {
					row.Managed.Candidate = t.Delivery.Candidate.Fingerprint
				}
				t.Execution, t.Claim, t.Active = "local:"+row.ID, row.LaunchID, true
				t.Attention = "Local phase launch pending"
				saveTicket(st, &t, a, "nativePhaseReserved")
			} else {
				if t.ProjectID != in.ProjectID || t.Kind != "task" || !t.Agent || t.Status != "Doing" || busy(t) || t.Delivery == nil || t.Delivery.Worktree.Path == "" || !acceptedScope(st, t) || !dependenciesDone(st, t) || t.ResumeHold || t.StopRequested || t.Archived || t.Deleted {
					return false, failure(409, "ticket_session_blocked", "Use an unblocked accepted task with its prepared owned Implementation worktree")
				}
				p := st.Projects[t.ProjectID]
				repeated := false
				for _, other := range st.LocalSessions {
					if other.TicketID == t.ID {
						repeated = true
					}
				}
				if p.LaneLocks[st.Workflows[t.Workflow].ID+"#*"] || p.LaneLocks[st.Workflows[t.Workflow].ID+"#"+t.Phase] {
					return false, failure(409, "capacity_blocked", "Implementation capacity is unavailable or the phase is locked")
				}
				phase, ok := phaseByID(st.Workflows[t.Workflow], t.Phase)
				if !ok || !phase.Agentic {
					return false, failure(409, "phase_gate", "A ticket harness requires its pinned agentic phase")
				}
				if err := phaseCapacity(st, t, phase); err != nil {
					return false, err
				}
				if repeated {
					limit := t.AttemptLimit
					if limit == 0 {
						limit = 20
					}
					if t.Attempts >= limit {
						return false, failure(409, "budget_exhausted", "Extend the finite attempt limit before another harness starts")
					}
					t.Attempts++
				}
				if t.Delivery.BindingVersion != row.BindingVersion {
					return false, failure(409, "launch_basis_changed", "Retained worktree binding changed")
				}
				row.Worktree = t.Delivery.Worktree
				t.Execution = "local:" + row.ID
				t.Claim = row.LaunchID
				t.Active = true
				t.Attention = "Local harness launch pending"
				saveTicket(st, &t, a, "localSessionReserved")
			}
		}
		st.LocalSessions[row.ID] = row
		b, _ := json.Marshal(row.ID)
		st.Receipts[key] = model.Receipt{Fingerprint: fingerprint, Result: b}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if row.State != "preparing" {
		return localInfo(row), nil
	}
	if row.Generation != ping.Generation {
		return nil, failure(409, "session_ownership_unknown", "Prior daemon ownership is unresolved; replacement is blocked")
	}
	s.gitMu.Lock()
	currentState, _, _, scopeErr := s.localSnapshot(ctx, token, row.ID)
	if scopeErr != nil {
		s.gitMu.Unlock()
		return nil, scopeErr
	}
	p = currentState.Projects[row.ProjectID]
	if p.Repository == nil || p.Repository.Version != row.BindingVersion {
		s.gitMu.Unlock()
		return nil, failure(409, "launch_basis_changed", "Repository authorization changed")
	}
	binding := gitwork.Binding{Repository: p.Repository.Repository, StorageRoot: p.Repository.StorageRoot}
	if row.TicketID == 0 && (row.Worktree.Path == "" || row.Worktree.Owner == "") {
		// A free console belongs in the exact granted checkout. A previously
		// prepared console worktree retains its original identity on retries.
		err = gitwork.VerifyCheckout(ctx, binding.Repository)
		if err == nil && row.Worktree.Path != "" && row.Worktree.Path != binding.Repository {
			err = errors.New("console directory changed")
		}
		if err == nil {
			row.Worktree.Path = binding.Repository
		}
	} else if row.Managed != nil && row.Managed.Role == "Refine" {
		var work gitwork.Worktree
		work, err = gitwork.PrepareConsole(ctx, binding, row.LaunchID[:24], row.ID)
		row.Worktree = modelWorktree(work)
	} else {
		err = gitwork.Verify(ctx, binding, ownedWorktree(row.Worktree))
	}
	s.gitMu.Unlock()
	if err != nil {
		if row.TicketID == 0 && row.Worktree.Owner == "" {
			return nil, failure(409, "project_root_unavailable", "The granted project root is unavailable or changed; reconcile it before retrying")
		}
		return nil, failure(409, "worktree_preparation_pending", "Retain this launch intent and reconcile its owned worktree before retrying")
	}
	// Persist the launch directory before dispatch so a retry cannot retarget it.
	if in.Harness == "codex" {
		hash, err := s.writeContextCredential(row)
		if err != nil {
			return nil, err
		}
		row.CredentialHash = hash
	}
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, err := executionActor(ctx, st, token)
		if err != nil {
			return false, err
		}
		current := st.LocalSessions[row.ID]
		if err = localAllowed(st, a, current); err != nil {
			return false, err
		}
		if current.LaunchID != row.LaunchID || current.State != "preparing" {
			return false, failure(409, "launch_basis_changed", "Launch intent changed")
		}
		current.Worktree = row.Worktree
		current.CredentialHash = row.CredentialHash
		if current.CredentialHash != "" && current.CredentialExpires.IsZero() {
			current.CredentialExpires = time.Now().UTC().Add(12 * time.Hour)
		}
		st.LocalSessions[row.ID] = current
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, err := executionActor(ctx, st, token)
		if err != nil {
			return false, err
		}
		current := st.LocalSessions[row.ID]
		if err = localAllowed(st, a, current); err != nil {
			return false, err
		}
		p := st.Projects[row.ProjectID]
		if row.Managed != nil && st.Settings.ExecutionMode != "local" {
			return false, failure(409, "execution_disabled", "Automatic local admission was disabled")
		}
		if current.LaunchID != row.LaunchID || current.State != "preparing" || p.Repository == nil || p.Repository.Version != row.BindingVersion || len(st.Accounts) > 1 || st.Settings.Agents || st.Settings.Handbrake || p.Handbrake {
			return false, failure(409, "launch_basis_changed", "Launch authority or repository changed")
		}
		if row.TicketID != 0 {
			t := st.Tickets[row.TicketID]
			basisValid := t.Status == "Doing" && acceptedScope(st, t)
			if row.Managed != nil {
				basisValid = t.Workflow == row.Managed.Workflow && t.Phase == row.Managed.Phase && t.StageVisit == row.Managed.Visit && executionBasis(st, t) == row.Managed.Basis && (row.Managed.Role == "Refine" || acceptedScope(st, t))
			}
			p := st.Projects[t.ProjectID]
			w := st.Workflows[t.Workflow]
			if t.Execution != "local:"+row.ID || t.StopRequested || t.ResumeHold || t.Archived || t.Deleted || !basisValid || !dependenciesDone(st, t) || p.LaneLocks[w.ID+"#*"] || p.LaneLocks[w.ID+"#"+t.Phase] {
				return false, failure(409, "launch_basis_changed", "Ticket admission changed")
			}
		}
		current.Worktree = row.Worktree
		launch := &localruntime.Launch{ID: row.ID, LaunchID: row.LaunchID, Generation: row.Generation, Program: s.localClient.Executable, Args: []string{"_terminal-fixture"}, Cwd: row.Worktree.Path, Cols: 100, Rows: 28}
		if in.Harness == "codex" {
			role := "Console"
			if row.TicketID == 0 && row.Worktree.Owner != "" {
				role = "ConsoleWorktree"
			}
			if row.TicketID != 0 {
				role = "Doing"
			}
			if row.Managed != nil {
				role = row.Managed.Role
			}
			launch.Native = true
			launch.Args = []string{"_codex-session", "--runtime-dir", s.localClient.Root, "--session", row.ID, "--launch", row.LaunchID, "--generation", row.Generation, "--codex", nativePath, "--role", role}
			launch.Env = map[string]string{"TELEPORT_CONFIG": filepath.Join(s.localClient.Root, row.ID+".client.json"), "TELEPORT_SERVER": s.localEndpoint, "TELEPORT_SESSION_ID": row.ID}
			if row.Managed != nil {
				launch.Env["TELEPORT_MANAGED_PROMPT"] = row.Managed.Prompt.Text
			}
		}
		out, err := s.localClient.Call(ctx, localruntime.Request{Operation: "launch", Launch: launch})
		if err != nil {
			return false, failure(503, "launch_ack_unknown", "Launch outcome is unknown; retry this exact request without replacing the instance")
		}
		current.State, current.ErrorCode = out.Status.State, out.Status.ErrorCode
		if current.State == "running" && row.TicketID != 0 {
			t := st.Tickets[row.TicketID]
			if t.Execution == "local:"+row.ID {
				t.Attention = ""
				saveTicket(st, &t, a, "localSessionStarted")
			}
		}
		if current.State == "failed" && row.TicketID != 0 {
			t := st.Tickets[row.TicketID]
			if t.Execution == "local:"+row.ID {
				t.Active, t.Claim, t.Execution = false, "", ""
				t.Attention = "Harness could not start; retained work is preserved"
				saveTicket(st, &t, a, "localSessionStartFailed")
			}
		}
		st.LocalSessions[row.ID] = current
		row = current
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return localInfo(row), nil
}
func (s *Service) sendLocalInput(ctx context.Context, token string, in SessionInput) (any, error) {
	if len(in.RequestID) < 1 || len(in.RequestID) > 100 || len(in.Data) < 1 || len(in.Data) > 16384 || strings.ContainsRune(in.RequestID, 0) {
		return nil, failure(422, "invalid_terminal_input", "Use a stable request ID and 1–16,384 terminal bytes")
	}
	fingerprint, err := s.localClient.Fingerprint(in.Data)
	if err != nil {
		return nil, failure(503, "runtime_unavailable", "Input was not authorized while the runtime is unavailable")
	}
	var item model.LocalInput
	var row model.LocalSession
	var key string
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, err := actor(st, token)
		if err != nil {
			return false, err
		}
		row = st.LocalSessions[in.ID]
		if err = localAllowed(st, a, row); err != nil {
			return false, err
		}
		key = a.ID + ":" + row.ID + ":" + in.RequestID
		if st.LocalInputs == nil {
			st.LocalInputs = map[string]model.LocalInput{}
		}
		if previous, ok := st.LocalInputs[key]; ok {
			if previous.Fingerprint != fingerprint {
				return false, failure(409, "input_identity_conflict", "Input request identity changed")
			}
			item = previous
			return false, nil
		}
		if row.State != "running" || row.StopRequested {
			return false, failure(409, "input_unavailable", "Reconnect the same live harness before sending input")
		}
		row.InputSequence++
		item = model.LocalInput{SessionID: row.ID, RequestID: in.RequestID, Actor: a.ID, Sequence: row.InputSequence, Fingerprint: fingerprint, State: "pending", At: time.Now().UTC()}
		st.LocalSessions[row.ID] = row
		st.LocalInputs[key] = item
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if item.State != "pending" {
		return intervention(item), nil
	}
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, err := actor(st, token)
		if err != nil {
			return false, err
		}
		row = st.LocalSessions[in.ID]
		if err = localAllowed(st, a, row); err != nil {
			return false, err
		}
		binding := st.Projects[row.ProjectID].Repository
		if len(st.Accounts) > 1 || row.State != "running" || row.StopRequested || binding == nil || binding.Version != row.BindingVersion {
			return false, failure(409, "input_unavailable", "Current local input authority is unavailable")
		}
		if row.TicketID != 0 {
			t := st.Tickets[row.TicketID]
			if t.StopRequested || t.ResumeHold || t.Status == "Cancelled" || t.Archived || t.Deleted || t.Execution != "local:"+row.ID {
				return false, failure(409, "ticket_input_blocked", "Ticket input authority has been withdrawn")
			}
		}
		request := daemonRequest(row, "input")
		request.Sequence, request.Data = item.Sequence, in.Data
		out, err := s.localClient.Call(ctx, request)
		if err != nil {
			return false, failure(503, "input_delivery_unknown", "Delivery is unconfirmed; retain this operation ID and never resend it as a new input")
		}
		item.State, item.Written = out.Receipt.State, out.Receipt.Written
		st.LocalInputs[key] = item
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return intervention(item), nil
}

// PumpLocal reconciles observations and durable stops; it never launches a replacement.
func (s *Service) PumpLocal(ctx context.Context) error {
	s.localMu.Lock()
	defer s.localMu.Unlock()
	if s.localClient == nil {
		return nil
	}
	st, err := s.db.Read(ctx)
	if err != nil || len(st.LocalSessions) == 0 {
		return err
	}
	out, observedErr := s.localClient.Call(ctx, localruntime.Request{Operation: "list"})
	observed := map[string]localruntime.Status{}
	for _, status := range out.Items {
		observed[status.ID] = status
	}
	return s.db.Update(ctx, func(st *model.State) (bool, error) {
		changed := false
		for id, row := range st.LocalSessions {
			if !activeLocal(row) {
				continue
			}
			status, ok := observed[id]
			if observedErr == nil && !ok && out.Generation == row.Generation && row.State == "preparing" {
				a := st.Accounts[row.Creator]
				p := st.Projects[row.ProjectID]
				t := st.Tickets[row.TicketID]
				withdrawn := row.StopRequested || a.ID == "" || a.Suspended || len(st.Accounts) > 1 || require(st, a, row.ProjectID, "Contributor") != nil || p.Repository == nil || p.Repository.Version != row.BindingVersion
				if row.TicketID != 0 {
					withdrawn = withdrawn || t.StopRequested || t.ResumeHold || t.Deleted || t.Archived || t.Status == "Cancelled" || t.Execution != "local:"+row.ID
				}
				if withdrawn {
					now := time.Now().UTC()
					row.State, row.ErrorCode, row.ExitedAt = "failed", "launch_cancelled_before_dispatch", &now
					st.LocalSessions[id] = row
					changed = true
					if row.TicketID != 0 && t.Execution == "local:"+row.ID {
						t.Execution, t.Claim, t.Active = "", "", false
						t.ResumeHold = true
						t.Attention = "Local launch cancelled before dispatch"
						saveTicket(st, &t, model.Account{ID: "local-runtime"}, "localLaunchCancelled")
					}
				}
				continue
			}
			if observedErr != nil || !ok || status.LaunchID != row.LaunchID || status.Generation != row.Generation {
				if row.State != "unknown" {
					row.State, row.ErrorCode = "unknown", "owned_runtime_unavailable"
					st.LocalSessions[id] = row
					changed = true
				}
				continue
			}
			a := st.Accounts[row.Creator]
			withdrawn := row.StopRequested || a.Suspended || a.ID == "" || require(st, a, row.ProjectID, "Contributor") != nil || len(st.Accounts) > 1
			binding := st.Projects[row.ProjectID].Repository
			withdrawn = withdrawn || binding == nil || binding.Version != row.BindingVersion
			if row.TicketID != 0 {
				t := st.Tickets[row.TicketID]
				withdrawn = withdrawn || t.StopRequested || t.Status == "Cancelled" || t.Archived || t.Deleted || t.Execution != "local:"+row.ID
			}
			if withdrawn && status.State == "running" {
				if _, err := s.localClient.Call(ctx, daemonRequest(row, "stop")); err != nil {
					status.ErrorCode = "owned_stop_pending"
				}
				status.State = "stopping"
			}
			if row.State != status.State || row.ErrorCode != status.ErrorCode || row.Attention != status.Attention {
				row.State, row.ErrorCode, row.Attention = status.State, status.ErrorCode, status.Attention
				row.ExitedAt, row.ExitCode = status.ExitedAt, status.ExitCode
				st.LocalSessions[id] = row
				changed = true
				if row.TicketID != 0 && (status.State == "exited" || status.State == "failed") {
					t := st.Tickets[row.TicketID]
					if t.Execution == "local:"+id {
						t.Execution, t.Claim, t.Active = "", "", false
						t.Attention = "Harness exited; inspect retained work before continuing"
						if row.Managed == nil {
							t.ResumeHold = true
						}
						saveTicket(st, &t, model.Account{ID: "local-runtime"}, "localSessionExitObserved")
					}
				}
			}
		}
		if observedErr != nil && changed {
			slog.Warn("local runtime ownership unavailable; replacement blocked")
		}
		return changed, nil
	})
}
