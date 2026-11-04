package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"teleport/internal/app"
	"teleport/internal/model"
)

const proposalSchema = `{"type":"object","additionalProperties":false,"properties":{"title":{"type":"string"},"items":{"type":"array","items":{"type":"object","additionalProperties":false,"properties":{"key":{"type":"string"},"parentKey":{"type":"string"},"kind":{"type":"string","enum":["epic","task"]},"title":{"type":"string"},"description":{"type":"string"},"criteria":{"type":"string"}},"required":["key","parentKey","kind","title","description","criteria"]}}},"required":["title","items"]}`

type proposalEventWriter struct {
	capture *limitedBuffer
	pending []byte
	output  io.Writer
}

func (w *proposalEventWriter) Write(p []byte) (int, error) {
	_, _ = w.capture.Write(p)
	w.pending = append(w.pending, p...)
	for {
		index := bytes.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		line := w.pending[:index]
		w.pending = w.pending[index+1:]
		var event struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
			Item struct {
				Type             string `json:"type"`
				Text             string `json:"text"`
				Command          string `json:"command"`
				AggregatedOutput string `json:"aggregated_output"`
			} `json:"item"`
		}
		if json.Unmarshal(line, &event) != nil {
			fmt.Fprintln(w.output, string(line))
			continue
		}
		if event.Type == "item.started" && event.Item.Type == "command_execution" {
			fmt.Fprintln(w.output, "$ "+event.Item.Command)
		}
		if event.Type == "item.completed" && event.Item.Type == "agent_message" {
			fmt.Fprintln(w.output, event.Item.Text)
		}
		if event.Type == "item.completed" && event.Item.Type == "command_execution" && event.Item.AggregatedOutput != "" {
			fmt.Fprintln(w.output, event.Item.AggregatedOutput)
		}
		if event.Type == "assistant" {
			for _, part := range event.Message.Content {
				if part.Type == "text" && part.Text != "" {
					fmt.Fprintln(w.output, part.Text)
				}
			}
		}
	}
	if len(w.pending) > 1<<20 {
		fmt.Fprintln(w.output, string(w.pending))
		w.pending = nil
	}
	return len(p), nil
}

func claudeProposalResult(data []byte) (json.RawMessage, *model.TokenEvidence, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	var final json.RawMessage
	var evidence *model.TokenEvidence
	failed := false
	for scanner.Scan() {
		var event struct {
			Type       string          `json:"type"`
			IsError    bool            `json:"is_error"`
			Structured json.RawMessage `json:"structured_output"`
			Cost       *float64        `json:"total_cost_usd"`
			Usage      *struct {
				Input    *int64 `json:"input_tokens"`
				Output   *int64 `json:"output_tokens"`
				Cached   *int64 `json:"cache_read_input_tokens"`
				Creation *int64 `json:"cache_creation_input_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Type != "result" {
			continue
		}
		failed = event.IsError
		final = event.Structured
		if u := event.Usage; u != nil && u.Input != nil && u.Output != nil && u.Cached != nil && u.Creation != nil && *u.Input >= 0 && *u.Output >= 0 && *u.Cached >= 0 && *u.Creation >= 0 && *u.Input <= 1_000_000_000_000 && *u.Output <= 1_000_000_000_000 && *u.Cached <= 1_000_000_000_000 && *u.Creation <= 1_000_000_000_000 {
			evidence = &model.TokenEvidence{Input: *u.Input + *u.Cached + *u.Creation, Output: *u.Output, Cached: u.Cached, Source: "claude-print-json"}
			if event.Cost != nil && *event.Cost >= 0 && *event.Cost < 1e9 {
				cost := int64(math.Round(*event.Cost * 1e6))
				evidence.EstimateUSDMicros = &cost
			}
		}
	}
	if failed {
		return nil, evidence, errors.New("proposal harness returned an error")
	}
	if scanner.Err() != nil || len(final) == 0 {
		return nil, evidence, errors.New("proposal result missing")
	}
	return final, evidence, nil
}

func runPersonaProposal(harness string, args []string, persona personaLaunch) error {
	if persona.ProposalJob == nil || len(args) == 0 || os.Getenv("TELEPORT_SESSION_ID") == "" {
		return errors.New("invalid proposal launch")
	}
	dir, err := os.MkdirTemp("", "tk-proposal-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	schemaPath, resultPath := filepath.Join(dir, "schema.json"), filepath.Join(dir, "result.json")
	if err = os.WriteFile(schemaPath, []byte(proposalSchema), 0600); err != nil {
		return err
	}
	endpoint, config := os.Getenv("TELEPORT_SERVER"), os.Getenv("TELEPORT_CONFIG")
	readConfig, sessionID := os.Getenv("TELEPORT_PROPOSAL_READ_CONFIG"), os.Getenv("TELEPORT_SESSION_ID")
	if endpoint == "" || !filepath.IsAbs(config) || !filepath.IsAbs(readConfig) || filepath.Dir(config) != filepath.Dir(readConfig) || filepath.Base(config) != sessionID+".client.json" || filepath.Base(readConfig) != sessionID+".read.client.json" {
		return errors.New("proposal session credential unavailable")
	}
	info, err := os.Lstat(config)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4096 {
		return errors.New("proposal session credential ownership changed")
	}
	credentialBody, err := os.ReadFile(config)
	if err != nil {
		return errors.New("proposal session credential unavailable")
	}
	var credentialMeta struct {
		Kind      string `json:"kind"`
		SessionID string `json:"sessionId"`
		Server    string `json:"server"`
	}
	if json.Unmarshal(credentialBody, &credentialMeta) != nil || credentialMeta.Kind != "teleport-local-context" || credentialMeta.SessionID != sessionID || credentialMeta.Server != endpoint {
		return errors.New("proposal session credential ownership changed")
	}
	creds, err := readCredentials(endpoint, config)
	if err != nil {
		return err
	}
	if err = os.Remove(config); err != nil {
		return errors.New("proposal session credential could not be protected")
	}
	var context []byte
	if persona.ProposalJob.PreviousID != "" {
		context, err = requestOperation(endpoint, creds, "getProposalRunContext", []byte(`{}`))
		if err != nil {
			return fmt.Errorf("proposal context unavailable: %w", err)
		}
	}
	if persona.Agent.Harness == "codex" {
		projectDir, e := os.Getwd()
		if e != nil {
			return e
		}
		instruction := args[len(args)-1] + " The granted project directory for source inspection is " + projectDir + "."
		args = append(append(append([]string{}, args[:len(args)-1]...), "--json", "--skip-git-repo-check", "--cd", dir, "--output-schema", schemaPath, "--output-last-message", resultPath), instruction)
	}
	cmd := exec.Command(harness, args...)
	var events bytes.Buffer
	stream := &proposalEventWriter{capture: &limitedBuffer{b: &events, remaining: 2 << 20}, output: os.Stdout}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(context), stream, os.Stderr
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "TELEPORT_AGENT_PERSONA=") && !strings.HasPrefix(value, "TELEPORT_CONFIG=") && !strings.HasPrefix(value, "TELEPORT_PROPOSAL_READ_CONFIG=") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "TERM=xterm-256color", "TELEPORT_CONFIG="+readConfig)
	if executable, e := os.Executable(); e == nil && filepath.Base(executable) == "tk" {
		cmd.Env = append(cmd.Env, "PATH="+filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	runErr := cmd.Run()
	var data []byte
	var usage *model.TokenEvidence
	var resultErr error
	if persona.Agent.Harness == "codex" {
		usage = codexSpellcheckUsage(events.Bytes())
		if runErr == nil {
			info, e := os.Lstat(resultPath)
			if e != nil || !info.Mode().IsRegular() || info.Size() > 256*1024 {
				resultErr = errors.New("proposal result is missing or too large")
			} else {
				data, resultErr = os.ReadFile(resultPath)
			}
		}
	} else {
		data, usage, resultErr = claudeProposalResult(events.Bytes())
	}
	if usage != nil {
		evidence, _ := json.Marshal(app.ProposalRunUsageInput{Usage: usage})
		if _, e := requestOperation(endpoint, creds, "reportProposalRunUsage", evidence); e != nil {
			return fmt.Errorf("proposal usage recording failed: %w", e)
		}
	}
	if runErr != nil {
		return fmt.Errorf("proposal harness failed: %w", runErr)
	}
	if resultErr != nil || len(data) > 256*1024 {
		return errors.New("proposal result unavailable or too large")
	}
	var output struct {
		Title string                     `json:"title"`
		Items []model.TicketProposalItem `json:"items"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&output) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("invalid proposal result")
	}
	job := persona.ProposalJob
	request := app.TicketProposalInput{TicketID: job.TicketID, ExpectedVersion: job.TicketVersion, PreviousID: job.PreviousID, RequestID: "proposal-" + os.Getenv("TELEPORT_SESSION_ID"), Title: output.Title, Items: output.Items}
	encoded, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if _, err = requestOperation(endpoint, creds, "submitTicketProposal", encoded); err != nil {
		return fmt.Errorf("proposal submission failed: %w", err)
	}
	fmt.Fprintln(os.Stdout, "Proposal saved for human review.")
	return nil
}
