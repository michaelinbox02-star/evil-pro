//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// detach makes the child survive this host process exiting, and keeps it from
// sharing our (nonexistent) console.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x00000200, // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	}
}
