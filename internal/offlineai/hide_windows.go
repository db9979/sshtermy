//go:build windows

package offlineai

import (
	"os/exec"
	"syscall"
)

// hideWindow: kein Konsolenfenster für llama-server.
func hideWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
