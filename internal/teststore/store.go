// Package teststore supplies isolated SQL installations to behavior tests.
package teststore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"teleport/internal/store"
)

// Location uses SQLite unless the disposable-cluster runner supplies a PostgreSQL URL.
// Every test receives a new database; never point this at an existing installation.
func Location(t *testing.T) string {
	t.Helper()
	base := os.Getenv("TELEPORT_TEST_POSTGRES")
	if base == "" {
		return filepath.Join(t.TempDir(), "database.db")
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatal("TELEPORT_TEST_POSTGRES requires a PostgreSQL URL")
	}
	root, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	var suffix [12]byte
	if _, err = rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "tk_test_" + hex.EncodeToString(suffix[:])
	if _, err = root.ExecContext(context.Background(), "CREATE DATABASE "+name); err != nil {
		root.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer root.Close()
		if _, err := root.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	u.Path = "/" + name
	return u.String()
}

func Open(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(Location(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}
