package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const ideaCaptureLimit = 20000 // Matches the existing saveIdea text contract.

func captureText(arguments []string, input io.Reader, out, diagnostics io.Writer) error {
	set := flag.NewFlagSet("import", flag.ContinueOnError)
	set.SetOutput(out)
	stdin := set.Bool("i", false, "read multiline UTF-8 text from stdin until EOF (Ctrl-D)")
	project := set.Int64("project", 0, "destination project ID; omitted means the private Idea inbox")
	title := set.String("title", "", "Idea title; otherwise file basename or first nonblank input line")
	request := set.String("request-id", "", "stable ID for retrying the same capture")
	endpoint := set.String("server", "", "server URL (otherwise TELEPORT_SERVER/default)")
	config := set.String("config", "", "private client credential file")
	machine := set.Bool("json", false, "print the full captured Idea as JSON")
	set.Usage = func() {
		set.SetOutput(out)
		defer set.SetOutput(io.Discard)
		fmt.Fprintln(out, "Usage: tk import FILE [flags]\n       tk import -i [flags]\n       producer | tk import -i")
		fmt.Fprintln(out, "Capture text as an Idea; no classifier, goal creation or harness is started.")
		fmt.Fprintf(out, "Multiline text is preserved exactly, up to %s UTF-8 bytes; Ctrl-D finishes terminal input.\n", "20,000")
		set.PrintDefaults()
	}
	files, err := parseInterspersed(set, arguments)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(files) > 1 || *stdin && len(files) != 0 || !*stdin && len(files) != 1 {
		return errors.New("choose exactly one text source: tk import FILE or tk import -i")
	}
	if *project < 0 || len(strings.TrimSpace(*title)) > 500 || len(*request) > 100 || strings.ContainsRune(*request, 0) || *request != "" && strings.TrimSpace(*request) == "" {
		return errors.New("use a nonnegative project ID, title up to 500 bytes and request ID up to 100 bytes")
	}
	if !*stdin {
		if files[0] == "-" {
			*stdin = true
		} else {
			info, err := os.Stat(files[0])
			if err != nil {
				return fmt.Errorf("cannot inspect capture file %q", files[0])
			}
			if !info.Mode().IsRegular() {
				return errors.New("import FILE requires a regular text file; use -i for stdin")
			}
			file, err := os.Open(files[0])
			if err != nil {
				return fmt.Errorf("cannot open capture file %q", files[0])
			}
			defer file.Close()
			info, err = file.Stat()
			if err != nil || !info.Mode().IsRegular() {
				return errors.New("import FILE requires a regular text file; use -i for stdin")
			}
			if info.Size() > ideaCaptureLimit {
				return errors.New("capture exceeds the current 20,000-byte Idea limit; no content was imported")
			}
			input = file
			if *title == "" {
				*title = filepath.Base(files[0])
			}
		}
	}
	if *endpoint == "" {
		*endpoint = env("TELEPORT_SERVER", "http://localhost:9999")
	}
	*endpoint = strings.TrimRight(*endpoint, "/")
	if *config == "" {
		*config = os.Getenv("TELEPORT_CONFIG")
		if *config == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			*config = filepath.Join(home, ".teleport", "client.json")
		}
	}
	creds, err := readCredentials(*endpoint, *config)
	if err != nil {
		return err
	}
	if *stdin {
		// Fail authentication before asking for a long, otherwise ephemeral paste.
		if _, err := requestOperation(*endpoint, creds, "getSession", []byte(`{}`)); err != nil {
			return err
		}
		if file, ok := input.(*os.File); ok {
			if info, err := file.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(diagnostics, "Paste multiline text, then press Ctrl-D to capture it (maximum 20,000 UTF-8 bytes):")
			}
		}
	}
	body, err := io.ReadAll(io.LimitReader(input, ideaCaptureLimit+1))
	if err != nil {
		return errors.New("could not read capture text; no content was imported")
	}
	if len(body) > ideaCaptureLimit {
		return errors.New("capture exceeds the current 20,000-byte Idea limit; no content was imported")
	}
	if !utf8.Valid(body) || strings.TrimSpace(string(body)) == "" || strings.ContainsRune(string(body), 0) {
		return errors.New("capture must be nonblank UTF-8 text without NUL bytes")
	}
	if *title == "" {
		for _, line := range strings.Split(string(body), "\n") {
			if value := strings.TrimSpace(line); value != "" {
				*title = value
				break
			}
		}
	}
	*title = strings.TrimSpace(*title)
	for len(*title) > 500 {
		_, width := utf8.DecodeLastRuneInString(*title)
		*title = (*title)[:len(*title)-width]
	}
	if *title == "" || !utf8.ValidString(*title) {
		return errors.New("provide a nonblank -title")
	}
	if *request == "" {
		*request = newRequestID()
	}
	encoded, err := json.Marshal(struct {
		ProjectID int64  `json:"projectId,omitempty"`
		Title     string `json:"title"`
		Text      string `json:"text"`
		Origin    string `json:"origin"`
		RequestID string `json:"requestId"`
	}{*project, *title, string(body), "import", *request})
	if err != nil {
		return err
	}
	fmt.Fprintf(diagnostics, "Request ID: %q (reuse -request-id for an uncertain retry)\n", *request)
	result, err := requestOperation(*endpoint, creds, "saveIdea", encoded)
	if err != nil && err.Error() == "400 Bad Request: Invalid request fields or JSON" {
		// Older servers reject the additive origin field before any mutation.
		legacy, marshalErr := json.Marshal(struct {
			ProjectID int64  `json:"projectId,omitempty"`
			Title     string `json:"title"`
			Text      string `json:"text"`
			RequestID string `json:"requestId"`
		}{*project, *title, string(body), *request})
		if marshalErr != nil {
			return marshalErr
		}
		result, err = requestOperation(*endpoint, creds, "saveIdea", legacy)
	}
	if err != nil {
		return err
	}
	var idea struct {
		ID        string `json:"id"`
		Version   int64  `json:"version"`
		Title     string `json:"title"`
		ProjectID int64  `json:"projectId"`
	}
	if err := json.Unmarshal(result, &idea); err != nil || idea.ID == "" {
		return errors.New("unexpected capture response; inspect Ideas before retrying the same request ID")
	}
	if *machine {
		var value any
		if err := json.Unmarshal(result, &value); err != nil {
			return err
		}
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	location := "the private Idea inbox"
	if idea.ProjectID != 0 {
		location = fmt.Sprintf("project #%d", idea.ProjectID)
	}
	fmt.Fprintf(out, "Captured %q in %s: %q\nClassification follows separately; no goals or agent sessions were created.\n", idea.ID, location, idea.Title)
	return nil
}
