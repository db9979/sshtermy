//go:build windows

package detach

import (
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/windows"
)

// Windows kennt kein SIGWINCH: Größe regelmäßig prüfen.
func watchSize(fd uintptr, stop <-chan struct{}, f func(cols, rows int)) {
	lc, lr, _ := term.GetSize(fd)
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			c, r, err := term.GetSize(fd)
			if err == nil && c > 0 && (c != lc || r != lr) {
				lc, lr = c, r
				f(c, r)
			}
		}
	}
}

// enableVT: Konsole soll Escape-Sequenzen der Gegenstelle selbst auswerten.
func enableVT(fd uintptr) func() {
	h := windows.Handle(fd)
	var old uint32
	if windows.GetConsoleMode(h, &old) != nil {
		return func() {}
	}
	_ = windows.SetConsoleMode(h, old|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING|windows.DISABLE_NEWLINE_AUTO_RETURN)
	return func() { _ = windows.SetConsoleMode(h, old) }
}
