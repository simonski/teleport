package store_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
	"teleport/internal/teststore"
)

func checksum(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }

func TestSnapshotRestorePreservesExactStateAndRefusesReplacement(t *testing.T) {
	ctx := context.Background()
	source, target := teststore.Open(t), teststore.Open(t)
	initial, err := app.Initialize(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	body, err := source.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = target.RestoreSnapshot(ctx, body, checksum(body)); err != nil {
		t.Fatal(err)
	}
	if err = target.RestoreSnapshot(ctx, body, checksum(body)); err != nil {
		t.Fatal("retry failed", err)
	}
	actual, err := target.Snapshot(ctx)
	if err != nil || string(actual) != string(body) {
		t.Fatal("state bytes changed", err)
	}
	if _, err = app.New(target).Call(ctx, "", "login", []byte(`{"username":"admin","password":"password"}`)); err != nil {
		t.Fatal("credentials lost", err)
	}
	changed, _ := json.Marshal(initial)
	if err = target.RestoreSnapshot(ctx, changed, checksum(changed)); err == nil {
		t.Fatal("different target state overwritten")
	}
}

func TestSnapshotRejectsInvalidAndRetainedWorkWithoutPartialState(t *testing.T) {
	ctx := context.Background()
	source := teststore.Open(t)
	state, err := app.Initialize(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"checksum", "unknown fields", "schema", "live script", "unknown harness"} {
		t.Run(scenario, func(t *testing.T) {
			target := teststore.Open(t)
			body, _ := json.Marshal(state)
			var copy model.State
			json.Unmarshal(body, &copy)
			switch scenario {
			case "unknown fields":
				body = append(body[:len(body)-1], []byte(`,"unexpected":"field"}`)...)
			case "schema":
				copy.Schema++
				body, _ = json.Marshal(copy)
			case "live script":
				copy.ScriptRuns = map[string]model.ScriptRun{"J1": {ID: "J1", State: "running"}}
				body, _ = json.Marshal(copy)
			case "unknown harness":
				copy.LocalSessions = map[string]model.LocalSession{"H1": {ID: "H1", State: "unknown"}}
				body, _ = json.Marshal(copy)
			}
			hash := checksum(body)
			if scenario == "checksum" {
				hash = "invalid"
			}
			if err := target.RestoreSnapshot(ctx, body, hash); err == nil {
				t.Fatal("unsafe restore accepted")
			}
			if _, err := target.Read(ctx); err == nil {
				t.Fatal("rejected restore wrote partial state")
			}
		})
	}
}
