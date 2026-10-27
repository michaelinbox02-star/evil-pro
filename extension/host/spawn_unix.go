//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach makes the child survive this host process exiting.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
