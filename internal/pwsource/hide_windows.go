package pwsource

import (
	"os/exec"
	"syscall"
)

// hideWindow: kein Konsolenfenster für op.exe, bw.exe & Co.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
