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
