package ui

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/safety"
)

func TestQuickIndex(t *testing.T) {
	a := &App{keys: newKeymap(nil)}
	quickIndex := a.quickIndex
	if l := a.quickLabel(0); l != "⇧F1" {
		t.Errorf("Anzeige %q, erwartet ⇧F1", l)
	}
	cases := map[string]int{"shift+f1": 0, "shift+f8": 7, "f13": 0, "f20": 7}
	for k, want := range cases {
		if i, ok := quickIndex(k); !ok || i != want {
			t.Errorf("%s: got %d,%v want %d", k, i, ok, want)
		}
	}
	for _, k := range []string{"f1", "shift+f9", "f12", "f21", "shift+f10", "ctrl+shift+f1", "shift+f1x"} {
		if _, ok := quickIndex(k); ok {
			t.Errorf("%s darf kein Schnellbefehl sein", k)
		}
	}
}

func TestQuickPresetFor(t *testing.T) {
	net := domain.Connection{Device: domain.DeviceNetwork}
	cases := []struct {
		c      domain.Connection
		remote string
		want   string
	}{
		{net, "", presetAruba.name},
		{domain.Connection{}, "Netzwerkgerät (ArubaOS-Switch)", presetAruba.name},
		{domain.Connection{}, "Netzwerkgerät (Cisco-1.25)", presetCisco.name},
		{domain.Connection{}, "Netzwerkgerät (ROSSSH)", presetMikroTik.name},
		{domain.Connection{}, "Netzwerkgerät (Comware-7.1.064)", presetComware.name},
		{domain.Connection{}, "Ubuntu 24.04 LTS · Linux 6.8 · /bin/bash", presetLinux.name},
		{domain.Connection{}, "Windows (OpenSSH) oder unbekannt", presetWindows.name},
		{domain.Connection{}, "", presetLinux.name},
	}
	for _, c := range cases {
		if got := quickPresetFor(c.c, c.remote).name; got != c.want {
			t.Errorf("%q: got %s want %s", c.remote, got, c.want)
		}
	}
	for _, p := range []quickPreset{presetAruba, presetCisco, presetMikroTik, presetComware, presetLinux, presetWindows} {
		if len(p.cmds) != maxQuick {
			t.Errorf("%s: %d Befehle, erwartet %d", p.name, len(p.cmds), maxQuick)
		}
		kind := "unix"
		if p.name == presetWindows.name {
			kind = "windows"
		}
		for _, c := range p.cmds {
			if f := safety.Check(c, kind); len(f) > 0 {
				t.Errorf("%s: Vorbelegung %q ist rot (%s)", p.name, c, f[0].Reason)
			}
		}
	}
}

func TestTourFitsWindow(t *testing.T) {
	s := NewStyles(Nacht)
	for step := range tourSteps {
		for _, sz := range [][2]int{{120, 30}, {80, 24}, {70, 24}} {
			m := &modal{kind: mTour, scroll: step}
			out := m.view(s, sz[0], sz[1])
			if w, h := lipgloss.Width(out), lipgloss.Height(out); w > sz[0] || h > sz[1]-2 {
				t.Errorf("Schritt %d bei %dx%d: %dx%d", step+1, sz[0], sz[1], w, h)
			}
		}
	}
}

func TestSortFKeys(t *testing.T) {
	in := [][2]string{{"Strg+Space", "p"}, {"F10", "m"}, {"F2", "z"}, {"Shift+F9", "e"}, {"F1", "h"}, {"F9", "q"}, {"Strg+Q", "x"}}
	got := sortFKeys(in)
	want := []string{"Strg+Space", "F1", "F2", "F9", "F10", "Shift+F9", "Strg+Q"}
	for i, w := range want {
		if got[i][0] != w {
			t.Fatalf("Reihenfolge %v, erwartet %v", got, want)
		}
	}
}
