//go:build windows

package detach

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// StartWindowProcess startet argv ohne Konsole: für „attach … -window“, das
// sein eigenes Fenster öffnet (internal/gui).
func StartWindowProcess(argv []string) error {
	c := exec.Command(argv[0], argv[1:]...)
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait() //nolint:errcheck
	return nil
}
