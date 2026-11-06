package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"teleport/internal/store"
)

func TestDatabaseConfigurationRefusesMissingPublicAndLinkedFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.json")
	if _, err := databaseLocation("", path); err == nil {
		t.Fatal("missing configuration accepted")
	}
	if err := os.WriteFile(path, []byte(`{"url":"postgresql:///teleport"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if location, err := databaseLocation("", path); err != nil || location != "postgresql:///teleport" {
		t.Fatal("private configuration rejected", err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseLocation("", path); err == nil {
		t.Fatal("public configuration accepted")
	}
	link := path + "-link"
	if err := os.Symlink(path, link); err == nil {
		if _, err := databaseLocation("", link); err == nil {
			t.Fatal("linked configuration accepted")
		}
	}
}

func TestDatabaseEnvironmentPasswordIsAbsentFromFlagHelp(t *testing.T) {
	t.Setenv("TELEPORT_DB", "postgresql://owner:private-password@example.test/teleport")
	output, err := os.CreateTemp(t.TempDir(), "help-")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	previous := os.Stderr
	os.Stderr = output
	defer func() { os.Stderr = previous }()
	_ = run([]string{"init", "-h"})
	body, err := os.ReadFile(output.Name())
	if err != nil || strings.Contains(string(body), "private-password") {
		t.Fatal("database secret appeared in flag help", err)
	}
}

func TestStorageRefusesFileAndUnconfiguredFallback(t *testing.T) {
	for _, location := range []string{"", "installation.legacy", "file:///installation", "postgresql:///"} {
		if db, err := store.Open(location, true); err == nil {
			db.Close()
			t.Fatal("unsupported storage accepted")
		}
	}
}

func TestDatabaseConnectionFailureProvidesSafeRecoveryAdvice(t *testing.T) {
	for _, command := range []string{"init", "serve"} {
		t.Run(command, func(t *testing.T) {
			t.Setenv("TELEPORT_DB", "postgresql://owner:private-password@127.0.0.1:1/teleport?connect_timeout=1")
			err := run([]string{command})
			if err == nil || !strings.Contains(err.Error(), "make db-up") || !strings.Contains(err.Error(), "external database") || !strings.Contains(err.Error(), "retry tk "+command) {
				t.Fatalf("missing recovery advice: %v", err)
			}
			if strings.Contains(err.Error(), "private-password") {
				t.Fatal("recovery advice exposed database credentials")
			}
		})
	}
}
