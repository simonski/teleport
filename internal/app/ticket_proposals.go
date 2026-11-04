package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"teleport/internal/model"
)

type TicketProposalInput struct {
	Kind            string                     `json:"kind,omitempty"`
	Usage           *model.TokenEvidence       `json:"usage,omitempty"`
	TicketID        int64                      `json:"ticketId"`
	ExpectedVersion int64                      `json:"expectedVersion"`
	RequestID       string                     `json:"requestId"`
	Title           string                     `json:"title"`
	PreviousID      string                     `json:"previousId,omitempty"`
	Items           []model.TicketProposalItem `json:"items"`
}
type TicketProposalFeedbackInput struct {
	TicketID        int64  `json:"ticketId"`
	ProposalID      string `json:"proposalId"`
	ExpectedVersion int64  `json:"expectedVersion"`
	RequestID       string `json:"requestId"`
	Text            string `json:"text"`
	ItemKey         string `json:"itemKey,omitempty"`
}
type TicketProposalList struct {
	Items      []model.TicketProposal `json:"items"`
	NextCursor string                 `json:"nextCursor,omitempty"`
}
type TicketProposalListInput struct {
	TicketID int64  `json:"ticketId"`
	Cursor   string `json:"cursor,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type TicketProposalAcceptInput struct {
	TicketID              int64  `json:"ticketId"`
	ProposalID            string `json:"proposalId"`
	ExpectedVersion       int64  `json:"expectedVersion"`
	ExpectedTicketVersion int64  `json:"expectedTicketVersion"`
}
type TicketProposalDiscardInput struct {
	TicketID              int64  `json:"ticketId"`
	ExpectedTicketVersion int64  `json:"expectedTicketVersion"`
	LatestID              string `json:"latestId"`
	LatestVersion         int64  `json:"latestVersion"`
	RequestID             string `json:"requestId"`
}
type TicketProposalDiscardResult struct {
	TicketID  int64 `json:"ticketId"`
	Discarded int   `json:"discarded"`
}
type ProposalRunUsageInput struct {
	Usage *model.TokenEvidence `json:"usage"`
}
type proposalFeedbackReceipt struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}

func ticketProposalOperations(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	if a.ServiceRunID != "" || a.ServiceSessionID != "" && operation != "submitTicketProposal" && operation != "getProposalRunContext" && operation != "reportProposalRunUsage" {
		return nil, false, failure(403, "human_required", "A human account is required")
	}
	if a.ServiceSessionReadOnly && (operation == "submitTicketProposal" || operation == "reportProposalRunUsage") {
		return nil, false, failure(403, "agent_scope", "This harness credential permits context reads only")
	}
	if len(raw) > 256*1024 {
		return nil, false, failure(413, "proposal_too_large", "Proposal exceeds the limit")
	}
	if operation == "reportProposalRunUsage" {
		var in ProposalRunUsageInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		row, ok := st.LocalSessions[a.ServiceSessionID]
		if a.ServiceSessionID == "" || !ok || row.ProposalRun == nil || row.State != "running" || row.StopRequested {
			return nil, false, failure(403, "agent_scope", "Proposal run is unavailable")
		}
		if !validProposalEvidence(row, in.Usage) {
			return nil, false, failure(422, "invalid_usage", "Invalid proposal usage evidence")
		}
		if row.ProposalRun.Usage != nil {
			if reflect.DeepEqual(row.ProposalRun.Usage, in.Usage) {
				return map[string]bool{"recorded": true}, false, nil
			}
			return nil, false, failure(409, "usage_conflict", "Usage evidence was already recorded")
		}
		row.ProposalRun.Usage = in.Usage
		applyUsagePrice(st, &row)
		st.LocalSessions[row.ID] = row
		return map[string]bool{"recorded": true}, true, nil
	}
	if operation == "getProposalRunContext" {
		if err := parse(raw, &struct{}{}); err != nil {
			return nil, false, err
		}
		if a.ServiceSessionID == "" {
			return nil, false, failure(403, "agent_required", "A proposal run is required")
		}
		row, ok := st.LocalSessions[a.ServiceSessionID]
		if !ok || row.ProposalRun == nil || row.State != "running" || row.StopRequested {
			return nil, false, failure(403, "agent_scope", "Proposal run is unavailable")
		}
		job := row.ProposalRun
		t := st.Tickets[job.TicketID]
		u := st.AgentUsages[job.UsageID]
		if t.ID == 0 || t.ProjectID != row.ProjectID || t.Version != job.TicketVersion || t.Archived || t.Deleted || u.Removed || !u.Enabled || u.Version != job.UsageVersion || u.Kind != "todo-proposal" || !usageMatchesTicket(u, t) {
			return nil, false, failure(409, "proposal_run_changed", "Proposal context changed")
		}
		if job.PreviousID == "" {
			return map[string]any{"previous": nil}, false, nil
		}
		p, ok := st.TicketProposals[job.PreviousID]
		if !ok || p.TicketID != t.ID || p.ProjectID != t.ProjectID || p.Version != job.PreviousVersion {
			return nil, false, failure(409, "proposal_stale", "Proposal feedback changed")
		}
		return map[string]any{"previous": p}, false, nil
	}
	if operation == "listTicketProposals" {
		var in TicketProposalListInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		t, err := ticketFor(st, a, in.TicketID, "Viewer")
		if err != nil {
			return nil, false, err
		}
		if in.Limit < 0 || len(in.Cursor) > 160 {
			return nil, false, failure(400, "invalid_page", "Invalid proposal pagination")
		}
		limit := in.Limit
		if limit == 0 || limit > 50 {
			limit = 50
		}
		out := TicketProposalList{Items: []model.TicketProposal{}}
		rows := []model.TicketProposal{}
		for _, p := range st.TicketProposals {
			if p.TicketID == in.TicketID && p.ProjectID == t.ProjectID && proposalCursor(p) > in.Cursor {
				p = visibleProposal(st, a, p)
				p.Ready = proposalRunReady(st, p)
				rows = append(rows, p)
			}
		}
		sort.Slice(rows, func(i, j int) bool { return proposalCursor(rows[i]) < proposalCursor(rows[j]) })
		size := 0
		for _, p := range rows {
			encoded, _ := json.Marshal(p)
			if len(out.Items) > 0 && (len(out.Items) >= limit || size+len(encoded) > 1<<20) {
				out.NextCursor = proposalCursor(out.Items[len(out.Items)-1])
				break
			}
			out.Items = append(out.Items, p)
			size += len(encoded)
		}
		return out, false, nil
	}
	if operation == "discardTicketProposals" {
		var in TicketProposalDiscardInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		t, err := ticketFor(st, a, in.TicketID, "Contributor")
		if err != nil {
			return nil, false, err
		}
		key, hash, prior, err := receipt(st, a, operation, in.RequestID, in)
		if err != nil {
			return nil, false, err
		}
		if prior != nil {
			var saved TicketProposalDiscardResult
			if json.Unmarshal(prior, &saved) != nil || saved.TicketID != t.ID {
				return nil, false, failure(500, "invalid_receipt", "Invalid discard receipt")
			}
			return saved, false, nil
		}
		if t.Version != in.ExpectedTicketVersion {
			return nil, false, failure(409, "proposal_stale", "Ticket changed; review proposals again")
		}
		if proposalOwnsTicket(st, t.ID) {
			return nil, false, failure(409, "proposal_running", "Stop the planning run and confirm exit before discarding proposals")
		}
		var latest model.TicketProposal
		for _, p := range st.TicketProposals {
			if p.TicketID == t.ID && p.ProjectID == t.ProjectID && (latest.ID == "" || proposalCursor(p) > proposalCursor(latest)) {
				latest = p
			}
		}
		if latest.ID == "" || latest.ID != in.LatestID || latest.Version != in.LatestVersion {
			return nil, false, failure(409, "proposal_stale", "Proposals changed; review them again")
		}
		result := TicketProposalDiscardResult{TicketID: t.ID}
		now := time.Now().UTC()
		for id, p := range st.TicketProposals {
			if p.TicketID == t.ID && p.ProjectID == t.ProjectID && p.AcceptedBy == "" && p.DiscardedBy == "" {
				p.DiscardedBy, p.DiscardedAt, p.Version = a.ID, &now, p.Version+1
				st.TicketProposals[id] = p
				result.Discarded++
			}
		}
		if result.Discarded == 0 {
			return nil, false, failure(409, "no_pending_proposals", "There are no proposals to discard")
		}
		remember(st, key, hash, result)
		return result, true, nil
	}
	if operation == "commentTicketProposal" {
		var in TicketProposalFeedbackInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		t, err := ticketFor(st, a, in.TicketID, "Contributor")
		if err != nil {
			return nil, false, err
		}
		key, hash, prior, err := receipt(st, a, operation, in.RequestID, in)
		if err != nil {
			return nil, false, err
		}
		if prior != nil {
			var saved proposalFeedbackReceipt
			if json.Unmarshal(prior, &saved) != nil {
				return nil, false, failure(500, "invalid_receipt", "Invalid proposal receipt")
			}
			p, ok := st.TicketProposals[saved.ID]
			if !ok || saved.Version < 2 || saved.Version > p.Version || int64(len(p.Feedback)) < saved.Version-1 {
				return nil, false, failure(500, "invalid_receipt", "Saved proposal feedback is unavailable")
			}
			if p.TicketID != t.ID || p.ProjectID != t.ProjectID {
				return nil, false, failure(404, "not_found", "Proposal not found")
			}
			p.Version = saved.Version
			p.Feedback = append([]model.ProposalFeedback{}, p.Feedback[:saved.Version-1]...)
			p.AcceptedBy = ""
			p.AcceptedAt = nil
			p.DiscardedBy = ""
			p.DiscardedAt = nil
			p.CreatedIDs = nil
			return p, false, nil
		}
		p, ok := st.TicketProposals[in.ProposalID]
		if !ok || p.TicketID != in.TicketID || p.ProjectID != t.ProjectID {
			return nil, false, failure(404, "not_found", "Proposal not found")
		}
		if p.Version != in.ExpectedVersion {
			return nil, false, failure(409, "version_conflict", "Proposal changed")
		}
		if p.DiscardedBy != "" {
			return nil, false, failure(409, "proposal_discarded", "Discarded proposals cannot receive feedback")
		}
		if p.AcceptedBy != "" {
			return nil, false, failure(409, "proposal_accepted", "Accepted proposals cannot receive new feedback")
		}
		for _, later := range st.TicketProposals {
			if later.PreviousID == p.ID {
				return nil, false, failure(409, "proposal_superseded", "Give feedback on the latest proposal")
			}
		}
		if !proposalRunReady(st, p) {
			return nil, false, failure(409, "proposal_run_unfinished", "The agent run has not completed successfully")
		}
		in.Text = strings.TrimSpace(in.Text)
		if in.Text == "" || len(in.Text) > 20000 || len(p.Feedback) >= 100 {
			return nil, false, failure(422, "invalid_feedback", "Provide bounded feedback")
		}
		if in.ItemKey != "" {
			found := false
			for _, item := range p.Items {
				if item.Key == in.ItemKey {
					found = true
					break
				}
			}
			if !found {
				return nil, false, failure(422, "invalid_feedback", "Feedback item is unavailable")
			}
		}
		old := p
		p.Feedback = append(append([]model.ProposalFeedback{}, p.Feedback...), model.ProposalFeedback{Actor: a.ID, Text: in.Text, ItemKey: in.ItemKey, At: time.Now().UTC()})
		p.Version++
		if !proposalStorageWithinLimits(st, p, &old) {
			return nil, false, failure(413, "proposal_too_large", "Proposal or retained ticket proposals exceed the storage limit")
		}
		st.TicketProposals[p.ID] = p
		remember(st, key, hash, proposalFeedbackReceipt{ID: p.ID, Version: p.Version})
		return p, true, nil
	}
	if operation == "acceptTicketProposal" {
		var in TicketProposalAcceptInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		t, err := ticketFor(st, a, in.TicketID, "Contributor")
		if err != nil {
			return nil, false, err
		}
		p, ok := st.TicketProposals[in.ProposalID]
		if !ok || p.TicketID != t.ID || p.ProjectID != t.ProjectID {
			return nil, false, failure(404, "not_found", "Proposal not found")
		}
		if p.AcceptedBy != "" {
			return visibleProposal(st, a, p), false, nil
		}
		if p.DiscardedBy != "" {
			return nil, false, failure(409, "proposal_discarded", "Discarded proposals cannot be accepted")
		}
		if !proposalRunReady(st, p) {
			return nil, false, failure(409, "proposal_run_unfinished", "The agent run has not completed successfully")
		}
		if p.Version != in.ExpectedVersion || t.Version != in.ExpectedTicketVersion || t.Version != p.TicketVersion {
			return nil, false, failure(409, "proposal_stale", "Ticket or proposal changed; review a current proposal")
		}
		for _, later := range st.TicketProposals {
			if later.PreviousID == p.ID {
				return nil, false, failure(409, "proposal_superseded", "Review the later proposal instead")
			}
		}
		if t.Archived || t.Deleted || t.Protected || t.Kind != "task" {
			return nil, false, failure(409, "proposal_unavailable", "Source ticket is unavailable")
		}
		epic, ok := st.Tickets[t.ParentID]
		if !ok || epic.Kind != "epic" || epic.ProjectID != t.ProjectID {
			return nil, false, failure(409, "invalid_hierarchy", "Source epic is unavailable")
		}
		goal, ok := st.Tickets[epic.ParentID]
		if !ok || goal.Kind != "goal" || goal.ProjectID != t.ProjectID {
			return nil, false, failure(409, "invalid_hierarchy", "Source goal is unavailable")
		}
		created := map[string]int64{}
		ids := []int64{}
		agent := false
		for _, item := range p.Items {
			parent := goal.ID
			if item.Kind == "task" {
				parent = created[item.ParentKey]
				if parent == 0 {
					return nil, false, failure(409, "invalid_hierarchy", "Proposed task parent is unavailable")
				}
			}
			body, _ := json.Marshal(CreateTicketInput{ProjectID: t.ProjectID, ParentID: parent, Kind: item.Kind, Title: item.Title, Description: item.Description, Criteria: item.Criteria, Agent: &agent, RequestID: randomID()})
			result, _, createErr := projectOperations(st, a, "createTicket", body)
			if createErr != nil {
				return nil, false, createErr
			}
			row := result.(model.Ticket)
			created[item.Key] = row.ID
			ids = append(ids, row.ID)
		}
		now := time.Now().UTC()
		p.AcceptedBy = a.ID
		p.AcceptedAt = &now
		p.CreatedIDs = ids
		p.Version++
		st.TicketProposals[p.ID] = p
		return p, true, nil
	}
	var in TicketProposalInput
	if err := parse(raw, &in); err != nil {
		return nil, false, err
	}
	minimum := "Contributor"
	if a.ServiceSessionID != "" {
		minimum = "Viewer"
	}
	t, err := ticketFor(st, a, in.TicketID, minimum)
	if err != nil {
		return nil, false, err
	}
	if a.ServiceSessionID != "" {
		row := st.LocalSessions[a.ServiceSessionID]
		job := row.ProposalRun
		if job == nil || row.State != "running" || row.StopRequested || job.TicketID != t.ID || row.ProjectID != t.ProjectID || job.TicketVersion != t.Version || job.PreviousID != in.PreviousID || job.TicketVersion != in.ExpectedVersion {
			return nil, false, failure(403, "agent_scope", "Proposal session is not authorized for this ticket and version")
		}
		usage := st.AgentUsages[job.UsageID]
		owner := st.Accounts[row.Creator]
		project := st.Projects[t.ProjectID]
		workflow := st.Workflows[t.Workflow]
		if t.Archived || t.Deleted || t.Protected || t.StopRequested || t.ResumeHold || st.Settings.Handbrake || project.Handbrake || project.LaneLocks[workflow.ID+"#*"] || project.LaneLocks[workflow.ID+"#"+t.Phase] || usage.Removed || !usage.Enabled || usage.Kind != "todo-proposal" || usage.Version != job.UsageVersion || !usageMatchesTicket(usage, t) || owner.ID == "" || owner.Suspended || require(st, owner, t.ProjectID, "Contributor") != nil || row.Persona == nil || st.AgentDefinitions[row.Persona.ID].Archived || st.AgentDefinitions[row.Persona.ID].Version != row.Persona.Version {
			return nil, false, failure(409, "proposal_run_changed", "Proposal authority or configuration changed")
		}
		if job.PreviousID != "" && st.TicketProposals[job.PreviousID].Version != job.PreviousVersion {
			return nil, false, failure(409, "proposal_stale", "Feedback changed during the run")
		}
	}
	key, hash, prior, err := receipt(st, a, operation, in.RequestID, in)
	if err != nil {
		return nil, false, err
	}
	if prior != nil {
		var p model.TicketProposal
		if json.Unmarshal(prior, &p) != nil {
			return nil, false, failure(500, "invalid_receipt", "Invalid proposal receipt")
		}
		if p.TicketID != t.ID || p.ProjectID != t.ProjectID {
			return nil, false, failure(404, "not_found", "Proposal not found")
		}
		return p, false, nil
	}
	if a.ServiceSessionID != "" && st.LocalSessions[a.ServiceSessionID].ProposalRun.ProposalID != "" {
		return nil, false, failure(409, "proposal_already_submitted", "This run already submitted a proposal")
	}
	if in.Usage != nil {
		u := in.Usage
		row := st.LocalSessions[a.ServiceSessionID]
		if a.ServiceSessionID == "" || !validProposalEvidence(row, u) || row.ProposalRun.Usage != nil && !reflect.DeepEqual(row.ProposalRun.Usage, u) {
			return nil, false, failure(422, "invalid_usage", "Invalid proposal usage evidence")
		}
	}
	if err = expected(t, in.ExpectedVersion); err != nil {
		return nil, false, err
	}
	if in.Kind == "" {
		in.Kind = "decomposition"
	}
	if in.Kind != "decomposition" {
		return nil, false, failure(422, "unsupported_proposal", "Unsupported proposal type")
	}
	if t.Archived || t.Deleted || t.Protected || t.Kind != "task" || strings.TrimSpace(in.Title) == "" || len(in.Title) > 500 || len(in.Items) == 0 || len(in.Items) > 100 {
		return nil, false, failure(422, "invalid_proposal", "Provide a visible task and bounded proposed work")
	}
	retained := 0
	for _, existing := range st.TicketProposals {
		if existing.TicketID == t.ID && existing.ProjectID == t.ProjectID {
			retained++
		}
	}
	if retained >= 500 {
		return nil, false, failure(409, "proposal_capacity", "This ticket has reached its retained proposal limit")
	}
	if in.PreviousID != "" {
		p, ok := st.TicketProposals[in.PreviousID]
		if !ok || p.TicketID != t.ID || p.ProjectID != t.ProjectID || p.AcceptedBy != "" || p.DiscardedBy != "" {
			return nil, false, failure(422, "invalid_proposal", "Previous proposal is unavailable")
		}
		for _, later := range st.TicketProposals {
			if later.PreviousID == p.ID {
				return nil, false, failure(409, "proposal_superseded", "Revise the latest proposal")
			}
		}
	}
	keys := map[string]string{}
	for _, item := range in.Items {
		if item.Key == "" || len(item.Key) > 80 || keys[item.Key] != "" || !contains([]string{"epic", "task"}, item.Kind) || strings.TrimSpace(item.Title) == "" || len(item.Title) > 500 || len(item.Description) > 20000 || len(item.Criteria) > 20000 {
			return nil, false, failure(422, "invalid_proposal", "Invalid proposed item")
		}
		if item.Kind == "epic" && item.ParentKey != "" || item.Kind == "task" && keys[item.ParentKey] != "epic" {
			return nil, false, failure(422, "invalid_proposal", "Tasks require an earlier proposed epic")
		}
		keys[item.Key] = item.Kind
	}
	if st.TicketProposals == nil {
		st.TicketProposals = map[string]model.TicketProposal{}
	}
	p := model.TicketProposal{ID: "ticket-proposal:" + randomID(), Kind: in.Kind, Version: 1, TicketID: t.ID, TicketVersion: t.Version, ProjectID: t.ProjectID, Title: in.Title, PreviousID: in.PreviousID, Actor: a.ID, At: time.Now().UTC(), Items: in.Items, Feedback: []model.ProposalFeedback{}}
	if a.ServiceSessionID != "" {
		row := st.LocalSessions[a.ServiceSessionID]
		p.SessionID, p.UsageID, p.UsageVersion, p.AgentID, p.AgentVersion = row.ID, row.ProposalRun.UsageID, row.ProposalRun.UsageVersion, row.Persona.ID, row.Persona.Version
	}
	if !proposalStorageWithinLimits(st, p, nil) {
		return nil, false, failure(413, "proposal_too_large", "Proposal or retained ticket proposals exceed the storage limit")
	}
	st.TicketProposals[p.ID] = p
	if a.ServiceSessionID != "" {
		row := st.LocalSessions[a.ServiceSessionID]
		row.ProposalRun.ProposalID = p.ID
		if in.Usage != nil {
			row.ProposalRun.Usage = in.Usage
		}
		applyUsagePrice(st, &row)
		st.LocalSessions[row.ID] = row
	}
	remember(st, key, hash, p)
	return p, true, nil
}

func visibleProposal(st *model.State, a model.Account, p model.TicketProposal) model.TicketProposal {
	if len(p.CreatedIDs) == 0 {
		return p
	}
	ids := make([]int64, 0, len(p.CreatedIDs))
	for _, id := range p.CreatedIDs {
		if _, err := ticketFor(st, a, id, "Viewer"); err == nil {
			ids = append(ids, id)
		}
	}
	p.CreatedIDs = ids
	return p
}
func proposalStorageWithinLimits(st *model.State, p model.TicketProposal, old *model.TicketProposal) bool {
	body, err := json.Marshal(p)
	if err != nil || len(body) > 256*1024 {
		return false
	}
	total := len(body)
	for _, other := range st.TicketProposals {
		if other.TicketID == p.TicketID && other.ProjectID == p.ProjectID && (old == nil || other.ID != old.ID) {
			data, e := json.Marshal(other)
			if e != nil {
				return false
			}
			total += len(data)
			if total > 4<<20 {
				return false
			}
		}
	}
	return total <= 4<<20
}

func validProposalEvidence(row model.LocalSession, u *model.TokenEvidence) bool {
	return u != nil && row.Persona != nil && (row.Persona.Harness == "codex" && u.Source == "codex-exec-json" && u.EstimateUSDMicros == nil || row.Persona.Harness == "claude" && u.Source == "claude-print-json") && u.Input >= 0 && u.Input <= 3_000_000_000_000 && u.Output >= 0 && u.Output <= 1_000_000_000_000 && (u.Cached == nil || *u.Cached >= 0 && *u.Cached <= u.Input) && (u.EstimateUSDMicros == nil || *u.EstimateUSDMicros >= 0 && *u.EstimateUSDMicros < 1_000_000_000_000_000)
}

func proposalCursor(p model.TicketProposal) string {
	return fmt.Sprintf("%020d:%s", p.At.UnixNano(), p.ID)
}

func proposalRunReady(st *model.State, p model.TicketProposal) bool {
	if p.SessionID == "" {
		return true
	}
	row, ok := st.LocalSessions[p.SessionID]
	return ok && row.ProposalRun != nil && row.ProposalRun.ProposalID == p.ID && row.State == "exited" && row.ExitCode != nil && *row.ExitCode == 0 && !row.StopRequested
}

// proposalAttention derives the latest reviewable proposal for each ticket.
// The flag is returned on ticket reads and is never written into ticket history.
func proposalAttention(st *model.State) map[int64]bool {
	latest := make(map[int64]model.TicketProposal)
	for _, p := range st.TicketProposals {
		t, ok := st.Tickets[p.TicketID]
		if !ok || t.ProjectID != p.ProjectID {
			continue
		}
		if previous, exists := latest[p.TicketID]; !exists || proposalCursor(p) > proposalCursor(previous) {
			latest[p.TicketID] = p
		}
	}
	attention := make(map[int64]bool)
	for id, p := range latest {
		attention[id] = p.AcceptedBy == "" && p.DiscardedBy == "" && proposalRunReady(st, p)
	}
	return attention
}

func proposalAdmission(st *model.State, t model.Ticket) error {
	if t.Kind != "task" || t.Archived || t.Deleted || t.Protected || busy(t) || t.StopRequested || t.ResumeHold {
		return failure(409, "proposal_unavailable", "Planning requires an available visible task")
	}
	w := st.Workflows[t.Workflow]
	phase, ok := phaseByID(w, t.Phase)
	if !ok {
		return failure(409, "workflow_mapping", "Ticket lane is unavailable")
	}
	p := st.Projects[t.ProjectID]
	if st.Settings.Handbrake || p.Handbrake || p.LaneLocks[w.ID+"#*"] || p.LaneLocks[w.ID+"#"+t.Phase] {
		return failure(409, "handbrake", "An applicable handbrake blocks planning")
	}
	if proposalOwnsTicket(st, t.ID) {
		return failure(409, "proposal_running", "A planning run already owns this ticket")
	}
	phase.Role = "Refine"
	return phaseCapacity(st, t, phase)
}

func proposalOwnsTicket(st *model.State, id int64) bool {
	for _, row := range st.LocalSessions {
		if row.ProposalRun != nil && row.ProposalRun.TicketID == id && activeLocal(row) {
			return true
		}
	}
	return false
}
