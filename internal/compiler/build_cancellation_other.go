//go:build !linux

package compiler

import "os/exec"

func configureBackendCancellation(_ *exec.Cmd) {}
