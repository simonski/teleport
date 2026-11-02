package app_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"teleport/internal/app"
	"teleport/internal/store"
)

func TestInitialAdminPasswordIsStrongRetrySafeAndNeverRotatesExistingCredentials(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "password.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	password := "fixture-strong-password-42"
	if _, _, err = app.InitializeWithOptions(ctx, db, app.InitializeOptions{AdminPassword: password}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = app.InitializeWithOptions(ctx, db, app.InitializeOptions{AdminPassword: password}); err != nil {
		t.Fatal("safe retry failed", err)
	}
	if _, _, err = app.InitializeWithOptions(ctx, db, app.InitializeOptions{AdminPassword: "another-strong-password"}); err == nil {
		t.Fatal("initialization rotated an existing password")
	}
	svc := app.New(db)
	for _, candidate := range []string{"password", password} {
		raw, _ := json.Marshal(map[string]string{"username": "admin", "password": candidate})
		_, err = svc.Call(ctx, "", "login", raw)
		if (err == nil) != (candidate == password) {
			t.Fatalf("unexpected login acceptance for fixture: %v", err)
		}
	}
}

func TestShortInitialAdminPasswordCannotInitializeStorage(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "empty.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, _, err = app.InitializeWithOptions(context.Background(), db, app.InitializeOptions{AdminPassword: "short"}); err == nil {
		t.Fatal("weak initial password accepted")
	}
}
