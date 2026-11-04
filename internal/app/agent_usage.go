package app

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"teleport/internal/model"
)

type AgentUsageInput struct {
	ID              string `json:"id,omitempty"`
	ProjectID       int64  `json:"projectId"`
	ExpectedVersion int64  `json:"expectedVersion,omitempty"`
	RequestID       string `json:"requestId"`
	Name            string `json:"name"`
	AgentID         string `json:"agentId"`
	Kind            string `json:"kind,omitempty"`
	View            string `json:"view,omitempty"`
	Workflow        string `json:"workflow,omitempty"`
	Phase           string `json:"phase,omitempty"`
	Enabled         bool   `json:"enabled"`
}
type RemoveAgentUsageInput struct {
	ID              string `json:"id"`
	ProjectID       int64  `json:"projectId"`
	ExpectedVersion int64  `json:"expectedVersion"`
	RequestID       string `json:"requestId"`
}

type AgentUsageList struct {
	Items []model.AgentUsage `json:"items"`
}

func agentUsageCall(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	if a.ServiceRunID != "" || a.ServiceSessionID != "" {
		return nil, false, failure(403, "human_required", "A human account is required")
	}
	if operation == "listAgentUsages" {
		var in struct {
			ProjectID int64 `json:"projectId"`
		}
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		if err := require(st, a, in.ProjectID, "Viewer"); err != nil {
			return nil, false, err
		}
		items := []model.AgentUsage{}
		for _, usage := range st.AgentUsages {
			if usage.ProjectID == in.ProjectID && !usage.Removed {
				items = append(items, usage)
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		return AgentUsageList{Items: items}, false, nil
	}
	if operation == "removeAgentUsage" {
		var in RemoveAgentUsageInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		if err := require(st, a, in.ProjectID, "Maintainer"); err != nil {
			return nil, false, err
		}
		key, hash, previous, err := receipt(st, a, operation, in.RequestID, in)
		if err != nil {
			return nil, false, err
		}
		if previous != nil {
			var result model.AgentUsage
			if json.Unmarshal(previous, &result) != nil {
				return nil, false, failure(500, "invalid_receipt", "Saved usage receipt is invalid")
			}
			return result, false, nil
		}
		row, ok := st.AgentUsages[in.ID]
		if !ok || row.ProjectID != in.ProjectID || row.Removed {
			return nil, false, failure(404, "not_found", "Agent usage not found")
		}
		if row.Version != in.ExpectedVersion {
			return nil, false, failure(409, "version_conflict", "Agent usage changed; reload before removing")
		}
		row.Removed = true
		row.Enabled = false
		row.Version++
		row.UpdatedBy = a.ID
		row.UpdatedAt = time.Now().UTC()
		st.AgentUsages[in.ID] = row
		remember(st, key, hash, row)
		return row, true, nil
	}
	var in AgentUsageInput
	if err := parse(raw, &in); err != nil {
		return nil, false, err
	}
	if err := require(st, a, in.ProjectID, "Maintainer"); err != nil {
		return nil, false, err
	}
	key, hash, previous, err := receipt(st, a, operation, in.RequestID, in)
	if err != nil {
		return nil, false, err
	}
	if previous != nil {
		var row model.AgentUsage
		if json.Unmarshal(previous, &row) != nil {
			return nil, false, failure(500, "invalid_receipt", "Saved usage receipt is invalid")
		}
		return row, false, nil
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Kind == "" {
		if old, ok := st.AgentUsages[in.ID]; ok {
			in.Kind = old.Kind
		} else {
			in.Kind = "todo-spellcheck"
		}
	}
	if !contains([]string{"todo-spellcheck", "todo-conversation", "todo-refine", "todo-proposal"}, in.Kind) {
		return nil, false, failure(422, "invalid_usage_kind", "Choose a supported usage")
	}
	if in.Kind == "todo-refine" && (in.View == "" || in.Workflow == "" || in.Phase == "") {
		return nil, false, failure(422, "invalid_usage", "Choose a Todo view, workflow and lane")
	}
	if in.View != "" || in.Workflow != "" || in.Phase != "" {
		if !contains([]string{"Todos", "Board", "Backlog", "Graph"}, in.View) {
			return nil, false, failure(422, "invalid_usage_view", "Choose a ticket view")
		}
		workflow, ok := st.Workflows[in.Workflow]
		if !ok || !workflow.Published || workflow.ProjectID != 0 && workflow.ProjectID != in.ProjectID {
			return nil, false, failure(422, "invalid_usage_workflow", "Choose a published workflow available to this project")
		}
		phase, ok := phaseByID(workflow, in.Phase)
		if !ok {
			return nil, false, failure(422, "invalid_usage_phase", "Choose a lane in the selected workflow")
		}
		if contains([]string{"todo-spellcheck", "todo-refine"}, in.Kind) && (in.View != "Todos" || phase.Role != "Todo" || phase.Agentic) {
			return nil, false, failure(422, "invalid_usage_view", "This action requires a human Todo lane in the Todos view")
		}
	}
	if in.Name == "" || len(in.Name) > 120 || strings.ContainsRune(in.Name, 0) {
		return nil, false, failure(422, "invalid_usage", "Provide an action name of up to 120 bytes")
	}
	if in.Kind == "todo-refine" {
		if in.AgentID != "" {
			return nil, false, failure(422, "invalid_usage", "Workflow refinement does not use an agent persona")
		}
	} else {
		agent, ok := st.AgentDefinitions[in.AgentID]
		if !ok || agent.Archived && (in.Enabled || in.ID == "" || st.AgentUsages[in.ID].AgentID != in.AgentID) {
			return nil, false, failure(422, "agent_unavailable", "Select an active agent")
		}
	}
	if st.AgentUsages == nil {
		st.AgentUsages = map[string]model.AgentUsage{}
	}
	count := 0
	for _, old := range st.AgentUsages {
		if old.ProjectID == in.ProjectID {
			count++
			if old.ID != in.ID && !old.Removed && strings.EqualFold(old.Name, in.Name) {
				return nil, false, failure(409, "usage_name_exists", "An action with that name already exists")
			}
		}
	}
	id, version := in.ID, int64(1)
	if id == "" {
		if in.ExpectedVersion != 0 {
			return nil, false, failure(409, "version_conflict", "New usage requires version zero")
		}
		if count >= 100 {
			return nil, false, failure(409, "usage_capacity", "The project action limit is full")
		}
		id = "usage:" + randomID()
	} else {
		old, ok := st.AgentUsages[id]
		if !ok || old.ProjectID != in.ProjectID || old.Removed {
			return nil, false, failure(404, "not_found", "Agent usage not found")
		}
		if old.Version != in.ExpectedVersion {
			return nil, false, failure(409, "version_conflict", "Agent usage changed; reload before saving")
		}
		version = old.Version + 1
	}
	row := model.AgentUsage{ID: id, ProjectID: in.ProjectID, Version: version, Name: in.Name, AgentID: in.AgentID, Kind: in.Kind, View: in.View, Workflow: in.Workflow, Phase: in.Phase, Enabled: in.Enabled, UpdatedBy: a.ID, UpdatedAt: time.Now().UTC()}
	st.AgentUsages[id] = row
	remember(st, key, hash, row)
	return row, true, nil
}

func usageMatchesTicket(u model.AgentUsage, t model.Ticket) bool {
	return u.Workflow == "" || u.Workflow == t.Workflow && u.Phase == t.Phase
}
