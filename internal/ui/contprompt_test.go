package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/termemu"
)

func TestContinuationPrompt(t *testing.T) {
	for _, l := range []string{"> ", ">", ">> ", "quote> ", "dquote> ", "heredoc> ", "pipe pipe> ", "cmdand dquote> ", "More? "} {
		if !termemu.ContinuationPrompt(l) {
			t.Errorf("%q: Fortsetzung nicht erkannt", l)
		}
	}
	for _, l := range []string{"user@web:~$ ", "switch> ", "HP-2920> ", "PS C:\\> ", "C:\\> ", "> ls", "root@x:/# "} {
		if termemu.ContinuationPrompt(l) {
			t.Errorf("%q: fälschlich als Fortsetzung erkannt", l)
		}
	}
}

// Ziel-Modus: der Befehl der KI bleibt am „>“ hängen ⇒ Strg+C, Schritt zählt
// als Fehlschlag mit Grund, der nächste Schritt wartet auf den echten Prompt.
func TestContinuationBrokenInGoalMode(t *testing.T) {
	g := newGold(t, i18n.DE, 120, 30, true)
	s, c := vgSession(g, "web-01", "deploy@web-01:~$ ")
	a := g.a
	s.state = stConnected
	cmd := "echo 'hallo > /tmp/x"
	s.goal.active = true
	s.goal.cur = &agent.Step{Suggestion: agent.Suggestion{Command: cmd}}
	a.runProposal(s, cmd, false)
	if got := c.await(cmd + "\r"); !strings.Contains(got, cmd+"\r") {
		t.Fatalf("Befehl nicht gesendet: %q", got)
	}
	c.Reset()
	_, _ = s.term.Write([]byte(cmd + "\r\n> "))
	s.run.at = time.Now().Add(-time.Second)
	s.lastOutput = time.Now().Add(-2 * time.Second)
	s.goal.waitSince = time.Now().Add(-2 * time.Second)

	if cmd := a.goalTick(s); cmd != nil || !s.goal.waiting {
		t.Fatal("Ziel-Modus fragt am „>“ schon den nächsten Schritt an")
	}
	a.commandTick(s)
	if got := c.await("\x03"); got != "\x03" {
		t.Fatalf("kein Strg+C am „>“: %q", got)
	}
	if s.run == nil || s.run.contWhy == "" {
		t.Fatal("Befehl gilt schon als fertig")
	}
	if cmd := a.goalTick(s); cmd != nil {
		t.Fatal("Ziel-Modus läuft weiter, bevor der Prompt zurück ist")
	}
	_, _ = s.term.Write([]byte("^C\r\ndeploy@web-01:~$ "))
	s.run.at = time.Now().Add(-time.Second)
	a.commandTick(s)
	if s.run != nil {
		t.Fatal("Befehl nach Strg+C nicht abgeschlossen")
	}
	hint := s.goal.steps[len(s.goal.steps)-1].ExitHint
	if !strings.Contains(hint, "Strg+C") || !strings.Contains(hint, "offenes Anführungszeichen '") {
		t.Fatalf("Grund für die KI fehlt: %q", hint)
	}
}

// Steht die Shell schon am „>“, kommt vor dem Befehl ein Strg+C mit Pause.
func TestSendLineBreaksContinuation(t *testing.T) {
	g := newGold(t, i18n.DE, 120, 30, true)
	s, c := vgSession(g, "web-01", "deploy@web-01:~$ cat <<EOF\r\n> ")
	s.state = stConnected
	g.a.sendLine(s, "uptime")
	if got := c.settle(); got != "\x03" {
		t.Fatalf("erst nur Strg+C erwartet, kam %q", got)
	}
	if got := c.await("uptime\r"); got != "\x03uptime\r" {
		t.Fatalf("Befehl nach der Pause fehlt: %q", got)
	}
}

// Ist „>“ der normale Prompt des Systems, merkt sich die Sitzung das nach
// dem ersten erfolglosen Strg+C und lässt den Prompt danach in Ruhe.
func TestContinuationHomePrompt(t *testing.T) {
	g := newGold(t, i18n.DE, 120, 30, true)
	s, c := vgSession(g, "ups", "> ")
	a := g.a
	s.state = stConnected
	a.beginCommand(s, "status")
	s.run.sent = true
	_, _ = s.term.Write([]byte("status\r\nok\r\n> "))
	s.run.at = time.Now().Add(-time.Second)
	s.lastOutput = time.Now().Add(-time.Second)
	a.commandTick(s)
	c.await("\x03")
	_, _ = s.term.Write([]byte("\r\n> "))
	s.run.at = time.Now().Add(-time.Second)
	a.commandTick(s)
	if !s.contHome || s.run != nil {
		t.Fatalf("„>“ als normaler Prompt nicht gelernt (contHome=%v)", s.contHome)
	}
	if s.fail != nil {
		t.Fatalf("Befehl fälschlich als Fehlschlag: %+v", s.fail)
	}
	c.Reset()
	a.sendLine(s, "ls")
	if got := c.await("ls\r"); got != "ls\r" {
		t.Fatalf("ohne Strg+C erwartet: %q", got)
	}
}
