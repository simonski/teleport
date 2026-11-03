package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func inputFile(t *testing.T, text string) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.WriteString(text); err != nil {
		t.Fatal(err)
	}
	w.Close()
	t.Cleanup(func() { r.Close() })
	return r
}
func TestLoginEnvironmentAndStdinPrecedence(t *testing.T) {
	t.Setenv("TELEPORT_USERNAME", "environment-user")
	t.Setenv("TELEPORT_PASSWORD", " env secret ")
	var output bytes.Buffer
	username, password, err := loginInput("", false, inputFile(t, ""), &output)
	if err != nil || username != "environment-user" || password != " env secret " || output.Len() != 0 {
		t.Fatal("environment login failed", err)
	}
	username, password, err = loginInput("explicit-user", true, inputFile(t, "stdin-secret\n"), &output)
	if err != nil || username != "explicit-user" || password != "stdin-secret" {
		t.Fatal("explicit credentials lost precedence", err)
	}
	t.Setenv("TELEPORT_USERNAME", "")
	username, password, err = loginInput("", true, inputFile(t, "legacy-secret\n"), &output)
	if err != nil || username != "admin" || password != "legacy-secret" {
		t.Fatal("stdin compatibility changed", err)
	}
}
func TestLoginRejectsEmptyAndOversizedPasswords(t *testing.T) {
	t.Setenv("TELEPORT_USERNAME", "user")
	t.Setenv("TELEPORT_PASSWORD", "")
	if _, _, err := loginInput("", false, inputFile(t, ""), &bytes.Buffer{}); err == nil {
		t.Fatal("empty password accepted")
	}
	if _, _, err := loginInput("", true, inputFile(t, strings.Repeat("x", 1027)), &bytes.Buffer{}); err == nil {
		t.Fatal("oversized password accepted")
	}
}
