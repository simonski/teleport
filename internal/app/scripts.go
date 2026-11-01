package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"teleport/internal/localruntime"
	"teleport/internal/model"
	"teleport/internal/scriptruntime"
)

var scriptBindings = map[string]bool{
	"listProjects": false, "createProject": false, "listTickets": false, "getTicket": false, "createTicket": false, "updateTicket": false, "transitionTicket": true, "listTicketHistory": false, "getTicketAgreements": false,
	"listWorkflows": false, "getWorkflow": false, "searchTickets": false, "listIdeas": false, "getIdea": false, "saveIdea": false, "getIdeaHistory": false,
	"acceptRefinement": true, "rejectRefinement": true, "approveTicket": true, "rejectTicket": true,
}

type ScriptBinding struct {
	Operation         string `json:"operation"`
	HumanConfirmation bool   `json:"humanConfirmation"`
}

func ScriptBindings() []ScriptBinding {
	result := []ScriptBinding{}
	for name, gate := range scriptBindings {
		result = append(result, ScriptBinding{name, gate})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Operation < result[j].Operation })
	return result
}
func scriptOperation(op string) bool {
	return contains([]string{"getScriptBindings", "startScriptRun", "listScriptRuns", "getScriptRun", "stopScriptRun", "confirmScriptCall", "scriptCall"}, op)
}
func scriptLive(state string) bool {
	return contains([]string{"starting", "running", "waiting", "stopping", "unknown"}, state)
}
func scriptContextAllowed(st *model.State, a model.Account, row model.ScriptRun) bool {
	for _, id := range row.Projects {
		if require(st, a, id, "Viewer") != nil {
			return false
		}
	}
	return true
}
func scriptActor(st *model.State, token string) (model.Account, bool) {
	if token == "" {
		return model.Account{}, false
	}
	hash := digest(token)
	for _, row := range st.ScriptRuns {
		if row.TokenHash == hash && contains([]string{"starting", "running", "waiting"}, row.State) && !row.StopRequested && time.Now().Before(row.ExpiresAt) {
			a, ok := st.Accounts[row.Creator]
			if !ok || a.Suspended || !scriptContextAllowed(st, a, row) {
				return model.Account{}, false
			}
			a.PasswordHash = ""
			a.ServiceRunID = row.ID
			for _, call := range row.Calls {
				if call.State == "dispatching" {
					a.ServiceCallID = call.ID
				}
			}
			return a, true
		}
	}
	return model.Account{}, false
}

type scriptDispatchKey struct{}

func scriptNeedsConfirmation(st *model.State, operation string, raw json.RawMessage) bool {
	if operation != "transitionTicket" {
		return scriptBindings[operation]
	}
	var in TransitionInput
	if json.Unmarshal(raw, &in) != nil {
		return false
	}
	t, ok := st.Tickets[in.ID]
	if !ok {
		return false
	}
	workflow := st.Workflows[t.Workflow]
	target, found := phaseByID(workflow, in.Target)
	if !found {
		target, found = phaseForRole(workflow, in.Target)
	}
	return found && target.Role == "Done" && target.ID != t.Phase
}

type scriptPermit struct{ RunID, CallID, Operation, Fingerprint string }

func checkScriptDispatch(ctx context.Context, st *model.State, a *model.Account, operation string, raw json.RawMessage) error {
	if a.ServiceRunID == "" {
		return nil
	}
	permit, ok := ctx.Value(scriptDispatchKey{}).(scriptPermit)
	if !ok || permit.RunID != a.ServiceRunID || permit.Operation != operation || permit.Fingerprint != digest(string(raw)) {
		return failure(403, "script_scope", "Script credentials require a recorded canonical binding dispatch")
	}
	for _, call := range st.ScriptRuns[a.ServiceRunID].Calls {
		if call.ID == permit.CallID && call.State == "dispatching" && call.Operation == operation && call.Fingerprint == digest(operation+"\x00"+string(raw)) {
			if _, exists := scriptBindings[operation]; !exists || scriptNeedsConfirmation(st, operation, raw) && !call.Approved {
				break
			}
			a.ServiceCallID = call.ID
			return nil
		}
	}
	return failure(403, "script_scope", "Script dispatch authority changed")
}

type StartScriptInput struct {
	RequestID string `json:"requestId"`
	Source    string `json:"source"`
	Mode      string `json:"mode"`
}
type ScriptIDInput struct {
	ID string `json:"id"`
}
type StopScriptInput struct {
	ID      string `json:"id"`
	Confirm bool   `json:"confirm"`
}
type ConfirmScriptInput struct {
	ID          string `json:"id"`
	CallID      string `json:"callId"`
	Fingerprint string `json:"fingerprint"`
	Decision    string `json:"decision"`
	Confirm     bool   `json:"confirm"`
}
type ScriptInfo struct {
	StopActor     string     `json:"stopActor,omitempty"`
	StopAt        *time.Time `json:"stopAt,omitempty"`
	ID            string     `json:"id"`
	Creator       string     `json:"creator"`
	RequestID     string     `json:"requestId,omitempty"`
	State         string     `json:"state"`
	RuntimeState  string     `json:"runtimeState"`
	StopRequested bool       `json:"stopRequested"`
	ErrorCode     string     `json:"errorCode,omitempty"`
	CallCount     int        `json:"callCount"`
	CreatedAt     time.Time  `json:"createdAt"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
}
type ScriptPage struct {
	Items      []ScriptInfo `json:"items"`
	NextCursor string       `json:"nextCursor,omitempty"`
}
type ScriptView struct {
	Run           ScriptInfo             `json:"run"`
	Source        string                 `json:"source"`
	Mode          string                 `json:"mode"`
	Calls         []model.ScriptCall     `json:"calls"`
	Output        *scriptruntime.Outcome `json:"output,omitempty"`
	PendingTicket *model.Ticket          `json:"pendingTicket,omitempty"`
}

func scriptInfo(row model.ScriptRun, private bool) ScriptInfo {
	info := ScriptInfo{ID: row.ID, Creator: row.Creator, State: row.State, RuntimeState: row.RuntimeState, StopRequested: row.StopRequested, ErrorCode: row.ErrorCode, CallCount: len(row.Calls), CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, FinishedAt: row.FinishedAt}
	if private {
		info.RequestID = row.RequestID
	}
	info.StopActor, info.StopAt = row.StopActor, row.StopAt
	return info
}
func scriptRequest(row model.ScriptRun, op string) localruntime.Request {
	return localruntime.Request{Operation: op, ID: row.ID, LaunchID: row.LaunchID, Generation: row.Generation}
}
func scriptBasis(st *model.State, a model.Account, arguments json.RawMessage) (string, *model.Ticket, error) {
	var input struct {
		ID              int64 `json:"id"`
		ExpectedVersion int64 `json:"expectedVersion"`
	}
	if json.Unmarshal(arguments, &input) != nil || input.ID == 0 {
		return "", nil, failure(422, "invalid_confirmation", "This action requires an exact ticket and version")
	}
	t, err := ticketFor(st, a, input.ID, "Contributor")
	if err != nil {
		return "", nil, err
	}
	if t.Version != input.ExpectedVersion {
		return "", nil, failure(409, "stale_confirmation", "Ticket changed; prepare a fresh exact action")
	}
	data, _ := json.Marshal(struct {
		Ticket                          model.Ticket
		ProjectVersion, SettingsVersion int64
	}{t, st.Projects[t.ProjectID].Version, st.Settings.Version})
	return digest(string(data)), &t, nil
}
func (s *Service) scriptCall(ctx context.Context, token, operation string, raw json.RawMessage) (any, error) {
	s.scriptMu.Lock()
	defer s.scriptMu.Unlock()
	if operation == "scriptCall" {
		return s.dispatchScriptCall(ctx, token, raw)
	}
	st, err := s.db.Read(ctx)
	if err != nil {
		return nil, err
	}
	a, err := actor(st, token)
	if err != nil {
		return nil, err
	}
	if a.ServiceRunID != "" || a.ServiceSessionID != "" {
		return nil, failure(403, "human_required", "Use a fresh human sign-in to manage script jobs")
	}
	if operation == "getScriptBindings" {
		if err = parse(raw, &struct{}{}); err != nil {
			return nil, err
		}
		return struct {
			Version  int             `json:"version"`
			Bindings []ScriptBinding `json:"bindings"`
		}{1, ScriptBindings()}, nil
	}
	if operation == "listScriptRuns" {
		var in ListInput
		if err = parse(raw, &in); err != nil {
			return nil, err
		}
		after, limit, err := pageBounds(in.Cursor, in.Limit)
		if err != nil {
			return nil, err
		}
		rows := []model.ScriptRun{}
		for _, row := range st.ScriptRuns {
			if row.Number > after && (row.Creator == a.ID || a.Administrator) {
				rows = append(rows, row)
			}
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Number < rows[j].Number })
		page := ScriptPage{Items: []ScriptInfo{}}
		for i, row := range rows {
			if i == limit {
				page.NextCursor = strconv.FormatInt(rows[i-1].Number, 10)
				break
			}
			page.Items = append(page.Items, scriptInfo(row, row.Creator == a.ID))
		}
		return page, nil
	}
	if operation == "startScriptRun" {
		var in StartScriptInput
		if err = parse(raw, &in); err != nil {
			return nil, err
		}
		return s.startScript(ctx, token, a, in)
	}
	var input ScriptIDInput
	if operation == "getScriptRun" {
		if err = parse(raw, &input); err != nil {
			return nil, err
		}
	}
	if operation == "stopScriptRun" {
		var in StopScriptInput
		if err = parse(raw, &in); err != nil {
			return nil, err
		}
		if !in.Confirm {
			return nil, failure(422, "confirmation_required", "Confirm stopping this script job")
		}
		input.ID = in.ID
	}
	if operation == "confirmScriptCall" {
		var in ConfirmScriptInput
		if err = parse(raw, &in); err != nil {
			return nil, err
		}
		return s.confirmScript(ctx, token, a, in)
	}
	row, ok := st.ScriptRuns[input.ID]
	if !ok {
		return nil, failure(404, "not_found", "Script run not found")
	}
	if operation == "getScriptRun" {
		if row.Creator != a.ID || !scriptContextAllowed(st, a, row) {
			return nil, failure(403, "private_run", "Script source and results belong to the initiating account with current context access")
		}
		view := ScriptView{Run: scriptInfo(row, true), Source: row.Source, Mode: row.Mode, Calls: append([]model.ScriptCall{}, row.Calls...)}
		if row.Output != "" {
			var out scriptruntime.Outcome
			if json.Unmarshal([]byte(row.Output), &out) == nil {
				view.Output = &out
			}
		}
		for _, call := range row.Calls {
			if call.State == "waiting" {
				_, view.PendingTicket, _ = scriptBasis(st, a, call.Arguments)
				break
			}
		}
		return view, nil
	}
	if row.Creator != a.ID && !a.Administrator {
		return nil, failure(403, "private_run", "Only the owner or administrator may stop this job")
	}
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		current, authErr := actor(st, token)
		row := st.ScriptRuns[input.ID]
		if authErr != nil {
			return false, authErr
		}
		if current.ServiceRunID != "" || current.ServiceSessionID != "" || current.ID != a.ID || row.Creator != current.ID && !current.Administrator {
			return false, failure(403, "forbidden", "Current script stop authority is unavailable")
		}
		if !scriptLive(row.State) {
			return false, nil
		}
		row.StopRequested = true
		now := time.Now().UTC()
		row.StopActor = current.ID
		row.StopAt = &now
		row.State = "stopping"
		for i := range row.Calls {
			if row.Calls[i].State == "waiting" || row.Calls[i].State == "approved" {
				row.Calls[i].State = "cancelled"
			}
		}
		st.ScriptRuns[row.ID] = row
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	latest, readErr := s.db.Read(ctx)
	if readErr != nil {
		return nil, readErr
	}
	row = latest.ScriptRuns[input.ID]
	if s.localClient != nil && scriptLive(row.State) {
		_, _ = s.localClient.Call(ctx, scriptRequest(row, "stop"))
	}
	return scriptInfo(row, row.Creator == a.ID), nil
}

func (s *Service) startScript(ctx context.Context, token string, a model.Account, in StartScriptInput) (any, error) {
	if len(in.Source) == 0 || len(in.Source) > scriptruntime.SourceLimit || strings.ContainsRune(in.Source, 0) || len(in.RequestID) < 1 || len(in.RequestID) > 100 || in.Mode != "script" && in.Mode != "expression" && in.Mode != "console" {
		return nil, failure(422, "invalid_script", "Provide bounded source, script/expression/console mode and a stable request ID")
	}
	if s.localClient == nil || s.localEndpoint == "" || !localruntime.Supported() {
		return nil, failure(503, "runtime_unavailable", "Configure the local owned interpreter runtime")
	}
	fingerprint := digest(in.Mode + "\x00" + in.Source)
	st, err := s.db.Read(ctx)
	if err != nil {
		return nil, err
	}
	for _, old := range st.ScriptRuns {
		if old.Creator == a.ID && old.RequestID == in.RequestID {
			if old.Fingerprint != fingerprint {
				return nil, failure(409, "request_conflict", "Run identity already belongs to different source")
			}
			return scriptInfo(old, true), nil
		}
	}
	if err = s.localClient.Ensure(ctx); err != nil {
		return nil, failure(503, "runtime_unavailable", "Owned script runtime unavailable")
	}
	ping, err := s.localClient.Call(ctx, localruntime.Request{Operation: "ping"})
	if err != nil {
		return nil, failure(503, "runtime_unavailable", "Owned script runtime unavailable")
	}
	if !ping.ScriptRuntime {
		return nil, failure(409, "runtime_upgrade_required", "Stop existing harnesses and shut down their idle runtime before starting script jobs with this binary; no runtime was replaced")
	}
	secret := randomID()
	var row model.ScriptRun
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		current, authErr := actor(st, token)
		if authErr != nil {
			return false, authErr
		}
		if current.ServiceRunID != "" || current.ServiceSessionID != "" || current.ID != a.ID {
			return false, failure(403, "forbidden", "Current account execution authority unavailable")
		}
		if st.ScriptRuns == nil {
			st.ScriptRuns = map[string]model.ScriptRun{}
		}
		if len(st.ScriptRuns) >= 256 {
			return false, failure(409, "script_retention_full", "Script retention limit reached; retained jobs have not been deleted")
		}
		active := 0
		for _, r := range st.ScriptRuns {
			if scriptLive(r.State) {
				active++
			}
		}
		if active >= 4 {
			return false, failure(409, "script_capacity", "Four script jobs are already active or unresolved")
		}
		st.NextScriptID++
		now := time.Now().UTC()
		row = model.ScriptRun{ID: "J" + strconv.FormatInt(st.NextScriptID, 10), Number: st.NextScriptID, Creator: a.ID, RequestID: in.RequestID, Source: in.Source, Mode: in.Mode, Fingerprint: fingerprint, TokenHash: digest(secret), LaunchID: randomID(), Generation: ping.Generation, State: "starting", RuntimeState: "preparing", Projects: []int64{}, Calls: []model.ScriptCall{}, CreatedAt: now, ExpiresAt: now.Add(120 * time.Second)}
		for id := range st.Projects {
			if role(st, current, id) != "" {
				row.Projects = append(row.Projects, id)
			}
		}
		st.ScriptRuns[row.ID] = row
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	config := scriptruntime.Config{ID: row.ID, LaunchID: row.LaunchID, Server: s.localEndpoint, Token: secret, Source: row.Source, Mode: row.Mode}
	data, _ := json.Marshal(config)
	path := filepath.Join(s.localClient.Root, row.ID+".script.json")
	file, writeErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if writeErr == nil {
		_, writeErr = file.Write(data)
		if writeErr == nil {
			writeErr = file.Sync()
		}
		closeErr := file.Close()
		if writeErr == nil {
			writeErr = closeErr
		}
	}
	updateErr := s.db.Update(ctx, func(st *model.State) (bool, error) {
		current := st.ScriptRuns[row.ID]
		if current.LaunchID != row.LaunchID {
			return false, failure(409, "script_changed", "Owned script identity changed")
		}
		owner, exists := st.Accounts[current.Creator]
		if writeErr != nil {
			current.State, current.RuntimeState, current.ErrorCode = "failed", "failed", "script_config_failed"
		} else if current.StopRequested || current.State != "starting" || !exists || owner.Suspended || !scriptContextAllowed(st, owner, current) || !time.Now().Before(current.ExpiresAt) {
			now := time.Now().UTC()
			current.State, current.RuntimeState, current.ErrorCode = "stopped", "not_started", "script_admission_withdrawn"
			current.StopRequested = true
			current.FinishedAt = &now
			if current.StopActor == "" {
				current.StopActor = "script-supervisor"
				current.StopAt = &now
			}
		} else {
			// Serialize final authority with actual process admission. Human
			// logout after Run does not withdraw the already admitted job.
			out, launchErr := s.localClient.Call(ctx, localruntime.Request{Operation: "launch", Generation: current.Generation, Launch: &localruntime.Launch{ID: current.ID, LaunchID: current.LaunchID, Generation: current.Generation, Script: true, Program: s.localClient.Executable, Args: []string{"_script-run", "--config", path}, Cwd: s.localClient.Root, Cols: 80, Rows: 24}})
			if launchErr != nil || out.Status == nil {
				current.State, current.ErrorCode = "unknown", "script_launch_unconfirmed"
			} else {
				current.RuntimeState = out.Status.State
				current.State = "running"
			}
		}
		st.ScriptRuns[row.ID] = current
		row = current
		return true, nil
	})
	if updateErr != nil {
		return nil, updateErr
	}
	return scriptInfo(row, true), nil
}

func (s *Service) confirmScript(ctx context.Context, token string, a model.Account, in ConfirmScriptInput) (any, error) {
	if !in.Confirm || !contains([]string{"approve", "decline"}, in.Decision) {
		return nil, failure(422, "confirmation_required", "Make an explicit exact-action decision")
	}
	err := s.db.Update(ctx, func(st *model.State) (bool, error) {
		current, authErr := actor(st, token)
		if authErr != nil {
			return false, authErr
		}
		row, exists := st.ScriptRuns[in.ID]
		if current.ServiceRunID != "" || current.ServiceSessionID != "" || current.ID != a.ID || !exists || row.Creator != a.ID || !scriptContextAllowed(st, current, row) {
			return false, failure(403, "private_run", "Current owner confirmation authority is unavailable")
		}
		if row.StopRequested || row.State != "waiting" || !time.Now().Before(row.ExpiresAt) {
			return false, failure(409, "confirmation_expired", "Script confirmation is no longer pending")
		}
		for i := range row.Calls {
			call := &row.Calls[i]
			if call.ID != in.CallID {
				continue
			}
			if call.State != "waiting" || call.Fingerprint != in.Fingerprint {
				return false, failure(409, "confirmation_changed", "Prepare a fresh exact action")
			}
			if in.Decision == "approve" {
				basis, _, err := scriptBasis(st, current, call.Arguments)
				if err != nil {
					return false, err
				}
				if basis != call.Basis {
					return false, failure(409, "confirmation_stale", "The reviewed decision basis changed")
				}
				call.Approved = true
				call.State = "approved"
			} else {
				call.State = "declined"
				call.ErrorCode = "human_declined"
				call.ErrorMessage = "The script action was declined; no domain rejection or cancellation was performed"
			}
			now := time.Now().UTC()
			call.ConfirmationActor = current.ID
			call.ConfirmedAt = &now
			call.Decision = in.Decision
			row.State = "running"
			st.ScriptRuns[row.ID] = row
			return true, nil
		}
		return false, failure(404, "not_found", "Pending script action not found")
	})
	return map[string]bool{"recorded": err == nil}, err
}

type ScriptCallInput struct {
	ID        string          `json:"id"`
	CallID    string          `json:"callId"`
	Operation string          `json:"operation"`
	Arguments json.RawMessage `json:"arguments"`
}
type ScriptCallResult struct {
	State  string          `json:"state"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

func scriptCallResult(call model.ScriptCall) ScriptCallResult {
	result := ScriptCallResult{State: call.State, Result: call.Result}
	if call.ErrorCode != "" {
		result.Error = &Error{Code: call.ErrorCode, Message: call.ErrorMessage}
	}
	return result
}
func (s *Service) dispatchScriptCall(ctx context.Context, token string, raw json.RawMessage) (any, error) {
	var in ScriptCallInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	_, supported := scriptBindings[in.Operation]
	number, err := strconv.Atoi(in.CallID)
	if err != nil || number < 1 || number > 128 || strconv.Itoa(number) != in.CallID || len(in.Operation) > 100 || len(in.Arguments) > 64<<10 || !json.Valid(in.Arguments) {
		return nil, failure(422, "invalid_script_call", "Use the next bounded call identity and an object argument")
	}
	fingerprint := digest(in.Operation + "\x00" + string(in.Arguments))
	var call model.ScriptCall
	dispatch := false
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, ok := scriptActor(st, token)
		if !ok || a.ServiceRunID != in.ID {
			return false, failure(403, "script_authority", "Script authority expired, stopped or changed")
		}
		row := st.ScriptRuns[in.ID]
		gate := scriptNeedsConfirmation(st, in.Operation, in.Arguments)
		for i := range row.Calls {
			current := &row.Calls[i]
			if current.ID != in.CallID {
				continue
			}
			if current.Fingerprint != fingerprint {
				return false, failure(409, "call_conflict", "Call identity belongs to different arguments")
			}
			call = *current
			if current.State == "approved" {
				basis, _, err := scriptBasis(st, a, in.Arguments)
				if err != nil || basis != current.Basis {
					current.State, current.ErrorCode, current.ErrorMessage = "rejected", "confirmation_stale", "The confirmed decision basis changed"
					call = *current
					st.ScriptRuns[row.ID] = row
					return true, nil
				}
				current.State = "dispatching"
				call = *current
				dispatch = true
				st.ScriptRuns[row.ID] = row
				return true, nil
			}
			if current.State == "dispatching" {
				row.State, row.ErrorCode = "unknown", "script_call_unconfirmed"
				st.ScriptRuns[row.ID] = row
				call.State, call.ErrorCode, call.ErrorMessage = "unknown", "call_outcome_unknown", "The prior dispatch outcome is unresolved; no replay was attempted"
				return true, nil
			}
			return false, nil
		}
		if number != len(row.Calls)+1 {
			return false, failure(409, "call_sequence", "Use the next script call identity")
		}
		for _, prior := range row.Calls {
			if contains([]string{"waiting", "approved", "dispatching", "unknown"}, prior.State) {
				return false, failure(409, "call_pending", "Resolve the prior action before another dispatch")
			}
		}
		encoded, _ := json.Marshal(row.Calls)
		if len(encoded)+len(in.Arguments)+2048 > 256<<10 {
			return false, failure(409, "call_retention_limit", "Script call retention limit reached before dispatch")
		}
		call = model.ScriptCall{ID: in.CallID, Operation: in.Operation, Arguments: append(json.RawMessage(nil), in.Arguments...), Fingerprint: fingerprint, State: "dispatching", At: time.Now().UTC()}
		if !supported {
			call.State, call.ErrorCode, call.ErrorMessage = "rejected", "binding_unavailable", "This canonical operation is unavailable in script binding version 1"
		} else if gate {
			basis, _, err := scriptBasis(st, a, in.Arguments)
			if err != nil {
				return false, err
			}
			call.Basis = basis
			call.State = "waiting"
			row.State = "waiting"
		} else {
			dispatch = true
		}
		for id := range st.Projects {
			if role(st, a, id) != "" && !containsProject(row.Projects, id) {
				row.Projects = append(row.Projects, id)
			}
		}
		row.Calls = append(row.Calls, call)
		st.ScriptRuns[row.ID] = row
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if !dispatch {
		return scriptCallResult(call), nil
	}
	permit := scriptPermit{RunID: in.ID, CallID: in.CallID, Operation: in.Operation, Fingerprint: digest(string(in.Arguments))}
	result, callErr := s.Call(context.WithValue(ctx, scriptDispatchKey{}, permit), token, in.Operation, in.Arguments)
	encoded, encodeErr := json.Marshal(result)
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		row := st.ScriptRuns[in.ID]
		for i := range row.Calls {
			current := &row.Calls[i]
			if current.ID != in.CallID || current.Fingerprint != fingerprint {
				continue
			}
			if callErr != nil {
				problem := ErrorResponse(callErr)
				current.State, current.ErrorCode, current.ErrorMessage = "rejected", problem.Code, problem.Message
				if problem.Status >= 500 || ctx.Err() != nil {
					current.State, current.ErrorCode, current.ErrorMessage = "unknown", "call_outcome_unknown", "Canonical effects are unconfirmed; no replay was attempted"
					row.State, row.ErrorCode = "unknown", "script_call_unconfirmed"
				}
			} else {
				current.State = "completed"
				used, _ := json.Marshal(row.Calls)
				if encodeErr != nil || len(encoded) > 128<<10 || len(used)+len(encoded) > 256<<10 {
					current.ErrorCode, current.ErrorMessage = "result_unavailable", "The canonical action completed, but its result exceeded retention; effects were not rolled back"
				} else {
					current.Result = encoded
				}
			}
			call = *current
			for id := range st.Projects {
				if owner := st.Accounts[row.Creator]; role(st, owner, id) != "" && !containsProject(row.Projects, id) {
					row.Projects = append(row.Projects, id)
				}
			}
			st.ScriptRuns[row.ID] = row
			return true, nil
		}
		return false, failure(409, "call_identity_lost", "Call outcome could not be recorded")
	})
	if err != nil {
		return nil, failure(503, "call_outcome_unknown", "Canonical effects may have committed; inspect receipts before another operation")
	}
	return scriptCallResult(call), nil
}
func containsProject(ids []int64, id int64) bool {
	for _, existing := range ids {
		if existing == id {
			return true
		}
	}
	return false
}

// PumpScripts observes the existing owner and never restarts source or calls.
func (s *Service) PumpScripts(ctx context.Context) error {
	s.scriptMu.Lock()
	defer s.scriptMu.Unlock()
	if s.localClient == nil {
		return nil
	}
	st, err := s.db.Read(ctx)
	if err != nil {
		return err
	}
	for _, row := range st.ScriptRuns {
		if !scriptLive(row.State) {
			continue
		}
		a, exists := st.Accounts[row.Creator]
		withdrawn := row.StopRequested || !exists || a.Suspended || !scriptContextAllowed(st, a, row) || !time.Now().Before(row.ExpiresAt)
		readRequest := scriptRequest(row, "read")
		readRequest.Cursor = row.Cursor
		out, observeErr := s.localClient.Call(ctx, readRequest)
		if observeErr != nil || out.Status == nil {
			err = s.db.Update(ctx, func(st *model.State) (bool, error) {
				current := st.ScriptRuns[row.ID]
				if current.State == "unknown" && (!withdrawn || current.StopRequested) {
					return false, nil
				}
				current.State, current.ErrorCode = "unknown", "script_owner_unavailable"
				if withdrawn {
					current.StopRequested = true
					if current.StopActor == "" {
						now := time.Now().UTC()
						current.StopActor = "script-supervisor"
						current.StopAt = &now
					}
					for i := range current.Calls {
						if contains([]string{"waiting", "approved"}, current.Calls[i].State) {
							current.Calls[i].State = "cancelled"
						}
					}
				}
				st.ScriptRuns[row.ID] = current
				return true, nil
			})
			if err != nil {
				return err
			}
			continue
		}
		status := *out.Status
		if withdrawn && status.State == "running" {
			_, _ = s.localClient.Call(ctx, scriptRequest(row, "stop"))
			status.State = "stopping"
		}
		var output []byte
		cursor := row.Cursor
		gap := out.Gap
		for count := 0; count < 4; count++ {
			output = append(output, out.Data...)
			cursor = out.Next
			if len(out.Data) < 65536 {
				break
			}
			request := scriptRequest(row, "read")
			request.Cursor = cursor
			out, observeErr = s.localClient.Call(ctx, request)
			if observeErr != nil {
				break
			}
			gap = gap || out.Gap
		}
		err = s.db.Update(ctx, func(st *model.State) (bool, error) {
			current := st.ScriptRuns[row.ID]
			changed := current.RuntimeState != status.State || len(output) > 0 || withdrawn && !current.StopRequested
			current.RuntimeState = status.State
			if current.State == "starting" && status.State == "running" {
				current.State = "running"
				changed = true
			}
			if withdrawn {
				current.StopRequested = true
				if current.StopActor == "" {
					now := time.Now().UTC()
					current.StopActor = "script-supervisor"
					current.StopAt = &now
				}
				current.State = "stopping"
				for i := range current.Calls {
					if contains([]string{"waiting", "approved"}, current.Calls[i].State) {
						current.Calls[i].State = "cancelled"
					}
				}
				if !time.Now().Before(current.ExpiresAt) {
					current.ErrorCode = "script_deadline"
				} else if !exists || a.Suspended {
					current.ErrorCode = "script_account_withdrawn"
				} else if !scriptContextAllowed(st, a, current) {
					current.ErrorCode = "script_context_withdrawn"
				}
			}
			if status.State == "unknown" {
				changed = true
				current.State, current.ErrorCode = "unknown", "script_owner_unavailable"
			}
			if gap {
				current.State, current.ErrorCode = "unknown", "script_output_gap"
			}
			if len(current.Output)+len(output) > scriptruntime.OutputLimit+4096 {
				current.State, current.ErrorCode = "unknown", "script_output_limit"
			} else {
				current.Output += string(output)
				current.Cursor = cursor
			}
			if status.State == "exited" || status.State == "failed" {
				now := time.Now().UTC()
				current.FinishedAt = &now
				var result scriptruntime.Outcome
				if current.StopRequested {
					current.State = "stopped"
				} else if current.ErrorCode == "script_output_gap" || current.ErrorCode == "script_output_limit" {
					current.State = "interrupted"
				} else if json.Unmarshal([]byte(strings.TrimSpace(current.Output)), &result) == nil {
					if result.Success {
						current.State = "completed"
					} else {
						current.State = "failed"
					}
				} else {
					current.State, current.ErrorCode = "interrupted", "script_result_unavailable"
				}
				for i := range current.Calls {
					call := &current.Calls[i]
					if call.State == "dispatching" || call.State == "unknown" {
						call.State, call.ErrorCode, call.ErrorMessage = "unknown", "call_outcome_unknown", "The recorded dispatch outcome is unresolved; no replay was attempted"
						if !current.StopRequested {
							current.State, current.ErrorCode = "unknown", "script_call_unconfirmed"
						}
					}
				}
				changed = true
			}
			st.ScriptRuns[row.ID] = current
			return changed, nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
