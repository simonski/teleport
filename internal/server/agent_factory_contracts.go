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
		{"removeAgentUsage", "Remove a project agent usage", app.RemoveAgentUsageInput{}, model.AgentUsage{}},
		{"listTicketProposals", "List retained ticket proposals", app.TicketProposalListInput{}, app.TicketProposalList{}},
		{"submitTicketProposal", "Submit a ticket proposal", app.TicketProposalInput{}, model.TicketProposal{}},
		{"commentTicketProposal", "Record proposal feedback", app.TicketProposalFeedbackInput{}, model.TicketProposal{}},
		{"acceptTicketProposal", "Accept a ticket proposal", app.TicketProposalAcceptInput{}, model.TicketProposal{}},
		{"discardTicketProposals", "Discard unaccepted proposals for a ticket", app.TicketProposalDiscardInput{}, app.TicketProposalDiscardResult{}},
		{"getProposalRunContext", "Read the current proposal run's prior feedback", struct{}{}, map[string]any{}},
		{"reportProposalRunUsage", "Record provider usage for the current proposal run", app.ProposalRunUsageInput{}, map[string]bool{}},
		{"startSpellcheck", "Start a scoped Todo spellcheck", app.SpellcheckInput{}, app.SpellcheckView{}},
		{"getSpellcheckRun", "Inspect an owned spellcheck run", app.SpellcheckStatusInput{}, app.SpellcheckView{}},
		{"getTicketSpellchecks", "Inspect spellcheck completion marks", app.IDInput{}, app.SpellcheckMarks{}},
		{"getUsageSummary", "Summarize authorized agent usage", app.UsageSummaryInput{}, app.UsageSummary{}},
		{"listUsageBreakdown", "Group authorized historical agent usage", app.UsageBreakdownInput{}, app.UsageBreakdownPage{}},
		{"getUsageAttribution", "Inspect an owned execution's attribution", app.SessionIDInput{}, model.ExecutionAttribution{}},
		{"listUsagePrices", "List published model prices", struct{}{}, app.UsagePriceList{}},
		{"listUsagePriceHistory", "List model price revisions", app.UsagePriceHistoryInput{}, app.Page[model.UsagePrice]{}},
		{"listUsageValuations", "List an owned run's valuation history", app.UsageValuationHistoryInput{}, app.Page[model.UsageValuation]{}},
		{"publishUsagePrice", "Publish a model price", app.PublishUsagePriceInput{}, model.UsagePrice{}},
	}
	for _, entry := range entries {
		action := "ACT-1515"
		if entry.name == "openAgentSession" {
			action = "ACT-1516"
		}
		if entry.name == "listAgentUsages" || entry.name == "saveAgentUsage" || entry.name == "removeAgentUsage" {
			action = "ACT-1517"
		}
		if entry.name == "listTicketProposals" || entry.name == "submitTicketProposal" || entry.name == "commentTicketProposal" || entry.name == "acceptTicketProposal" || entry.name == "discardTicketProposals" || entry.name == "getProposalRunContext" || entry.name == "reportProposalRunUsage" {
			action = "ACT-1519"
		}
		if entry.name == "startSpellcheck" || entry.name == "getSpellcheckRun" || entry.name == "getTicketSpellchecks" {
			action = "ACT-1518"
		}
		if entry.name == "getUsageSummary" || entry.name == "listUsageBreakdown" || entry.name == "getUsageAttribution" {
			action = "ACT-1109"
		}
		if entry.name == "listUsagePrices" || entry.name == "listUsagePriceHistory" || entry.name == "listUsageValuations" {
			action = "ACT-1119"
		}
		if entry.name == "publishUsagePrice" {
			action = "ACT-1121"
		}
		Operations = append(Operations, Operation{entry.name, action, entry.summary})
		contracts[entry.name] = contract{entry.input, entry.output}
	}
}
