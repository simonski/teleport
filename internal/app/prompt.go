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
	if !t.Agent {
		return nil, false, failure(409, "agent_ticket_required", "Human tickets have no harness prompt")
	}
	phase, ok := phaseByID(st.Workflows[t.Workflow], t.Phase)
	if !ok {
		return nil, false, failure(409, "workflow_mapping", "Pinned phase is unavailable")
	}
	view := PromptView{TicketID: t.ID, TicketVersion: t.Version, Workflow: t.Workflow, Phase: t.Phase, Basis: executionBasis(st, t), Prompt: composeManagedPrompt(st, t, phase)}
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
