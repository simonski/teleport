//go:build darwin || linux

package localruntime

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type nativeDispatch struct {
	RequestID   string          `json:"requestId"`
	OperationID string          `json:"operationId"`
	RPCID       json.RawMessage `json:"rpcId"`
	Result      json.RawMessage `json:"result"`
}
type nativePending struct {
	RequestID string
	Answered  bool
	Epoch     uint64
}
type nativeEventClient interface {
	Call(context.Context, Request) (Response, error)
}
type nativeProxy struct {
	failureCode     string
	openCommands    map[string]bool
	everCommand     bool
	cleanupAck      bool
	protocolFailed  bool
	telemetryFailed bool
	stopping        bool
	turnID          string
	mu              sync.Mutex
	writeMu         sync.Mutex
	input           io.WriteCloser
	connection      *websocket.Conn
	connected       chan struct{}
	pending         map[string]nativePending
	pendingEpoch    uint64
	operations      map[string]NativeReceipt
	threadID        string
	startedThread   bool
	queries         map[string]string
	client          nativeEventClient
	identity        Request
	key             string
	cwd             string
	role            string
}

func nativeSessionInstructions(role string) string {
	location := "Work only in the provided worktree."
	if role == "Console" {
		location = "Work in the provided project root."
	}
	return "\nThis is an owned teleport local harness. Read tk skill for supported context APIs. " + location + " Teleport human scope/code approval is never supplied by native OS permission or an agent. Direct terminal input is outside normal ticket supervision."
}

func constrainNativePhase(role, method, cwd string, params map[string]any) {
	if role != "Refine" && role != "Review" {
		return
	}
	delete(params, "permissions")
	params["approvalPolicy"] = "never"
	params["cwd"] = cwd
	if method == "thread/start" {
		params["sandbox"] = "read-only"
	} else if method == "turn/start" {
		params["sandboxPolicy"] = map[string]any{"type": "readOnly"}
	}
}

func nativeSocket(root, id string) string { return socketPath(filepath.Join(root, "native-"+id)) }
func (p *nativeProxy) report(server bool, message []byte, captured ...string) {
	if server {
		data, valid := compactNativeTelemetry(message)
		if !valid {
			p.mu.Lock()
			p.telemetryFailed = true
			p.mu.Unlock()
			message = []byte(`{"method":"teleport/captureError"}`)
		} else if len(data) == 0 {
			return
		} else {
			message = data
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request := p.identity
	request.Operation = "nativeEvent"
	request.NativeEvent = &NativeEvent{Server: server, Message: append([]byte(nil), message...)}
	var wire nativeMessage
	_ = json.Unmarshal(message, &wire)
	rpcID := wire.ID
	if wire.Method == "serverRequest/resolved" {
		var params struct {
			RequestID json.RawMessage `json:"requestId"`
		}
		_ = json.Unmarshal(wire.Params, &params)
		rpcID = params.RequestID
	}
	p.mu.Lock()
	request.NativeEvent.RequestID = p.pending[nativeID(rpcID)].RequestID
	p.mu.Unlock()
	if len(captured) > 0 {
		request.NativeEvent.RequestID = captured[0]
	}
	if _, err := p.client.Call(ctx, request); err != nil {
		p.mu.Lock()
		p.telemetryFailed = true
		p.mu.Unlock()
		request.NativeEvent = &NativeEvent{Server: true, Message: json.RawMessage(`{"method":"teleport/captureError"}`)}
		_, _ = p.client.Call(ctx, request)
	}
}

func (p *nativeProxy) retainFailure(code string, invalidProtocol bool) {
	if !validNativeFailure(code) {
		code = "native_protocol_read_failed"
	}
	p.mu.Lock()
	if p.stopping && code == "native_protocol_eof" {
		p.mu.Unlock()
		return
	}
	if invalidProtocol {
		p.protocolFailed = true
	}
	if p.failureCode != "" {
		p.mu.Unlock()
		return
	}
	p.failureCode = code
	p.mu.Unlock()
	slog.Warn("native interpreter failure", "session", p.identity.ID, "code", code)
	event, _ := json.Marshal(map[string]any{"method": "teleport/protocolFailure", "params": map[string]string{"code": code}})
	p.report(true, event)
}

func nativeParseFailure(err error) string {
	switch err.Error() {
	case "native protocol is not JSON":
		return "native_protocol_invalid_json"
	case "native thread identity changed":
		return "native_protocol_thread_changed"
	case "native event thread identity changed":
		return "native_protocol_event_thread_changed"
	default:
		return "native_protocol_read_failed"
	}
}
func (p *nativeProxy) send(message []byte) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return p.sendLocked(message)
}
func (p *nativeProxy) sendLocked(message []byte) error {
	if pipe, ok := p.input.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = pipe.SetWriteDeadline(time.Now().Add(2 * time.Second))
		defer pipe.SetWriteDeadline(time.Time{})
	}
	_, err := p.input.Write(append(message, '\n'))
	return err
}
func (p *nativeProxy) accept(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	if p.connection != nil {
		p.mu.Unlock()
		http.Error(w, "one native TUI owns this session", 409)
		return
	}
	connection, err := websocket.Accept(w, r, nil)
	if err != nil {
		p.mu.Unlock()
		return
	}
	p.connection = connection
	close(p.connected)
	p.mu.Unlock()
	defer connection.CloseNow()
	connection.SetReadLimit(8 << 20)
	for {
		typ, data, err := connection.Read(context.Background())
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			return
		}
		var message nativeMessage
		if json.Unmarshal(data, &message) != nil {
			return
		}
		p.mu.Lock()
		if message.Method != "" {
			var params map[string]any
			_ = json.Unmarshal(message.Params, &params)
			if message.Method == "initialize" {
				if params == nil {
					params = map[string]any{}
				}
				capabilities, _ := params["capabilities"].(map[string]any)
				if capabilities == nil {
					capabilities = map[string]any{}
				}
				capabilities["experimentalApi"] = true
				params["capabilities"] = capabilities
				message.Params, _ = json.Marshal(params)
				data, _ = json.Marshal(message)
			}
			deny := ""
			if message.Method == "thread/start" {
				if p.startedThread {
					deny = "Create another teleport session for another native thread"
				} else {
					p.startedThread = true
					if params == nil {
						params = map[string]any{}
					}
					params["cwd"] = p.cwd
					instructions, _ := params["developerInstructions"].(string)
					params["developerInstructions"] = instructions + nativeSessionInstructions(p.role)
					if p.role == "Refine" || p.role == "Review" {
						constrainNativePhase(p.role, message.Method, p.cwd, params)
					}
					message.Params, _ = json.Marshal(params)
					data, _ = json.Marshal(message)
				}
			}
			if message.Method != "thread/start" {
				if requested, _ := params["threadId"].(string); requested != "" && (p.threadID == "" || requested != p.threadID) {
					deny = "Only this teleport session's native thread is available"
				}
			}
			if message.Method == "turn/start" && (p.role == "Refine" || p.role == "Review") {
				if params == nil {
					params = map[string]any{}
				}
				constrainNativePhase(p.role, message.Method, p.cwd, params)
				message.Params, _ = json.Marshal(params)
				data, _ = json.Marshal(message)
			}
			if message.Method == "thread/fork" {
				deny = "Fork requires a separate teleport session"
			}
			if deny != "" {
				reply, _ := json.Marshal(map[string]any{"id": message.ID, "error": map[string]any{"code": -32600, "message": deny}})
				p.mu.Unlock()
				_ = connection.Write(context.Background(), websocket.MessageText, reply)
				continue
			}
			if len(message.ID) > 0 {
				p.queries[string(message.ID)] = message.Method
			}
		} else if len(message.ID) > 0 {
			captured, known := p.pending[nativeID(message.ID)]
			p.mu.Unlock()
			if !known {
				continue
			}
			written, err := p.writeNativeAnswer(data, message.ID, captured)
			if written {
				p.report(false, data, captured.RequestID)
			}
			if err != nil {
				return
			}
			continue
		}
		p.mu.Unlock()
		if p.send(data) != nil {
			return
		}
	}
}
func (p *nativeProxy) fromEngine(data []byte) ([]byte, error) {
	var message nativeMessage
	if json.Unmarshal(data, &message) != nil {
		p.mu.Lock()
		p.protocolFailed = true
		p.mu.Unlock()
		return nil, errors.New("native protocol is not JSON")
	}
	p.mu.Lock()
	var captured string
	if message.Method == "serverRequest/resolved" {
		var params struct {
			RequestID json.RawMessage `json:"requestId"`
		}
		if json.Unmarshal(message.Params, &params) == nil {
			key := nativeID(params.RequestID)
			if pending, ok := p.pending[key]; ok {
				pending.Answered = true
				p.pending[key] = pending
				captured = pending.RequestID
			}
		}
	}
	var scope struct {
		ThreadID string `json:"threadId"`
	}
	if json.Unmarshal(message.Params, &scope) == nil && scope.ThreadID != "" && p.threadID != "" && scope.ThreadID != p.threadID {
		p.protocolFailed = true
		p.mu.Unlock()
		return nil, errors.New("native event thread identity changed")
	}
	if message.Method == "turn/started" {
		var params struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if json.Unmarshal(message.Params, &params) == nil {
			p.turnID = params.Turn.ID
		}
	}
	if message.Method == "item/started" || message.Method == "item/completed" {
		var params struct {
			Item struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				ExitCode *int   `json:"exitCode"`
				Status   string `json:"status"`
			} `json:"item"`
		}
		if json.Unmarshal(message.Params, &params) == nil && params.Item.Type == "commandExecution" {
			if p.openCommands == nil {
				p.openCommands = map[string]bool{}
			}
			if message.Method == "item/started" {
				p.openCommands[params.Item.ID] = true
				p.everCommand = true
			} else if params.Item.ExitCode != nil || params.Item.Status == "declined" {
				delete(p.openCommands, params.Item.ID)
			}
		}
	}
	if message.Method == "thread/started" {
		var params struct {
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
		}
		if json.Unmarshal(message.Params, &params) == nil && params.Thread.ID != "" {
			if p.threadID != "" && p.threadID != params.Thread.ID {
				p.protocolFailed = true
				p.mu.Unlock()
				return nil, errors.New("native thread identity changed")
			}
			p.threadID = params.Thread.ID
		}
	}
	if len(message.ID) > 0 && message.Method != "" {
		request, _ := decodeNativeQuestion(message)
		p.pendingEpoch++
		p.pending[nativeID(message.ID)] = nativePending{RequestID: request.ID, Epoch: p.pendingEpoch}
		captured = request.ID
	}
	if len(message.ID) > 0 && message.Method == "" {
		method := p.queries[string(message.ID)]
		delete(p.queries, string(message.ID))
		if strings.HasPrefix(method, "teleport/") {
			if method == "teleport/clean" && len(message.Error) == 0 {
				p.cleanupAck = true
			}
			p.mu.Unlock()
			return nil, nil
		}
		if method == "thread/list" || method == "thread/loaded/list" {
			var result map[string]json.RawMessage
			if json.Unmarshal(message.Result, &result) == nil {
				result["data"] = json.RawMessage(`[]`)
				result["nextCursor"] = json.RawMessage(`null`)
				message.Result, _ = json.Marshal(result)
				data, _ = json.Marshal(message)
			}
		}
	}
	agentMessage := false
	if message.Method == "item/completed" {
		var params struct {
			Item struct {
				Type string `json:"type"`
			} `json:"item"`
		}
		_ = json.Unmarshal(message.Params, &params)
		agentMessage = params.Item.Type == "agentMessage"
	}
	relevant := agentMessage || message.Method == "thread/started" || message.Method == "thread/status/changed" || message.Method == "serverRequest/resolved" || message.Method == "turn/started" || message.Method == "turn/completed" || len(message.ID) > 0 && message.Method != ""
	p.mu.Unlock()
	if relevant {
		p.report(true, data, captured)
	}
	return data, nil
}

func (p *nativeProxy) writeNativeAnswer(data []byte, rpcID json.RawMessage, captured nativePending) (bool, error) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	key := nativeID(rpcID)
	pending, known := p.pending[key]
	if !known || pending.Answered || pending.RequestID != captured.RequestID || pending.Epoch != captured.Epoch || p.stopping {
		return false, nil
	}
	pending.Answered = true
	p.pending[key] = pending
	return true, p.sendLocked(data)
}
func (p *nativeProxy) respond(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+p.key)) != 1 {
		http.Error(w, "unauthorized", 401)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	defer r.Body.Close()
	var dispatch nativeDispatch
	if json.NewDecoder(r.Body).Decode(&dispatch) != nil || dispatch.OperationID == "" {
		http.Error(w, "invalid response", 400)
		return
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	p.mu.Lock()
	if previous, ok := p.operations[dispatch.OperationID]; ok {
		p.mu.Unlock()
		json.NewEncoder(w).Encode(previous)
		return
	}
	key := nativeID(dispatch.RPCID)
	pending, ok := p.pending[key]
	if !ok || pending.Answered || pending.RequestID == "" || pending.RequestID != dispatch.RequestID || p.telemetryFailed || p.stopping {
		p.mu.Unlock()
		http.Error(w, "request resolved", 409)
		return
	}
	pending.Answered = true
	p.pending[key] = pending
	result := NativeReceipt{OperationID: dispatch.OperationID, State: "uncertain"}
	p.operations[dispatch.OperationID] = result
	message, _ := json.Marshal(map[string]any{"id": dispatch.RPCID, "result": dispatch.Result})
	if p.sendLocked(message) == nil {
		result.State = "delivered"
	}
	p.operations[dispatch.OperationID] = result
	p.mu.Unlock()
	json.NewEncoder(w).Encode(result)
}

// RunNative owns the native engine and TUI, but never calls a model by itself.
// The native TUI starts turns only from the user's actual terminal input.
func RunNative(ctx context.Context, root, id, launch, generation, codex, role string) error {
	if !sessionID.MatchString(id) || !filepath.IsAbs(codex) || len(launch) != 64 || len(generation) != 64 {
		return errors.New("invalid native session identity")
	}
	key := os.Getenv("TELEPORT_NATIVE_KEY")
	if len(key) != 64 {
		return errors.New("native response capability missing")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	environment := runtimeEnv()
	for _, name := range []string{"TELEPORT_CONFIG", "TELEPORT_SERVER", "TELEPORT_SESSION_ID"} {
		if value := os.Getenv(name); value != "" {
			environment = append(environment, name+"="+value)
		}
	}
	engine := exec.Command(codex, "app-server", "--stdio")
	engine.Dir, engine.Env = cwd, environment
	engine.Stderr = os.Stderr
	input, err := engine.StdinPipe()
	if err != nil {
		return err
	}
	output, err := engine.StdoutPipe()
	if err != nil {
		return err
	}
	if err = engine.Start(); err != nil {
		return errors.New("native engine could not start")
	}
	proxy := &nativeProxy{input: input, connected: make(chan struct{}), pending: map[string]nativePending{}, operations: map[string]NativeReceipt{}, queries: map[string]string{}, client: New(root, ""), identity: Request{ID: id, LaunchID: launch, Generation: generation}, key: key, cwd: cwd, role: role}
	stopCtx, stop := context.WithCancel(ctx)
	defer stop()
	engineDone := make(chan error, 1)
	protocolDone := make(chan string, 1)
	// Drain the pipe before Wait closes it. Keep collecting exit evidence even
	// after the TUI disconnects; a disconnected viewer is not a command exit.
	go func() {
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 65536), 8<<20)
		for scanner.Scan() {
			data, parseErr := proxy.fromEngine(append([]byte(nil), scanner.Bytes()...))
			if parseErr != nil {
				proxy.retainFailure(nativeParseFailure(parseErr), true)
				stop()
				continue
			}
			if len(data) == 0 {
				continue
			}
			proxy.mu.Lock()
			connection, stopping := proxy.connection, proxy.stopping
			proxy.mu.Unlock()
			if connection == nil || stopping {
				continue
			}
			writeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			writeErr := connection.Write(writeCtx, websocket.MessageText, data)
			cancel()
			if writeErr != nil {
				proxy.retainFailure("native_frontend_write_failed", false)
				stop()
			}
		}
		code := "native_protocol_eof"
		if errors.Is(scanner.Err(), bufio.ErrTooLong) {
			code = "native_protocol_frame_too_large"
		} else if scanner.Err() != nil {
			code = "native_protocol_read_failed"
		}
		proxy.retainFailure(code, true)
		proxy.mu.Lock()
		code = proxy.failureCode
		proxy.mu.Unlock()
		protocolDone <- code
		engineDone <- engine.Wait()
		stop()
	}()
	var tui *exec.Cmd
	var tuiDone chan error
	defer func() {
		proxy.mu.Lock()
		proxy.stopping = true
		thread, turn := proxy.threadID, proxy.turnID
		proxy.mu.Unlock()
		if thread != "" {
			if turn != "" {
				proxy.internal("teleport/interrupt", "turn/interrupt", map[string]any{"threadId": thread, "turnId": turn})
			}
			proxy.internal("teleport/clean", "thread/backgroundTerminals/clean", map[string]any{"threadId": thread})
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				proxy.mu.Lock()
				ready := len(proxy.openCommands) == 0 && (!proxy.everCommand || proxy.cleanupAck)
				proxy.mu.Unlock()
				if ready {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
		}
		if tui != nil && tui.Process != nil {
			_ = tui.Process.Kill()
			<-tuiDone
		}
		_ = input.Close()
		engineExited := false
		var engineResult error
		select {
		case engineResult = <-engineDone:
			engineExited = true
		case <-time.After(2 * time.Second):
			_ = engine.Process.Kill()
			_ = output.Close()
			engineResult = <-engineDone
		}
		proxy.mu.Lock()
		confirmed := engineExited && engineResult == nil && !proxy.protocolFailed && len(proxy.openCommands) == 0 && (!proxy.everCommand || proxy.cleanupAck)
		connection := proxy.connection
		failed := proxy.failureCode != ""
		proxy.mu.Unlock()
		if failed && engine.ProcessState != nil {
			exit, _ := json.Marshal(map[string]any{"method": "teleport/engineExit", "params": map[string]int{"exitCode": engine.ProcessState.ExitCode()}})
			proxy.report(true, exit)
		}
		event, _ := json.Marshal(map[string]any{"method": "teleport/nativeExit", "params": map[string]any{"confirmed": confirmed}})
		proxy.report(true, event)
		if connection != nil {
			_ = connection.CloseNow()
		}
	}()
	listener, err := net.Listen("unix", nativeSocket(root, id))
	if err != nil {
		return errors.New("native proxy ownership unavailable")
	}
	defer listener.Close()
	if err = os.Chmod(nativeSocket(root, id), 0600); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", proxy.accept)
	mux.HandleFunc("POST /respond", proxy.respond)
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+key)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		stop()
		w.WriteHeader(200)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	defer server.Close()
	go server.Serve(listener)
	tui = exec.Command(codex, "--remote", "unix://"+nativeSocket(root, id), "--no-alt-screen", "-C", cwd)
	if prompt := os.Getenv("TELEPORT_MANAGED_PROMPT"); prompt != "" {
		tui.Args = append(tui.Args, prompt)
	}
	tui.Dir, tui.Env = cwd, environment
	tui.Stdin, tui.Stdout, tui.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err = tui.Start(); err != nil {
		return errors.New("native TUI could not start")
	}
	tuiDone = make(chan error, 1)
	go func() { tuiDone <- tui.Wait() }()
	select {
	case err = <-tuiDone:
		tuiDone <- err
		return err
	case <-stopCtx.Done():
		proxy.mu.Lock()
		code := proxy.failureCode
		proxy.mu.Unlock()
		if code != "" {
			return errors.New(code)
		}
		return nil
	case code := <-protocolDone:
		if code != "" {
			return errors.New(code)
		}
		return nil
	}
}

func (p *nativeProxy) internal(label, method string, params any) {
	id, _ := json.Marshal("teleport:" + label)
	p.mu.Lock()
	p.queries[string(id)] = label
	p.mu.Unlock()
	message, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	_ = p.send(message)
}
