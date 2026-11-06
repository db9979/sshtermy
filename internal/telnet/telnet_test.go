package telnet

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuf) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func waitFor(t *testing.T, f func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if f() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout")
}

// Fake-Server wie ein Switch: verlangt Echo/SGA, will NAWS und Terminaltyp.
func TestNegotiation(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	srvGot := make(chan []byte, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.Write([]byte{iac, will, optEcho, iac, will, optSGA, iac, do, optNAWS, iac, do, optTType, iac, do, 39 /* NEW-ENVIRON */})
		c.Write([]byte{iac, sb, optTType, ttSend, iac, se})
		c.Write([]byte("Username: "))
		c.Write([]byte{'a', iac, iac, 'b', '\r', 0, '\n'})
		buf := make([]byte, 0, 256)
		tmp := make([]byte, 256)
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		for !bytes.Contains(buf, []byte("x\xff\xffy\r\x00")) {
			n, err := c.Read(tmp)
			buf = append(buf, tmp[:n]...)
			if err != nil {
				break
			}
		}
		srvGot <- buf
	}()
	out := &syncBuf{}
	conn, err := Dial(context.Background(), ln.Addr().String(), 100, 30, out)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	waitFor(t, func() bool { return bytes.Contains([]byte(out.String()), []byte("\r\n")) })
	if got := out.String(); got != "Username: a\xffb\r\n" {
		t.Fatalf("data %q", got)
	}
	waitFor(t, conn.Echo)
	conn.Write([]byte("x\xffy\r"))
	got := <-srvGot
	for _, want := range [][]byte{
		{iac, do, optEcho},
		{iac, do, optSGA},
		{iac, will, optNAWS},
		{iac, sb, optNAWS, 0, 100, 0, 30, iac, se},
		{iac, will, optTType},
		append(append([]byte{iac, sb, optTType, ttIs}, TermType...), iac, se),
		{iac, wont, 39},
		[]byte("x\xff\xffy\r\x00"),
	} {
		if !bytes.Contains(got, want) {
			t.Errorf("server did not get % x\nall: % x", want, got)
		}
	}
}

func TestServerCloses(t *testing.T) {
	a, b := net.Pipe()
	c := newConn(a, 80, 24, io.Discard)
	b.Close()
	select {
	case err := <-c.Done:
		if err != nil {
			t.Fatalf("logout should be a clean end, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no Done")
	}
}
