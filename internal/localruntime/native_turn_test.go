//go:build darwin || linux

package localruntime

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNativeTurnCapturesProtocolCompletionWithoutTerminalGuessing(t *testing.T) {
	e := &entry{Native: true, NativeThreadID: "thread", Status: Status{ID: "H1", State: "running", LaunchID: "launch", Generation: "generation"}}
	d := &daemon{root: t.TempDir(), generation: "generation", entries: map[string]*entry{"H1": e}}
	event := func(wire string) {
		t.Helper()
		if out := d.nativeEvent(e, &NativeEvent{Server: true, Message: json.RawMessage(wire)}); out.Error != "" {
			t.Fatal(out.Error)
		}
	}
	event(`{"method":"turn/started","params":{"threadId":"thread","turn":{"id":"turn"}}}`)
	event(`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"type":"agentMessage","phase":"commentary","text":"not a final result"}}}`)
	if e.NativeTurn.State != "inProgress" || e.NativeTurn.Text != "" {
		t.Fatal("commentary became a result", e.NativeTurn)
	}
	event(`{"method":"item/completed","params":{"threadId":"thread","turnId":"foreign","item":{"type":"agentMessage","text":"foreign result"}}}`)
	event(`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed","items":[{"type":"agentMessage","phase":"final_answer","text":"exact result"}]}}}`)
	if e.NativeTurn.State != "completed" || e.NativeTurn.Text != "exact result" || e.NativeTurn.Truncated {
		t.Fatal(e.NativeTurn)
	}
	settled := *e.NativeTurn
	event(`{"method":"turn/started","params":{"threadId":"thread","turn":{"id":"turn"}}}`)
	event(`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"failed"}}}`)
	if *e.NativeTurn != settled {
		t.Fatal("duplicate events rewrote settled output")
	}
	out := d.call(Request{Operation: "nativeTurn", ID: "H1", LaunchID: "launch", Generation: "generation"})
	e.NativeTurn.Text = "later mutation"
	if out.Turn == nil || out.Turn.Text != "exact result" {
		t.Fatal("native turn response borrowed mutable runtime storage", out)
	}
	event(`{"method":"turn/started","params":{"threadId":"thread","turn":{"id":"next"}}}`)
	event(`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"next","status":"failed"}}}`)
	if e.NativeTurn.State != "failed" || e.NativeTurn.ErrorCode != "native_turn_failed" || e.NativeTurn.Text != "" {
		t.Fatal(e.NativeTurn)
	}
}

func TestNativeTurnRetentionPrunesRetiredTextWithExplicitGap(t *testing.T) {
	d := &daemon{entries: map[string]*entry{}}
	for i := 0; i < 300; i++ {
		id := fmt.Sprintf("H%d", i+1)
		d.entries[id] = &entry{Status: Status{State: "exited"}, NativeTurn: &NativeTurn{Text: strings.Repeat("x", NativeTurnTextLimit), At: time.Unix(int64(i), 0)}}
	}
	d.entries["active"] = &entry{Status: Status{State: "running"}, NativeTurn: &NativeTurn{Text: "Keep active output", At: time.Unix(0, 0)}}
	d.trimNativeTurns()
	if first := d.entries["H1"].NativeTurn; first.Text != "" || !first.Truncated || first.ErrorCode != "native_turn_retention_gap" {
		t.Fatal("old output disappeared without a gap")
	}
	if d.entries["active"].NativeTurn.Text != "Keep active output" {
		t.Fatal("active output was pruned")
	}
	total := 0
	for _, entry := range d.entries {
		encoded, _ := json.Marshal(entry.NativeTurn)
		total += len(encoded)
	}
	if total > 8<<20 {
		t.Fatal("native text retention exceeded its aggregate limit")
	}
}
func TestNativeTurnOversizeCannotBecomeSuccessfulOutput(t *testing.T) {
	turn := &NativeTurn{}
	data, _ := json.Marshal(map[string]any{"type": "agentMessage", "text": strings.Repeat("x", NativeTurnTextLimit+1)})
	turn.capture(data)
	turn.capture(json.RawMessage(`{"type":"agentMessage","text":"later"}`))
	if !turn.Truncated || turn.Text != "" || turn.ErrorCode != "native_result_too_large" {
		t.Fatal(turn)
	}
}
