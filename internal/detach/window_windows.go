//go:build windows

package detach

import (
	"os/exec"
	"syscall"
)

// candidates: In Windows Terminal (WT_SESSION gesetzt) ein neues WT-Fenster,
// sonst eine eigene Konsole (openFallback). Ist Windows Terminal als
// Standard-Terminal eingestellt, wird auch daraus ein WT-Fenster.
func candidates(env func(string) string) []launcher {
	if env("WT_SESSION") != "" {
		return []launcher{{"wt.exe", []string{"-w", "new", "new-tab", "--title", "{title}", "{cmd}"}}}
	}
	return nil
}

func command(path string, args []string) *exec.Cmd { return exec.Command(path, args...) }

const createNewConsole = 0x00000010

func openFallback(title string, argv []string) error {
	c := exec.Command(argv[0], argv[1:]...)
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait() //nolint:errcheck
	return nil
}
