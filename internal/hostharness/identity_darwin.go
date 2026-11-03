//go:build darwin

package hostharness

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func processIdentity(pid int) (string, error) {
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || p == nil || p.Proc.P_pid != int32(pid) || p.Eproc.Pcred.P_ruid != uint32(os.Geteuid()) {
		return "", errors.New("process unavailable")
	}
	return fmt.Sprintf("%d:%d:%d", pid, p.Proc.P_starttime.Sec, p.Proc.P_starttime.Usec), nil
}
func signalProcess(pid int, identity string, force bool) error {
	current, err := processIdentity(pid)
	if err != nil || current != identity {
		return errors.New("process changed; refresh the list")
	}
	signal := unix.SIGTERM
	if force {
		signal = unix.SIGKILL
	}
	if err = unix.Kill(pid, signal); err != nil {
		return errors.New("could not signal the process")
	}
	return nil
}
