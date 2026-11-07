//go:build windows

package localsh

import (
	"context"
	"os/exec"

	"github.com/charmbracelet/x/xpty"
)

// startPty: ConPTY (Windows 10 1809 und neuer).
func startPty(cmd *exec.Cmd, cols, rows int) (ptyFile, error) {
	p, err := xpty.NewConPty(cols, rows)
	if err != nil {
		return nil, err
	}
	if err := p.Start(cmd); err != nil {
		_ = p.Close()
		return nil, err
	}
	return p, nil
}

// waitProcess: cmd.Wait funktioniert mit ConPTY nicht (golang/go#62710).
func waitProcess(cmd *exec.Cmd) error { return xpty.WaitProcess(context.Background(), cmd) }
