//go:build darwin || linux

package localruntime

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

type receipt struct {
	InputReceipt
	Fingerprint string `json:"fingerprint"`
}
type entry struct {
	ReplayGap bool `json:"replayGap"`
	Status
	Fingerprint  string            `json:"fingerprint"`
	LastInput    int64             `json:"lastInput"`
	Receipts     map[int64]receipt `json:"receipts"`
	cmd          *exec.Cmd
	pty          *os.File
	fd           int
	pgid         int
	parentReaped bool
	output       []byte
}
type daemon struct {
	closing    bool
	mu         sync.Mutex
	root       string
	generation string
	config     configuration
	entries    map[string]*entry
	server     *http.Server
	wg         sync.WaitGroup
}
type replaySnapshot struct {
	Base int64  `json:"base"`
	Next int64  `json:"next"`
	Data []byte `json:"data"`
}

func (d *daemon) persistReplay(e *entry) error {
	data, err := json.Marshal(replaySnapshot{Base: e.Base, Next: e.Next, Data: e.output})
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(d.root, e.ID+".output"), data)
}

func Supported() bool { return true }
func privateRoot(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) == "/" {
		return errors.New("use an absolute private runtime directory")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return errors.New("runtime directory must be owned by the current user with mode 0700")
	}
	return nil
}
func lockFile(ctx context.Context, path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err == nil {
			return f, nil
		}
		if err != unix.EWOULDBLOCK || ctx.Err() != nil {
			f.Close()
			return nil, errors.New("runtime ownership lock is unavailable")
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}
func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".runtime-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err == nil {
		dir, openErr := os.Open(filepath.Dir(path))
		if openErr != nil {
			return openErr
		}
		err = dir.Sync()
		dir.Close()
	}
	return err
}
func runtimeEnv() []string {
	result := []string{}
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "TELEPORT_") {
			result = append(result, value)
		}
	}
	return result
}
func (c *Client) Ensure(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	if err := privateRoot(c.Root); err != nil {
		return err
	}
	lock, err := lockFile(ctx, filepath.Join(c.Root, "startup.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err = os.Stat(filepath.Join(c.Root, "control.json")); os.IsNotExist(err) {
		b, _ := json.Marshal(configuration{Protocol: 1, Token: identity()})
		if err = writeAtomic(filepath.Join(c.Root, "control.json"), b); err != nil {
			return err
		}
	}
	if _, err = c.Call(ctx, Request{Operation: "ping"}); err == nil {
		return nil
	}
	log, err := os.OpenFile(filepath.Join(c.Root, "daemon.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	command := exec.Command(c.Executable, "_session-daemon", "--runtime-dir", c.Root)
	command.Env = runtimeEnv()
	command.Stdout, command.Stderr = log, log
	command.SysProcAttr = &unix.SysProcAttr{Setsid: true}
	if err = command.Start(); err != nil {
		return errors.New("owned session daemon could not start")
	}
	go func() { _ = command.Wait() }()
	for ctx.Err() == nil {
		if _, err = c.Call(ctx, Request{Operation: "ping"}); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
		case <-time.After(50 * time.Millisecond):
		}
	}
	return ErrUnavailable
}
func (d *daemon) persist() error {
	b, err := json.Marshal(d.entries)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(d.root, "sessions.json"), b)
}
func Run(ctx context.Context, root string) error {
	if err := privateRoot(root); err != nil {
		return err
	}
	lockCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	lock, err := lockFile(lockCtx, filepath.Join(root, "daemon.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	config, err := loadConfig(root)
	if err != nil {
		return err
	}
	d := &daemon{root: root, config: config, generation: identity(), entries: map[string]*entry{}}
	if b, readErr := os.ReadFile(filepath.Join(root, "sessions.json")); readErr == nil {
		if len(b) > 64<<20 || json.Unmarshal(b, &d.entries) != nil || d.entries == nil || len(d.entries) > 1000 {
			return errors.New("runtime_state_corrupt")
		}
		for id, e := range d.entries {
			if e == nil || !sessionID.MatchString(id) || e.ID != id {
				return errors.New("runtime_state_corrupt")
			}
			if e.State != "exited" && e.State != "failed" {
				e.State, e.ErrorCode = "unknown", "daemon_ownership_lost"
			}
			data, readErr := os.ReadFile(filepath.Join(root, id+".output"))
			if readErr != nil && !os.IsNotExist(readErr) {
				return errors.New("retained_output_unavailable")
			}
			var replay replaySnapshot
			if readErr == nil && (len(data) > 3<<20 || json.Unmarshal(data, &replay) != nil || replay.Base < 0 || replay.Next < replay.Base || replay.Next-replay.Base != int64(len(replay.Data)) || len(replay.Data) > ReplayLimit) {
				return errors.New("runtime_state_corrupt")
			}
			if replay.Next != e.Next {
				e.ReplayGap = true
			}
			e.output, e.Base, e.Next = replay.Data, replay.Base, replay.Next
		}
	} else if !os.IsNotExist(readErr) {
		return readErr
	}
	if err = d.persist(); err != nil {
		return err
	}
	socket := socketPath(root)
	if info, statErr := os.Lstat(socket); statErr == nil {
		if info.Mode()&os.ModeSocket == 0 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
			return errors.New("runtime_socket_ownership_unknown")
		}
		connection, dialErr := net.DialTimeout("unix", socket, 100*time.Millisecond)
		if dialErr == nil {
			connection.Close()
			return errors.New("runtime_socket_already_active")
		}
		if err = os.Remove(socket); err != nil {
			return err
		}
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err = os.Chmod(socket, 0600); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /call", d.handle)
	d.server = &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, IdleTimeout: time.Second}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				d.mu.Lock()
				d.closing = true
				for _, e := range d.entries {
					if e.cmd != nil && e.State == "running" {
						e.State = "stopping"
						_ = d.killOwnedGroup(e)
					}
				}
				d.mu.Unlock()
				d.server.Close()
				return
			case <-ticker.C:
				d.mu.Lock()
				for id, e := range d.entries {
					if e.pty != nil {
						if err := d.persistReplay(e); err != nil {
							e.ErrorCode = "output_persistence_failed"
							slog.Error("terminal replay persistence failed", "session", id)
						}
					}
				}
				if err := d.persist(); err != nil {
					slog.Error("runtime manifest persistence failed")
				}
				d.mu.Unlock()
			}
		}
	}()
	slog.Info("local runtime ready", "generation", d.generation)
	err = d.server.Serve(listener)
	d.wg.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func (d *daemon) handle(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+d.config.Token)) != 1 {
		http.Error(w, "unauthorized", 401)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	var in Request
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid_request", 400)
		return
	}
	d.mu.Lock()
	out := d.call(in)
	d.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if out.Error != "" {
		w.WriteHeader(409)
	}
	json.NewEncoder(w).Encode(out)
}
func (d *daemon) call(in Request) Response {
	if d.closing && in.Operation == "launch" {
		return Response{Error: "runtime_shutdown_pending"}
	}
	if in.Operation == "ping" {
		return Response{Generation: d.generation}
	}
	if in.Operation == "list" {
		items := []Status{}
		for _, e := range d.entries {
			items = append(items, e.Status)
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		return Response{Items: items, Generation: d.generation}
	}
	if in.Operation == "launch" {
		return d.launch(in.Launch)
	}
	if in.Operation == "shutdown" {
		for _, e := range d.entries {
			if e.State != "exited" && e.State != "failed" {
				return Response{Error: "live_or_unknown_sessions"}
			}
		}
		d.closing = true
		go func() { time.Sleep(50 * time.Millisecond); _ = d.server.Close() }()
		return Response{Generation: d.generation}
	}
	e := d.entries[in.ID]
	if e == nil || e.LaunchID != in.LaunchID || e.Generation != in.Generation {
		return Response{Error: "session_identity_unknown"}
	}
	status := e.Status
	out := Response{Status: &status, Generation: d.generation}
	switch in.Operation {
	case "read":
		if in.Cursor < 0 || in.Cursor > e.Next && !e.ReplayGap && e.State != "unknown" {
			return Response{Error: "invalid_cursor"}
		}
		cursor := in.Cursor
		out.Gap = cursor < e.Base || e.State == "unknown" || e.ReplayGap
		if cursor > e.Next {
			cursor = e.Base
		}
		if cursor < e.Base {
			cursor = e.Base
		}
		end := min(e.Next, cursor+65536)
		out.Data = append([]byte(nil), e.output[cursor-e.Base:end-e.Base]...)
		out.Cursor, out.Next = cursor, end
	case "resize":
		if !validSize(in.Cols, in.Rows) || e.pty == nil || e.State != "running" {
			return Response{Error: "resize_unavailable"}
		}
		if pty.Setsize(e.pty, &pty.Winsize{Cols: in.Cols, Rows: in.Rows}) != nil {
			return Response{Error: "resize_failed"}
		}
		if unix.SetNonblock(e.fd, true) != nil {
			return Response{Error: "pty_nonblocking_failed"}
		}
		e.Cols, e.Rows = in.Cols, in.Rows
		status = e.Status
	case "input":
		return d.input(e, in)
	case "stop":
		if e.State == "exited" || e.State == "failed" || e.State == "stopping" {
			return out
		}
		if e.cmd == nil || e.State != "running" {
			return Response{Error: "owned_process_unavailable"}
		}
		e.State = "stopping"
		if d.persist() != nil {
			e.State = "running"
			return Response{Error: "stop_intent_persistence_failed"}
		}
		if err := d.killOwnedGroup(e); err != nil {
			e.ErrorCode = "stop_failed"
			return Response{Error: "stop_failed"}
		}
		status = e.Status
		slog.Info("owned harness stop requested", "session", e.ID)
	default:
		return Response{Error: "unsupported_operation"}
	}
	return out
}
func validSize(cols, rows uint16) bool { return cols >= 20 && cols <= 400 && rows >= 5 && rows <= 200 }
func (d *daemon) launch(in *Launch) Response {
	if in == nil || !sessionID.MatchString(in.ID) || len(in.LaunchID) != 64 || !filepath.IsAbs(in.Program) || !filepath.IsAbs(in.Cwd) || !validSize(in.Cols, in.Rows) {
		return Response{Error: "invalid_launch"}
	}
	client := New(d.root, "")
	fingerprint, err := client.Fingerprint(in)
	if err != nil {
		return Response{Error: "launch_fingerprint_failed"}
	}
	if e := d.entries[in.ID]; e != nil {
		if e.Fingerprint != fingerprint {
			return Response{Error: "launch_identity_conflict"}
		}
		status := e.Status
		return Response{Status: &status, Generation: d.generation}
	}
	if in.Generation != d.generation {
		return Response{Error: "daemon_generation_changed"}
	}
	active := 0
	for _, e := range d.entries {
		if e.State != "exited" && e.State != "failed" {
			active++
		}
	}
	if active >= 16 || len(d.entries) >= 1000 {
		return Response{Error: "runtime_capacity_exhausted"}
	}
	e := &entry{Status: Status{ID: in.ID, LaunchID: in.LaunchID, Generation: d.generation, State: "starting", Cols: in.Cols, Rows: in.Rows, StartedAt: time.Now().UTC()}, Fingerprint: fingerprint, Receipts: map[int64]receipt{}}
	d.entries[in.ID] = e
	if d.persist() != nil {
		delete(d.entries, in.ID)
		return Response{Error: "launch_intent_persistence_failed"}
	}
	cmd := exec.Command(in.Program, in.Args...)
	cmd.Dir, cmd.Env = in.Cwd, runtimeEnv()
	for key, value := range in.Env {
		if strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			e.State = "failed"
			e.ErrorCode = "invalid_environment"
			_ = d.persist()
			return Response{Error: "invalid_environment"}
		}
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: in.Cols, Rows: in.Rows})
	if err != nil {
		e.State = "failed"
		e.ErrorCode = "process_start_failed"
		_ = d.persist()
		status := e.Status
		return Response{Status: &status}
	}
	e.cmd, e.pty, e.State = cmd, terminal, "running"
	e.pgid = cmd.Process.Pid
	e.fd = int(terminal.Fd())
	if err = unix.SetNonblock(e.fd, true); err != nil {
		e.State = "stopping"
		e.ErrorCode = "pty_nonblocking_failed"
		_ = d.killOwnedGroup(e)
	}
	if d.persist() != nil {
		e.State = "stopping"
		e.ErrorCode = "launch_ack_persistence_failed"
		_ = d.killOwnedGroup(e)
	}
	d.wg.Add(1)
	go d.collect(e)
	slog.Info("owned harness started", "session", e.ID)
	status := e.Status
	return Response{Status: &status, Generation: d.generation}
}

// The daemon is the sole reaper. Group signalling and WNOHANG reaping share
// the mutex: a signalled group ID is still pinned by its unreaped owned leader.
func (d *daemon) killOwnedGroup(e *entry) error {
	if e.cmd == nil || e.parentReaped {
		return errors.New("owned group cannot be signalled after leader reaping")
	}
	err := unix.Kill(-e.pgid, unix.SIGKILL)
	if err == unix.ESRCH {
		return nil
	}
	return err
}
func (d *daemon) collect(e *entry) {
	defer d.wg.Done()
	readDone := make(chan struct{})
	readStop := make(chan struct{})
	go func() {
		defer close(readDone)
		buffer := make([]byte, 32768)
		for {
			select {
			case <-readStop:
				return
			default:
			}
			if _, err := unix.Poll([]unix.PollFd{{Fd: int32(e.fd), Events: unix.POLLIN}}, 50); err != nil && err != unix.EINTR {
				return
			}
			n, err := unix.Read(e.fd, buffer)
			if n > 0 {
				d.mu.Lock()
				e.output = append(e.output, buffer[:n]...)
				e.Next += int64(n)
				if len(e.output) > ReplayLimit {
					e.output = append([]byte(nil), e.output[len(e.output)-ReplayLimit:]...)
				}
				e.Base = e.Next - int64(len(e.output))
				d.mu.Unlock()
			}
			if err != nil && err != unix.EAGAIN && err != unix.EINTR || n == 0 && err == nil {
				return
			}
		}
	}()
	var waitStatus unix.WaitStatus
	code := -1
	waitVerified := false
	for {
		d.mu.Lock()
		pid, err := unix.Wait4(e.pgid, &waitStatus, unix.WNOHANG, nil)
		if pid == e.pgid {
			e.parentReaped = true
			waitVerified = true
			if waitStatus.Exited() {
				code = waitStatus.ExitStatus()
			}
		}
		if err != nil && err != unix.EINTR {
			e.parentReaped = true
			e.State, e.ErrorCode = "unknown", "owned_wait_unavailable"
			_ = d.persist()
		}
		d.mu.Unlock()
		if e.parentReaped {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Release Go's process resources after the explicit OS wait. Its ECHILD is
	// expected; the exit evidence above is the authoritative wait result.
	_ = e.cmd.Process.Release()
	_ = e.cmd.Wait()
	for waitVerified {
		d.mu.Lock()
		groupErr := unix.Kill(-e.pgid, 0)
		gone := groupErr == unix.ESRCH
		if !gone && e.ErrorCode != "owned_children_exit_unconfirmed" {
			if e.State != "stopping" {
				e.State = "unknown"
			}
			e.ErrorCode = "owned_children_exit_unconfirmed"
			_ = d.persist()
		}
		d.mu.Unlock()
		if gone {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	select {
	case <-readDone:
	case <-time.After(time.Second):
		close(readStop)
		<-readDone
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_ = e.pty.Close()
	e.pty = nil
	if !waitVerified {
		return
	}
	now := time.Now().UTC()
	e.ExitedAt, e.ExitCode, e.State = &now, &code, "exited"
	if e.ErrorCode == "owned_children_exit_unconfirmed" {
		e.ErrorCode = ""
	}
	if d.persistReplay(e) != nil {
		e.ErrorCode = "output_persistence_failed"
	}
	if d.persist() != nil {
		e.ErrorCode = "exit_persistence_failed"
		slog.Error("owned harness exit persistence failed", "session", e.ID)
	}
	slog.Info("owned harness exit observed", "session", e.ID, "exit_code", code)
}
func (d *daemon) input(e *entry, in Request) Response {
	if in.Sequence < 1 || len(in.Data) == 0 || len(in.Data) > 16384 {
		return Response{Error: "invalid_input"}
	}
	fingerprint, _ := New(d.root, "").Fingerprint(in.Data)
	if previous, ok := e.Receipts[in.Sequence]; ok {
		if previous.Fingerprint != fingerprint {
			return Response{Error: "input_identity_conflict"}
		}
		r := previous.InputReceipt
		if r.State == "pending" {
			r.State = "uncertain"
		}
		return Response{Receipt: &r}
	}
	if in.Sequence <= e.LastInput {
		return Response{Error: "input_sequence_unavailable"}
	}
	if e.State != "running" || e.pty == nil {
		return Response{Error: "input_unavailable"}
	}
	e.LastInput = in.Sequence
	recorded := receipt{InputReceipt: InputReceipt{Sequence: in.Sequence, State: "pending"}, Fingerprint: fingerprint}
	e.Receipts[in.Sequence] = recorded
	for seq := range e.Receipts {
		if seq < in.Sequence-31 {
			delete(e.Receipts, seq)
		}
	}
	if d.persist() != nil {
		return Response{Error: "input_intent_persistence_failed"}
	}
	deadline := time.Now().Add(250 * time.Millisecond)
	for recorded.Written < len(in.Data) && time.Now().Before(deadline) {
		fd := e.fd
		if _, err := unix.Poll([]unix.PollFd{{Fd: int32(fd), Events: unix.POLLOUT}}, 25); err != nil && err != unix.EINTR {
			break
		}
		n, err := unix.Write(fd, in.Data[recorded.Written:])
		if n > 0 {
			recorded.Written += n
		}
		if err != nil && err != unix.EAGAIN && err != unix.EINTR {
			break
		}
	}
	recorded.State = "uncertain"
	if recorded.Written == len(in.Data) {
		recorded.State = "delivered"
	} else if recorded.Written == 0 {
		recorded.State = "not_delivered"
	}
	e.Receipts[in.Sequence] = recorded
	if d.persist() != nil {
		return Response{Error: "input_ack_persistence_failed"}
	}
	r := recorded.InputReceipt
	return Response{Receipt: &r}
}
