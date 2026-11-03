// native-codex is an owned protocol fixture. It never contacts a provider.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/coder/websocket"
	"teleport/tools/toolhistory"
)

type message struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
}

var history *toolhistory.Call

func main() {
	history = toolhistory.Start("tools/fixtures/native-codex")
	defer history.End()
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("codex-cli 0.166.0")
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "app-server" {
		engine()
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "--remote" {
		cwd, err := os.Getwd()
		if err != nil {
			history.Exit(2)
		}
		valid := false
		for i, arg := range os.Args {
			if (arg == "-C" || arg == "--cd") && i+1 < len(os.Args) && os.Args[i+1] == cwd {
				valid = true
			}
		}
		if !valid {
			fmt.Fprintln(os.Stderr, "native fixture requires the exact session directory")
			history.Exit(2)
		}
		tui(strings.TrimPrefix(os.Args[2], "unix://"))
		return
	}
	fmt.Fprintln(os.Stderr, "native fixture requires an explicit fixture role")
	history.Exit(2)
}
func engine() {
	encoder := json.NewEncoder(os.Stdout)
	send := func(value any) {
		if encoder.Encode(value) != nil {
			history.Exit(1)
		}
	}
	event := func(method string, params any) { send(map[string]any{"method": method, "params": params}) }
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 65536), 8<<20)
	var turn int
	var managedRole string
	for scanner.Scan() {
		var m message
		if json.Unmarshal(scanner.Bytes(), &m) != nil {
			history.Exit(2)
		}
		if m.Method == "" {
			event("serverRequest/resolved", map[string]any{"threadId": "fixture-thread", "requestId": m.ID})
			answer := "Native fixture completed"
			if managedRole != "" {
				answer = `{"outcome":"success","proposal":"Update fixture.txt in the owned worktree","criteria":"fixture.txt contains delivered and main changes only after human approval"}`
			}
			event("item/completed", map[string]any{"threadId": "fixture-thread", "turnId": fmt.Sprint(turn), "item": map[string]any{"type": "agentMessage", "text": answer, "phase": "final_answer"}})
			event("turn/completed", map[string]any{"threadId": "fixture-thread", "turn": map[string]any{"id": fmt.Sprint(turn), "status": "completed"}})
			continue
		}
		result := any(map[string]any{})
		switch m.Method {
		case "initialize":
			result = map[string]any{"userAgent": "zero-provider-fixture"}
		case "thread/start":
			result = map[string]any{"thread": map[string]any{"id": "fixture-thread"}}
		case "turn/start":
			turn++
			var params struct {
				Input []struct {
					Text string `json:"text"`
				} `json:"input"`
			}
			_ = json.Unmarshal(m.Params, &params)
			if len(params.Input) > 0 && strings.HasPrefix(params.Input[0].Text, "[system]") {
				prompt := params.Input[0].Text
				managedRole = "Refine"
				if strings.Contains(prompt, "[phase]\nImplement only") {
					managedRole = "Doing"
				}
				if strings.Contains(prompt, "[phase]\nReview is") {
					managedRole = "Review"
				}
			}
			result = map[string]any{"turn": map[string]any{"id": fmt.Sprint(turn)}}
		}
		if len(m.ID) > 0 {
			send(map[string]any{"id": m.ID, "result": result})
		}
		switch m.Method {
		case "thread/start":
			event("thread/started", map[string]any{"thread": map[string]any{"id": "fixture-thread"}})
			if os.Getenv("TK_NATIVE_FIXTURE_LARGE") == "1" {
				event("fixture/large", map[string]any{"data": strings.Repeat("x", 9<<20)})
			}
		case "turn/start":
			event("turn/started", map[string]any{"threadId": "fixture-thread", "turn": map[string]any{"id": fmt.Sprint(turn)}})
			if managedRole == "Doing" || managedRole == "Review" {
				if managedRole == "Doing" {
					if err := os.WriteFile(filepath.Join(".", "fixture.txt"), []byte("delivered\n"), 0600); err != nil {
						panic(err)
					}
					for _, args := range [][]string{{"add", "fixture.txt"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@localhost", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + os.DevNull, "commit", "-m", "Implement fixture"}} {
						if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
							panic(string(out))
						}
					}
				}
				event("item/completed", map[string]any{"threadId": "fixture-thread", "turnId": fmt.Sprint(turn), "item": map[string]any{"type": "agentMessage", "text": `{"outcome":"success","findings":"Fixture inspected the exact candidate; fixture.txt contains delivered"}`, "phase": "final_answer"}})
				event("turn/completed", map[string]any{"threadId": "fixture-thread", "turn": map[string]any{"id": fmt.Sprint(turn), "status": "completed"}})
				continue
			}
			send(map[string]any{"id": fmt.Sprintf("ask-%d", turn), "method": "item/tool/requestUserInput", "params": map[string]any{"threadId": "fixture-thread", "turnId": fmt.Sprint(turn), "itemId": "fixture-item", "isBlocking": true, "questions": []any{map[string]any{"id": "direction", "header": "Direction", "question": "Fixture direction?", "isOther": false, "options": []any{map[string]any{"label": "Continue", "description": "Continue fixture"}, map[string]any{"label": "Stop", "description": "Stop fixture"}}}, map[string]any{"id": "secret", "header": "Secret", "question": "Fixture secret?", "isSecret": true}}}})
		}
	}
}
func tui(path string) {
	prompt := ""
	if last := os.Args[len(os.Args)-1]; strings.HasPrefix(last, "[system]") {
		prompt = last
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}
	connection, _, err := websocket.Dial(context.Background(), "ws://native/", &websocket.DialOptions{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		history.Exit(1)
	}
	defer connection.CloseNow()
	connection.SetReadLimit(64 << 20)
	var mu sync.Mutex
	send := func(value any) {
		data, _ := json.Marshal(value)
		mu.Lock()
		defer mu.Unlock()
		_ = connection.Write(context.Background(), websocket.MessageText, data)
	}
	request := func(id, method string) {
		params := map[string]any{"threadId": "fixture-thread"}
		if method == "turn/start" && prompt != "" {
			params["input"] = []any{map[string]any{"type": "text", "text": prompt}}
		}
		send(map[string]any{"id": id, "method": method, "params": params})
	}
	request("init", "initialize")
	var pending json.RawMessage
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			mu.Lock()
			id := append(json.RawMessage(nil), pending...)
			mu.Unlock()
			if len(id) > 0 {
				send(map[string]any{"id": id, "result": map[string]any{"answers": map[string]any{"direction": map[string]any{"answers": []string{"Continue"}}, "secret": map[string]any{"answers": []string{"fixture-secret"}}}}})
			} else {
				request("turn", "turn/start")
			}
		}
	}()
	for {
		_, data, err := connection.Read(context.Background())
		if err != nil {
			return
		}
		var m message
		if json.Unmarshal(data, &m) != nil {
			return
		}
		if string(m.ID) == `"init"` {
			request("thread", "thread/start")
		}
		if string(m.ID) == `"thread"` {
			fmt.Println("native fixture connected")
			request("turn", "turn/start")
		}
		if m.Method == "fixture/large" && len(data) > 9<<20 {
			fmt.Println("native fixture large frame received")
		}
		if m.Method == "item/tool/requestUserInput" {
			mu.Lock()
			pending = m.ID
			mu.Unlock()
			fmt.Println("Fixture direction? Fixture secret?")
		}
		if m.Method == "serverRequest/resolved" {
			mu.Lock()
			pending = nil
			mu.Unlock()
			fmt.Println("native fixture answer resolved")
		}
	}
}
