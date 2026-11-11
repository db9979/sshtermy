package wol

import (
	"os/exec"
	"syscall"
)

// hideWindow: die ARP-Abfrage soll kein Konsolenfenster aufblitzen lassen.
func hideWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
