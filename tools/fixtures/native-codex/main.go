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
	"strings"
	"sync"

	"github.com/coder/websocket"
)

type message struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("codex-cli 0.166.0")
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "app-server" {
		engine()
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "--remote" {
		tui(strings.TrimPrefix(os.Args[2], "unix://"))
		return
	}
	fmt.Fprintln(os.Stderr, "native fixture requires an explicit fixture role")
	os.Exit(2)
}
func engine() {
	encoder := json.NewEncoder(os.Stdout)
	send := func(value any) {
		if encoder.Encode(value) != nil {
			os.Exit(1)
		}
	}
	event := func(method string, params any) { send(map[string]any{"method": method, "params": params}) }
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 65536), 8<<20)
	var turn int
	for scanner.Scan() {
		var m message
		if json.Unmarshal(scanner.Bytes(), &m) != nil {
			os.Exit(2)
		}
		if m.Method == "" {
			event("serverRequest/resolved", map[string]any{"threadId": "fixture-thread", "requestId": m.ID})
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
			result = map[string]any{"turn": map[string]any{"id": fmt.Sprint(turn)}}
		}
		if len(m.ID) > 0 {
			send(map[string]any{"id": m.ID, "result": result})
		}
		switch m.Method {
		case "thread/start":
			event("thread/started", map[string]any{"thread": map[string]any{"id": "fixture-thread"}})
		case "turn/start":
			event("turn/started", map[string]any{"threadId": "fixture-thread", "turn": map[string]any{"id": fmt.Sprint(turn)}})
			send(map[string]any{"id": fmt.Sprintf("ask-%d", turn), "method": "item/tool/requestUserInput", "params": map[string]any{"threadId": "fixture-thread", "turnId": fmt.Sprint(turn), "itemId": "fixture-item", "isBlocking": true, "questions": []any{map[string]any{"id": "direction", "header": "Direction", "question": "Fixture direction?", "isOther": false, "options": []any{map[string]any{"label": "Continue", "description": "Continue fixture"}, map[string]any{"label": "Stop", "description": "Stop fixture"}}}, map[string]any{"id": "secret", "header": "Secret", "question": "Fixture secret?", "isSecret": true}}}})
		}
	}
}
func tui(path string) {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}
	connection, _, err := websocket.Dial(context.Background(), "ws://native/", &websocket.DialOptions{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer connection.CloseNow()
	var mu sync.Mutex
	send := func(value any) {
		data, _ := json.Marshal(value)
		mu.Lock()
		defer mu.Unlock()
		_ = connection.Write(context.Background(), websocket.MessageText, data)
	}
	request := func(id, method string) {
		send(map[string]any{"id": id, "method": method, "params": map[string]any{"threadId": "fixture-thread"}})
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
