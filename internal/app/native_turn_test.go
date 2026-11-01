package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestNativeTurnEvidenceRemainsCreatorPrivateAndRechecksRole(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	svc.ConfigureLocalRuntime(t.TempDir(), "", false)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Native evidence", RequestID: "p"}).(model.Project)
	otherToken := "foreign-administrator-fixture"
	hash := sha256.Sum256([]byte(otherToken))
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		st.Accounts["other-admin"] = model.Account{ID: "other-admin", Username: "other-admin", Administrator: true}
		st.Sessions[hex.EncodeToString(hash[:])] = model.Session{AccountID: "other-admin", Expires: time.Now().Add(time.Hour)}
		st.LocalSessions = map[string]model.LocalSession{"H1": {ID: "H1", Creator: "admin", ProjectID: p.ID, State: "running"}}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	denied := func(credential, code string) {
		t.Helper()
		_, err := svc.Call(context.Background(), credential, "getSessionTurn", json.RawMessage(`{"id":"H1"}`))
		if err == nil || app.ErrorResponse(err).Code != code {
			t.Fatal("wrong evidence authorization boundary", code, err)
		}
	}
	denied(otherToken, "private_session")
	denied(token, "native_turn_unavailable")
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		account := st.Accounts["admin"]
		account.Administrator = false
		st.Accounts[account.ID] = account
		project := st.Projects[p.ID]
		project.Members["admin"] = "Viewer"
		st.Projects[p.ID] = project
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	denied(token, "forbidden")
}
