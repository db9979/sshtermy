//go:build !windows

package localsock

import (
	"net"
	"path/filepath"
	"time"
)

func listen(dir string) (net.Listener, string, error) {
	p := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", p)
	if err != nil {
		return nil, "", err
	}
	return ln, "unix:" + p, nil
}

func dial(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, 5*time.Second)
}
