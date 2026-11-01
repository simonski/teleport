package app

import (
	"teleport/internal/localruntime"
	"testing"
)

func TestManagedOutcomeRequiresCompleteTypedEvidence(t *testing.T) {
	valid := `{"outcome":"success","proposal":"Add the requested behaviour","criteria":"The stated check passes"}`
	turn := localruntime.NativeTurn{State: "completed", TurnID: "turn-1", Text: valid}
	if result, err := managedOutcome("Refine", &turn); err != nil || result.Proposal == "" {
		t.Fatalf("complete refinement rejected: %+v %v", result, err)
	}
	for _, text := range []string{`Done`, `{"outcome":"success"}`, valid + ` {}`, `{"outcome":"approve"}`, `{"outcome":"question","question":""}`} {
		turn.Text = text
		if _, err := managedOutcome("Refine", &turn); err == nil {
			t.Fatalf("untyped or incomplete result accepted: %s", text)
		}
	}
	turn.Text, turn.State = valid, "inProgress"
	if _, err := managedOutcome("Refine", &turn); err == nil {
		t.Fatal("running turn treated as a result")
	}
	turn.State, turn.Truncated = "completed", true
	if _, err := managedOutcome("Refine", &turn); err == nil {
		t.Fatal("truncated evidence accepted")
	}
}
