package server

import "teleport/internal/app"

func init() {
	entries := []struct {
		name, action, summary string
		input, output         any
	}{
		{"getScriptBindings", "ACT-1213", "Read versioned canonical script bindings without execution", struct{}{}, struct {
			Version  int                 `json:"version"`
			Bindings []app.ScriptBinding `json:"bindings"`
		}{}},
		{"startScriptRun", "ACT-1215", "Explicitly start a bounded account-owned isolated script job", app.StartScriptInput{}, app.ScriptInfo{}},
		{"listScriptRuns", "ACT-1216", "Read private-owner or administrator operational job metadata", app.ListInput{}, app.ScriptPage{}},
		{"getScriptRun", "ACT-1216", "Reconnect initiating-account private source and receipts", app.ScriptIDInput{}, app.ScriptView{}},
		{"stopScriptRun", "ACT-1216", "Withdraw authority and request owned interpreter stop", app.StopScriptInput{}, app.ScriptInfo{}},
		{"confirmScriptCall", "ACT-1217", "Record a fresh exact-action human decision without borrowing credentials", app.ConfirmScriptInput{}, struct {
			Recorded bool `json:"recorded"`
		}{}},
		{"scriptCall", "ACT-1215", "Private run-scoped canonical dispatch with durable per-call receipts", app.ScriptCallInput{}, app.ScriptCallResult{}},
	}
	for _, entry := range entries {
		Operations = append(Operations, Operation{entry.name, entry.action, entry.summary})
		contracts[entry.name] = contract{entry.input, entry.output}
	}
}
