//go:build linux || darwin

package ui

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/safety"
)

// Lokale Shell: Tab startet, Befehle laufen, Profil kommt vom eigenen
// Rechner (Unix-Regeln, kein Netzwerkgerät), exit beendet ohne Auto-Reconnect.
func TestLocalSession(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("kein sh")
	}
	a, st := rawTestApp(t)
	closed := make(chan shellClosedMsg, 1)
	go func() {
		for m := range a.events {
			if c, ok := m.(shellClosedMsg); ok {
				closed <- c
			}
		}
	}()
	dir := t.TempDir()
	c, err := st.UpsertConnection(domain.Connection{Alias: "Hier", Protocol: domain.ProtoLocal, LocalShell: sh, LocalDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{id: 1, conn: c, chipArmed: -1}
	a.sessions = []*session{s}
	msg := a.connectCmd(s)()
	if e, ok := msg.(connectErrMsg); ok {
		t.Fatal(e.err)
	}
	a.Update(msg)
	if s.state != stConnected || s.netDev() || s.netDevice() {
		t.Fatalf("Zustand %v, netDev %v", s.state, s.netDev())
	}
	if s.prof.Dialect == "" || s.prof.Shell != sh || a.safetyKind(s) != "unix" {
		t.Fatalf("Profil %+v, Regeln %s", s.prof, a.safetyKind(s))
	}
	if !a.agentFilesOK(s) {
		t.Fatal("Agent soll lokal Skripte ablegen dürfen")
	}
	if got := safety.Check("rm -rf /", a.safetyKind(s)); len(got) == 0 {
		t.Fatal("Gefahren-Check greift lokal nicht")
	}
	ctx := a.agentContext(s, false)
	if ctx.Net || !strings.Contains(ctx.Profile, "Shell: "+sh) || ctx.Shell != "sh" {
		t.Fatalf("KI-Kontext: %+v", ctx)
	}
	s.term.SendText("echo lo$((1+1))cal; pwd\r")
	waitFor(t, func() bool {
		scr := s.term.PlainLines(20)
		return strings.Contains(scr, "lo2cal") && strings.Contains(scr, dir)
	})
	s.term.SendText("exit\r")
	select {
	case m := <-closed:
		if m.dropped {
			t.Fatal("exit gilt als Abriss")
		}
		a.onShellClosed(s, m)
		if s.state != stClosed || s.rc.active {
			t.Fatalf("nach exit: %v, reconnect %v", s.state, s.rc.active)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shell endet nicht")
	}
}
