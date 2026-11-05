//go:build !windows

package sshx

import (
	"errors"
	"net"
	"os"

	"github.com/db9979/sshtermy/internal/i18n"
)

// macOS/Linux: der Agent lauscht auf dem Unix-Socket aus SSH_AUTH_SOCK
// (macOS startet ihn per launchd automatisch).
func dialAgent() (net.Conn, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, errors.New(i18n.T("SSH_AUTH_SOCK ist nicht gesetzt"))
	}
	return net.Dial("unix", sock)
}
