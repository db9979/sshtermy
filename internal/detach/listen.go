package detach

import (
	"net"

	"github.com/db9979/sshtermy/internal/localsock"
)

func listen(dir string) (net.Listener, string, error) { return localsock.Listen(dir) }

func dial(network, addr string) (net.Conn, error) { return localsock.Dial(network + ":" + addr) }
