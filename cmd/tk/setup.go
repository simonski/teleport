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
	"strconv"
	"strings"
)

func setupProject(args []string, input io.Reader, out, diagnostics io.Writer) error {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	endpoint := flags.String("server", env("TELEPORT_SERVER", "http://localhost:9999"), "server URL")
	home, _ := os.UserHomeDir()
	config := flags.String("config", env("TELEPORT_CONFIG", filepath.Join(home, ".teleport", "client.json")), "client credentials")
	machine := flags.Bool("json", false, "print the project as JSON")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: tk setup [-server URL] [-config FILE] [-json]")
	}
	*endpoint = strings.TrimRight(*endpoint, "/")
	creds, err := readCredentials(*endpoint, *config)
	if err != nil {
		return err
	}
	// Check before any prompts or configuration changes. Creation enforces uniqueness again.
	body, err := requestOperation(*endpoint, creds, "listProjects", []byte(`{"query":"Teleport","limit":200}`))
	if err != nil {
		return err
	}
	var page struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
		NextCursor string `json:"nextCursor"`
	}
	if err = json.Unmarshal(body, &page); err != nil {
		return err
	}
	for _, p := range page.Items {
		if strings.EqualFold(strings.TrimSpace(p.Name), "Teleport") {
			return errors.New("Teleport project already exists; setup made no changes")
		}
	}
	if page.NextCursor != "" {
		return errors.New("too many matching projects to verify uniqueness; setup made no changes")
	}
	session, err := requestOperation(*endpoint, creds, "getSession", []byte(`{}`))
	if err != nil {
		return err
	}
	var current struct {
		Account struct {
			Administrator bool `json:"administrator"`
		} `json:"account"`
		Settings struct {
			Version       int64  `json:"version"`
			ExecutionMode string `json:"executionMode"`
		} `json:"settings"`
	}
	if err = json.Unmarshal(session, &current); err != nil {
		return err
	}
	scanner := bufio.NewScanner(io.LimitReader(input, 8192))
	ask := func(label string) (string, error) {
		fmt.Fprint(diagnostics, label)
		if !scanner.Scan() {
			return "", errors.New("setup input ended; no changes made")
		}
		return strings.TrimSpace(scanner.Text()), nil
	}
	description, err := ask("Teleport project description [empty]: ")
	if err != nil {
		return err
	}
	if len(description) > 20000 {
		return errors.New("description is too long")
	}
	maximum, err := ask("Maximum project agents [empty = unrestricted, 0–40]: ")
	if err != nil {
		return err
	}
	var max *int
	if maximum != "" {
		n, e := strconv.Atoi(maximum)
		if e != nil || n < 0 || n > 40 {
			return errors.New("maximum agents must be 0–40")
		}
		max = &n
	}
	mode := ""
	if current.Account.Administrator {
		mode, err = ask("Server execution mode [keep/off/demo/local]: ")
		if err != nil {
			return err
		}
		if mode != "" && mode != "keep" && mode != "off" && mode != "demo" && mode != "local" {
			return errors.New("choose keep, off, demo or local")
		}
	}
	request := newRequestID()
	data, _ := json.Marshal(map[string]any{"name": "Teleport", "key": "teleport", "description": description, "requestId": request})
	fmt.Fprintf(diagnostics, "Request ID: %q (reuse when reconciling an uncertain creation)\n", request)
	result, err := requestOperation(*endpoint, creds, "createProject", data)
	if err != nil {
		return err
	}
	var project struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}
	if json.Unmarshal(result, &project) != nil || project.ID == 0 {
		return errors.New("project creation response was invalid; inspect projects before retrying")
	}
	if max != nil {
		data, _ = json.Marshal(map[string]any{"projectId": project.ID, "expectedVersion": project.Version, "refineWorkers": 1, "doingWorkers": 1, "maxAgents": *max})
		result, err = requestOperation(*endpoint, creds, "setLaneCapacity", data)
		if err != nil {
			return fmt.Errorf("Teleport project was created, but agent capacity was not configured: %w", err)
		}
	}
	if mode != "" && mode != "keep" {
		data, _ = json.Marshal(map[string]any{"mode": mode, "expectedVersion": current.Settings.Version})
		if _, err = requestOperation(*endpoint, creds, "setExecutionMode", data); err != nil {
			return fmt.Errorf("Teleport project was created, but server execution mode was not configured: %w", err)
		}
	}
	return printResponse(out, result, *machine)
}
