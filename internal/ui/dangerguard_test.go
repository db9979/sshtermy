package ui

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/store"
	"github.com/db9979/sshtermy/internal/termemu"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

// has wartet kurz, bis sub beim Server ankam (der Emulator schreibt asynchron).
func (s *syncBuf) has(sub string) bool {
	for i := 0; i < 100; i++ {
		if strings.Contains(s.String(), sub) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// sent: kam Enter beim Server an? (der Emulator schreibt asynchron)
func (s *syncBuf) sent() bool {
	for i := 0; i < 100; i++ {
		if strings.Contains(s.String(), "\r") {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func dangerApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	a, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	a.w, a.h, a.scr = 120, 40, scrTerminal
	return a
}

// typedSession: Terminal mit Prompt und getippter Zeile (Echo des Servers).
func typedSession(c domain.Connection, prompt, typed string) (*session, *syncBuf) {
	out := &syncBuf{}
	term := termemu.New(100, 20, out)
	_, _ = term.Write([]byte(prompt + typed))
	return &session{id: 1, conn: c, term: term, state: stConnected, chipArmed: -1, quickArmed: -1}, out
}

func TestDangerGuardProduction(t *testing.T) {
	a := dangerApp(t)
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	prod := domain.Connection{Alias: "web-prod", Host: "10.0.0.5", Username: "admin"}
	s, out := typedSession(prod, "admin@web-prod:~$ ", "rm -rf /srv/www")
	a.sessions = []*session{s}

	a.terminalKey(s, enter)
	if a.modal == nil || a.modal.kind != mDanger || !strings.Contains(a.modal.danger.cmd, "rm -rf") {
		t.Fatalf("kein Gefahren-Dialog: %+v", a.modal)
	}
	if strings.Contains(out.String(), "\r") {
		t.Fatal("Enter ging trotz Dialog an den Server")
	}
	a.dangerKey(enter) // ⏎ allein reicht nicht
	if a.modal == nil {
		t.Fatal("⏎ hat den Dialog geschlossen")
	}
	a.dangerKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if a.modal != nil || !out.sent() {
		t.Fatalf("nach j nicht ausgeführt: %q", out.String())
	}

	// Esc: nichts geht raus
	s2, out2 := typedSession(prod, "admin@web-prod:~$ ", "sudo reboot")
	a.sessions = []*session{s2}
	a.terminalKey(s2, enter)
	a.dangerKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.modal != nil || out2.sent() {
		t.Fatal("Esc hat trotzdem ausgeführt")
	}

	// harmlose Befehle laufen ohne Rückfrage
	s3, out3 := typedSession(prod, "admin@web-prod:~$ ", "df -h")
	a.sessions = []*session{s3}
	a.terminalKey(s3, enter)
	if a.modal != nil || !out3.sent() {
		t.Fatal("harmloser Befehl wurde aufgehalten")
	}
}

func TestDangerGuardSwitchAndScope(t *testing.T) {
	a := dangerApp(t)
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	sw := domain.Connection{Alias: "core-sw", Group: "Produktion", Device: domain.DeviceNetwork}
	s, _ := typedSession(sw, "core-sw# ", "reload")
	a.sessions = []*session{s}
	a.terminalKey(s, enter)
	if a.modal == nil || a.modal.kind != mDanger {
		t.Fatal("reload auf Produktiv-Switch nicht abgefangen")
	}
	a.modal = nil

	// jede Verbindung ist geschützt, auch ohne „prod“ im Namen
	lab := domain.Connection{Alias: "lab-sw", Device: domain.DeviceNetwork}
	s2, _ := typedSession(lab, "lab-sw# ", "reload")
	a.sessions = []*session{s2}
	a.terminalKey(s2, enter)
	if a.modal == nil || a.modal.kind != mDanger {
		t.Fatal("Verbindung ohne prod im Namen nicht geschützt")
	}
	a.modal = nil

	// nur der Schalter im Formular nimmt eine Verbindung aus
	lab.GuardOff = true
	s3, out3 := typedSession(lab, "lab-sw# ", "write erase")
	a.sessions = []*session{s3}
	a.terminalKey(s3, enter)
	if a.modal != nil || !out3.sent() {
		t.Fatal("Schutz aus, trotzdem abgefangen")
	}
}
