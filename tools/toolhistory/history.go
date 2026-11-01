// Package toolhistory records development-tool invocations without their arguments.
package toolhistory

import (
	"encoding/csv"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
)

var fields = []string{"invocation_id", "tool", "started_at", "finished_at", "duration_seconds", "outcome", "exit_code", "error_type"}

type Call struct {
	tool, path, identity string
	started              time.Time
	once                 sync.Once
	done                 chan struct{}
	signals              chan os.Signal
}

func Start(tool string) *Call {
	path := os.Getenv("TELEPORT_TOOL_HISTORY")
	if path == "" {
		_, source, _, _ := runtime.Caller(0)
		if filepath.IsAbs(source) {
			path = filepath.Join(filepath.Dir(source), "..", "..", "tool_calls_history.csv")
		} else {
			// Trimmed binaries need either the repository cwd or the inherited path.
			folder, _ := os.Getwd()
			for {
				if _, err := os.Stat(filepath.Join(folder, "go.mod")); err == nil {
					path = filepath.Join(folder, "tool_calls_history.csv")
					break
				}
				parent := filepath.Dir(folder)
				if parent == folder {
					break
				}
				folder = parent
			}
		}
	}
	started := time.Now()
	call := &Call{tool: tool, path: path, identity: fmt.Sprintf("%d-%d", os.Getpid(), started.UnixNano()), started: started,
		done: make(chan struct{}), signals: make(chan os.Signal, 1)}
	signal.Notify(call.signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-call.signals:
			code := 128 + int(sig.(syscall.Signal))
			call.finish("interrupted", code, sig.String())
			os.Exit(code)
		case <-call.done:
		}
	}()
	return call
}

// End records normal returns and panics. Use as a defer in main.
func (call *Call) End() {
	if failure := recover(); failure != nil {
		call.finish("failure", 2, "panic")
		panic(failure)
	}
	call.finish("success", 0, "")
}

// Exit records an explicit process exit before os.Exit bypasses deferred calls.
func (call *Call) Exit(code int) {
	outcome := "success"
	if code != 0 {
		outcome = "failure"
	}
	call.finish(outcome, code, "")
	os.Exit(code)
}

func (call *Call) finish(outcome string, code int, errorType string) {
	call.once.Do(func() {
		signal.Stop(call.signals)
		close(call.done)
		finished := time.Now()
		row := []string{call.identity, call.tool, call.started.UTC().Format(time.RFC3339Nano), finished.UTC().Format(time.RFC3339Nano),
			fmt.Sprintf("%.6f", finished.Sub(call.started).Seconds()), outcome, strconv.Itoa(code), errorType}
		if err := appendRecord(call.path, row); err != nil {
			fmt.Fprintf(os.Stderr, "tool history: unable to record %s: %v\n", call.tool, err)
		}
	})
}

func appendRecord(path string, row []string) error {
	if path == "" {
		return fmt.Errorf("repository unavailable; set TELEPORT_TOOL_HISTORY")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = lockFile(lock); err != nil {
		return err
	}
	defer unlockFile(lock)
	output, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer output.Close()
	info, err := output.Stat()
	if err != nil {
		return err
	}
	writer := csv.NewWriter(output)
	writer.UseCRLF = true
	if info.Size() == 0 {
		if err = writer.Write(fields); err != nil {
			return err
		}
	}
	if err = writer.Write(row); err != nil {
		return err
	}
	writer.Flush()
	if err = writer.Error(); err != nil {
		return err
	}
	return output.Sync()
}
