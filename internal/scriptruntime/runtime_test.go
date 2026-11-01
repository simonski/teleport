package scriptruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestLanguageAsyncBindingsAndNoAmbientHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0
	result := Eval(ctx, `const items=[];for(let i=0;i<3;i++){items.push((await tk("fixture",{i})).value)}console.log("count",items.length);return {items,ambient:[typeof process,typeof require,typeof fetch,typeof document,typeof os,typeof std,typeof __tk,typeof __log],version:tk.version};`, "script", func(_ context.Context, op string, args json.RawMessage) (json.RawMessage, error) {
		calls++
		if op != "fixture" {
			t.Fatal(op)
		}
		return json.RawMessage(`{"value":7}`), nil
	})
	if !result.Success || calls != 3 || len(result.Logs) != 1 || result.Logs[0] != "count 3" || !strings.Contains(string(result.Value), `"items":[7,7,7]`) || strings.Count(string(result.Value), `"undefined"`) != 8 {
		t.Fatal(result, calls)
	}
}
func TestTimeoutMemoryAndDisabledImports(t *testing.T) {
	for _, source := range []string{`while(true){}`, `await new Promise(()=>{});`, `return new ArrayBuffer(1000000000);`, `return await import("os");`, `return await import("file:///etc/passwd");`} {
		t.Run(source, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
			defer cancel()
			start := time.Now()
			result := Eval(ctx, source, "script", nil)
			if result.Success || result.Error == "" || time.Since(start) > 3*time.Second {
				t.Fatal("unbounded or ambient execution", result, time.Since(start))
			}
		})
	}
}
func TestExpressionAndOutputLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if result := Eval(ctx, "[1,2,3].reduce((a,b)=>a+b,0)", "expression", nil); !result.Success || string(result.Value) != "6" {
		t.Fatal(result)
	}
	result := Eval(ctx, `console.log("\u0001".repeat(60000));throw new Error("fixture");`, "script", nil)
	encoded, _ := json.Marshal(result)
	if result.Success || len(encoded) > OutputLimit {
		t.Fatal("encoded error output exceeded limit", len(encoded))
	}
}

func TestConsoleChoosesSyntaxBeforeExecutingExactlyOnce(t *testing.T) {
	for _, source := range []string{`1 + 2`, `1 + 2;`, `const x=1; return x+2;`, `await tk("fixture", {})`} {
		calls := 0
		out := Eval(context.Background(), source, "console", func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
			calls++
			return json.RawMessage(`3`), nil
		})
		if !out.Success || string(out.Value) != "3" || calls > 1 {
			t.Fatal(source, out, calls)
		}
	}
	for _, source := range []string{`await tk("fixture", {}); throw new SyntaxError("runtime failure");`, `(await tk("fixture", {}), (()=>{throw new SyntaxError("runtime failure")})())`} {
		calls := 0
		out := Eval(context.Background(), source, "console", func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
			calls++
			return json.RawMessage(`3`), nil
		})
		if out.Success || calls != 1 || !strings.Contains(out.Error, "runtime failure") {
			t.Fatal("Runtime failure retried executed code", out, calls)
		}
	}
	out := Eval(context.Background(), `[typeof document,typeof fetch,typeof process]`, "console", nil)
	if !out.Success || string(out.Value) != `["undefined","undefined","undefined"]` {
		t.Fatal("Console escaped its isolated realm", out)
	}
}
