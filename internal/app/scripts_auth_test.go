//go:build darwin || linux

package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"teleport/internal/localruntime"
	"teleport/internal/model"
	"teleport/internal/store"
)

func TestScriptAdmissionAndAssentReauthenticateCapturedHuman(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "database.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = Initialize(ctx, db); err != nil {
		t.Fatal(err)
	}
	svc := New(db)
	value, err := svc.Call(ctx, "", "login", json.RawMessage(`{"username":"admin","password":"password"}`))
	if err != nil {
		t.Fatal(err)
	}
	token := value.(LoginResult).Token
	st, err := db.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	captured := st.Accounts["admin"]
	err = db.Update(ctx, func(st *model.State) (bool, error) {
		delete(st.Sessions, digest(token))
		st.ScriptRuns = map[string]model.ScriptRun{"J1": {ID: "J1", Creator: "admin", State: "waiting", ExpiresAt: time.Now().Add(time.Hour), Calls: []model.ScriptCall{{ID: "1", State: "waiting", Fingerprint: "exact"}}}}
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.confirmScript(ctx, token, captured, ConfirmScriptInput{ID: "J1", CallID: "1", Fingerprint: "exact", Decision: "approve", Confirm: true}); err == nil || ErrorResponse(err).Status != 401 {
		t.Fatal("revoked sign-in supplied assent", err)
	}
	runtimeRoot := t.TempDir()
	if err = os.Chmod(runtimeRoot, 0700); err != nil {
		t.Fatal(err)
	}
	configuration, _ := json.Marshal(map[string]any{"protocol": 1, "token": strings.Repeat("f", 64)})
	if err = os.WriteFile(filepath.Join(runtimeRoot, "control.json"), configuration, 0600); err != nil {
		t.Fatal(err)
	}
	daemonCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- localruntime.Run(daemonCtx, runtimeRoot) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	client := localruntime.New(runtimeRoot, "")
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err = client.Call(ctx, localruntime.Request{Operation: "ping"}); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture daemon unavailable")
		}
		time.Sleep(10 * time.Millisecond)
	}
	svc.ConfigureLocalRuntime(runtimeRoot, "/usr/bin/false", false, "http://localhost:9999")
	if _, err = svc.startScript(ctx, token, captured, StartScriptInput{RequestID: "revoked-run", Source: "return 1;", Mode: "script"}); err == nil || ErrorResponse(err).Status != 401 {
		t.Fatal("revoked sign-in admitted a new job", err)
	}
	st, err = db.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.ScriptRuns) != 1 || st.ScriptRuns["J1"].Calls[0].Approved {
		t.Fatal("revoked human mutated run or approval")
	}
}
