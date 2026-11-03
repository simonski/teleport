package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"teleport/internal/app"
	"teleport/internal/server"
)

func TestCommandCatalogueCoversEveryShippedOperationAndHasNoDuplicatePaths(t *testing.T) {
	operations, paths := map[string]bool{}, map[string]bool{}
	for _, command := range commandCatalogue() {
		if paths[command.Command] {
			t.Fatalf("duplicate command path %s", command.Command)
		}
		paths[command.Command] = true
		operations[command.Operation] = true
		if command.Operation != "" && command.Operation != "login" {
			if command.Input == nil {
				t.Fatalf("missing typed input for %s", command.Command)
			}
			var output bytes.Buffer
			_, done, err := expandOperation(command, []string{"-h"}, &output)
			if err != nil || !done || !strings.Contains(output.String(), command.Operation) {
				t.Fatalf("command help failed for %s: %v", command.Command, err)
			}
		}
	}
	for _, operation := range server.Operations {
		if !operations[operation.Name] {
			t.Errorf("operation is callable but undiscoverable: %s", operation.Name)
		}
	}
}

func TestCommandHelpAndMachineCatalogueAreOffline(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("TELEPORT_SERVER", "http://127.0.0.1:1")
	for _, arguments := range [][]string{nil, {"help"}, {"commands", "-json"}, {"help", "agent"}, {"agent"}, {"agent", "ls", "-h"}, {"help", "ticket", "show"}, {"help", "-h", "-h"}, {"-h", "-h", "-h"}} {
		var output bytes.Buffer
		_, done, err := expandCommand(arguments, &output)
		if err != nil || !done || output.Len() == 0 {
			t.Fatalf("offline help failed for %v: %v", arguments, err)
		}
		if len(arguments) > 1 && arguments[1] == "-json" && !json.Valid(output.Bytes()) {
			t.Fatal("machine catalogue is not JSON")
		}
	}
}

func TestDefaultHelpListsCommandsAndNounHelpListsOnlyItsActions(t *testing.T) {
	for _, arguments := range [][]string{nil, {"help"}, {"-h"}} {
		var output bytes.Buffer
		_, done, err := expandCommand(arguments, &output)
		if err != nil || !done {
			t.Fatalf("overview for %v: %v", arguments, err)
		}
		text := output.String()
		for _, name := range []string{"tk agent ", "tk ticket ", "tk init ", "tk import ", "tk logout "} {
			if !strings.Contains(text, name) {
				t.Errorf("overview missing %s", name)
			}
		}
		if strings.Contains(text, "tk agent ls ") || strings.Contains(text, "tk ticket show ") {
			t.Fatal("overview expanded action lists")
		}
	}
	for _, arguments := range [][]string{{"agent"}, {"help", "agent"}} {
		var output bytes.Buffer
		_, done, err := expandCommand(arguments, &output)
		if err != nil || !done || !strings.Contains(output.String(), "tk agent ls ") || strings.Contains(output.String(), "tk ticket show ") {
			t.Fatalf("wrong action list for %v: %v", arguments, err)
		}
	}
	var full bytes.Buffer
	_, _, err := expandCommand([]string{"commands"}, &full)
	if err != nil || !strings.Contains(full.String(), "tk agent ls ") || !strings.Contains(full.String(), "tk ticket show ") {
		t.Fatal("explicit catalogue is incomplete")
	}
}

func TestNounActionsUseTypedFieldsAndPreserveExactLargeIDs(t *testing.T) {
	for _, arguments := range [][]string{
		{"ticket", "show", "9007199254740993", "-server", "http://fixture", "-config", "private.json"},
		{"ticket", "get", "-id", "9007199254740993", "-server", "http://fixture", "-config", "private.json"},
	} {
		var output bytes.Buffer
		expanded, done, err := expandCommand(arguments, &output)
		if err != nil || done || expanded[0] != "getTicket" {
			t.Fatalf("alias failed: %v", err)
		}
		if expanded[2] != `{"id":9007199254740993}` {
			t.Fatalf("ID precision was lost: %s", expanded[2])
		}
	}
	var output bytes.Buffer
	expanded, _, err := expandCommand([]string{"agent", "list", "-project", "2", "-limit", "50", "-include-hidden=false"}, &output)
	if err != nil || expanded[0] != "listSessions" || !strings.Contains(expanded[2], `"projectId":2`) || !strings.Contains(expanded[2], `"includeHidden":false`) {
		t.Fatalf("typed list filters failed: %v %v", expanded, err)
	}
	expanded, _, err = expandCommand([]string{"agent", "input", "session-id", "-field-data", "[65,66]", "-request-id", "input-retry"}, &output)
	if err != nil {
		t.Fatal(err)
	}
	var typed app.SessionInput
	if err := json.Unmarshal([]byte(expanded[2]), &typed); err != nil || string(typed.Data) != "AB" || typed.ID != "session-id" {
		t.Fatalf("reserved field spelling does not match the real API type: %v", err)
	}
}

func TestNounActionsRejectConflictingSelectorsAndMalformedRequests(t *testing.T) {
	for _, arguments := range [][]string{
		{"ticket", "show", "7", "-id", "8"},
		{"ticket", "show", "7", "-data", `{"id":8}`},
		{"ticket", "show", "-id", "7", "-data", `{"id":7}`},
		{"ticket", "show", "-data", `[]`},
		{"ticket", "show", "-data", `{"unknown":7}`},
		{"ticket", "show", "-data", `{"id":7,"id":8}`},
		{"ticket", "show", "-data", `{"id":7}`, "-data", `{"id":8}`},
		{"ticket", "show", "-data", ""},
		{"ticket", "show", "-f", ""},
		{"ticket", "edit", "7"},
		{"ticket", "show", "-id", "not-an-integer"},
		{"ticket", "show", "-data", strings.Repeat(" ", 1<<20) + `{"id":7}`},
		{"agent", "ls", "-project"},
		{"agent", "not-an-action"},
	} {
		var output bytes.Buffer
		if _, _, err := expandCommand(arguments, &output); err == nil {
			t.Errorf("invalid request accepted: %v", arguments[:2])
		}
	}
}

func TestNounAgentListCallsExistingAPIWithCurrentCredentials(t *testing.T) {
	calls := 0
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/tools/listSessions" || r.Header.Get("Authorization") != "Bearer fixture-token" || r.Method != "POST" {
			t.Error("wrong operation, identity or method")
		}
		var input map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&input)
		if string(input["projectId"]) != "2" {
			t.Error("project filter changed")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"items":[],"nextCursor":""}`)
	}))
	defer fixture.Close()
	config := filepath.Join(t.TempDir(), "client.json")
	if err := saveCredentials(config, credentials{fixture.URL, "fixture-token"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"agent", "ls", "-project", "2", "-server", fixture.URL, "-config", config}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("agent ls started another operation")
	}
}

func TestNounRequestFilesPreserveNullAndSecretFieldsStayOffFlags(t *testing.T) {
	file := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(file, []byte(`{"id":7,"expectedVersion":2,"due":null}`), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	expanded, _, err := expandCommand([]string{"ticket", "edit", "-f", file}, &output)
	if err != nil || !strings.Contains(expanded[2], `"due":null`) {
		t.Fatalf("explicit clear changed: %v", err)
	}
	_, _, err = expandCommand([]string{"account", "password", "-new-password", "sensitive"}, &output)
	if err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatal("secret flag accepted or echoed")
	}
	_, _, err = expandCommand([]string{"account", "password", "-data", `{"oldPassword":"sensitive","newPassword":"sensitive"}`}, &output)
	if err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatal("secret argv JSON accepted or echoed")
	}
}

func TestLegacyOperationsAndExplicitAPIFallbackKeepTheirArguments(t *testing.T) {
	for _, arguments := range [][]string{{"listProjects", "-data", `{"limit":1}`}, {"api", "futureOperation", "-data", `{}`}} {
		var output bytes.Buffer
		expanded, done, err := expandCommand(arguments, &output)
		expected := arguments
		if arguments[0] == "api" {
			expected = arguments[1:]
		}
		if err != nil || done || strings.Join(expanded, "|") != strings.Join(expected, "|") {
			t.Fatalf("legacy API arguments changed: %v", err)
		}
	}
}

func TestExplicitAPIEscapeCannotReachNativeMaintenanceOrPrivateLaunchers(t *testing.T) {
	for _, name := range []string{"init", "serve", "import", "help", "commands", "skill", "version", "tools", "spec", "api", "_session-daemon", "_script-run", "", "../init", "init?x=y"} {
		var output bytes.Buffer
		if _, _, err := expandCommand([]string{"api", name}, &output); err == nil {
			t.Errorf("native/private name escaped API dispatch: %q", name)
		}
	}
}

func TestInvalidFlagValuesNeverEchoPrivateInputOrPolluteJSONStdout(t *testing.T) {
	var output bytes.Buffer
	_, _, err := expandCommand([]string{"agent", "ls", "-json", "-limit", "private-invalid-number"}, &output)
	if err == nil || output.Len() != 0 || strings.Contains(err.Error(), "private-invalid-number") {
		t.Fatal("flag parser leaked input or polluted stdout")
	}
	_, _, err = expandCommand([]string{"ticket", "show", "-data", `{"\u001b]52;c;payload\u0007":7}`}, &output)
	if err == nil || strings.ContainsAny(err.Error(), "\x1b\a") {
		t.Fatal("an untrusted JSON key executed terminal controls")
	}
}
