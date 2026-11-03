//go:build darwin || linux

package hostharness

import (
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSignalProcessRejectsChangedIdentityAndStopsOwnedFixture(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	identity, err := processIdentity(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if err = signalProcess(cmd.Process.Pid, identity+"-stale", false); err == nil {
		t.Fatal("stale identity signalled process")
	}
	if err = unix.Kill(cmd.Process.Pid, 0); err != nil {
		t.Fatal("stale identity affected fixture", err)
	}
	if err = signalProcess(cmd.Process.Pid, identity, false); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("fixture ignored termination")
	}
}

func TestInterpreterDetectionUsesScriptPathOnly(t *testing.T) {
	cases := []struct{ exe, command, want string }{
		{"node", "node /opt/node_modules/@anthropic-ai/claude-code/cli.js --argument", "claude"},
		{"node", "node /opt/node_modules/@google/gemini-cli/dist/index.js", "gemini"},
		{"node", "node -e /tmp/claude", ""},
		{"node", "node /tmp/other.js /tmp/claude", ""},
		{"python3", "python3 /usr/local/bin/aider", "aider"},
		{"python3", "python3 -m aider", "aider"},
	}
	for _, c := range cases {
		if got := interpreterHarness(c.exe, c.command); got != c.want {
			t.Errorf("%s: got %q want %q", c.exe, got, c.want)
		}
	}
}
