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
		{domain.Connection{}, "Juniper Junos", presetJunos.name},
		{domain.Connection{}, "SSH-2.0-FortiSSH", presetFortiOS.name},
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
	for _, p := range []quickPreset{presetAruba, presetCisco, presetMikroTik, presetJunos, presetFortiOS, presetComware, presetLinux, presetWindows} {
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
	in := [][2]string{{"Alt+0", "w"}, {"Strg+Space", "p"}, {"Esc", "z"}, {"F10", "m"}, {"F2", "z"}, {"Strg+F", "s"}, {"Shift+F9", "e"},
		{"F1", "h"}, {"⇧F1", "q1"}, {"Strg+T", "t"}, {"F9", "q"}, {"F7/F8", "Tabs"}, {"Strg+Q", "x"}}
	got := sortFKeys(in, "Strg+Space")
	want := []string{"Alt+0", "Strg+Space", "F1", "F2", "F7/F8", "F9", "F10", "⇧F1", "Shift+F9", "Esc", "Strg+F", "Strg+T", "Strg+Q"}
	if len(got) != len(want) {
		t.Fatalf("Länge %d, erwartet %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i][0] != w {
			t.Fatalf("Reihenfolge %v, erwartet %v", got, want)
		}
	}
}

func TestFooterFitsNarrowWindow(t *testing.T) {
	a := &App{keys: newKeymap(nil), s: NewStyles(Nacht)}
	keys := [][2]string{{"Strg+Space", "Befehle"}, {"F1", "Hilfe"}, {"F2", "Ziel"}, {"F3", "KI fragen"}, {"F4", "Tunnel"},
		{"F5", "Dateien"}, {"F6", "Host"}, {"F9", "Schnellbefehle"}, {"F10", "Manager"}, {"Shift+F9", "erklären"}, {"Strg+Q", "beenden"}}
	for _, w := range []int{200, 120, 90, 60, 40, 20} {
		a.w = w
		got := a.fitFooter(keys)
		width := 0
		hasHelp := false
		for _, k := range got {
			width += lipgloss.Width(a.s.Key.Render(k[0]))
			if k[1] != "" {
				width += lipgloss.Width(a.s.KeyDesc.Render(k[1]))
			}
			hasHelp = hasHelp || k[0] == "F1"
		}
		if width > w || !hasHelp {
			t.Errorf("Breite %d: Fußzeile %d breit, Hilfe sichtbar=%v (%v)", w, width, hasHelp, got)
		}
		if w == 200 && len(got) != len(keys) {
			t.Errorf("breites Fenster: Tasten fehlen")
		}
	}
}
