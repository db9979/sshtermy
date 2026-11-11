package update

import (
	"os/exec"
	"syscall"
)

// hideWindow: der Selbsttest soll kein Konsolenfenster aufblitzen lassen.
func hideWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
