package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func removeProjectCommand(args []string, input io.Reader, out, diagnostics io.Writer) error {
	flags := flag.NewFlagSet("project rm", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	endpoint := flags.String("server", env("TELEPORT_SERVER", "http://localhost:9999"), "server URL")
	home, _ := os.UserHomeDir()
	config := flags.String("config", env("TELEPORT_CONFIG", filepath.Join(home, ".teleport", "client.json")), "credential file")
	machine := flags.Bool("json", false, "print result as JSON")
	flags.Usage = func() {
		fmt.Fprintln(diagnostics, "Usage: tk project rm NAME [-server URL] [-config FILE] [-json]")
		flags.PrintDefaults()
	}
	names, err := parseInterspersed(flags, args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(names) != 1 || strings.TrimSpace(names[0]) == "" {
		return errors.New("provide exactly one project name")
	}
	*endpoint = strings.TrimRight(*endpoint, "/")
	creds, err := readCredentials(*endpoint, *config)
	if err != nil {
		return err
	}
	id, err := projectIDByName(*endpoint, creds, names[0])
	if err != nil {
		return err
	}
	request, _ := json.Marshal(map[string]any{"id": id})
	body, err := requestOperation(*endpoint, creds, "previewProjectRemoval", request)
	if err != nil {
		return err
	}
	var preview struct {
		ID                                  int64  `json:"id"`
		Name                                string `json:"name"`
		Revision                            int64  `json:"revision"`
		Tickets, Ideas, Sessions, Workflows int
		Warning                             string `json:"warning"`
	}
	if err = json.Unmarshal(body, &preview); err != nil || preview.ID != id || preview.Name == "" {
		return errors.New("invalid removal preview; no changes made")
	}
	fmt.Fprintf(diagnostics, "Remove project %q: %d tickets (including protected defaults), %d Ideas, %d session records and %d project workflows.\n%s\nType the exact project name to confirm: ", preview.Name, preview.Tickets, preview.Ideas, preview.Sessions, preview.Workflows, preview.Warning)
	scanner := bufio.NewScanner(io.LimitReader(input, 4096))
	if !scanner.Scan() || scanner.Text() != preview.Name {
		return errors.New("project removal cancelled; no changes made")
	}
	request, _ = json.Marshal(map[string]any{"id": id, "expectedRevision": preview.Revision, "confirm": true})
	body, err = requestOperation(*endpoint, creds, "deleteProject", request)
	if err != nil {
		return err
	}
	return printResponse(out, body, *machine)
}
