//go:build !(windows || darwin || linux)

package gui

import (
	"errors"

	tea "charm.land/bubbletea/v2"
)

// Available: Dieser Build hat kein eigenes Fenster (z. B. Linux ohne cgo,
// uConsole). sshTermy läuft dann wie gewohnt im Terminal.
func Available() bool { return false }

// Run gibt es in diesem Build nicht.
func Run(tea.Model, Options) (tea.Model, error) {
	return nil, errors.New("dieser Build hat kein eigenes Fenster")
}
