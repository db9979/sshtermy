//go:build windows

package sshx

import (
	"net"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
)

// Unter Windows lauscht der mitgelieferte OpenSSH-Agent (Dienst "ssh-agent")
// auf einer Named Pipe. Neuere Pageant-Versionen und 1Password/KeePassXC
// bieten dieselbe Pipe an. SSH_AUTH_SOCK darf auf eine andere Pipe zeigen.
func dialAgent() (net.Conn, error) {
	pipe := `\\.\pipe\openssh-ssh-agent`
	if p := os.Getenv("SSH_AUTH_SOCK"); p != "" && len(p) > 2 && p[:2] == `\\` {
		pipe = p
	}
	t := 3 * time.Second
	return winio.DialPipe(pipe, &t)
}
