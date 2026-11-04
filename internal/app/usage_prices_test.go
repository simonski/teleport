package app

import (
	"teleport/internal/model"
	"testing"
)

func TestUsagePriceRevisionsPreservePriorEstimates(t *testing.T) {
	for text, want := range map[string]int64{"0": 0, "0.5": 500000, "1.000001": 1000001} {
		got, ok := usdMicros(text)
		if !ok || got != want {
			t.Fatalf("%q = %d, %v", text, got, ok)
		}
	}
	for _, text := range []string{"-1", "1.0000001", "abc", "1000000", "1.-1"} {
		if _, ok := usdMicros(text); ok {
			t.Fatalf("invalid price %q accepted", text)
		}
	}
	cached := int64(20)
	row := model.LocalSession{Persona: &model.AgentDefinition{Harness: "codex", Model: "example"}, Spellcheck: &model.SpellcheckRun{Usage: &model.TokenEvidence{Input: 120, Cached: &cached, Output: 30, Source: "codex-exec-json"}}}
	st := &model.State{UsagePrices: map[string][]model.UsagePrice{"codex:example": {{Version: 1, InputMicros: 1000000, CachedMicros: 500000, OutputMicros: 2000000}}}}
	if !applyUsagePrice(st, &row) || row.Spellcheck.Valuations[0].EstimateMicros != 170 {
		t.Fatal("initial estimate incorrect")
	}
	if applyUsagePrice(st, &row) {
		t.Fatal("same price duplicated a valuation")
	}
	st.UsagePrices["codex:example"] = append(st.UsagePrices["codex:example"], model.UsagePrice{Version: 2, InputMicros: 1000000, CachedMicros: 500000, OutputMicros: 4000000})
	if !applyUsagePrice(st, &row) || len(row.Spellcheck.Valuations) != 2 || row.Spellcheck.Valuations[0].EstimateMicros != 170 || row.Spellcheck.Valuations[1].EstimateMicros != 230 {
		t.Fatalf("prior estimate lost: %+v", row.Spellcheck.Valuations)
	}
}
