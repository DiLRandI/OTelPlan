package compiler

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func configureBackendCancellation(command *exec.Cmd) {
	command.SysProcAttr = new(syscall.SysProcAttr)
	command.SysProcAttr.Setpgid = true
	command.Cancel = func() error {
		if command.Process == nil || command.Process.Pid <= 0 {
			return os.ErrProcessDone
		}

		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}

		if err != nil {
			return fmt.Errorf("stop backend process group: %w", err)
		}

		return nil
	}
}
