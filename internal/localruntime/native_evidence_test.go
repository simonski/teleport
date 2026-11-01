//go:build darwin || linux

package localruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type nativeEventClientFunc func(context.Context, Request) (Response, error)

func (fn nativeEventClientFunc) Call(ctx context.Context, request Request) (Response, error) {
	return fn(ctx, request)
}

func nativeEvidenceFixture(t *testing.T) (*daemon, *entry, *nativeProxy) {
	t.Helper()
	e := &entry{Native: true, NativeThreadID: "thread", NativeKey: "private-key-sentinel", Status: Status{ID: "H1", State: "running", LaunchID: "launch", Generation: "generation"}, NativeTurn: &NativeTurn{ThreadID: "thread", TurnID: "turn", State: "inProgress"}}
	d := &daemon{root: t.TempDir(), generation: "generation", entries: map[string]*entry{"H1": e}}
	client := nativeEventClientFunc(func(_ context.Context, request Request) (Response, error) {
		out := d.call(request)
		if out.Error != "" {
			return out, errors.New(out.Error)
		}
		return out, nil
	})
	p := &nativeProxy{threadID: "thread", pending: map[string]nativePending{}, queries: map[string]string{}, client: client, identity: Request{ID: "H1", LaunchID: "launch", Generation: "generation"}}
	return d, e, p
}

func TestNativeEvidenceCompactsLargeHistoryWithoutChangingTerminalWire(t *testing.T) {
	_, e, p := nativeEvidenceFixture(t)
	large, _ := json.Marshal(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread", "turn": map[string]any{"id": "turn", "status": "completed", "items": []any{map[string]any{"type": "commandExecution", "output": strings.Repeat("private-command-output", 20000)}, map[string]any{"type": "agentMessage", "phase": "commentary", "text": "private commentary"}, map[string]any{"type": "agentMessage", "phase": "final_answer", "text": "Exact final result"}}}}})
	forwarded, err := p.fromEngine(large)
	if err != nil || !bytes.Equal(forwarded, large) {
		t.Fatal("terminal stream changed", err)
	}
	if e.NativeTurn.State != "completed" || e.NativeTurn.Text != "Exact final result" || e.NativeTurn.Truncated || e.NativeFailure != nil {
		t.Fatal("bounded final evidence was lost")
	}
	encoded, _ := compactNativeTelemetry(large)
	if len(encoded) > nativeTelemetryLimit || bytes.Contains(encoded, []byte("private-command-output")) || bytes.Contains(encoded, []byte("private commentary")) {
		t.Fatal("telemetry retained unrelated content")
	}
}

func TestNativeEvidenceOversizedEscapedOutputCannotBecomeSuccess(t *testing.T) {
	_, e, p := nativeEvidenceFixture(t)
	data, _ := json.Marshal(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread", "turn": map[string]any{"id": "turn", "status": "completed", "items": []any{map[string]any{"type": "agentMessage", "phase": "final_answer", "text": strings.Repeat("<", NativeTurnTextLimit)}, map[string]any{"type": "agentMessage", "phase": "final_answer", "text": "A later short message"}}}}})
	if _, err := p.fromEngine(data); err != nil {
		t.Fatal(err)
	}
	if !e.NativeTurn.Truncated || e.NativeTurn.Text != "" || e.NativeTurn.ErrorCode != "native_result_too_large" {
		t.Fatal("oversized output was replaced by a false short success")
	}
	before := *e.NativeTurn
	if _, err := p.fromEngine([]byte(`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed","items":[{"type":"agentMessage","text":"rewrite"}]}}}`)); err != nil {
		t.Fatal(err)
	}
	if *e.NativeTurn != before {
		t.Fatal("a settled turn was rewritten")
	}
}

func TestNativeEvidenceFailurePersistsWithoutPayloadOrRelease(t *testing.T) {
	d, e, p := nativeEvidenceFixture(t)
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	_, err := p.fromEngine([]byte("private-malformed-protocol-sentinel"))
	if err == nil {
		t.Fatal("invalid JSON accepted")
	}
	p.retainFailure(nativeParseFailure(err), true)
	if e.NativeFailure == nil || e.NativeFailure.Code != "native_protocol_invalid_json" || e.NativeTurn.State != "unknown" || e.NativeReleased {
		t.Fatal("failure did not preserve uncertainty")
	}
	if !strings.Contains(logs.String(), "native_protocol_invalid_json") || strings.Contains(logs.String(), "private-malformed-protocol-sentinel") || strings.Contains(logs.String(), e.NativeKey) {
		t.Fatal("diagnostic log omitted the code or exposed private content")
	}
	data, readErr := os.ReadFile(filepath.Join(d.root, "sessions.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	var retained map[string]struct {
		NativeFailure *NativeFailure `json:"nativeFailure"`
	}
	if json.Unmarshal(data, &retained) != nil || retained["H1"].NativeFailure == nil || retained["H1"].NativeFailure.Code != "native_protocol_invalid_json" {
		t.Fatal("a fresh reader cannot recover the failure reason")
	}
	d.nativeEvent(e, &NativeEvent{Server: true, Message: json.RawMessage(`{"method":"teleport/engineExit","params":{"exitCode":17}}`)})
	out := d.call(Request{Operation: "nativeTurn", ID: "H1", LaunchID: "launch", Generation: "generation"})
	*e.NativeFailure.EngineExitCode = 99
	if out.NativeFailure == nil || out.NativeFailure.EngineExitCode == nil || *out.NativeFailure.EngineExitCode != 17 {
		t.Fatal("failure response borrowed mutable state")
	}
	if response := d.nativeEvent(e, &NativeEvent{Server: true, Message: json.RawMessage(`{"method":"teleport/protocolFailure","params":{"code":"private-unrecognized-payload"}}`)}); response.Error != "invalid_native_failure" {
		t.Fatal("arbitrary diagnostic code accepted")
	}
}

func TestNativeEvidenceExpectedShutdownEOFDoesNotInventFailure(t *testing.T) {
	_, e, p := nativeEvidenceFixture(t)
	p.stopping = true
	p.retainFailure("native_protocol_eof", true)
	if e.NativeFailure != nil || p.protocolFailed {
		t.Fatal("cooperative shutdown became a protocol fault")
	}
}

func TestNativeEvidenceFiltersUnknownAssistantPhases(t *testing.T) {
	turn := &NativeTurn{}
	turn.capture(json.RawMessage(`{"type":"agentMessage","phase":"analysis","text":"private reasoning sentinel"}`))
	if turn.Text != "" {
		t.Fatal("unsupported assistant phase became final text")
	}
}

func TestNativeEvidenceUnknownStateCannotBecomeUnboundedDiagnostic(t *testing.T) {
	_, e, p := nativeEvidenceFixture(t)
	data, _ := json.Marshal(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread", "turn": map[string]any{"id": "turn", "status": "private-unrecognized-state-sentinel"}}})
	if _, err := p.fromEngine(data); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(e.NativeTurn)
	if e.NativeTurn.State != "unknown" || e.NativeTurn.ErrorCode != "native_turn_unknown_state" || bytes.Contains(encoded, []byte("private-unrecognized")) {
		t.Fatal("unknown state escaped into retained diagnostic fields")
	}
}
