//go:build !windows

package detach

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/x/term"
)

func watchSize(fd uintptr, stop <-chan struct{}, f func(cols, rows int)) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	defer signal.Stop(ch)
	for {
		select {
		case <-stop:
			return
		case <-ch:
			if c, r, err := term.GetSize(fd); err == nil && c > 0 {
				f(c, r)
			}
		}
	}
}

func enableVT(uintptr) func() { return func() {} }
