package ui

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/detach"
	"github.com/db9979/sshtermy/internal/termemu"
)

func TestDetachedTermSize(t *testing.T) {
	a := splitApp(1)
	s := a.sessions[0]
	c0, r0 := a.termSize(s)
	srv, err := detach.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	// Fenster startet noch: Größe bleibt die des Hauptfensters
	s.detach = &detachState{srv: srv}
	if c, r := a.termSize(s); c != c0 || r != r0 {
		t.Fatalf("ohne Fenster %dx%d, erwartet %dx%d", c, r, c0, r0)
	}
	// angedockt: das Fenster bestimmt die Größe, die Fläche hier nicht
	s.detach.conn, s.detach.cols, s.detach.rows = &detach.Conn{}, 77, 22
	if c, r := a.termSize(s); c != 77 || r != 22 {
		t.Fatalf("angedockt %dx%d", c, r)
	}
	if c, r := a.paneSize(s); c != c0 || r != r0 {
		t.Fatalf("Fläche hier %dx%d", c, r)
	}
	s.detach.conn = nil // Bye auf leerer Conn vermeiden
	s.endDetach("")
	if s.detached() {
		t.Fatal("endDetach muss aufräumen")
	}
}

func TestDetachedFooter(t *testing.T) {
	a := splitApp(1)
	a.keys = activeKeys
	s := a.sessions[0]
	s.state = stConnected
	has := func(want string) bool {
		for _, k := range a.terminalKeys(s) {
			if k[1] == want {
				return true
			}
		}
		return false
	}
	if !has("Fenster") || has("zurückholen") {
		t.Fatal("verbunden: Fenster-Taste erwartet")
	}
	s.detach = &detachState{}
	if has("Fenster") || !has("zurückholen") || has("Ziel") {
		t.Fatal("abgetrennt: nur zurückholen, keine KI-Tasten")
	}
}

func TestReplayScreen(t *testing.T) {
	term := termemu.New(20, 4, io.Discard)
	_, _ = term.Write([]byte("\x1b[31mrot\x1b[0m\r\nzweite"))
	out := replayScreen(term)
	if !strings.Contains(out, "rot") || !strings.Contains(out, "zweite") || !strings.HasPrefix(out, "\x1b[0m\x1b[H\x1b[2J") {
		t.Fatalf("Bildschirm fehlt: %q", out)
	}
	if !strings.HasSuffix(out, "\x1b[2;7H") {
		t.Fatalf("Cursor falsch: %q", out[len(out)-12:])
	}
}

type lockedBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func TestMuteReplies(t *testing.T) {
	var got lockedBuf
	term := termemu.New(20, 4, &got) // ohne Close: vt.Close und Read sind untereinander nicht race-frei
	term.MuteReplies(true)
	_, _ = term.Write([]byte("\x1b[c")) // Abfrage: das echte Terminal antwortet
	term.SendText("ls\r")
	time.Sleep(100 * time.Millisecond)
	if g := got.String(); g != "ls\r" {
		t.Fatalf("stumm: nur der Befehl darf ankommen, kam %q", g)
	}
	term.MuteReplies(false)
	_, _ = term.Write([]byte("\x1b[c"))
	time.Sleep(100 * time.Millisecond)
	if g := got.String(); !strings.Contains(g, "\x1b[?") {
		t.Fatalf("ohne Stummschaltung muss der Emulator antworten: %q", g)
	}
}

// Der Verlauf geht mit ins eigene Fenster, damit man dort zurückblättern kann.
func TestReplayScreenHistory(t *testing.T) {
	term := termemu.New(20, 4, io.Discard)
	for i := 0; i < 30; i++ {
		_, _ = term.Write([]byte(fmt.Sprintf("zeile %d\r\n", i)))
	}
	out := replayScreen(term)
	if strings.Count(out, "zeile ") != 30 {
		t.Fatalf("Verlauf fehlt oder doppelt: %d Zeilen", strings.Count(out, "zeile "))
	}
	if strings.Index(out, "zeile 0\x1b") > strings.Index(out, "zeile 29") {
		t.Fatal("Reihenfolge falsch")
	}
	// im Vollbild-Programm (vim, less) nur der Bildschirm
	_, _ = term.Write([]byte("\x1b[?1049hvim"))
	if out := replayScreen(term); strings.Contains(out, "zeile ") {
		t.Fatal("Alt-Bildschirm: kein Verlauf")
	}
}
