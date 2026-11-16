package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestTTYFixLine(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"docker exec -u 0 gitea vi /data/gitea/conf/app.ini", "docker exec -it -u 0 gitea vi /data/gitea/conf/app.ini"},
		{"sudo docker exec gitea bash", "sudo docker exec -it gitea bash"},
		{"docker exec -i gitea nano /etc/x", "docker exec -t -i gitea nano /etc/x"},
		{"docker exec --tty gitea top", "docker exec -i --tty gitea top"},
		{"docker exec -u0 gitea vim a", "docker exec -it -u0 gitea vim a"},
		{"docker exec --user root -w /data gitea /bin/sh", "docker exec -it --user root -w /data gitea /bin/sh"},
		{"podman container exec web less /var/log/x", "podman container exec -it web less /var/log/x"},
		{"docker run --rm alpine sh", "docker run -it --rm alpine sh"},
		{"kubectl exec -n prod web -- bash", "kubectl exec -it -n prod web -- bash"},
		{"docker exec gitea psql", "docker exec -it gitea psql"},
		// nichts zu tun
		{"docker exec -it gitea vi /x", ""},
		{"docker exec -ti -u 0 gitea vi /x", ""},
		{"docker exec gitea cat /data/gitea/conf/app.ini", ""},
		{"docker exec gitea sh -c 'ls /'", ""},
		{"docker exec gitea psql -c 'select 1'", ""},
		{"docker exec -d gitea vi /x", ""},
		{"cat dump.sql | docker exec -i db psql", ""},
		{"docker run --rm alpine", ""},
		{"docker ps", ""},
		{"vi /etc/hosts", ""},
		{"kubectl exec web -- ls", ""},
	} {
		got, _ := ttyFixLine(c.in)
		if got != c.want {
			t.Errorf("%q → %q, erwartet %q", c.in, got, c.want)
		}
	}
	if f := ttyFlagOf("docker exec -i g vi", "docker exec -t -i g vi"); f != "-t" {
		t.Errorf("ttyFlagOf = %q", f)
	}
}

// Kopieren aus einem Kasten im Panel: ohne Rahmen, umbrochene Zeilen wieder zu
// einer zusammengesetzt, echte Zeilenwechsel bleiben.
func TestPanelSelStripsFrame(t *testing.T) {
	s := &session{pplain: []string{
		"┌──────────────────────────────────────────┐",
		"│ docker exec -u 0 gitea vi                │",
		"│ /data/gitea/conf/app.ini                 │",
		"│ zweite Zeile                             │",
		"└──────────────────────────────────────────┘",
		"Text danach",
	}}
	s.psel = &panelSel{a: pselPos{0, 0}, b: pselPos{5, 11}}
	want := "docker exec -u 0 gitea vi /data/gitea/conf/app.ini\nzweite Zeile\nText danach"
	if got := s.panelSelText(); got != want {
		t.Fatalf("Markierung = %q\nerwartet   %q", got, want)
	}
	// Doppelklick-artig mitten in einer Zeile: kein Rahmen
	s.psel = &panelSel{a: pselPos{1, 2}, b: pselPos{1, 44}}
	if got := s.panelSelText(); got != "docker exec -u 0 gitea vi" {
		t.Fatalf("eine Zeile = %q", got)
	}
}

// Enter auf „docker exec … vi“ ohne -it: Zeile wird ergänzt, nichts läuft.
// Nochmal genau so getippt geht die Zeile durch.
func TestTTYFixOnEnter(t *testing.T) {
	a := dangerApp(t)
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	c := domain.Connection{Alias: "klebdocker", Host: "10.0.0.7", Username: "sadmin"}
	cmd := "docker exec -u 0 gitea vi /data/gitea/conf/app.ini"
	s, out := typedSession(c, "sadmin@klebdocker:~$ ", cmd)
	a.sessions = []*session{s}
	a.terminalKey(s, enter)
	if !out.has("\x15docker exec -it -u 0 gitea vi") {
		t.Fatalf("Zeile nicht ergänzt: %q", out.String())
	}
	if strings.Contains(out.String(), "\r") {
		t.Fatal("Enter ging trotz Korrektur an den Server")
	}
	if s.ttyFixed != cmd {
		t.Fatalf("ttyFixed = %q", s.ttyFixed)
	}

	// bewusst ohne -it: zweites Mal läuft es
	s2, out2 := typedSession(c, "sadmin@klebdocker:~$ ", cmd)
	s2.ttyFixed = cmd
	a.sessions = []*session{s2}
	a.terminalKey(s2, enter)
	if !out2.sent() || strings.Contains(out2.String(), "\x15") {
		t.Fatalf("nochmal getippt wurde nicht ausgeführt: %q", out2.String())
	}
}
