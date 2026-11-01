// Package localruntime owns local PTYs independently of HTTP viewers and SQL clients.
package localruntime

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const ReplayLimit = 2 << 20

var ErrUnavailable = errors.New("local runtime is unavailable")
var sessionID = regexp.MustCompile(`^[HJ][1-9][0-9]{0,15}$`)

type Status struct {
	ID         string     `json:"id"`
	LaunchID   string     `json:"launchId"`
	Generation string     `json:"generation"`
	State      string     `json:"state"`
	Cols       uint16     `json:"cols"`
	Rows       uint16     `json:"rows"`
	StartedAt  time.Time  `json:"startedAt"`
	ExitedAt   *time.Time `json:"exitedAt,omitempty"`
	ExitCode   *int       `json:"exitCode,omitempty"`
	ErrorCode  string     `json:"errorCode,omitempty"`
	Attention  string     `json:"attention,omitempty"`
	Base       int64      `json:"base"`
	Next       int64      `json:"next"`
}
type Launch struct {
	Script     bool              `json:"script,omitempty"`
	Native     bool              `json:"native,omitempty"`
	Generation string            `json:"generation"`
	ID         string            `json:"id"`
	LaunchID   string            `json:"launchId"`
	Program    string            `json:"program"`
	Args       []string          `json:"args"`
	Cwd        string            `json:"cwd"`
	Env        map[string]string `json:"env,omitempty"`
	Cols       uint16            `json:"cols"`
	Rows       uint16            `json:"rows"`
}
type InputReceipt struct {
	Sequence int64  `json:"sequence"`
	State    string `json:"state"`
	Written  int    `json:"written"`
}
type Request struct {
	NativeEvent *NativeEvent `json:"nativeEvent,omitempty"`
	NativeReply *NativeReply `json:"nativeReply,omitempty"`
	Operation   string       `json:"operation"`
	ID          string       `json:"id,omitempty"`
	LaunchID    string       `json:"launchId,omitempty"`
	Generation  string       `json:"generation,omitempty"`
	Launch      *Launch      `json:"launch,omitempty"`
	Cursor      int64        `json:"cursor,omitempty"`
	Sequence    int64        `json:"sequence,omitempty"`
	Data        []byte       `json:"data,omitempty"`
	Cols        uint16       `json:"cols,omitempty"`
	Rows        uint16       `json:"rows,omitempty"`
}
type Response struct {
	ScriptRuntime bool            `json:"scriptRuntime"`
	RequestsGap   bool            `json:"requestsGap"`
	Requests      []NativeRequest `json:"requests,omitempty"`
	NativeReceipt *NativeReceipt  `json:"nativeReceipt,omitempty"`
	Generation    string          `json:"generation,omitempty"`
	Status        *Status         `json:"status,omitempty"`
	Items         []Status        `json:"items,omitempty"`
	Receipt       *InputReceipt   `json:"receipt,omitempty"`
	Data          []byte          `json:"data,omitempty"`
	Cursor        int64           `json:"cursor"`
	Next          int64           `json:"next"`
	Gap           bool            `json:"gap"`
	Error         string          `json:"error,omitempty"`
}
type configuration struct {
	Protocol int    `json:"protocol"`
	Token    string `json:"token"`
}
type Client struct {
	Root       string
	Executable string
}

func New(root, executable string) *Client { return &Client{Root: root, Executable: executable} }
func identity() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func socketPath(root string) string {
	sum := sha256.Sum256([]byte(root))
	// Keep macOS Unix socket paths below its 104-byte limit.
	return filepath.Join("/tmp", fmt.Sprintf("teleport-%d-%x.sock", os.Getuid(), sum[:12]))
}
func loadConfig(root string) (configuration, error) {
	var c configuration
	b, err := os.ReadFile(filepath.Join(root, "control.json"))
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	if err != nil || c.Protocol != 1 || len(c.Token) != 64 {
		return c, errors.New("invalid runtime control configuration")
	}
	return c, nil
}
func (c *Client) Fingerprint(value any) (string, error) {
	config, err := loadConfig(c.Root)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	h := hmac.New(sha256.New, []byte(config.Token))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (c *Client) Call(ctx context.Context, in Request) (Response, error) {
	var out Response
	config, err := loadConfig(c.Root)
	if err != nil {
		return out, ErrUnavailable
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", socketPath(c.Root))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	body, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://runtime/call", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	r.Header.Set("Authorization", "Bearer "+config.Token)
	r.Header.Set("Content-Type", "application/json")
	response, err := client.Do(r)
	if err != nil {
		return out, ErrUnavailable
	}
	defer response.Body.Close()
	if err = json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&out); err != nil {
		return out, ErrUnavailable
	}
	if response.StatusCode != 200 || out.Error != "" {
		return out, fmt.Errorf("local runtime: %s", out.Error)
	}
	return out, nil
}
