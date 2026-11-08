package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/containers"
	"github.com/db9979/sshtermy/internal/domain"
)

func TestContainerPicker(t *testing.T) {
	a := dangerApp(t)
	s, out := typedSession(domain.Connection{Alias: "web"}, "$ ", "")
	a.sessions = append(a.sessions, s)
	res := containers.Parse("docker|a1|web|nginx|running|Up 3 hours\nk8s|shop|api|  |Running|true 2\n@done\n")
	if a.showContainers(s, res); a.pick == nil || len(a.pick.items) != 2 {
		t.Fatal("keine Liste")
	}
	if !strings.HasPrefix(a.pick.items[0].label, "● web") || !strings.Contains(a.pick.items[1].hint, "k8s") {
		t.Fatalf("%+v", a.pick.items)
	}
	// Strg+R fügt den Neustart nur ein (kein \r)
	a.pickerKey(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if !out.has("docker restart web") || strings.Contains(out.String(), "\r") {
		t.Fatalf("eingefügt: %q", out.String())
	}
	// Pod: Start/Stopp gibt es nicht, Liste bleibt offen
	a.showContainers(s, res)
	a.pick.sel = 1
	a.pickerKey(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if a.pick == nil {
		t.Fatal("Liste sollte offen bleiben")
	}
	// leeres Ergebnis mit Rechte-Fehler: Hinweis statt Liste
	a.pick = nil
	a.showContainers(s, containers.Parse("@err docker|permission denied while trying to connect\n@done\n"))
	if a.pick != nil {
		t.Fatal("ohne Container keine Liste")
	}
}
