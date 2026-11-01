package app

import (
	"encoding/json"
	"strings"

	"teleport/internal/model"
)

type PermissionInspectionInput struct {
	ProjectID int64    `json:"projectId"`
	Adapter   string   `json:"adapter"`
	Command   []string `json:"command"`
}
type AdapterCapabilities struct {
	Adapter                string   `json:"adapter"`
	Mode                   string   `json:"mode"`
	SchemaInspectedVersion string   `json:"schemaInspectedVersion"`
	ManagedExecution       bool     `json:"managedExecution"`
	MissingProofs          []string `json:"missingProofs"`
}
type PermissionInspection struct {
	Allowed            bool                `json:"allowed"`
	Executable         bool                `json:"executable"`
	WinningRestriction string              `json:"winningRestriction"`
	PolicyVersion      int64               `json:"policyVersion"`
	Capabilities       AdapterCapabilities `json:"capabilities"`
}

func capabilityOperations(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	if operation != "getEffectivePermission" {
		return nil, false, failure(404, "unsupported_operation", "Operation is not available in this build")
	}
	var in PermissionInspectionInput
	if err := parse(raw, &in); err != nil {
		return nil, false, err
	}
	if err := require(st, a, in.ProjectID, "Viewer"); err != nil {
		return nil, false, err
	}
	if !contains([]string{"codex-structured", "codex-pty"}, in.Adapter) {
		return nil, false, failure(422, "unsupported_adapter", "Select an explicitly declared Codex adapter")
	}
	if len(in.Command) < 1 || len(in.Command) > 128 || strings.TrimSpace(in.Command[0]) == "" {
		return nil, false, failure(422, "invalid_command", "Inspect an argv array of 1–128 entries")
	}
	total := 0
	for _, argument := range in.Command {
		total += len(argument)
		if len(argument) > 4096 || strings.ContainsRune(argument, 0) {
			return nil, false, failure(422, "invalid_command", "Command arguments must fit 4 KiB and contain no NUL")
		}
	}
	if total > 65536 {
		return nil, false, failure(422, "invalid_command", "Command arguments must fit 64 KiB combined")
	}
	mode := "structured JSONL"
	if in.Adapter == "codex-pty" {
		mode = "interactive PTY"
	}
	capabilities := AdapterCapabilities{Adapter: in.Adapter, Mode: mode, SchemaInspectedVersion: "codex-cli 0.166.0", ManagedExecution: false, MissingProofs: []string{"fresh server authorization before every protected command, including native automatic allows and console commands", "durable owned process/command identity, stop and exit recovery", "safe tool-result continuation without uncertain command replay", "read-only phase filesystem enforcement", "separately authorized live provider acceptance"}}
	return PermissionInspection{WinningRestriction: "adapter_capability_unverified", PolicyVersion: st.Settings.Version, Capabilities: capabilities}, false, nil
}
