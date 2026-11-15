//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// detachProcess: ohne Konsole und eigene Prozessgruppe, überlebt das
// Schließen des Terminalfensters.
func detachProcess(c *exec.Cmd) {
	const detached, newGroup, noWindow = 0x00000008, 0x00000200, 0x08000000
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detached | newGroup | noWindow, HideWindow: true}
}
