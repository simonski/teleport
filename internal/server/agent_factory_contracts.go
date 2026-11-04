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
		{"getAgentInventory", "Read the installed harness and model inventory", struct{}{}, app.AgentInventory{}},
		{"getInstalledSkill", "Read an installed skill", app.InstalledSkillInput{}, app.InstalledSkillView{}},
		{"saveInstalledSkill", "Create or edit an installed skill", app.InstalledSkillInput{}, app.InstalledSkillView{}},
		{"saveAgentDefinition", "Create or update an agent definition", app.SaveAgentDefinitionInput{}, model.AgentDefinition{}},
		{"listProjectAgents", "List agent identities available to a project", struct {
			ProjectID int64 `json:"projectId"`
		}{}, app.AgentDefinitionPage{}},
		{"openAgentSession", "Start a project-scoped conversation with an agent persona", app.OpenAgentSessionInput{}, app.LocalSessionInfo{}},
		{"listAgentUsages", "List project agent usages", struct {
			ProjectID int64 `json:"projectId"`
		}{}, app.AgentUsageList{}},
		{"saveAgentUsage", "Configure a project agent usage", app.AgentUsageInput{}, model.AgentUsage{}},
		{"startSpellcheck", "Start a scoped Todo spellcheck", app.SpellcheckInput{}, app.SpellcheckView{}},
		{"getSpellcheckRun", "Inspect an owned spellcheck run", app.SpellcheckStatusInput{}, app.SpellcheckView{}},
		{"getTicketSpellchecks", "Inspect spellcheck completion marks", app.IDInput{}, app.SpellcheckMarks{}},
	}
	for _, entry := range entries {
		action := "ACT-1515"
		if entry.name == "openAgentSession" {
			action = "ACT-1516"
		}
		if entry.name == "listAgentUsages" || entry.name == "saveAgentUsage" {
			action = "ACT-1517"
		}
		if entry.name == "startSpellcheck" || entry.name == "getSpellcheckRun" || entry.name == "getTicketSpellchecks" {
			action = "ACT-1518"
		}
		Operations = append(Operations, Operation{entry.name, action, entry.summary})
		contracts[entry.name] = contract{entry.input, entry.output}
	}
}
