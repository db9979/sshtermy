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
	a.sessions, a.cur = []*session{s2}, 0
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
	a.sessions, a.cur = []*session{s2}, 0
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

// Nach dem Bestätigen durch Halten: weitere Wiederholungen derselben Taste
// kommen nicht als zusätzliche Enter (bzw. jjj) beim Server an.
func TestDangerHeldKeyEaten(t *testing.T) {
	a := dangerApp(t)
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	prod := domain.Connection{Alias: "web-prod", Host: "10.0.0.5", Username: "admin"}
	s, out := typedSession(prod, "admin@web-prod:~$ ", "rm -rf /srv/www")
	a.sessions = []*session{s}

	a.terminalKey(s, enter)
	d := a.modal.danger
	d.hold.last = time.Now().Add(-time.Second) // Dialog-⏎ ist längst losgelassen
	d.hold.press(time.Now())                   // neuer Druck
	d.hold.start = time.Now().Add(-2 * holdFill)
	a.dangerKey(enter) // Wiederholung, Balken voll
	if a.modal != nil || !out.sent() {
		t.Fatalf("Halten hat nicht ausgeführt: %q", out.String())
	}
	time.Sleep(50 * time.Millisecond)
	n := strings.Count(out.String(), "\r")

	now := time.Now()
	for i := 0; i < 20; i++ { // Taste noch unten
		if !a.heldEat.eat(tea.KeyPressMsg{Code: tea.KeyEnter, IsRepeat: i%2 == 0}, now.Add(time.Duration(i)*30*time.Millisecond)) {
			t.Fatalf("Wiederholung %d nicht verschluckt", i)
		}
	}
	if c := strings.Count(out.String(), "\r"); c != n {
		t.Fatalf("Wiederholungen gingen an den Server: %d statt %d", c, n)
	}
	// nach der Pause zählt ⏎ wieder
	if a.heldEat.eat(enter, now.Add(time.Second+20*30*time.Millisecond)) {
		t.Fatal("neues ⏎ nach dem Loslassen verschluckt")
	}
	// Loslass-Ereignis beendet sofort
	a.heldEat.arm("enter", now, true)
	a.heldEat.release(tea.KeyReleaseMsg{Code: tea.KeyEnter})
	if a.heldEat.eat(enter, now.Add(10*time.Millisecond)) {
		t.Fatal("⏎ nach Loslassen verschluckt")
	}

	// j: erste Wiederholung kommt später (Tastatur-Verzögerung), auch verschlucken
	a.heldEat.arm("j", now, false)
	if !a.heldEat.eat(tea.KeyPressMsg{Code: 'j', Text: "j"}, now.Add(500*time.Millisecond)) {
		t.Fatal("j-Wiederholung nicht verschluckt")
	}
	if a.heldEat.eat(tea.KeyPressMsg{Code: 'k', Text: "k"}, now.Add(520*time.Millisecond)) {
		t.Fatal("andere Taste verschluckt")
	}
	if a.heldEat.key != "" {
		t.Fatal("andere Taste beendet das Verschlucken nicht")
	}

	// über Update: Wiederholung landet nicht im Terminal, nach Loslassen schon
	a = dangerApp(t)
	s2, out2 := typedSession(prod, "admin@web-prod:~$ ", "ls")
	a.sessions, a.cur = []*session{s2}, 0
	a.Update(tea.KeyPressMsg{Code: 'a', Text: "a"}) // erste Taste nach dem Start geht nicht ans Terminal
	jKey := tea.KeyPressMsg{Code: 'x', Text: "x"}
	a.heldEat.arm("x", time.Now(), false)
	a.Update(jKey)
	time.Sleep(50 * time.Millisecond)
	if strings.Contains(out2.String(), "x") {
		t.Fatal("verschluckte Wiederholung kam über Update ans Terminal")
	}
	a.Update(tea.KeyReleaseMsg{Code: 'x', Text: "x"}) // losgelassen
	a.Update(jKey)
	if !out2.has("x") {
		t.Fatalf("Taste nach dem Loslassen kam nicht an: %q", out2.String())
	}
}
