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
	"unicode/utf8"
)

func importFixture(t *testing.T, denied bool) (string, string, *[]map[string]any) {
	t.Helper()
	saved := []map[string]any{}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("capture changed the current identity")
		}
		w.Header().Set("Content-Type", "application/json")
		if denied {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":{"message":"Sign in to continue"}}`)
			return
		}
		switch r.URL.Path {
		case "/api/tools/getSession":
			io.WriteString(w, `{"account":{"id":"fixture"}}`)
		case "/api/tools/saveIdea":
			var input map[string]any
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			saved = append(saved, input)
			input["id"], input["version"] = "idea:41", 1
			json.NewEncoder(w).Encode(input)
		default:
			t.Error("capture attempted another operation: " + r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	t.Cleanup(fixture.Close)
	config := filepath.Join(t.TempDir(), "client.json")
	if err := saveCredentials(config, credentials{fixture.URL, "fixture-token"}); err != nil {
		t.Fatal(err)
	}
	return fixture.URL, config, &saved
}

func TestImportFileAndMultilineStdinPreserveTextAndCaptureOnlyIdeas(t *testing.T) {
	endpoint, config, saved := importFixture(t, false)
	text := "# Goals\r\nKeep Unicode: 月 🌙\n\n  preserve indentation and a trailing newline\n"
	file := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(file, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	for _, source := range [][]string{{file, "-project", "2"}, {"-i"}, {"-"}} {
		var out, diagnostics bytes.Buffer
		arguments := append(append([]string{}, source...), "-server", endpoint, "-config", config, "-request-id", "capture-retry", "-json")
		if err := captureText(arguments, strings.NewReader(text), &out, &diagnostics); err != nil {
			t.Fatal(err)
		}
		if !json.Valid(out.Bytes()) || strings.Contains(out.String(), "Request ID:") || !strings.Contains(diagnostics.String(), "capture-retry") {
			t.Fatal("result/diagnostic streams are not separated")
		}
		captured := (*saved)[len(*saved)-1]
		if captured["text"] != text || captured["requestId"] != "capture-retry" || captured["agent"] != nil {
			t.Fatal("capture changed source text, retry identity or execution intent")
		}
		if source[0] == file {
			if captured["projectId"] != float64(2) || captured["title"] != "notes.txt" {
				t.Fatal("explicit project or file title was lost")
			}
		} else if captured["projectId"] != nil || captured["title"] != "# Goals" {
			t.Fatal("stdin did not default to a private inbox with a useful title")
		}
	}
}

func TestImportRejectsOversizeEmptyInvalidAndConflictingInputWithoutSaving(t *testing.T) {
	endpoint, config, saved := importFixture(t, false)
	for _, text := range []string{strings.Repeat("x", ideaCaptureLimit+1), " \n\t", "invalid\x00text", string([]byte{0xff, 0xfe})} {
		var out, diagnostics bytes.Buffer
		if err := captureText([]string{"-i", "-server", endpoint, "-config", config}, strings.NewReader(text), &out, &diagnostics); err == nil {
			t.Fatal("invalid capture was accepted")
		}
	}
	for _, arguments := range [][]string{nil, {"-i", "file.txt"}, {"one.txt", "two.txt"}, {"-i", "-project", "-1"}} {
		var out, diagnostics bytes.Buffer
		if err := captureText(arguments, strings.NewReader("text"), &out, &diagnostics); err == nil {
			t.Fatal("conflicting capture selectors were accepted")
		}
	}
	if len(*saved) != 0 {
		t.Fatal("invalid capture caused a mutation")
	}
}

type unreadableCapture struct{ read bool }

func (r *unreadableCapture) Read(_ []byte) (int, error) {
	r.read = true
	return 0, io.EOF
}

func TestImportAuthenticatesBeforeConsumingAnInteractivePaste(t *testing.T) {
	endpoint, config, saved := importFixture(t, true)
	var out, diagnostics bytes.Buffer
	input := &unreadableCapture{}
	err := captureText([]string{"-i", "-server", endpoint, "-config", config}, input, &out, &diagnostics)
	if err == nil || input.read || len(*saved) != 0 {
		t.Fatal("unauthenticated capture consumed or saved a paste")
	}
}

func TestImportBoundaryAndDerivedUnicodeTitleAreNotTruncatedOrCorrupted(t *testing.T) {
	endpoint, config, saved := importFixture(t, false)
	text := strings.Repeat("月", 200) + "\n" + strings.Repeat("x", ideaCaptureLimit-601)
	var out, diagnostics bytes.Buffer
	if len(text) != ideaCaptureLimit {
		t.Fatal("bad boundary fixture")
	}
	if err := captureText([]string{"-i", "-server", endpoint, "-config", config}, strings.NewReader(text), &out, &diagnostics); err != nil {
		t.Fatal(err)
	}
	idea := (*saved)[0]
	title := idea["title"].(string)
	if idea["text"] != text || len(title) > 500 || !utf8.ValidString(title) || !strings.HasPrefix(idea["requestId"].(string), "cli-") {
		t.Fatal("boundary capture or request identity was corrupted")
	}
	if !strings.Contains(out.String(), "Classification follows separately") || strings.Contains(out.String(), text) {
		t.Fatal("capture claimed classification or dumped the pasted body")
	}
}

func TestImportHelpRequiresNoServerOrHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("TELEPORT_SERVER", "http://127.0.0.1:1")
	var out, diagnostics bytes.Buffer
	if err := captureText([]string{"-h"}, &unreadableCapture{}, &out, &diagnostics); err != nil || !strings.Contains(out.String(), "Ctrl-D") {
		t.Fatalf("offline import help failed: %v", err)
	}
}

func TestImportTerminalOutputEscapesControlsWithoutChangingStoredText(t *testing.T) {
	endpoint, config, saved := importFixture(t, false)
	text := "\x1b]52;c;payload\a\nrest of the idea\n"
	var out, diagnostics bytes.Buffer
	err := captureText([]string{"-i", "-server", endpoint, "-config", config, "-request-id", "request\n\x1b[2J"}, strings.NewReader(text), &out, &diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(out.String()+diagnostics.String(), "\x1b\a") || (*saved)[0]["text"] != text {
		t.Fatal("terminal controls were executed or source text was changed")
	}
}
