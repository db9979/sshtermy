package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/codeagent"
	"github.com/db9979/sshtermy/internal/domain"
)

func waitApp(t *testing.T) (*App, *session, *session) {
	t.Helper()
	a := dangerApp(t)
	s1, _ := typedSession(domain.Connection{Alias: "web1"}, "$ ", "")
	s2, _ := typedSession(domain.Connection{Alias: "db1"}, "", "")
	s2.id = 2
	a.sessions, a.cur = []*session{s1, s2}, 0
	return a, s1, s2
}

// Verdeckter Tab hängt an [Y/n]: Marke, Einblendung, Alt+G springt hin.
func TestWaitMarkHiddenTab(t *testing.T) {
	a, _, s2 := waitApp(t)
	var sent []string
	old := desktopSend
	desktopSend = func(_, body string, _ func()) { sent = append(sent, body) }
	defer func() { desktopSend = old }()

	s2.term.Write([]byte("Need to get 12 MB.\r\nDo you want to continue? [Y/n] "))
	s2.lastOutput = time.Now().Add(-2 * time.Second)
	a.waitTick(s2, time.Now())
	if s2.notice != noticeWait || !toastHas(a, "ja/nein") {
		t.Fatalf("keine Marke/Einblendung: %v %+v", s2.notice, a.toasts)
	}
	if len(sent) != 0 {
		t.Fatal("Desktop-Meldung, obwohl das Fenster vorn ist")
	}
	if !strings.Contains(a.tabStatus(s2), "ja/nein") {
		t.Fatalf("Tab-Liste: %q", a.tabStatus(s2))
	}
	// nur einmal melden
	n := len(a.toasts)
	a.waitTick(s2, time.Now())
	if len(a.toasts) != n {
		t.Fatal("zweimal gemeldet")
	}
	a.coreMsg(nil)
	if cmd := a.jumpNextNotice(); cmd != nil || a.cur != 1 {
		t.Fatalf("Alt+G springt nicht: %d", a.cur)
	}
}

// Was du gesehen hast, bevor du gewechselt hast (offenes less), meldet sich nicht.
func TestWaitMarkSeenBeforeSwitch(t *testing.T) {
	a, _, s2 := waitApp(t)
	a.cur = 1
	s2.term.Write([]byte("Passwort für admin: "))
	s2.lastOutput = time.Now().Add(-2 * time.Second)
	a.waitTick(s2, time.Now()) // sichtbar: gesehen
	if s2.notice != noticeNone {
		t.Fatal("sichtbarer Tab markiert")
	}
	a.cur = 0
	a.waitTick(s2, time.Now())
	if s2.notice != noticeNone || len(a.toasts) != 0 {
		t.Fatal("gesehene Rückfrage nach dem Wechsel gemeldet")
	}
}

// KI-Coding-Agent im verdeckten Tab: arbeitet → wartet auf Freigabe → Meldung.
func TestCodeAgentBlockedNotifies(t *testing.T) {
	a, _, s2 := waitApp(t)
	var sent []string
	old := desktopSend
	desktopSend = func(_, body string, _ func()) { sent = append(sent, body) }
	defer func() { desktopSend = old }()
	a.blurred = true
	s2.run = &cmdRun{cmd: "claude", at: time.Now()}
	s2.term.Write([]byte("✻ Thinking… (3s · esc to interrupt)\r\n  ? for shortcuts"))
	a.waitTick(s2, time.Now())
	if s2.agentW.name != "Claude Code" || s2.agentW.state != codeagent.Working {
		t.Fatalf("nicht erkannt: %+v", s2.agentW)
	}
	if !strings.Contains(a.agentGlyph(s2), "◆") {
		t.Fatal("kein Zeichen im Tab")
	}
	s2.term.Write([]byte("\x1b[2J\x1b[H Do you want to proceed?\r\n ❯ 1. Yes\r\n   2. No"))
	a.waitTick(s2, time.Now())
	if s2.agentW.state != codeagent.Blocked || s2.notice != noticeAgent {
		t.Fatalf("Freigabe nicht gemeldet: %+v %v", s2.agentW, s2.notice)
	}
	if len(sent) != 1 || !strings.Contains(sent[0], "Freigabe") {
		t.Fatalf("Desktop-Meldung: %v", sent)
	}
	// weiter gearbeitet: Marke weg
	s2.term.Write([]byte("\x1b[2J\x1b[H✻ Running… (esc to interrupt)"))
	a.waitTick(s2, time.Now())
	if s2.notice != noticeNone {
		t.Fatal("Marke bleibt nach der Freigabe")
	}
	// Agent beendet, zurück an der Shell
	s2.run = nil
	s2.term.Write([]byte("\x1b[2J\x1b[Huser@db1:~$ "))
	a.waitTick(s2, time.Now().Add(10*time.Second))
	if s2.agentW.name != "" {
		t.Fatalf("Agent bleibt nach dem Ende: %+v", s2.agentW)
	}
}
