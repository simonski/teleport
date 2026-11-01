package app_test

import (
	"context"
	"encoding/json"
	"testing"

	"teleport/internal/app"
	"teleport/internal/teststore"
)

func TestInitializationPreservesIdentityAndAuthentication(t *testing.T) {
	db := teststore.Open(t)
	ctx := context.Background()
	first, err := app.Initialize(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.Initialize(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if first.InstallationID == "" || first.InstallationID != second.InstallationID {
		t.Fatal("init changed installation identity")
	}
	if len(second.Workflows) != 2 || second.Settings.Agents || second.Settings.Registrations {
		t.Fatal("unsafe bootstrap defaults")
	}
	svc := app.New(db)
	login, err := svc.Call(ctx, "", "login", json.RawMessage(`{"username":"admin","password":"password"}`))
	if err != nil {
		t.Fatal(err)
	}
	token := login.(app.LoginResult).Token
	if _, err := svc.Call(ctx, token, "getSession", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Call(ctx, token, "logout", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Call(ctx, token, "getSession", json.RawMessage(`{}`)); err == nil {
		t.Fatal("revoked session still accepted")
	}
}
