package sshx

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"time"
)

// serveSOCKS5 ist ein minimaler SOCKS5-Server nach RFC 1928 für -D:
// nur "no authentication" und CONNECT (kein BIND/UDP). Er lauscht nur dort,
// wo der Tunnel gebunden ist (Standard 127.0.0.1), daher keine Auth nötig.
// Die Namensauflösung passiert auf dem SSH-Server (ATYP Domain wird
// unverändert weitergereicht), so leaken keine DNS-Anfragen lokal.
func serveSOCKS5(c net.Conn, dial func(string) (net.Conn, error), relay func(net.Conn)) error {
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return err
	}
	if hdr[0] != 5 {
		return errors.New("socks: keine Version 5")
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(c, methods); err != nil {
		return err
	}
	ok := false
	for _, m := range methods {
		ok = ok || m == 0
	}
	if !ok {
		c.Write([]byte{5, 0xff})
		return errors.New("socks: kein unterstütztes Auth-Verfahren")
	}
	c.Write([]byte{5, 0})

	req := make([]byte, 4)
	if _, err := io.ReadFull(c, req); err != nil {
		return err
	}
	if req[1] != 1 { // nur CONNECT
		reply(c, 7)
		return errors.New("socks: nur CONNECT")
	}
	var host string
	switch req[3] {
	case 1:
		b := make([]byte, 4)
		if _, err := io.ReadFull(c, b); err != nil {
			return err
		}
		host = net.IP(b).String()
	case 3:
		l := make([]byte, 1)
		if _, err := io.ReadFull(c, l); err != nil {
			return err
		}
		b := make([]byte, l[0])
		if _, err := io.ReadFull(c, b); err != nil {
			return err
		}
		host = string(b)
	case 4:
		b := make([]byte, 16)
		if _, err := io.ReadFull(c, b); err != nil {
			return err
		}
		host = net.IP(b).String()
	default:
		reply(c, 8)
		return errors.New("socks: Adresstyp")
	}
	pb := make([]byte, 2)
	if _, err := io.ReadFull(c, pb); err != nil {
		return err
	}
	target := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb))))
	out, err := dial(target)
	if err != nil {
		reply(c, 5) // connection refused
		return err
	}
	reply(c, 0)
	_ = c.SetDeadline(time.Time{})
	relay(out)
	return nil
}

func reply(c net.Conn, code byte) {
	c.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
}
