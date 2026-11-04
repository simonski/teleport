package app

import (
	"context"
	"encoding/json"
	"teleport/internal/model"
)

// freezeAttribution is called inside the launch reservation transaction.
func freezeAttribution(st *model.State, a model.Account, project int64, ticketID int64, agent *model.AgentDefinition, usageID string, supervised bool) *model.ExecutionAttribution {
	p := st.Projects[project]
	out := &model.ExecutionAttribution{ProjectID: project, ProjectVersion: p.Version, HierarchyStatus: "not_applicable"}
	switch {
	case supervised:
		out.CallerKind = "system"
		out.CallerID = "native-supervisor"
	case a.ServiceSessionID != "":
		out.CallerKind = "agent"
		out.CallerID = a.ID
		out.ParentSessionID = a.ServiceSessionID
		if parent, ok := st.LocalSessions[a.ServiceSessionID]; ok {
			if parent.Attribution != nil {
				out.InitiatorID = parent.Attribution.InitiatorID
			} else if parent.Managed == nil {
				out.InitiatorID = parent.Creator
			}
		}
	case a.ServiceRunID != "":
		out.CallerKind = "automation"
		out.CallerID = a.ServiceRunID
		out.CallerCallID = a.ServiceCallID
		out.InitiatorID = a.ID
	default:
		out.CallerKind = "human"
		out.CallerID = a.ID
		out.InitiatorID = a.ID
	}
	if agent != nil {
		out.AgentID = agent.ID
		out.AgentName = agent.Name
		out.AgentVersion = agent.Version
	}
	out.UsageID = usageID
	if ticketID == 0 {
		return out
	}
	t, ok := st.Tickets[ticketID]
	if !ok || t.ProjectID != project {
		out.HierarchyStatus = "unknown"
		return out
	}
	out.TicketID = t.ID
	out.TicketVersion = t.Version
	out.HierarchyStatus = "unknown"
	if t.Kind == "goal" {
		out.GoalID = t.ID
		out.GoalVersion = t.Version
		out.HierarchyStatus = "complete"
		return out
	}
	if t.Kind == "epic" {
		out.EpicID = t.ID
		out.EpicVersion = t.Version
	} else if t.Kind == "task" {
		epic, ok := st.Tickets[t.ParentID]
		if !ok || epic.Kind != "epic" || epic.ProjectID != project {
			return out
		}
		out.EpicID = epic.ID
		out.EpicVersion = epic.Version
		t = epic
	} else {
		return out
	}
	goal, ok := st.Tickets[t.ParentID]
	if !ok || goal.Kind != "goal" || goal.ProjectID != project {
		return out
	}
	out.GoalID = goal.ID
	out.GoalVersion = goal.Version
	out.HierarchyStatus = "complete"
	return out
}

// Retained sessions predate hierarchy snapshots. Preserve only fields whose
// original attribution is already stored on the session.
func retainedAttribution(row model.LocalSession) model.ExecutionAttribution {
	if row.Attribution != nil {
		return *row.Attribution
	}
	out := model.ExecutionAttribution{ProjectID: row.ProjectID, CallerKind: "unknown", HierarchyStatus: "unknown"}
	if row.Managed == nil && row.Creator != "" {
		out.CallerKind = "human"
		out.CallerID = row.Creator
		out.InitiatorID = row.Creator
	}
	if row.Persona != nil {
		out.AgentID = row.Persona.ID
		out.AgentName = row.Persona.Name
		out.AgentVersion = row.Persona.Version
	}
	if row.Spellcheck != nil {
		out.UsageID = row.Spellcheck.UsageID
		out.TicketID = row.Spellcheck.TicketID
		out.TicketVersion = row.Spellcheck.TicketVersion
	} else if row.TicketID != 0 {
		out.TicketID = row.TicketID
	}
	return out
}

func (s *Service) usageAttribution(ctx context.Context, token string, raw json.RawMessage) (any, error) {
	var in SessionIDInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	st, err := s.db.Read(ctx)
	if err != nil {
		return nil, err
	}
	a, err := directoryActor(st, token)
	if err != nil {
		return nil, err
	}
	row, ok := st.LocalSessions[in.ID]
	if !ok {
		return nil, failure(404, "not_found", "Usage session not found")
	}
	if err = localAllowed(st, a, row); err != nil {
		return nil, err
	}
	out := retainedAttribution(row)
	if out.TicketID != 0 {
		ticket, exists := st.Tickets[out.TicketID]
		if !exists || role(st, a, ticket.ProjectID) == "" {
			out.TicketID = 0
			out.TicketVersion = 0
			out.EpicID = 0
			out.EpicVersion = 0
			out.GoalID = 0
			out.GoalVersion = 0
			out.HierarchyStatus = "unavailable"
		}
	}
	return out, nil
}
