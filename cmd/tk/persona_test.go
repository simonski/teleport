package main

import (
	"encoding/json"
	"strings"
	"testing"

	"teleport/internal/model"
)

func TestPersonaArgumentsUseSystemContextWithoutInitialUserPrompt(t *testing.T) {
	for _, harness := range []string{"codex", "claude"} {
		t.Run(harness, func(t *testing.T) {
			body, _ := json.Marshal(personaLaunch{Agent: model.AgentDefinition{Name: "Reviewer", Goal: "Review work", Prompt: "Ask first", Harness: harness, Model: "example-model", Thinking: "high"}, Skills: []string{"/skills/review/SKILL.md"}, Ticket: `{"id":7,"title":"Example"}`})
			args, err := personaArguments(string(body))
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(args, " ")
			for _, text := range []string{"Reviewer", "Review work", "/skills/review/SKILL.md", "example-model", "high", "ticket"} {
				if !strings.Contains(joined, text) {
					t.Errorf("missing %q: %q", text, joined)
				}
			}
			if harness == "codex" && !strings.Contains(joined, "developer_instructions=") {
				t.Fatal("Codex role was not set as developer instructions")
			}
			if harness == "claude" && args[0] != "--append-system-prompt" {
				t.Fatal("Claude role was not appended to system context")
			}
		})
	}
}

func TestPersonaAlwaysReceivesTeleportSkillInstructions(t *testing.T) {
	for _, harness := range []string{"codex", "claude"} {
		body, _ := json.Marshal(personaLaunch{Agent: model.AgentDefinition{Name: "Example", Harness: harness}})
		args, err := personaArguments(string(body))
		if err != nil || !strings.Contains(strings.Join(args, " "), "tk skill") {
			t.Fatalf("%s: bundled Teleport skill not included: %v", harness, err)
		}
	}
}

func TestProposalPersonaStartsCodexWithScopedPrompt(t *testing.T) {
	body, _ := json.Marshal(personaLaunch{Agent: model.AgentDefinition{Name: "Planner", Harness: "codex"}, ProposalJob: &model.ProposalRun{TicketID: 7, TicketVersion: 2}})
	args, err := personaArguments(string(body))
	if err != nil || len(args) == 0 || args[0] != "exec" || !strings.Contains(strings.Join(args, " "), "proposal") || !strings.Contains(strings.Join(args, " "), "read-only") {
		t.Fatalf("invalid proposal invocation: %v %v", args, err)
	}
}
