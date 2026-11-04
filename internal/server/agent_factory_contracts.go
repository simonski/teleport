package server

import (
	"teleport/internal/app"
	"teleport/internal/model"
)

func init() {
	entries := []struct {
		name, summary string
		input, output any
	}{
		{"listAgentDefinitions", "List administrator-owned agent definitions", app.ListInput{}, app.AgentDefinitionPage{}},
		{"listInstalledAgentSkills", "List installed skill names", struct{}{}, app.AgentSkills{}},
		{"saveAgentDefinition", "Create or update an agent definition", app.SaveAgentDefinitionInput{}, model.AgentDefinition{}},
		{"listProjectAgents", "List agent identities available to a project", struct {
			ProjectID int64 `json:"projectId"`
		}{}, app.AgentDefinitionPage{}},
		{"openAgentSession", "Start a project-scoped conversation with an agent persona", app.OpenAgentSessionInput{}, app.LocalSessionInfo{}},
	}
	for _, entry := range entries {
		action := "ACT-1515"
		if entry.name == "openAgentSession" {
			action = "ACT-1516"
		}
		Operations = append(Operations, Operation{entry.name, action, entry.summary})
		contracts[entry.name] = contract{entry.input, entry.output}
	}
}
