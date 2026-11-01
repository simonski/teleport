//go:build darwin || linux

package localruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Client, context.Context) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(configuration{Protocol: 1, Token: identity()})
	if err := writeAtomic(filepath.Join(root, "control.json"), b); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, root) }()
	client := New(root, "")
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := client.Call(ctx, Request{Operation: "ping"}); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("daemon did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("owned runtime did not stop")
		}
	})
	return client, ctx
}
func await(t *testing.T, c *Client, ctx context.Context, in Request, predicate func(Response) bool) Response {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		out, err := c.Call(ctx, in)
		if err != nil && !errors.Is(err, ErrUnavailable) {
			t.Fatal(err)
		}
		if err == nil && predicate(out) {
			return out
		}
		if time.Now().After(deadline) {
			t.Fatalf("session did not reach expected state: %+v", out)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func TestPTYIdentityInputReceiptsAndReconnect(t *testing.T) {
	c, ctx := fixture(t)
	launch := Launch{ID: "H1", LaunchID: identity(), Program: "/bin/cat", Cwd: t.TempDir(), Cols: 80, Rows: 24}
	ping, _ := c.Call(ctx, Request{Operation: "ping"})
	launch.Generation = ping.Generation
	first, err := c.Call(ctx, Request{Operation: "launch", Launch: &launch})
	if err != nil {
		t.Fatal(err)
	}
	if first.Status.State != "running" {
		t.Fatalf("launch failed: %+v", first)
	}
	id := Request{ID: launch.ID, LaunchID: launch.LaunchID, Generation: first.Status.Generation}
	input := id
	input.Operation, input.Sequence, input.Data = "input", 1, []byte("first input\n")
	r, err := c.Call(ctx, input)
	if err != nil || r.Receipt.State != "delivered" {
		t.Fatal("PTY write failed", r, err)
	}
	read := id
	read.Operation = "read"
	out := await(t, c, ctx, read, func(out Response) bool { return strings.Count(string(out.Data), "first input") >= 2 })
	// A new API/controller client reconnects without starting another process.
	reconnected := New(c.Root, "")
	retry, err := reconnected.Call(ctx, Request{Operation: "launch", Launch: &launch})
	if err != nil || retry.Status.LaunchID != launch.LaunchID || retry.Status.Generation != first.Status.Generation {
		t.Fatal("launch retry replaced identity", err)
	}
	if _, err = reconnected.Call(ctx, input); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	out, err = reconnected.Call(ctx, read)
	if err != nil || strings.Count(string(out.Data), "first input") != 2 {
		t.Fatal("input replay duplicated effects", string(out.Data), err)
	}
	input.Data = []byte("different input\n")
	if _, err = reconnected.Call(ctx, input); err == nil {
		t.Fatal("conflicting input identity accepted")
	}
	resize := id
	resize.Operation, resize.Cols, resize.Rows = "resize", 120, 40
	if _, err = reconnected.Call(ctx, resize); err != nil {
		t.Fatal(err)
	}
	read.Cursor = out.Next
	out, err = reconnected.Call(ctx, read)
	if err != nil || len(out.Data) != 0 || out.Status.Cols != 120 {
		t.Fatal("cursor or resize lost", out, err)
	}
	stop := id
	stop.Operation = "stop"
	if _, err = c.Call(ctx, stop); err != nil {
		t.Fatal(err)
	}
	out = await(t, c, ctx, idWithOperation(id, "read"), func(out Response) bool { return out.Status.State == "exited" })
	if out.Status.ExitedAt == nil || out.Status.ExitCode == nil {
		t.Fatal("exit was not observed")
	}
	manifest, err := os.ReadFile(filepath.Join(c.Root, "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), "first input") {
		t.Fatal("raw keystrokes entered receipt audit")
	}
}
func idWithOperation(in Request, operation string) Request { in.Operation = operation; return in }

func TestOwnedGroupStopIncludesChildrenAndNaturalExitWaitsForThem(t *testing.T) {
	for _, natural := range []bool{false, true} {
		t.Run(map[bool]string{false: "explicit stop", true: "leader exits first"}[natural], func(t *testing.T) {
			c, ctx := fixture(t)
			ping, _ := c.Call(ctx, Request{Operation: "ping"})
			script := "trap '' HUP; sleep 30 & wait"
			if natural {
				script = "trap '' HUP; (trap '' HUP; sleep 1) & exit 0"
			}
			launch := Launch{ID: "H1", LaunchID: identity(), Generation: ping.Generation, Program: "/bin/sh", Args: []string{"-c", script}, Cwd: t.TempDir(), Cols: 80, Rows: 24}
			out, err := c.Call(ctx, Request{Operation: "launch", Launch: &launch})
			if err != nil {
				t.Fatal(err)
			}
			id := Request{ID: launch.ID, LaunchID: launch.LaunchID, Generation: out.Status.Generation}
			if natural {
				out = await(t, c, ctx, idWithOperation(id, "read"), func(out Response) bool { return out.Status.State == "unknown" })
				if out.Status.ExitCode != nil {
					t.Fatal("child outlived a falsely confirmed exit")
				}
				if _, err = c.Call(ctx, idWithOperation(id, "stop")); err == nil {
					t.Fatal("reaped-leader group accepted PID-only signalling")
				}
			}
			if !natural {
				time.Sleep(100 * time.Millisecond)
				if _, err = c.Call(ctx, idWithOperation(id, "stop")); err != nil {
					t.Fatal(err)
				}
			}
			out = await(t, c, ctx, idWithOperation(id, "read"), func(out Response) bool { return out.Status.State == "exited" })
			if out.Status.ExitedAt == nil {
				t.Fatal("owned group exit missing")
			}
		})
	}
}

func TestReplayEnvelopeKeepsItsOwnOffsetsAcrossMetadataMismatch(t *testing.T) {
	c, ctx := fixture(t)
	if _, err := c.Call(ctx, Request{Operation: "shutdown"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	e := &entry{Status: Status{ID: "H1", LaunchID: identity(), Generation: identity(), State: "exited", Base: 900, Next: 1000}, Receipts: map[int64]receipt{}}
	metadata, _ := json.Marshal(map[string]*entry{"H1": e})
	replay, _ := json.Marshal(replaySnapshot{Base: 100, Next: 106, Data: []byte("actual")})
	if err := writeAtomic(filepath.Join(c.Root, "sessions.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(filepath.Join(c.Root, "H1.output"), replay); err != nil {
		t.Fatal(err)
	}
	restartCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(restartCtx, c.Root) }()
	await(t, c, ctx, Request{Operation: "ping"}, func(Response) bool { return true })
	out, err := c.Call(ctx, Request{Operation: "read", ID: e.ID, LaunchID: e.LaunchID, Generation: e.Generation, Cursor: 1000})
	if err != nil || !out.Gap || out.Cursor != 100 || out.Next != 106 || string(out.Data) != "actual" {
		t.Fatal("snapshot bytes acquired invented offsets", out, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestUnknownOwnerRefusesReplacementAndInput(t *testing.T) {
	c, ctx := fixture(t)
	// Seed an unrecoverable prior-generation intent in the durable state, then
	// load it through a distinct runtime after the first runtime is idle/stopped.
	if _, err := c.Call(ctx, Request{Operation: "shutdown"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	launch := Launch{ID: "H1", LaunchID: identity(), Program: "/bin/cat", Cwd: t.TempDir(), Cols: 80, Rows: 24}
	launch.Generation = identity()
	fingerprint, _ := c.Fingerprint(&launch)
	e := &entry{Status: Status{ID: launch.ID, LaunchID: launch.LaunchID, Generation: identity(), State: "running"}, Fingerprint: fingerprint, Receipts: map[int64]receipt{}}
	b, _ := json.Marshal(map[string]*entry{"H1": e})
	if err := writeAtomic(filepath.Join(c.Root, "sessions.json"), b); err != nil {
		t.Fatal(err)
	}
	restartCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(restartCtx, c.Root) }()
	await(t, c, ctx, Request{Operation: "ping"}, func(Response) bool { return true })
	out, err := c.Call(ctx, Request{Operation: "launch", Launch: &launch})
	if err != nil || out.Status.State != "unknown" {
		t.Fatal("unproven intent started replacement", out, err)
	}
	input := Request{Operation: "input", ID: e.ID, LaunchID: e.LaunchID, Generation: e.Generation, Sequence: 1, Data: []byte("never\n")}
	if _, err = c.Call(ctx, input); err == nil {
		t.Fatal("unknown process accepted input")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
