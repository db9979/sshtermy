package main

import (
	"os"
	"runtime"

	"github.com/charmbracelet/x/term"

	"github.com/db9979/sshtermy/internal/gui"
)

// wantWindow: eigenes Fenster (internal/gui) statt Terminal?
//   - -tui / SSHTERMY_WINDOW=0: nie
//   - -window / SSHTERMY_WINDOW=1: immer, wenn der Build ein Fenster hat
//   - sonst automatisch, wenn kein Terminal da ist: Doppelklick im Explorer,
//     sshTermy.app auf dem Mac, Starter im Linux-Menü.
//
// Wer sshTermy in einem Terminal startet, bekommt es also wie bisher.
func wantWindow(forceWindow, forceTUI bool) bool {
	if !gui.Available() || forceTUI || os.Getenv("SSHTERMY_WINDOW") == "0" {
		return false
	}
	if forceWindow || os.Getenv("SSHTERMY_WINDOW") == "1" {
		return true
	}
	if runtime.GOOS == "windows" {
		return ownConsole()
	}
	if term.IsTerminal(os.Stdin.Fd()) || term.IsTerminal(os.Stdout.Fd()) {
		return false
	}
	if runtime.GOOS == "linux" {
		return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
	}
	return true
}
