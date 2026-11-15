package detach

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestOnlcr(t *testing.T) {
	if got := string(onlcr([]byte("a\nb\r\nc\n"), false)); got != "a\r\nb\r\nc\r\n" {
		t.Fatalf("%q", got)
	}
	if got := string(onlcr([]byte("\nx"), true)); got != "\nx" {
		t.Fatalf("CR aus dem vorigen Block: %q", got)
	}
}

func TestScreenPrelude(t *testing.T) {
	s := NewScreen()
	s.Write([]byte("\x1b[?1049h\x1b[?1000;1006h\x1b[?2004h\x1b[?25l\x1b[>1u\x1b]0;sshTermy · web1\x07"))
	s.Write([]byte("\x1b[?1000l"))
	pre := string(s.prelude())
	for _, want := range []string{"\x1b[?1049h", "\x1b[?1006h", "\x1b[?2004h", "\x1b[?25l", "\x1b[>1u", "\x1b]0;sshTermy · web1\x07"} {
		if !strings.Contains(pre, want) {
			t.Errorf("fehlt %q in %q", want, pre)
		}
	}
	if strings.Contains(pre, "?1000h") {
		t.Error("abgeschalteter Modus wird vorgespielt")
	}
	if !strings.HasPrefix(pre, "\x1b[?1049h") {
		t.Error("Alt-Bildschirm nicht zuerst")
	}
}

// Ein Fenster dockt an, geht weg, ein zweites dockt an: der Dienst läuft
// durch, das zweite bekommt die Modi und die weitere Ausgabe.
func TestHubReattach(t *testing.T) {
	dir := t.TempDir()
	h, err := ListenHub(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if !HubAlive(dir) {
		t.Fatal("Dienst antwortet nicht")
	}
	scr := NewScreen()
	scr.Write([]byte("\x1b[?1049h"))

	accept := func() *Conn {
		ch := make(chan *Conn, 1)
		go func() { c, _, _, _ := h.Accept(); ch <- c }()
		cl, err := Dial(dir, 80, 24)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cl.Close)
		c := <-ch
		scr.Attach(c, "anderswo")
		m, err := cl.Next()
		if err != nil || !bytes.Contains(m.Output, []byte("\x1b[?1049h")) {
			t.Fatalf("Vorspann fehlt: %v %q", err, m.Output)
		}
		go func() {
			for {
				if _, err := cl.Next(); err != nil {
					return
				}
			}
		}()
		return c
	}
	c1 := accept()
	if !scr.Attached() {
		t.Fatal("nicht angedockt")
	}
	scr.Drop(c1)
	if scr.Attached() {
		t.Fatal("nach dem Weggehen noch angedockt")
	}
	scr.Write([]byte("läuft ohne Fenster weiter\n")) // darf nicht blockieren
	accept()
	time.Sleep(10 * time.Millisecond)
	scr.Bye("")
}
