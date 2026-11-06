package ui

import (
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

const probeOut = "have tmux\nhave screen\ntmux 1 sshtermy\ntmux 0 my work\n" +
	"There are screens on:\n\t4711.build\t(06.11.2026 10:00:00)\t(Detached)\n\t4712.pts-0.host\t(Attached)\n2 Sockets in /run/screen/S-u.\n"

func TestParsePersistProbe(t *testing.T) {
	p := parsePersistProbe(probeOut)
	if !p.tmux || !p.screen {
		t.Fatalf("installiert nicht erkannt: %+v", p)
	}
	want := []muxSession{{"tmux", "sshtermy", true}, {"tmux", "my work", false}, {"screen", "build", false}, {"screen", "pts-0.host", true}}
	if len(p.sessions) != len(want) {
		t.Fatalf("Sitzungen: %+v", p.sessions)
	}
	for i, w := range want {
		if p.sessions[i] != w {
			t.Errorf("%d: %+v, will %+v", i, p.sessions[i], w)
		}
	}
	if p := parsePersistProbe("No Sockets found in /run/screen.\n"); p.tmux || p.screen || len(p.sessions) != 0 {
		t.Errorf("leer: %+v", p)
	}
}

func TestPlanPersist(t *testing.T) {
	both := parsePersistProbe(probeOut)
	none := persistProbe{}
	onlyScreen := persistProbe{screen: true}
	fresh := persistProbe{tmux: true}
	c := func(kind, name string, view bool) domain.Connection {
		return domain.Connection{Persist: kind, PersistName: name, PersistView: view}
	}
	cases := []struct {
		name string
		conn domain.Connection
		p    persistProbe
		cmd  string // "" = Hinweis
		pick int
	}{
		{"vorhanden", c("auto", "", false), both, "tmux new -A -s sshtermy", 0},
		{"neu", c("tmux", "", false), fresh, "tmux new -A -s sshtermy", 0},
		{"andere laufen", c("tmux", "neu", false), both, "tmux new -A -s neu", 2},
		{"nichts installiert", c("auto", "", false), none, "", 0},
		{"tmux fehlt", c("tmux", "", false), onlyScreen, "", 0},
		{"auto nimmt screen", c("auto", "x", false), onlyScreen, "screen -S x", 0},
		{"screen vorhanden", c("screen", "build", false), both, "screen -x build", 0},
		{"zuschauen", c("tmux", "", true), both, "tmux attach -r -t sshtermy", 0},
		{"zuschauen ohne Sitzung", c("tmux", "", true), fresh, "", 0},
		{"zuschauen screen", c("screen", "build", true), both, "", 0},
		{"Name mit Leerzeichen", c("tmux", "my work", false), both, "tmux new -A -s 'my work'", 0},
	}
	for _, k := range cases {
		pl := planPersist(k.conn, k.p)
		if k.cmd == "" {
			if pl.hint == "" {
				t.Errorf("%s: Hinweis erwartet, %+v", k.name, pl)
			}
			continue
		}
		if pl.hint != "" || pl.mux.attachCmd() != k.cmd || len(pl.pick) != k.pick {
			t.Errorf("%s: %q pick=%d hint=%q, will %q pick=%d", k.name, pl.mux.attachCmd(), len(pl.pick), pl.hint, k.cmd, k.pick)
		}
	}
}

func TestPersistFormRoundTrip(t *testing.T) {
	a, _ := rawTestApp(t)
	a.openConnForm(domain.Connection{Port: 22, Auth: domain.AuthAgent, Persist: "tmux", PersistName: "ops", PersistView: true})
	var c domain.Connection
	applyPersistFields(a.form, &c)
	if c.Persist != "tmux" || c.PersistName != "ops" || !c.PersistView {
		t.Fatalf("%+v", c)
	}
	c = domain.Connection{Protocol: domain.ProtoTelnet}
	applyPersistFields(a.form, &c)
	if c.Persist != "" || c.PersistView {
		t.Errorf("Telnet: %+v", c)
	}
}

func TestWatchCommand(t *testing.T) {
	got := watchCommand(domain.Connection{Host: "spark", Port: 2222, Username: "dominik"}, "sshtermy")
	if got != "ssh -t -p 2222 dominik@spark tmux attach -r -t sshtermy" {
		t.Error(got)
	}
}

func TestPersistHiddenForSwitch(t *testing.T) {
	a, _ := rawTestApp(t)
	a.openConnForm(newConnDefaults())
	f := a.form
	if fl := f.get("persist"); fl.hidden(f) {
		t.Fatal("bei SSH sichtbar erwartet")
	}
	if f.get("pname").hidden(f) == false {
		t.Error("Name sichtbar, obwohl aus")
	}
	setChoice(t, f, "device", deviceLabels[2])
	if !f.get("persist").hidden(f) {
		t.Error("bei Netzwerkgerät ausgeblendet erwartet")
	}
}
