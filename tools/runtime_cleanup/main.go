// runtime_cleanup stops only explicitly selected owned test-fixture runtimes.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"teleport/internal/localruntime"
	"teleport/tools/toolhistory"
)

var history *toolhistory.Call

func main() {
	history = toolhistory.Start("tools/runtime_cleanup")
	defer history.End()
	flags := flag.NewFlagSet("runtime_cleanup", flag.ContinueOnError)
	folder := flags.String("fixture", "", "explicit owned fixture directory")
	stop := flags.Bool("stop", false, "request owned stops and idle shutdown")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return
		}
		history.Exit(2)
	}
	if !filepath.IsAbs(*folder) || !strings.HasPrefix(filepath.Base(*folder), "teleport-terminal-proof-") && !strings.HasPrefix(filepath.Base(*folder), "teleport-script-proof-") && !strings.HasPrefix(filepath.Base(*folder), "teleport-workflow-proof-") {
		fail("Select the exact owned terminal/script/workflow fixture directory")
	}
	root := filepath.Join(*folder, "runtime")
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		fail("Private fixture runtime is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := localruntime.New(root, "")
	initial, err := client.Call(ctx, localruntime.Request{Operation: "list"})
	if err != nil {
		fail("Owned fixture daemon is unavailable; no takeover attempted")
	}
	for _, row := range initial.Items {
		fmt.Println(row.ID, row.State)
		if *stop && row.State != "exited" && row.State != "failed" {
			if _, err = client.Call(ctx, localruntime.Request{Operation: "stop", ID: row.ID, LaunchID: row.LaunchID, Generation: row.Generation}); err != nil {
				fail("Owned stop could not be confirmed")
			}
		}
	}
	if !*stop {
		return
	}
	for {
		out, err := client.Call(ctx, localruntime.Request{Operation: "list"})
		if err != nil {
			fail("Ownership observation failed; retain fixture evidence")
		}
		done := true
		for _, row := range out.Items {
			done = done && (row.State == "exited" || row.State == "failed")
		}
		if done {
			break
		}
		select {
		case <-ctx.Done():
			fail("Fixture exit remains unconfirmed; evidence retained")
		case <-time.After(50 * time.Millisecond):
		}
	}
	if _, err = client.Call(ctx, localruntime.Request{Operation: "shutdown"}); err != nil {
		fail("Idle fixture shutdown failed")
	}
	fmt.Println("PASS: owned fixture exits observed; idle daemon shutdown requested")
}
func fail(message string) { fmt.Fprintln(os.Stderr, message); history.Exit(1) }
