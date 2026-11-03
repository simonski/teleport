package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupRejectsExistingProjectBeforePromptOrMutation(t *testing.T) {
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":1,"name":"Teleport"}]}`))
	}))
	defer server.Close()
	config := filepath.Join(t.TempDir(), "client.json")
	if err := saveCredentials(config, credentials{server.URL, "token"}); err != nil {
		t.Fatal(err)
	}
	var output, diagnostics bytes.Buffer
	input := &unreadableCapture{}
	err := setupProject([]string{"-server", server.URL, "-config", config}, input, &output, &diagnostics)
	if err == nil || !strings.Contains(err.Error(), "already exists") || input.read || len(calls) != 1 {
		t.Fatalf("setup changed existing project: %v %v", err, calls)
	}
}
func TestSetupCreatesProjectAndAppliesChosenSettings(t *testing.T) {
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		var data map[string]any
		json.NewDecoder(r.Body).Decode(&data)
		switch r.URL.Path {
		case "/api/tools/listProjects":
			w.Write([]byte(`{"items":[]}`))
		case "/api/tools/getSession":
			w.Write([]byte(`{"account":{"administrator":true},"settings":{"version":3}}`))
		case "/api/tools/createProject":
			if data["name"] != "Teleport" {
				t.Error(data)
			}
			w.Write([]byte(`{"id":1,"version":1}`))
		case "/api/tools/setLaneCapacity":
			if data["maxAgents"] != float64(2) {
				t.Error(data)
			}
			w.Write([]byte(`{"id":1,"version":2}`))
		case "/api/tools/setExecutionMode":
			if data["expectedVersion"] != float64(3) || data["mode"] != "off" {
				t.Error(data)
			}
			w.Write([]byte(`{}`))
		default:
			t.Error(r.URL.Path)
		}
	}))
	defer server.Close()
	config := filepath.Join(t.TempDir(), "client.json")
	if err := saveCredentials(config, credentials{server.URL, "token"}); err != nil {
		t.Fatal(err)
	}
	var output, diagnostics bytes.Buffer
	if err := setupProject([]string{"-server", server.URL, "-config", config, "-json"}, strings.NewReader("A project\n2\noff\n"), &output, &diagnostics); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(output.Bytes()) || len(calls) != 5 {
		t.Fatal(output.String(), calls)
	}
}
