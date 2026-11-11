package ui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
)

// fakeShell sammelt, was zum Server geht.
type fakeShell struct {
	mu  sync.Mutex
	out strings.Builder
}

func (f *fakeShell) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.out.Write(p)
}
func (f *fakeShell) Resize(int, int) error { return nil }
func (f *fakeShell) Close() error          { return nil }
func (f *fakeShell) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.out.String()
}

// Der Auslöser von tsz schaltet die Sitzung in die Übertragung: das
// Terminal sieht den Auslöser nicht, weitere Ausgabe geht an trzsz, und der
// Abbruch per Strg+C meldet sich beim Server.
func TestTrzszTriggerAndCancel(t *testing.T) {
	a := dangerApp(t)
	a.intro = nil
	s, _ := typedSession(domain.Connection{Alias: "srv"}, "$ ", "")
	sh := &fakeShell{}
	s.setShell(sh)
	a.sessions, a.cur = []*session{s}, 0
	ev := make(chan tea.Msg, 10)
	w := outWriter{s, ev}
	_, _ = w.Write([]byte("tsz a.txt\r\n\x1b7\x07::TRZSZ:TRAN"))
	_, _ = w.Write([]byte("SFER:S:1.1.7:1730971234567:0\r\n"))
	if s.trz.Load() == nil {
		t.Fatal("Auslöser über zwei Blöcke nicht erkannt")
	}
	if strings.Contains(s.term.PlainLines(5), "SFER:S") { // Rest nach der Erkennung bleibt draußen
		t.Fatalf("Auslöser im Terminal: %q", s.term.PlainLines(5))
	}
	var start tea.Msg
	for start == nil {
		select {
		case x := <-ev:
			if _, ok := x.(trzStartMsg); ok {
				start = x
			}
		case <-time.After(time.Second):
			t.Fatal("keine Startnachricht")
		}
	}
	for len(ev) > 0 {
		<-ev
	}
	a.events = ev
	_, ok := a.trzMsg(start)
	if !ok || a.modal == nil || a.modal.kind != mConfirm {
		t.Fatal("Server schickt Dateien: Rückfrage fehlt")
	}
	a.modal.guardUntil = time.Time{} // Tastensperre nach dem Aufgehen vorbei
	cmd := a.modalKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if cmd == nil {
		t.Fatal("Annehmen startet nichts")
	}
	done := make(chan tea.Msg, 4)
	go func() {
		b, ok := cmd().(tea.BatchMsg)
		if !ok {
			b = tea.BatchMsg{cmd}
		}
		for _, c := range b {
			if c == nil {
				continue
			}
			go func(c tea.Cmd) {
				if m, ok := c().(trzDoneMsg); ok {
					done <- m
				}
			}(c)
		}
	}()
	// Server antwortet nicht weiter, Nutzer bricht ab
	time.Sleep(100 * time.Millisecond)
	a.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	var m tea.Msg
	select {
	case m = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Abbruch kommt nicht an")
	}
	a.trzMsg(m)
	if s.trz.Load() != nil {
		t.Fatal("Übertragung bleibt aktiv")
	}
	got := sh.String()
	if !strings.Contains(got, "#ACT:") || !strings.Contains(got, "#fail:") {
		t.Fatalf("zum Server: %q", got)
	}
	// danach geht Ausgabe wieder ins Terminal; gleicher Auslöser nicht noch einmal
	_, _ = w.Write([]byte("::TRZSZ:TRANSFER:S:1.1.7:1730971234567:0\r\nfertig\r\n"))
	if s.trz.Load() != nil {
		t.Fatal("alter Auslöser startet neu")
	}
}

func TestZmodemHint(t *testing.T) {
	a := dangerApp(t)
	s, _ := typedSession(domain.Connection{Alias: "srv"}, "$ ", "")
	sh := &fakeShell{}
	s.setShell(sh)
	a.sessions = []*session{s}
	a.events = make(chan tea.Msg, 4)
	ev := make(chan tea.Msg, 4)
	_, _ = outWriter{s, ev}.Write([]byte("rz waiting\r\n**\x18B0100000023be50\r\n"))
	var m tea.Msg
	for m == nil {
		select {
		case x := <-ev:
			if _, ok := x.(zmodemMsg); ok {
				m = x
			}
		case <-time.After(time.Second):
			t.Fatal("keine ZMODEM-Meldung")
		}
	}
	a.trzMsg(m)
	if !strings.Contains(sh.String(), "\x18\x18\x18\x18\x18") {
		t.Fatal("ZMODEM nicht abgebrochen")
	}
}

func TestSplitPaths(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	os.WriteFile(f, []byte("x"), 0o600)
	got := splitPaths(dir + string(filepath.Separator) + `; "` + f + `" ;`)
	if len(got) != 1 || got[0] != f {
		t.Fatalf("splitPaths = %v", got)
	}
}
