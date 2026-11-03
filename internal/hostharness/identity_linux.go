//go:build linux

package hostharness

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func processIdentity(pid int) (string, error) {
	path := fmt.Sprintf("/proc/%d", pid)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return "", errors.New("process unavailable")
	}
	body, err := os.ReadFile(path + "/stat")
	if err != nil {
		return "", err
	}
	end := strings.LastIndexByte(string(body), ')')
	if end < 0 {
		return "", errors.New("invalid process identity")
	}
	fields := strings.Fields(string(body[end+1:]))
	if len(fields) <= 19 {
		return "", errors.New("invalid process identity")
	}
	if _, err = strconv.ParseUint(fields[19], 10, 64); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%s", pid, fields[19]), nil
}
func signalProcess(pid int, identity string, force bool) error {
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return errors.New("cannot safely open process identity")
	}
	defer unix.Close(fd)
	current, err := processIdentity(pid)
	if err != nil || current != identity {
		return errors.New("process changed; refresh the list")
	}
	signal := unix.SIGTERM
	if force {
		signal = unix.SIGKILL
	}
	if err = unix.PidfdSendSignal(fd, signal, nil, 0); err != nil {
		return errors.New("could not signal the process")
	}
	return nil
}
