package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"teleport/internal/model"
)

func TestClaudeProposalResultReportsProviderUsage(t *testing.T) {
	data := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"Planning"}]}}` + "\n" + `{"type":"result","structured_output":{"title":"Plan","items":[]},"is_error":false,"usage":{"input_tokens":100,"cache_read_input_tokens":20,"cache_creation_input_tokens":10,"output_tokens":30},"total_cost_usd":0.0003}` + "\n")
	result, usage, err := claudeProposalResult(data)
	if err != nil || !strings.Contains(string(result), "Plan") || usage == nil || usage.Input != 130 || usage.Output != 30 || usage.Cached == nil || *usage.Cached != 20 || usage.EstimateUSDMicros == nil || *usage.EstimateUSDMicros != 300 {
		t.Fatalf("unexpected result %s %#v %v", result, usage, err)
	}
	var capture, terminal bytes.Buffer
	writer := &proposalEventWriter{capture: &limitedBuffer{b: &capture, remaining: 2 << 20}, output: &terminal}
	_, _ = writer.Write(data)
	if !strings.Contains(terminal.String(), "Planning") || strings.Contains(terminal.String(), "structured_output") {
		t.Fatal("stream did not show readable planning output")
	}
}

func TestClaudeProposalPersonaUsesRestrictedPrintMode(t *testing.T) {
	body, _ := json.Marshal(personaLaunch{Agent: model.AgentDefinition{Name: "Planner", Harness: "claude"}, ProposalJob: &model.ProposalRun{TicketID: 7, TicketVersion: 2}})
	args, err := personaArguments(string(body))
	joined := strings.Join(args, " ")
	if err != nil || !strings.Contains(joined, "--print") || !strings.Contains(joined, "--include-partial-messages") || !strings.Contains(joined, "--json-schema") || !strings.Contains(joined, "--safe-mode") || !strings.Contains(joined, "--tools Read") {
		t.Fatalf("invalid Claude invocation: %s %v", joined, err)
	}
}

func TestProposalStreamShowsHarnessProgressWithoutRepeatingOutput(t *testing.T) {
	var captured, shown bytes.Buffer
	w := &proposalEventWriter{capture: &limitedBuffer{b: &captured, remaining: 1 << 20}, output: &shown}
	for _, line := range []string{
		`{"type":"thread.started"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.started","item":{"id":"1","type":"command_execution","command":"tk ticket show 7"}}`,
		`{"type":"item.updated","item":{"id":"1","type":"command_execution","aggregated_output":"first\n"}}`,
		`{"type":"item.completed","item":{"id":"1","type":"command_execution","aggregated_output":"first\nsecond\n"}}`,
		`{"type":"item.completed","item":{"type":"reasoning","text":"Inspecting the ticket"}}`,
		`{"type":"stream_event","event":{"type":"message_start"}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Live answer"}}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Live answer"}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","content":[{"type":"text","text":"Read result"}]}]}}`,
	} {
		_, _ = w.Write([]byte(line + "\n"))
	}
	text := shown.String()
	for _, want := range []string{"thread started", "$ tk ticket show 7", "first", "second", "Inspecting the ticket", "Live answer", "Read result"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %q", want, text)
		}
	}
	if strings.Count(text, "first") != 1 || strings.Count(text, "Live answer") != 1 {
		t.Fatalf("duplicated streamed output: %q", text)
	}
}

func TestProposalDisplaysActualPromptAndPreviousFeedback(t *testing.T) {
	body, _ := json.Marshal(personaLaunch{Agent: model.AgentDefinition{Name: "Planner", Goal: "Plan work", Harness: "codex"}, Ticket: `{"title":"Build a rocket"}`, ProposalJob: &model.ProposalRun{TicketID: 7, TicketVersion: 2}})
	args, err := personaArguments(string(body))
	if err != nil {
		t.Fatal(err)
	}
	var shown bytes.Buffer
	showProposalInput(&shown, args, []byte(`{"feedback":"Split the epic"}`), "codex")
	for _, want := range []string{"Starting codex", "Plan work", "Build a rocket", "Produce a proposal for ticket 7", "Split the epic", "Harness output"} {
		if !strings.Contains(shown.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
}
