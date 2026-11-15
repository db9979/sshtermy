//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detachProcess: eigene Sitzung ohne Terminal, überlebt das Schließen des Fensters.
func detachProcess(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
