//go:build windows || darwin || linux

package gui

import (
	"time"

	"github.com/db9979/sshtermy/internal/detach"
)

// RunAttach: abgetrennter Tab (⇧F10) im zweiten eigenen Fenster. Das
// Fenster ist ein schlichtes Terminal am Tab; schließen holt den Tab zurück.
// Rückgabe wie ein Exit-Code.
func RunAttach(dir string, opts Options) int {
	c, err := detach.Dial(dir, startCols, startRows)
	if err != nil {
		return 1
	}
	defer c.Close()
	opts.attach = true
	w, err := newWindow(opts, c)
	if err != nil {
		return 1
	}
	w.onResize = func(cols, rows int) { _ = c.Resize(cols, rows) }
	w.onClose = c.Close
	w.onQuit = c.Close
	out := w.output()
	go func() {
		defer close(w.done)
		for {
			m, err := c.Next()
			if err != nil {
				return
			}
			switch {
			case m.Bye:
				if m.Text != "" {
					_, _ = out.Write([]byte("\x1b[0m\x1b[?1049l\r\n" + m.Text + "\r\n"))
					time.Sleep(1200 * time.Millisecond)
				}
				return
			case m.Title != "":
				_, _ = out.Write([]byte("\x1b]0;" + m.Title + "\a"))
			default:
				_, _ = out.Write(m.Output)
			}
		}
	}()
	if err := w.run(); err != nil {
		return 1
	}
	return 0
}
