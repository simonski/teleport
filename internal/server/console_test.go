package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConsoleCommandUsesCLICommandDefinitions(t *testing.T) {
	tests := []struct {
		args    []string
		op      string
		machine bool
	}{
		{[]string{"tk", "ticket", "show", "7"}, "getTicket", false},
		{[]string{"tk", "project", "ls", "-json"}, "listProjects", true},
		{[]string{"tk", "api", "getTicket", "-data", `{"id":7}`}, "getTicket", false},
	}
	for _, tc := range tests {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			result, err := resolveConsoleCommand(ConsoleCommandInput{Args: tc.args}, "test")
			if err != nil {
				t.Fatal(err)
			}
			if result.Operation != tc.op || result.Machine != tc.machine {
				t.Fatalf("unexpected result: %+v", result)
			}
			var data map[string]any
			if err := json.Unmarshal(result.Data, &data); err != nil {
				t.Fatal(err)
			}
			if tc.op == "getTicket" && data["id"] != float64(7) {
				t.Fatalf("unexpected input: %s", result.Data)
			}
		})
	}
}

func TestConsoleCommandHelpAndRestrictions(t *testing.T) {
	result, err := resolveConsoleCommand(ConsoleCommandInput{Args: []string{"tk", "ticket", "show", "-h"}}, "test")
	if err != nil || !strings.Contains(result.Output, "getTicket") {
		t.Fatalf("help: %+v, %v", result, err)
	}
	for _, args := range [][]string{
		{"tk", "serve"},
		{"tk", "login"},
		{"tk", "project", "rm", "7"},
		{"tk", "ticket", "show", "-f", "/etc/passwd"},
		{"tk", "api", "getTicket", "-data", `{"token":"secret"}`},
		{"tk", "ticket", "show", "7", "-server", "https://example.test"},
		{"globalThis.injected=true"},
	} {
		if _, err := resolveConsoleCommand(ConsoleCommandInput{Args: args}, "test"); err == nil {
			t.Errorf("expected rejection: %v", args)
		}
	}
}
