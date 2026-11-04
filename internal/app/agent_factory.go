package app

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"teleport/internal/model"
)

type SaveAgentDefinitionInput struct {
	ID              string   `json:"id,omitempty"`
	ExpectedVersion int64    `json:"expectedVersion,omitempty"`
	RequestID       string   `json:"requestId"`
	Name            string   `json:"name"`
	Goal            string   `json:"goal"`
	Prompt          string   `json:"prompt"`
	Skills          []string `json:"skills"`
	Harness         string   `json:"harness"`
	Model           string   `json:"model,omitempty"`
	Thinking        string   `json:"thinking,omitempty"`
	Archived        bool     `json:"archived"`
}
type AgentDefinitionPage struct {
	Items      []model.AgentDefinition `json:"items"`
	NextCursor string                  `json:"nextCursor"`
}
type AgentSkills struct {
	Items []string `json:"items"`
}

var agentSkillName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

// Skill names are discovered from known local skill roots; arbitrary paths are never accepted.
func installedAgentSkills() map[string]string {
	found := map[string]string{}
	home, err := os.UserHomeDir()
	if err != nil {
		return found
	}
	for _, root := range []string{filepath.Join(home, ".agents", "skills"), filepath.Join(home, ".codex", "skills"), filepath.Join(home, ".claude", "skills")} {
		entries, _ := os.ReadDir(root)
		for _, entry := range entries {
			name := entry.Name()
			if !entry.IsDir() || !agentSkillName.MatchString(name) {
				continue
			}
			file := filepath.Join(root, name, "SKILL.md")
			info, err := os.Lstat(file)
			if err == nil && info.Mode().IsRegular() {
				if _, ok := found[name]; !ok {
					found[name] = file
				}
			}
		}
	}
	return found
}
func agentFactoryCall(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	if a.ServiceRunID != "" || a.ServiceSessionID != "" {
		return nil, false, failure(403, "human_required", "A human account is required")
	}
	if operation == "listProjectAgents" {
		var in struct {
			ProjectID int64 `json:"projectId"`
		}
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		if err := require(st, a, in.ProjectID, "Viewer"); err != nil {
			return nil, false, err
		}
		items := []model.AgentDefinition{}
		for _, v := range st.AgentDefinitions {
			if !v.Archived {
				v.Prompt = ""
				v.Goal = ""
				v.Skills = nil
				items = append(items, v)
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
		return AgentDefinitionPage{Items: items}, false, nil
	}
	if !a.Administrator {
		return nil, false, failure(403, "administrator_required", "Agent factory requires an administrator")
	}
	switch operation {
	case "getAgentInventory":
		if err := parse(raw, &struct{}{}); err != nil {
			return nil, false, err
		}
		return agentInventory(), false, nil
	case "listInstalledAgentSkills":
		if err := parse(raw, &struct{}{}); err != nil {
			return nil, false, err
		}
		items := []string{}
		for name := range installedAgentSkills() {
			items = append(items, name)
		}
		sort.Strings(items)
		return AgentSkills{Items: items}, false, nil
	case "listAgentDefinitions":
		var in ListInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		_, limit, err := pageBounds("", in.Limit)
		if err != nil {
			return nil, false, err
		}
		after := ""
		if in.Cursor != "" {
			data, err := base64.RawURLEncoding.DecodeString(in.Cursor)
			if err != nil || !strings.HasPrefix(string(data), "agent:") {
				return nil, false, failure(400, "invalid_cursor", "Invalid agent cursor")
			}
			after = string(data)
		}
		items := []model.AgentDefinition{}
		for _, v := range st.AgentDefinitions {
			if v.ID > after && (in.IncludeHidden || !v.Archived) {
				items = append(items, v)
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		page := AgentDefinitionPage{Items: []model.AgentDefinition{}}
		if len(items) > limit {
			page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(items[limit-1].ID))
			items = items[:limit]
		}
		page.Items = items
		return page, false, nil
	case "saveAgentDefinition":
		var in SaveAgentDefinitionInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		key, hash, saved, err := receipt(st, a, operation, in.RequestID, in)
		if err != nil {
			return nil, false, err
		}
		if saved != nil {
			var result model.AgentDefinition
			if json.Unmarshal(saved, &result) != nil {
				return nil, false, failure(500, "invalid_receipt", "Saved agent receipt is invalid")
			}
			return result, false, nil
		}
		in.Name = strings.TrimSpace(in.Name)
		in.Goal = strings.TrimSpace(in.Goal)
		in.Prompt = strings.TrimSpace(in.Prompt)
		in.Model = strings.TrimSpace(in.Model)
		if in.Name == "" || len(in.Name) > 120 || in.Goal == "" || len(in.Goal) > 2000 || len(in.Prompt) > 12000 || len(in.Model) > 100 || strings.ContainsRune(in.Name, 0) || strings.ContainsRune(in.Goal, 0) || strings.ContainsRune(in.Prompt, 0) || strings.ContainsRune(in.Model, 0) || !contains([]string{"codex", "claude"}, in.Harness) || !contains([]string{"", "low", "medium", "high", "xhigh", "max", "ultra"}, in.Thinking) || in.Harness == "claude" && in.Thinking == "ultra" || len(in.Skills) > 30 {
			return nil, false, failure(422, "invalid_agent_definition", "Provide a name, goal, supported harness and bounded settings")
		}
		skills := installedAgentSkills()
		seen := map[string]bool{}
		for _, name := range in.Skills {
			if seen[name] || skills[name] == "" {
				return nil, false, failure(422, "skill_unavailable", "Choose distinct installed skills")
			}
			seen[name] = true
		}
		if st.AgentDefinitions == nil {
			st.AgentDefinitions = map[string]model.AgentDefinition{}
		}
		for _, row := range st.AgentDefinitions {
			if row.ID != in.ID && !row.Archived && !in.Archived && strings.EqualFold(row.Name, in.Name) {
				return nil, false, failure(409, "agent_name_exists", "An active agent already has that name")
			}
		}
		id, version := in.ID, int64(1)
		if id == "" {
			if in.ExpectedVersion != 0 {
				return nil, false, failure(409, "version_conflict", "New agent requires version zero")
			}
			if len(st.AgentDefinitions) >= 100 {
				return nil, false, failure(409, "agent_capacity", "The agent catalogue is full")
			}
			id = "agent:" + randomID()
		} else {
			old, ok := st.AgentDefinitions[id]
			if !ok {
				return nil, false, failure(404, "not_found", "Agent not found")
			}
			if old.Version != in.ExpectedVersion {
				return nil, false, failure(409, "version_conflict", "Agent changed; reload before saving")
			}
			version = old.Version + 1
		}
		row := model.AgentDefinition{ID: id, Version: version, Name: in.Name, Goal: in.Goal, Prompt: in.Prompt, Skills: append([]string{}, in.Skills...), Harness: in.Harness, Model: in.Model, Thinking: in.Thinking, Archived: in.Archived, UpdatedBy: a.ID, UpdatedAt: time.Now().UTC()}
		st.AgentDefinitions[id] = row
		remember(st, key, hash, row)
		return row, true, nil
	}
	return nil, false, failure(404, "unknown_operation", "Unknown agent factory operation")
}
