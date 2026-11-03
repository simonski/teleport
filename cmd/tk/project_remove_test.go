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

func TestProjectRemoveRequiresExactConfirmationAndSendsReviewedRevision(t *testing.T) {
	deleted := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tools/listProjects":
			w.Write([]byte(`{"items":[{"id":3,"name":"Flight"}]}`))
		case "/api/tools/previewProjectRemoval":
			w.Write([]byte(`{"id":3,"name":"Flight","revision":9,"tickets":4,"ideas":2,"sessions":1,"warning":"History is removed"}`))
		case "/api/tools/deleteProject":
			var data map[string]any
			json.NewDecoder(r.Body).Decode(&data)
			if data["id"] != float64(3) || data["expectedRevision"] != float64(9) || data["confirm"] != true {
				t.Error(data)
			}
			deleted++
			w.Write([]byte(`{"id":3,"name":"Flight"}`))
		default:
			t.Error(r.URL.Path)
		}
	}))
	defer server.Close()
	config := filepath.Join(t.TempDir(), "client.json")
	if err := saveCredentials(config, credentials{server.URL, "token"}); err != nil {
		t.Fatal(err)
	}
	args := []string{"Flight", "-server", server.URL, "-config", config, "-json"}
	var out, diag bytes.Buffer
	if err := removeProjectCommand(args, strings.NewReader("flight\n"), &out, &diag); err == nil || deleted != 0 || !strings.Contains(diag.String(), "4 tickets") {
		t.Fatal("incorrect confirmation accepted", err)
	}
	out.Reset()
	diag.Reset()
	if err := removeProjectCommand(args, strings.NewReader("Flight\n"), &out, &diag); err != nil || deleted != 1 || !json.Valid(out.Bytes()) {
		t.Fatal("reviewed removal failed", err)
	}
}
