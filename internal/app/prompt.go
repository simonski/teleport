package app

import (
	"encoding/json"
	"teleport/internal/model"
)

type PromptInput struct {
	ID        int64  `json:"id"`
	SessionID string `json:"sessionId,omitempty"`
}
type PromptView struct {
	TicketID      int64                `json:"ticketId"`
	TicketVersion int64                `json:"ticketVersion"`
	Workflow      string               `json:"workflow"`
	Phase         string               `json:"phase"`
	Basis         string               `json:"basis"`
	SessionID     string               `json:"sessionId,omitempty"`
	Saved         bool                 `json:"saved"`
	Hypothetical  bool                 `json:"hypothetical,omitempty"`
	Prompt        model.PromptSnapshot `json:"prompt"`
}

func ticketPrompt(st *model.State, a model.Account, raw json.RawMessage) (any, bool, error) {
	var in PromptInput
	if err := parse(raw, &in); err != nil {
		return nil, false, err
	}
	if a.ServiceSessionID != "" || a.ServiceRunID != "" {
		return nil, false, failure(403, "human_required", "Prompt inspection requires a human account")
	}
	t, err := ticketFor(st, a, in.ID, "Viewer")
	if err != nil {
		return nil, false, err
	}
	preview := t
	if !t.Agent {
		if in.SessionID != "" {
			return nil, false, failure(404, "prompt_not_found", "No saved managed launch prompt matches this ticket")
		}
		preview.Workflow = st.Projects[t.ProjectID].AgentWorkflow
		phase, ok := phaseForRole(st.Workflows[preview.Workflow], "Refine")
		if !ok {
			return nil, false, failure(409, "workflow_mapping", "The project agent workflow needs one Refine phase for this preview")
		}
		preview.Agent, preview.Phase = true, phase.ID
	}
	phase, ok := phaseByID(st.Workflows[preview.Workflow], preview.Phase)
	if !ok {
		return nil, false, failure(409, "workflow_mapping", "Pinned phase is unavailable")
	}
	view := PromptView{TicketID: t.ID, TicketVersion: t.Version, Workflow: preview.Workflow, Phase: preview.Phase, Basis: executionBasis(st, t), Hypothetical: !t.Agent, Prompt: composeManagedPrompt(st, preview, phase)}
	if in.SessionID != "" {
		row, ok := st.LocalSessions[in.SessionID]
		if !ok || row.TicketID != t.ID || row.Managed == nil {
			return nil, false, failure(404, "prompt_not_found", "No saved managed launch prompt matches this ticket")
		}
		if err := localAllowed(st, a, row); err != nil {
			return nil, false, err
		}
		view.Saved, view.SessionID, view.Workflow, view.Phase, view.Basis, view.Prompt = true, row.ID, row.Managed.Workflow, row.Managed.Phase, row.Managed.Basis, row.Managed.Prompt
		view.TicketVersion = row.Managed.RequestVersion
	}
	return view, false, nil
}
