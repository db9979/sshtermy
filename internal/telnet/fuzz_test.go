package telnet

import (
	"io"
	"net"
	"testing"
)

// FuzzParse: Telnet-Steuerfolgen (IAC, Unterverhandlung) vom Gerät.
func FuzzParse(f *testing.F) {
	f.Add([]byte{iac, will, 1, iac, do, 31, iac, sb, 24, 1, iac, se, 'h', 'i', '\r', 0})
	f.Add([]byte{iac, sb, 24, 1, iac, iac, 'x'})
	f.Add([]byte{iac, iac, iac, dont, 3, '\r', '\n'})
	f.Fuzz(func(t *testing.T, b []byte) {
		a, z := net.Pipe()
		go func() { _, _ = io.Copy(io.Discard, z) }()
		defer a.Close()
		defer z.Close()
		c := &Conn{c: a, Done: make(chan error, 1), cols: 80, rows: 24}
		out := c.parse(b, nil)
		if len(out) > len(b) {
			t.Fatalf("mehr Daten heraus (%d) als hinein (%d)", len(out), len(b))
		}
	})
}
