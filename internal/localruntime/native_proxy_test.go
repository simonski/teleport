//go:build darwin || linux

package localruntime

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
)

type nativeInput struct{ bytes.Buffer }

func TestReadOnlyPhaseCannotOverrideTurnPermissions(t *testing.T) {
	for _, role := range []string{"Refine", "Review"} {
		params := map[string]any{"permissions": "unrestricted-profile", "approvalPolicy": "on-request", "cwd": "/foreign", "sandboxPolicy": map[string]any{"type": "dangerFullAccess"}}
		constrainNativePhase(role, "turn/start", "/owned", params)
		if _, ok := params["permissions"]; ok {
			t.Fatal("named permissions override survived")
		}
		if params["cwd"] != "/owned" || params["approvalPolicy"] != "never" || params["sandboxPolicy"].(map[string]any)["type"] != "readOnly" {
			t.Fatal("read-only turn authority was broadened", params)
		}
	}
	params := map[string]any{"permissions": "native-profile"}
	constrainNativePhase("Doing", "turn/start", "/owned", params)
	if params["permissions"] != "native-profile" {
		t.Fatal("implementation native permissions were replaced")
	}
}

func (*nativeInput) Close() error { return nil }

func TestNativeProxyRejectsOldTurnAndSerializesReplies(t *testing.T) {
	input := &nativeInput{}
	rpc := json.RawMessage(`7`)
	p := &nativeProxy{input: input, key: "secret", pending: map[string]nativePending{nativeID(rpc): {RequestID: "new-turn"}}, operations: map[string]NativeReceipt{}}
	respond := func(scoped, op string) int {
		data, _ := json.Marshal(nativeDispatch{RequestID: scoped, OperationID: op, RPCID: rpc, Result: json.RawMessage(`{"answers":{}}`)})
		request := httptest.NewRequest("POST", "/respond", bytes.NewReader(data))
		request.Header.Set("Authorization", "Bearer secret")
		recorder := httptest.NewRecorder()
		p.respond(recorder, request)
		return recorder.Code
	}
	if respond("old-turn", "stale") != 409 || input.Len() != 0 {
		t.Fatal("old dialog answered a new RPC request")
	}
	var group sync.WaitGroup
	statuses := make(chan int, 2)
	for _, op := range []string{"gui", "native-race"} {
		group.Add(1)
		go func(op string) { defer group.Done(); statuses <- respond("new-turn", op) }(op)
	}
	group.Wait()
	close(statuses)
	delivered := 0
	for status := range statuses {
		if status == 200 {
			delivered++
		} else if status != 409 {
			t.Fatal(status)
		}
	}
	if delivered != 1 || bytes.Count(input.Bytes(), []byte("\n")) != 1 {
		t.Fatal("response raced into duplicate delivery", delivered, input.String())
	}
	p.pending[nativeID(rpc)] = nativePending{RequestID: "third-turn"}
	p.telemetryFailed = true
	if respond("third-turn", "capture-failed") != 409 {
		t.Fatal("uncaptured request accepted a GUI response")
	}
}

func TestNativeCommandExitRequiresActualExitEvidence(t *testing.T) {
	p := &nativeProxy{queries: map[string]string{`"cleanup"`: "teleport/clean"}}
	for _, data := range []string{`{"method":"item/started","params":{"item":{"id":"command","type":"commandExecution"}}}`, `{"method":"item/completed","params":{"item":{"id":"command","type":"commandExecution","status":"completed"}}}`} {
		if _, err := p.fromEngine([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if !p.everCommand || len(p.openCommands) != 1 {
		t.Fatal("a status without exit evidence released a command")
	}
	_, err := p.fromEngine([]byte(`{"method":"item/completed","params":{"item":{"id":"command","type":"commandExecution","exitCode":0}}}`))
	if err != nil || len(p.openCommands) != 0 {
		t.Fatal("verified exit was not retained", err)
	}
	_, err = p.fromEngine([]byte(`{"id":"cleanup","result":{}}`))
	if err != nil || !p.cleanupAck {
		t.Fatal("native cleanup acknowledgement unavailable", err)
	}
}

func TestNativeForeignThreadCannotCertifyCleanup(t *testing.T) {
	for _, event := range []string{`{"method":"thread/started","params":{"thread":{"id":"foreign"}}}`, `{"method":"item/completed","params":{"threadId":"foreign","item":{"id":"command","type":"commandExecution","exitCode":0}}}`} {
		p := &nativeProxy{threadID: "owned"}
		if _, err := p.fromEngine([]byte(event)); err == nil || !p.protocolFailed {
			t.Fatal("foreign thread retained cleanup authority", event, err)
		}
	}
}

func TestNativeReplyRechecksScopeAfterWaitingForWriter(t *testing.T) {
	input := &nativeInput{}
	rpc := json.RawMessage(`7`)
	p := &nativeProxy{input: input, key: "secret", threadID: "owned", pending: map[string]nativePending{nativeID(rpc): {RequestID: "old-turn"}}, operations: map[string]NativeReceipt{}, client: New(t.TempDir(), "")}
	p.writeMu.Lock()
	finished := make(chan int, 1)
	go func() {
		data, _ := json.Marshal(nativeDispatch{RequestID: "old-turn", OperationID: "delayed", RPCID: rpc, Result: json.RawMessage(`{}`)})
		request := httptest.NewRequest("POST", "/respond", bytes.NewReader(data))
		request.Header.Set("Authorization", "Bearer secret")
		recorder := httptest.NewRecorder()
		p.respond(recorder, request)
		finished <- recorder.Code
	}()
	_, err := p.fromEngine([]byte(`{"method":"serverRequest/resolved","params":{"threadId":"owned","requestId":7}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !p.pending[nativeID(rpc)].Answered {
		t.Fatal("resolution left helper dispatchable")
	}
	_, err = p.fromEngine([]byte(`{"id":7,"method":"item/tool/requestUserInput","params":{"threadId":"owned","turnId":"new","questions":[{"id":"a","question":"Next?"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	p.telemetryFailed = false
	p.mu.Unlock()
	p.writeMu.Unlock()
	if <-finished != 409 || input.Len() != 0 {
		t.Fatal("old response reached reused RPC after waiting for writer")
	}
}

func TestNativeTUIAnswerRechecksCapturedScopeAfterWaitingForWriter(t *testing.T) {
	input := &nativeInput{}
	rpc := json.RawMessage(`7`)
	captured := nativePending{RequestID: "old-turn", Epoch: 1}
	p := &nativeProxy{input: input, pending: map[string]nativePending{nativeID(rpc): captured}}
	p.writeMu.Lock()
	finished := make(chan bool, 1)
	go func() {
		written, _ := p.writeNativeAnswer([]byte(`{"id":7,"result":{}}`), rpc, captured)
		finished <- written
	}()
	p.mu.Lock()
	p.pending[nativeID(rpc)] = nativePending{RequestID: "new-turn", Epoch: 2}
	p.mu.Unlock()
	p.writeMu.Unlock()
	if <-finished || input.Len() != 0 {
		t.Fatal("old native TUI answer reached a reused RPC")
	}
}

func TestNativeAnswerHistorySurvivesResolutionArrivingFirst(t *testing.T) {
	request, err := decodeNativeQuestion(nativeMessage{ID: json.RawMessage(`7`), Method: "item/tool/requestUserInput", Params: json.RawMessage(`{"threadId":"thread","turnId":"turn","questions":[{"id":"secret","isSecret":true,"question":"Secret?"}]}`)})
	if err != nil {
		t.Fatal(err)
	}
	e := &entry{Native: true, NativeThreadID: "thread", Status: Status{State: "running"}, NativeRequests: []NativeRequest{request}}
	d := &daemon{root: t.TempDir(), entries: map[string]*entry{"H1": e}}
	resolved := d.nativeEvent(e, &NativeEvent{Server: true, RequestID: request.ID, Message: json.RawMessage(`{"method":"serverRequest/resolved","params":{"threadId":"thread","requestId":7}}`)})
	if resolved.Error != "" {
		t.Fatal(resolved.Error)
	}
	answer := d.nativeEvent(e, &NativeEvent{RequestID: request.ID, Message: json.RawMessage(`{"id":7,"result":{"answers":{"secret":{"answers":["private"]}}}}`)})
	if answer.Error != "" {
		t.Fatal(answer.Error)
	}
	if e.NativeRequests[0].State != "resolved" || e.NativeRequests[0].Source != "native client" || e.NativeRequests[0].Answers["secret"].Answers[0] != "[redacted]" {
		t.Fatal("late history lost or reopened resolution", e.NativeRequests[0])
	}
}
