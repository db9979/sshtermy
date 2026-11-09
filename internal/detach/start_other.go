//go:build !windows

package detach

import "os/exec"

// StartWindowProcess startet argv: für „attach … -window“, das sein eigenes
// Fenster öffnet (internal/gui).
func StartWindowProcess(argv []string) error {
	c := exec.Command(argv[0], argv[1:]...)
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait() //nolint:errcheck
	return nil
}
