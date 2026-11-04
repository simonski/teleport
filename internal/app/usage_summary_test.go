package app

import (
	"teleport/internal/model"
	"testing"
	"time"
)

func TestUsageWindowCountsOwnedEvidenceAndPreservesUnknown(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	start := now.Add(-30 * time.Minute)
	end := start.Add(time.Minute)
	cached := int64(20)
	st := &model.State{Projects: map[int64]model.Project{1: {ID: 1, Members: map[string]string{"reader": "Viewer"}}, 2: {ID: 2}}, LocalSessions: map[string]model.LocalSession{
		"H1": {ProjectID: 1, Harness: "codex-pty", CreatedAt: start, ExitedAt: &end, State: "exited", Spellcheck: &model.SpellcheckRun{State: "completed", Usage: &model.TokenEvidence{Input: 120, Cached: &cached, Output: 30, Source: "codex-exec-json"}}},
		"H2": {ProjectID: 1, Harness: "codex-pty", CreatedAt: start, State: "unknown", Spellcheck: &model.SpellcheckRun{State: "running"}},
		"H3": {ProjectID: 1, Harness: "codex-pty", CreatedAt: start, State: "running"},
		"H4": {ProjectID: 2, Harness: "codex-pty", CreatedAt: start, State: "exited", Spellcheck: &model.SpellcheckRun{State: "completed"}},
		"H5": {ProjectID: 1, Harness: "shell", CreatedAt: start, State: "running"},
	}}
	got := usageWindow(st, model.Account{ID: "reader"}, 0, "hour", now.Add(-time.Hour), now)
	if got.Invocations != 2 || got.Succeeded != 1 || got.Pending != 1 || got.MeasuredRuns != 1 || got.MissingRuns != 1 || got.InteractiveSessions != 1 || got.Tokens == nil || *got.Tokens != 150 || got.Cached == nil || *got.Cached != 20 || got.EstimatedUSDMicros != nil || !got.ElapsedPartial || got.ElapsedSeconds != 60 {
		t.Fatalf("unexpected aggregate: %+v", got)
	}
	none := usageWindow(st, model.Account{ID: "reader"}, 0, "hour", now.Add(-time.Minute), now)
	if none.Tokens != nil || none.Invocations != 0 {
		t.Fatalf("missing evidence became zero: %+v", none)
	}
}
