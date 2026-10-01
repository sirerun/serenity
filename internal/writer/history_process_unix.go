//go:build darwin || linux

package writer

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

func configureHistoryProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil {
		return errors.New("writer: nil history rewrite command")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return errors.New("writer: history rewrite process was not started")
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	return nil
}
