package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
)

func typeKeys(a *App, s string) {
	for _, r := range s {
		a.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// Gleich nach der Server-Auswahl lostippen: nichts geht verloren, „x“
// schließt keinen Tab, ⏎ verbindet nicht doppelt und wird nie vorausgeschickt.
func TestTypeAheadWhileConnecting(t *testing.T) {
	t.Setenv("SSHTERMY_NO_INTRO", "1")
	a := dangerApp(t)
	c := domain.Connection{Alias: "web", Host: "10.0.0.5", Username: "admin"}
	s, out := typedSession(c, "", "")
	s.state = stConnecting
	a.sessions, a.cur = []*session{s}, 0

	typeKeys(a, "lxs")
	a.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	a.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	typeKeys(a, "s -x")
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(a.sessions) != 1 || s.state != stConnecting {
		t.Fatalf("Tippen beim Verbinden hat den Tab verändert: %d Tabs, Zustand %v", len(a.sessions), s.state)
	}
	if got := string(s.ahead); got != "ls -x" {
		t.Fatalf("vorausgetippt %q, erwartet %q", got, "ls -x")
	}
	if card := ansi.Strip(a.connectCard(s, 58)); !strings.Contains(card, "ls -x") {
		t.Fatalf("Karte zeigt das Getippte nicht:\n%s", card)
	}

	// verbunden, Prompt steht: Text landet dort, ohne ⏎
	s.state, s.connectedAt = stConnected, time.Now()
	_, _ = s.term.Write([]byte("admin@web:~$ "))
	s.lastOutput = time.Now().Add(-time.Second)
	a.flushAhead(s)
	time.Sleep(50 * time.Millisecond)
	if got := out.String(); got != "ls -x" {
		t.Fatalf("an den Server ging %q, erwartet %q", got, "ls -x")
	}
	if len(s.ahead) != 0 {
		t.Fatal("Puffer nicht geleert")
	}
}

// Kommt statt des Prompts eine Passwortabfrage, landet der Text nicht dort.
func TestTypeAheadNotIntoPasswordPrompt(t *testing.T) {
	t.Setenv("SSHTERMY_NO_INTRO", "1")
	a := dangerApp(t)
	s, out := typedSession(domain.Connection{Alias: "sw", Host: "10.0.0.9"}, "", "")
	s.state = stConnecting
	a.sessions, a.cur = []*session{s}, 0
	typeKeys(a, "geheim")

	s.state, s.connectedAt = stConnected, time.Now().Add(-aheadWait-time.Second)
	_, _ = s.term.Write([]byte("Password: "))
	s.lastOutput = time.Now().Add(-time.Second)
	a.flushAhead(s)
	time.Sleep(50 * time.Millisecond)
	if out.String() != "" {
		t.Fatalf("Text ging an die Passwortabfrage: %q", out.String())
	}
	if len(s.ahead) != 0 {
		t.Fatal("nach Ablauf nicht verworfen")
	}

	// Esc beim Verbinden bricht weiter ab
	s2, _ := typedSession(domain.Connection{Alias: "x", Host: "h"}, "", "")
	s2.state = stConnecting
	a.sessions, a.cur = []*session{s2}, 0
	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if len(a.sessions) != 0 {
		t.Fatal("Esc schließt den Tab beim Verbinden nicht mehr")
	}
}
