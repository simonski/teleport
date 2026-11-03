//go:build !darwin && !linux

package hostharness

import (
	"context"
	"errors"
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

func List(context.Context) ([]Process, error) {
	return nil, errors.New("local process discovery is unavailable on this platform")
}
func Stop(context.Context, int, string, bool) error {
	return errors.New("local process control is unavailable on this platform")
}
