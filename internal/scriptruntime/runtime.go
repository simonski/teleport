// Package scriptruntime embeds a bare, bounded JavaScript realm in an owned child.
package scriptruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	quickjs "github.com/buke/quickjs-go"
)

const SourceLimit = 64 << 10
const OutputLimit = 128 << 10

type Outcome struct {
	Success bool            `json:"success"`
	Value   json.RawMessage `json:"value,omitempty"`
	Logs    []string        `json:"logs"`
	Error   string          `json:"error,omitempty"`
}
type Config struct {
	ID       string `json:"id"`
	LaunchID string `json:"launchId"`
	Server   string `json:"server"`
	Token    string `json:"token"`
	Source   string `json:"source"`
	Mode     string `json:"mode"`
}
type Call func(context.Context, string, json.RawMessage) (json.RawMessage, error)

func Eval(ctx context.Context, source, mode string, call Call) (out Outcome) {
	out.Logs = []string{}
	defer func() {
		encoded, _ := json.Marshal(out)
		if len(encoded) > OutputLimit {
			out.Success = false
			out.Error = "Script output limit reached; inspect committed call receipts"
			out.Value = nil
			out.Logs = []string{}
		}
	}()
	if len(source) == 0 || len(source) > SourceLimit || strings.ContainsRune(source, 0) || mode != "script" && mode != "expression" && mode != "console" {
		out.Error = "Invalid bounded script source or mode"
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	rt := quickjs.NewRuntime(quickjs.WithModuleImport(false))
	defer rt.Close()
	rt.SetMemoryLimit(64 << 20)
	rt.SetMaxStackSize(1 << 20)
	rt.SetCanBlock(false)
	rt.SetInterruptHandler(func() int {
		if ctx.Err() != nil {
			return 1
		}
		return 0
	})
	realm := rt.NewBareContext()
	if realm == nil {
		out.Error = "JavaScript context unavailable"
		return
	}
	defer realm.Close()
	realm.Globals().Set("__tk", realm.NewFunction(func(c *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) != 2 || !args[0].IsString() || !args[1].IsString() {
			return c.ThrowError(errors.New("tk requires an operation and JSON arguments"))
		}
		operation, raw := args[0].ToString(), args[1].ToString()
		if len(operation) > 100 || len(raw) > 64<<10 || !json.Valid([]byte(raw)) {
			return c.ThrowError(errors.New("API arguments exceed the binding limit"))
		}
		result, err := call(ctx, operation, json.RawMessage(raw))
		if err != nil {
			return c.ThrowError(err)
		}
		if len(result) > 128<<10 || !json.Valid(result) {
			return c.ThrowError(errors.New("API result unavailable or exceeds the binding limit; inspect call receipts"))
		}
		return c.ParseJSON(string(result))
	}))
	logBytes := 0
	realm.Globals().Set("__log", realm.NewFunction(func(c *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) != 1 || !args[0].IsString() {
			return c.ThrowError(errors.New("Invalid log value"))
		}
		message := args[0].ToString()
		logBytes += len(message)
		if logBytes > 64<<10 || len(out.Logs) >= 256 {
			return c.ThrowError(errors.New("Script log limit reached"))
		}
		out.Logs = append(out.Logs, message)
		return c.NewUndefined()
	}))
	bootstrap := realm.Eval(`(()=>{"use strict";const call=globalThis.__tk,log=globalThis.__log;delete globalThis.__tk;delete globalThis.__log;const tk=async(operation,args={})=>call(operation,JSON.stringify(args));Object.defineProperty(tk,"version",{value:1});Object.freeze(tk);Object.defineProperty(globalThis,"tk",{value:tk});Object.defineProperty(globalThis,"console",{value:Object.freeze({log:(...items)=>log(items.map(value=>typeof value==="string"?value:JSON.stringify(value)).join(" "))})});})()`)
	if bootstrap.IsException() {
		out.Error = "JavaScript binding bootstrap failed"
		bootstrap.Free()
		return
	}
	bootstrap.Free()
	wrap := func(body string) string { return "(async()=>{\"use strict\";\n" + body + "\n})()" }
	program := wrap(source)
	if mode == "expression" {
		program = wrap("return (" + source + "\n);")
	} else if mode == "console" {
		expression := strings.TrimSuffix(strings.TrimSpace(source), ";")
		candidate := wrap("return (" + expression + "\n);")
		probe := realm.Eval(candidate, quickjs.EvalFlagCompileOnly(true))
		if probe.IsException() {
			_ = realm.Exception() // Clear the parse exception before statement execution.
		} else {
			program = candidate
		}
		probe.Free()
	}
	value := realm.Eval(program, quickjs.EvalAwait(true))
	defer value.Free()
	if value.IsException() {
		out.Error = realm.Exception().Error()
		if len(out.Error) > 4096 {
			out.Error = out.Error[:4096]
		}
		return
	}
	if value.IsUndefined() {
		out.Value = json.RawMessage(`null`)
	} else {
		out.Value = json.RawMessage(value.JSONStringify())
	}
	if realm.HasException() || !json.Valid(out.Value) {
		out.Error = "Result must be JSON serializable"
		out.Value = nil
		return
	}
	encoded, _ := json.Marshal(out)
	if len(encoded) > OutputLimit {
		out.Error = "Script output limit reached; inspect committed call receipts"
		out.Value = nil
		out.Logs = []string{}
		return
	}
	out.Success = true
	return
}

// RunFile is called only in the separate owned interpreter process. Its private
// config is never bound into JavaScript, and HTTP is available only to tk calls.
func RunFile(parent context.Context, path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("Absolute owned script configuration required")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("Private script configuration unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	var config Config
	if json.NewDecoder(io.LimitReader(file, SourceLimit*6+4096)).Decode(&config) != nil || len(config.Token) != 64 || len(config.LaunchID) != 64 || !strings.HasPrefix(config.ID, "J") {
		return errors.New("Invalid owned script configuration")
	}
	ctx, cancel := context.WithTimeout(parent, 120*time.Second)
	defer cancel()
	// A runtime/library defect cannot leave an unbounded child alive.
	hardStop := time.AfterFunc(121*time.Second, func() { os.Exit(124) })
	defer hardStop.Stop()
	sequence := 0
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("Script API redirects are forbidden") }}
	call := func(ctx context.Context, operation string, args json.RawMessage) (json.RawMessage, error) {
		if sequence >= 128 {
			return nil, errors.New("Script API call limit reached")
		}
		sequence++
		body, _ := json.Marshal(map[string]any{"id": config.ID, "callId": fmt.Sprint(sequence), "operation": operation, "arguments": args})
		for {
			request, err := http.NewRequestWithContext(ctx, "POST", config.Server+"/api/tools/scriptCall", bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			request.Header.Set("Authorization", "Bearer "+config.Token)
			request.Header.Set("Content-Type", "application/json")
			response, err := client.Do(request)
			if err != nil {
				// Retry only the same coordinator receipt lookup. A persisted
				// dispatch intent without a result is never dispatched again.
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(300 * time.Millisecond):
					continue
				}
			}
			var result struct {
				State  string          `json:"state"`
				Result json.RawMessage `json:"result"`
				Error  struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			err = json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&result)
			response.Body.Close()
			if err != nil {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(300 * time.Millisecond):
					continue
				}
			}
			if response.StatusCode != 200 || result.Error.Code != "" {
				return nil, fmt.Errorf("%s: %s", result.Error.Code, result.Error.Message)
			}
			if result.State != "waiting" {
				return result.Result, nil
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
	}
	result := Eval(ctx, config.Source, config.Mode, call)
	return json.NewEncoder(os.Stdout).Encode(result)
}
