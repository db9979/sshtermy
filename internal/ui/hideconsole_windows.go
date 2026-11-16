package ui

import (
	"os/exec"
	"syscall"
)

// hideConsole: der Befehl der Kopfzeile soll kein Konsolenfenster aufblitzen lassen.
func hideConsole(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
