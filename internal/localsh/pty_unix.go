//go:build !windows

package localsh

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type unixPty struct{ *os.File }

func (u unixPty) Resize(cols, rows int) error {
	return pty.Setsize(u.File, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}) //nolint:gosec
}

// startPty: eigene Sitzung mit Steuerterminal (Strg+C, Job-Control).
func startPty(cmd *exec.Cmd, cols, rows int) (ptyFile, error) {
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}) //nolint:gosec
	if err != nil {
		return nil, err
	}
	return unixPty{f}, nil
}

func waitProcess(cmd *exec.Cmd) error { return cmd.Wait() }
