//go:build darwin || linux

// Package hostharness observes same-user harness processes without exposing arguments or terminal contents.
package hostharness

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Process struct {
	PID       int    `json:"pid"`
	ParentPID int    `json:"parentPid"`
	Harness   string `json:"harness"`
	State     string `json:"state"`
	Elapsed   string `json:"elapsed"`
	Identity  string `json:"identity"`
	Managed   bool   `json:"managed"`
}
type processRow struct {
	pid, ppid                  int
	state, elapsed, executable string
}

var names = map[string]bool{"codex": true, "claude": true, "gemini": true, "copilot": true, "opencode": true, "aider": true, "goose": true, "amp": true}

type cappedOutput struct{ bytes.Buffer }

func (b *cappedOutput) Write(p []byte) (int, error) {
	if len(p) > 8<<20-b.Len() {
		return 0, errors.New("process inventory exceeds limit")
	}
	return b.Buffer.Write(p)
}
func runPS(ctx context.Context, args ...string) ([]byte, error) {
	var output cappedOutput
	cmd := exec.CommandContext(ctx, "ps", args...)
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return nil, errors.New("cannot inspect local processes")
	}
	return output.Bytes(), nil
}

func scan(ctx context.Context) ([]processRow, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := runPS(ctx, "-u", strconv.Itoa(os.Geteuid()), "-o", "pid=", "-o", "ppid=", "-o", "state=", "-o", "etime=", "-o", "comm=")
	if err != nil {
		return nil, errors.New("cannot inspect local processes")
	}
	if len(out) > 8<<20 {
		return nil, errors.New("process inventory exceeds limit")
	}
	rows := []processRow{}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 4096), 65536)
	for scanner.Scan() {
		f := strings.Fields(scanner.Text())
		if len(f) < 5 {
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		ppid, e2 := strconv.Atoi(f[1])
		if e1 != nil || e2 != nil || pid < 1 {
			continue
		}
		rows = append(rows, processRow{pid, ppid, f[2], f[3], filepath.Base(strings.Join(f[4:], " "))})
	}
	if scanner.Err() != nil {
		return nil, errors.New("could not read process inventory")
	}
	return rows, nil
}

// interpreterHarness examines only an interpreter's script path; arguments are never returned or logged.
func interpreterHarness(executable, command string) string {
	executable = strings.ToLower(executable)
	if executable != "node" && executable != "nodejs" && !strings.HasPrefix(executable, "python") {
		return ""
	}
	words := strings.Fields(command)
	if len(words) < 2 {
		return ""
	}
	for index, value := range words[1:] {
		if value == "-m" && index+2 < len(words) && strings.HasPrefix(executable, "python") && (words[index+2] == "aider" || strings.HasPrefix(words[index+2], "aider.")) {
			return "aider"
		}
		if value == "-e" || value == "--eval" || value == "-c" || value == "--print" {
			return ""
		}
		if strings.HasPrefix(value, "-") {
			continue
		}
		normalized := strings.ToLower(value)
		base := filepath.Base(normalized)
		if strings.Contains(normalized, "/") {
			if names[base] {
				return base
			}
			switch {
			case strings.Contains(normalized, "/@anthropic-ai/claude-code/") && strings.HasSuffix(normalized, ".js"):
				return "claude"
			case strings.Contains(normalized, "/@google/gemini-cli/") && strings.HasSuffix(normalized, ".js"):
				return "gemini"
			case strings.Contains(normalized, "/@github/copilot/") && strings.HasSuffix(normalized, ".js"):
				return "copilot"
			case strings.HasSuffix(normalized, "/codex.js"):
				return "codex"
			}
		}
		return ""
	}
	return ""
}

func interpreterNames(ctx context.Context, rows []processRow) map[int]string {
	candidates := map[int]string{}
	for _, r := range rows {
		name := strings.ToLower(r.executable)
		if name == "node" || name == "nodejs" || strings.HasPrefix(name, "python") {
			candidates[r.pid] = name
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := runPS(ctx, "-u", strconv.Itoa(os.Geteuid()), "-o", "pid=", "-o", "args=")
	if err != nil || len(output) > 8<<20 {
		return nil
	}
	result := map[int]string{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 4096), 65536)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		split := strings.IndexAny(line, " \t")
		if split < 1 {
			continue
		}
		pid, err := strconv.Atoi(line[:split])
		if err != nil {
			continue
		}
		if executable, ok := candidates[pid]; ok {
			if name := interpreterHarness(executable, strings.TrimSpace(line[split:])); name != "" {
				result[pid] = name
			}
		}
	}
	return result
}

func List(ctx context.Context) ([]Process, error) {
	rows, err := scan(ctx)
	if err != nil {
		return nil, err
	}
	byPID := map[int]processRow{}
	for _, r := range rows {
		byPID[r.pid] = r
	}
	interpreted := interpreterNames(ctx, rows)
	self, _ := os.Executable()
	selfName := filepath.Base(self)
	result := []Process{}
	for _, r := range rows {
		name := strings.ToLower(r.executable)
		if interpreted[r.pid] != "" {
			name = interpreted[r.pid]
		}
		if !names[name] || strings.HasPrefix(r.state, "Z") {
			continue
		}
		identity, err := processIdentity(r.pid)
		if err != nil {
			continue
		}
		managed := false
		parent := r.ppid
		for depth := 0; parent > 1 && depth < 64; depth++ {
			a, ok := byPID[parent]
			if !ok || a.ppid == parent {
				break
			}
			if a.executable == "tk" || a.executable == selfName {
				managed = true
				break
			}
			parent = a.ppid
		}
		result = append(result, Process{r.pid, r.ppid, name, r.state, r.elapsed, identity, managed})
	}
	return result, nil
}
func Stop(ctx context.Context, pid int, identity string, force bool) error {
	if pid <= 1 || pid == os.Getpid() || identity == "" {
		return errors.New("invalid process identity")
	}
	rows, err := List(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.PID == pid {
			if r.Identity != identity {
				return errors.New("process changed; refresh the list")
			}
			if r.Managed {
				return errors.New("use Teleport session controls for managed processes")
			}
			return signalProcess(pid, identity, force)
		}
	}
	return fmt.Errorf("process is no longer present; refresh the list")
}
