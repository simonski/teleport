package main

import "testing"

func TestCodexSpellcheckUsageUsesFinalReportedCounters(t *testing.T) {
	data := []byte("{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":120,\"cached_input_tokens\":20,\"output_tokens\":30}}\n")
	got := codexSpellcheckUsage(data)
	if got == nil || got.Input != 120 || got.Output != 30 || got.Cached == nil || *got.Cached != 20 {
		t.Fatalf("unexpected usage: %+v", got)
	}
	if codexSpellcheckUsage([]byte(`{"type":"turn.completed","usage":{"input_tokens":1,"cached_input_tokens":2,"output_tokens":3}}`)) != nil {
		t.Fatal("invalid cache counter accepted")
	}
	if codexSpellcheckUsage([]byte(`{"type":"turn.completed"}`)) != nil {
		t.Fatal("missing usage became zero")
	}
}
