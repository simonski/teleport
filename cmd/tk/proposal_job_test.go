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
	if err != nil || !strings.Contains(joined, "--print") || !strings.Contains(joined, "--json-schema") || !strings.Contains(joined, "--safe-mode") || !strings.Contains(joined, "--tools Read") {
		t.Fatalf("invalid Claude invocation: %s %v", joined, err)
	}
}
