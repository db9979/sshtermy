package termemu

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func TestMouseTrackingAndForward(t *testing.T) {
	out := &syncBuf{}
	tm := New(40, 5, out)
	defer tm.Close()
	if tm.MouseTracking() != MouseOff {
		t.Fatal("Maus sollte anfangs aus sein")
	}
	// so schaltet z. B. htop die Maus ein
	tm.Write([]byte("\x1b[?1002h\x1b[?1006h"))
	if got := tm.MouseTracking(); got != MouseDrag {
		t.Fatalf("Level = %v, erwartet MouseDrag", got)
	}
	tm.SendMouse(uv.MouseClickEvent{X: 4, Y: 2, Button: uv.MouseLeft})
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(out.String(), "\x1b[<0;5;3M") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(out.String(), "\x1b[<0;5;3M") {
		t.Fatalf("SGR-Klick nicht gesendet: %q", out.String())
	}
	tm.Write([]byte("\x1b[?1002l"))
	if tm.MouseTracking() != MouseOff {
		t.Fatal("Maus sollte wieder aus sein")
	}
}

func TestSelectionText(t *testing.T) {
	tm := New(20, 3, &syncBuf{})
	defer tm.Close()
	for i := 1; i <= 6; i++ {
		tm.Write([]byte("zeile " + string(rune('0'+i)) + " /etc/hosts\r\n"))
	}
	tm.Write([]byte("$ "))
	sb := tm.ScrollbackLen()
	if sb < 3 {
		t.Fatalf("Scrollback = %d", sb)
	}
	// zwei Zeilen zurück: oben steht Scrollback-Zeile sb-2 (zeile 3)
	p := tm.ScreenPos(0, 0, 2)
	if p.Line != sb-2 {
		t.Fatalf("ScreenPos = %+v", p)
	}
	got := tm.Text(Sel{A: Pos{p.Line, 0}, B: Pos{p.Line + 1, 6}})
	if got != "zeile 3 /etc/hosts\nzeile 4" {
		t.Fatalf("Text = %q", got)
	}
	w := tm.WordAt(Pos{p.Line, 10})
	if s := tm.Text(w); s != "/etc/hosts" {
		t.Fatalf("WordAt = %q", s)
	}
	v := tm.Render(2, &Sel{A: Pos{p.Line, 0}, B: Pos{p.Line, 4}})
	if !strings.Contains(v, "\x1b[7m") || !strings.Contains(ansi.Strip(v), "zeile 3") {
		t.Fatalf("Render ohne Hervorhebung: %q", v)
	}
	if n := len(strings.Split(v, "\n")); n != 3 {
		t.Fatalf("Render: %d Zeilen", n)
	}
}

func TestLineText(t *testing.T) {
	tm := New(30, 3, io.Discard)
	defer tm.Close()
	tm.Write([]byte("äß 日本 http://x.de"))
	text, cols := tm.LineText(0)
	if !strings.HasPrefix(text, "äß 日本 http://x.de") || len(cols) != 31 {
		t.Fatalf("LineText = %q (%d)", text, len(cols))
	}
	// Spalte 0 „ä“, 3 „日“, 4 zweite Hälfte von „日“, 8 „h“
	if cols[0] != 0 || cols[3] != 5 || cols[4] != 5 || text[cols[8]] != 'h' {
		t.Fatalf("Spalten: %v", cols[:10])
	}
}
