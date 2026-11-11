package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
)

// Ein langer Befehl im verdeckten Tab markiert den Tab und meldet sich;
// der sichtbare Tab im Vordergrund meldet nichts.
func TestNoticeDoneInHiddenTab(t *testing.T) {
	a := dangerApp(t)
	var sent []string
	old := desktopSend
	desktopSend = func(title, body string) { sent = append(sent, title+"|"+body) }
	defer func() { desktopSend = old }()
	s1, _ := typedSession(domain.Connection{Alias: "web1"}, "$ ", "")
	s2, _ := typedSession(domain.Connection{Alias: "db1"}, "$ ", "")
	s2.id = 2
	a.sessions, a.cur = []*session{s1, s2}, 0

	a.noticeDone(s1, "make", 30*time.Second, false, false) // sichtbar, Fenster vorn
	if s1.notice != noticeNone || len(a.toasts) != 0 {
		t.Fatalf("sichtbarer Tab meldet: %v %d", s1.notice, len(a.toasts))
	}
	a.noticeDone(s2, "apt upgrade", 3*time.Second, false, false) // zu kurz
	if s2.notice != noticeNone {
		t.Fatal("kurzer Befehl meldet sich")
	}
	a.noticeDone(s2, "apt upgrade", 75*time.Second, true, false)
	if s2.notice != noticeFail || len(a.toasts) != 1 || !strings.Contains(a.toasts[0].text, "apt upgrade") {
		t.Fatalf("verdeckter Tab: notice %v toasts %v", s2.notice, a.toasts)
	}
	if len(sent) != 0 {
		t.Fatalf("Desktop-Meldung, obwohl das Fenster vorn ist: %v", sent)
	}
	if !strings.Contains(ansi.Strip(a.tabStrip()), "× 2 db1") {
		t.Fatalf("Tab ohne Markierung: %q", ansi.Strip(a.tabStrip()))
	}
	// Fenster im Hintergrund: Desktop-Benachrichtigung, auch für den sichtbaren Tab
	a.Update(tea.BlurMsg{})
	a.noticeDone(s1, "make", 12*time.Second, false, false)
	if len(sent) != 1 || !strings.Contains(sent[0], "web1") || !strings.Contains(sent[0], "make") {
		t.Fatalf("Desktop-Meldung: %v", sent)
	}
	// Wechsel auf den Tab löscht die Markierung
	a.Update(tea.FocusMsg{})
	a.focusSession(1)
	a.Update(nil)
	if s2.notice != noticeNone {
		t.Fatal("Markierung bleibt nach dem Wechsel")
	}
	// gesperrt: kein Befehl in der Desktop-Meldung
	a.blurred = true
	a.lock = &lockState{}
	a.noticeDone(s1, "geheim --token", 20*time.Second, false, false)
	if last := sent[len(sent)-1]; strings.Contains(last, "geheim") {
		t.Fatalf("Befehl trotz Sperre verraten: %q", last)
	}
}

func TestBellAndClipboard(t *testing.T) {
	a := dangerApp(t)
	old := desktopSend
	desktopSend = func(string, string) {}
	defer func() { desktopSend = old }()
	s1, _ := typedSession(domain.Connection{Alias: "a"}, "$ ", "")
	s2, _ := typedSession(domain.Connection{Alias: "b"}, "$ ", "")
	s2.id = 2
	a.sessions, a.cur = []*session{s1, s2}, 0
	_, _ = s2.term.Write([]byte("\a"))
	a.onTick()
	if s2.notice != noticeBell {
		t.Fatalf("Glocke im verdeckten Tab: %v", s2.notice)
	}
	_, _ = s1.term.Write([]byte("\x1b]52;c;a29waWVydA==\x07"))
	if cmd := a.takeClipboard(s1); cmd == nil {
		t.Fatal("OSC 52 nicht übernommen")
	}
	a.toggleOSC52()
	_, _ = s1.term.Write([]byte("\x1b]52;c;a29waWVydA==\x07"))
	if cmd := a.takeClipboard(s1); cmd != nil {
		t.Fatal("OSC 52 trotz Abschalten übernommen")
	}
}

func TestLockScreen(t *testing.T) {
	a := dangerApp(t)
	s1, out := typedSession(domain.Connection{Alias: "web1"}, "$ ", "")
	a.sessions, a.cur = []*session{s1}, 0
	a.intro = nil
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.LockAfterMin = 5 })
	a.lastKey = time.Now().Add(-6 * time.Minute)
	a.onTick()
	if a.lock == nil || a.scr != scrTerminal || !a.st.V.Unlocked() {
		t.Fatalf("Leerlauf mit Sitzung: lock %v scr %v unlocked %v", a.lock != nil, a.scr, a.st.V.Unlocked())
	}
	v := ansi.Strip(a.View().Content)
	if !strings.Contains(v, "gesperrt") || strings.Contains(v, "web1") {
		t.Fatalf("Sperrbild zeigt Inhalt:\n%s", v)
	}
	// Tippen geht ins Passwortfeld, nicht zum Server
	for _, r := range "falsch" {
		a.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if strings.Contains(out.String(), "falsch") {
		t.Fatal("Eingabe ging an den Server")
	}
	_, cmd := a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	a.Update(cmd())
	if a.lock == nil || a.lock.err == "" {
		t.Fatal("falsches Passwort entsperrt")
	}
	for _, r := range "correct horse" {
		a.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	_, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	a.Update(cmd())
	if a.lock != nil {
		t.Fatal("richtiges Passwort entsperrt nicht")
	}
	a.onTick()
	if a.lock != nil {
		t.Fatal("sperrt direkt nach dem Entsperren wieder")
	}
	// ohne Sitzungen: ganzer Store gesperrt (wie bisher)
	a.sessions = nil
	a.lockNow()
	if a.scr != scrUnlock || a.st.V.Unlocked() {
		t.Fatal("ohne Sitzungen nicht ganz gesperrt")
	}
}

func TestCycleLockAfter(t *testing.T) {
	a := dangerApp(t)
	var got []int
	for i := 0; i < 6; i++ {
		a.cycleLockAfter(1)
		got = append(got, a.st.Settings().LockAfterMin)
	}
	if fmt.Sprint(got) != "[5 15 30 60 0 5]" {
		t.Fatalf("Stufen %v", got)
	}
	if fmtDur(75*time.Second) != "1:15 min" || fmtDur(9*time.Second) != "9 s" || fmtDur(3725*time.Second) != "1:02 h" {
		t.Fatal("fmtDur")
	}
}
