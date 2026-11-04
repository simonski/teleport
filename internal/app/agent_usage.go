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
	Enabled         bool   `json:"enabled"`
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
			if usage.ProjectID == in.ProjectID {
				items = append(items, usage)
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		return AgentUsageList{Items: items}, false, nil
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
	if in.Name == "" || len(in.Name) > 120 || strings.ContainsRune(in.Name, 0) {
		return nil, false, failure(422, "invalid_usage", "Provide an action name of up to 120 bytes")
	}
	agent, ok := st.AgentDefinitions[in.AgentID]
	if !ok || agent.Archived && (in.Enabled || in.ID == "" || st.AgentUsages[in.ID].AgentID != in.AgentID) {
		return nil, false, failure(422, "agent_unavailable", "Select an active agent")
	}
	if st.AgentUsages == nil {
		st.AgentUsages = map[string]model.AgentUsage{}
	}
	count := 0
	for _, old := range st.AgentUsages {
		if old.ProjectID == in.ProjectID {
			count++
			if old.ID != in.ID && strings.EqualFold(old.Name, in.Name) {
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
		if !ok || old.ProjectID != in.ProjectID {
			return nil, false, failure(404, "not_found", "Agent usage not found")
		}
		if old.Version != in.ExpectedVersion {
			return nil, false, failure(409, "version_conflict", "Agent usage changed; reload before saving")
		}
		version = old.Version + 1
	}
	row := model.AgentUsage{ID: id, ProjectID: in.ProjectID, Version: version, Name: in.Name, AgentID: in.AgentID, Kind: "todo-spellcheck", Enabled: in.Enabled, UpdatedBy: a.ID, UpdatedAt: time.Now().UTC()}
	st.AgentUsages[id] = row
	remember(st, key, hash, row)
	return row, true, nil
}
