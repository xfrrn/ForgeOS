package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureHarnessCancellation(cmd *exec.Cmd) {
	// Tools inherit this turn's process group, separate from the long-lived Worker.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
