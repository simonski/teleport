package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"teleport/internal/app"
	"teleport/internal/store"
	"teleport/internal/teststore"
)

func TestInitReadsStrongAdminPasswordFromStdin(t *testing.T) {
	folder := t.TempDir()
	password := "fixture-initial-password-2026"
	input, err := os.CreateTemp(folder, "stdin-")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if _, err = input.WriteString(password + "\n"); err != nil {
		t.Fatal(err)
	}
	if _, err = input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	previous := os.Stdin
	os.Stdin = input
	t.Cleanup(func() { os.Stdin = previous })
	path := teststore.Location(t)
	if err = run([]string{"init", "--db", path, "--admin-password-stdin"}); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := app.New(db)
	for _, candidate := range []string{password, "password"} {
		raw, _ := json.Marshal(map[string]string{"username": "admin", "password": candidate})
		_, err = svc.Call(context.Background(), "", "login", raw)
		if (err == nil) != (candidate == password) {
			t.Fatal("CLI initialization used the wrong credential policy")
		}
	}
}
