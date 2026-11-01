package app_test

import (
	"context"
	"encoding/json"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestNativeCapabilityInspectionNeverAuthorizesExecution(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Capability", RequestID: "p"}).(model.Project)
	before := call(t, svc, token, "getSession", struct{}{}).(map[string]any)
	for _, adapter := range []string{"codex-structured", "codex-pty"} {
		permission := call(t, svc, token, "getEffectivePermission", app.PermissionInspectionInput{ProjectID: p.ID, Adapter: adapter, Command: []string{"git", "status"}}).(app.PermissionInspection)
		if permission.Allowed || permission.Executable || permission.Capabilities.ManagedExecution || len(permission.Capabilities.MissingProofs) == 0 {
			t.Fatal("inspection substituted for execution proof")
		}
	}
	settings := before["settings"].(model.Settings)
	raw, _ := json.Marshal(app.ExecutionModeInput{Mode: "codex", ExpectedVersion: settings.Version})
	if _, err := svc.Call(context.Background(), token, "setExecutionMode", raw); err == nil {
		t.Fatal("unsupported native mode enabled")
	}
	after := call(t, svc, token, "getSession", struct{}{}).(map[string]any)
	if after["revision"] != before["revision"] {
		t.Fatal("inspection or refused launch changed domain state")
	}
}
