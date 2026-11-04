// native-codex is an owned protocol fixture. It never contacts a provider.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

func proposalFixture(context []byte) string {
	result := `{"title":"Fixture breakdown","items":[{"key":"e","parentKey":"","kind":"epic","title":"Proposed epic","description":"Epic description","criteria":"Epic criteria"},{"key":"t","parentKey":"e","kind":"task","title":"Proposed task","description":"Task description","criteria":"Task criteria"}]}`
	if strings.Contains(string(context), "Split the epic") {
		result = strings.ReplaceAll(result, "Proposed epic", "Revised epic")
	}
	return result
}

func main() {
	history = toolhistory.Start("tools/fixtures/native-codex")
	defer history.End()
	for _, arg := range os.Args {
		if arg == "--json-schema" {
			context, err := io.ReadAll(os.Stdin)
			if err != nil {
				history.Exit(2)
			}
			fmt.Println(`{"type":"stream_event","event":{"type":"message_start"}}`)
			fmt.Println(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Planning proposal fixture"}}}`)
			fmt.Println(`{"type":"stream_event","event":{"type":"message_stop"}}`)
			time.Sleep(time.Second)
			if strings.Contains(strings.Join(os.Args, " "), "fixture-failure") {
				fmt.Println(`{"type":"result","is_error":true,"usage":{"input_tokens":100,"cache_read_input_tokens":20,"cache_creation_input_tokens":0,"output_tokens":30},"total_cost_usd":0.0003}`)
				history.Exit(1)
			}
			fmt.Println(`{"type":"result","is_error":false,"structured_output":` + proposalFixture(context) + `,"usage":{"input_tokens":100,"cache_read_input_tokens":20,"cache_creation_input_tokens":0,"output_tokens":30},"total_cost_usd":0.0003}`)
			return
		}
	}
	if len(os.Args) > 1 && os.Args[1] == "exec" {
		var output string
		var schema string
		for i, arg := range os.Args {
			if arg == "--output-last-message" && i+1 < len(os.Args) {
				output = os.Args[i+1]
			}
			if arg == "--output-schema" && i+1 < len(os.Args) {
				schema = os.Args[i+1]
			}
		}
		if schema != "" {
			data, err := os.ReadFile(schema)
			if err == nil && strings.Contains(string(data), `"parentKey"`) {
				context, readErr := io.ReadAll(os.Stdin)
				if readErr != nil {
					history.Exit(2)
				}
				fmt.Println(`{"type":"thread.started","thread_id":"fixture"}`)
				fmt.Println(`{"type":"turn.started"}`)
				fmt.Println(`{"type":"item.started","item":{"id":"cmd","type":"command_execution","command":"tk ticket show"}}`)
				fmt.Println(`{"type":"item.updated","item":{"id":"cmd","type":"command_execution","aggregated_output":"Planning proposal fixture\n"}}`)
				time.Sleep(time.Second)
				fmt.Println(`{"type":"item.completed","item":{"id":"cmd","type":"command_execution","aggregated_output":"Planning proposal fixture\n"}}`)
				fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":120,"cached_input_tokens":20,"output_tokens":30}}`)
				if strings.Contains(strings.Join(os.Args, " "), "fixture-failure") {
					history.Exit(1)
				}
				result := proposalFixture(context)
				if output == "" || os.WriteFile(output, []byte(result), 0600) != nil {
					history.Exit(2)
				}
				return
			}
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil || output == "" {
			history.Exit(2)
		}
		parts := strings.SplitN(string(data), "Ticket data: ", 2)
		var ticket struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		}
		if len(parts) != 2 || json.Unmarshal([]byte(parts[1]), &ticket) != nil {
			history.Exit(2)
		}
		if strings.Contains(ticket.Title, "slow") {
			time.Sleep(2 * time.Second)
		}
		fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":120,"cached_input_tokens":20,"output_tokens":30}}`)
		if strings.Contains(ticket.Title, "invalid") {
			if os.WriteFile(output, []byte(`{"title":"changed"}`), 0600) != nil {
				history.Exit(2)
			}
			return
		}
		ticket.Title = strings.ReplaceAll(ticket.Title, "teh", "the")
		ticket.Description = strings.ReplaceAll(ticket.Description, "teh", "the")
		result, _ := json.Marshal(ticket)
		if os.WriteFile(output, result, 0600) != nil {
			history.Exit(2)
		}
		return
	}
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
	if len(os.Args) > 2 && os.Args[1] == "-c" && strings.HasPrefix(os.Args[2], "developer_instructions=") && os.Getenv("TK_PERSONA_EXPECT") != "" {
		joined := strings.Join(os.Args[1:], " ")
		if !strings.Contains(joined, os.Getenv("TK_PERSONA_EXPECT")) || !strings.Contains(joined, "SKILL.md") || !strings.Contains(joined, "model_reasoning_effort=high") {
			fmt.Fprintln(os.Stderr, "persona fixture configuration missing")
			history.Exit(2)
		}
		if expected := os.Getenv("TK_PERSONA_EXPECT_TICKET"); expected != "" && !strings.Contains(joined, expected) {
			fmt.Fprintln(os.Stderr, "persona ticket context missing")
			history.Exit(2)
		}
		fmt.Println("persona configuration verified")
	}
	if len(os.Args) == 1 || os.Getenv("TK_PERSONA_EXPECT") != "" && len(os.Args) > 2 && os.Args[1] == "-c" {
		fmt.Println("direct Codex fixture ready")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if scanner.Text() == "quit" {
				return
			}
			fmt.Println("direct fixture received:", scanner.Text())
		}
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
