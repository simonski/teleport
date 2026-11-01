package server

import "teleport/internal/app"
import "teleport/internal/localruntime"

func init() {
	entries := []struct {
		name, action, summary string
		input, output         any
	}{
		{"getSessionRequests", "ACT-1015", "Read exact native pending requests and retained replies", app.NativeRequestsInput{}, app.NativeRequestPage{}},
		{"respondToAgent", "ACT-1015", "Answer an exact native request without human ticket approval", app.NativeResponseInput{}, localruntime.NativeReceipt{}},
		{"getLocalHarnesses", "ACT-1009", "Discover verified local harness choices without starting execution", struct{}{}, app.LocalHarnesses{}},
		{"listSessions", "ACT-1002", "Read authorized session identities without launching", app.ListInput{}, app.LocalSessionPage{}},
		{"openConsoleSession", "ACT-1009", "Explicitly start a private free harness in an owned console worktree", app.OpenLocalSessionInput{}, app.LocalSessionInfo{}},
		{"startTicketSession", "ACT-1001", "Explicitly attach a local harness to an eligible prepared task", app.OpenLocalSessionInput{}, app.LocalSessionInfo{}},
		{"attachSession", "ACT-1003", "Reconnect bounded replay from the existing owned PTY", app.AttachSessionInput{}, app.SessionView{}},
		{"sendSessionInput", "ACT-1005", "Send authorized terminal input with at-most-once receipt", app.SessionInput{}, app.SessionIntervention{}},
		{"resizeSession", "ACT-1006", "Resize the existing owned PTY", app.SessionResize{}, struct {
			Resized bool `json:"resized"`
		}{}},
		{"stopSession", "ACT-1008", "Record an explicit owned stop without claiming immediate exit", app.SessionStop{}, app.LocalSessionInfo{}},
		{"shutdownLocalRuntime", "ACT-1008", "Stop an idle owned local runtime without active or unknown sessions", struct {
			Confirm bool `json:"confirm"`
		}{}, struct {
			Shutdown bool `json:"shutdown"`
		}{}},
		{"listSessionInterventions", "ACT-1007", "Read bounded metadata-only terminal intervention audit", app.InterventionList{}, app.InterventionPage{}},
	}
	for _, e := range entries {
		Operations = append(Operations, Operation{e.name, e.action, e.summary})
		contracts[e.name] = contract{e.input, e.output}
	}
}
