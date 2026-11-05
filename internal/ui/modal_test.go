package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/store"
	"github.com/db9979/sshtermy/internal/version"
)

func TestYesNo(t *testing.T) {
	cases := []struct {
		k    tea.KeyPressMsg
		want int
	}{
		{tea.KeyPressMsg{Code: 'j', Text: "j"}, 1},
		{tea.KeyPressMsg{Code: 'j', Text: "J", ShiftedCode: 'J', Mod: tea.ModCapsLock}, 1}, // Feststelltaste
		{tea.KeyPressMsg{Code: 'y'}, 1},                                                    // ohne Text
		{tea.KeyPressMsg{Code: 0, BaseCode: 'j'}, 1},                                       // Windows-VT-Eingabe
		{tea.KeyPressMsg{Code: 'N', Text: "N"}, -1},
		{tea.KeyPressMsg{Code: tea.KeyEscape}, -1},
		{tea.KeyPressMsg{Code: 'z', BaseCode: 'y', Text: "z"}, 0}, // deutsches z ≠ y
		{tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}, 0},
		{tea.KeyPressMsg{Code: 'x', Text: "x"}, 0},
	}
	for _, c := range cases {
		if got := yesNo(c.k); got != c.want {
			t.Errorf("yesNo(%+v) = %d, want %d", c.k, got, c.want)
		}
	}
}

func TestHostKeyModalClosesOnCapitalJ(t *testing.T) {
	a := &App{}
	r := make(chan bool, 1)
	a.modal = hostKeyModal(hostKeyAskMsg{host: "h:22", fp: "SHA256:x", typ: "ssh-ed25519", reply: r})
	a.modalKey(tea.KeyPressMsg{Code: 'j', ShiftedCode: 'J', Text: "J"})
	if a.modal != nil || !<-r {
		t.Fatal("Dialog bleibt offen oder Antwort falsch")
	}
}

func TestAboutShowsCoffeeLink(t *testing.T) {
	m := (&App{}).aboutModal()
	if !strings.Contains(m.body, version.Coffee) || !strings.Contains(m.body, "b = Kaffee") {
		t.Fatalf("Kaffee-Link fehlt im Über-Dialog:\n%s", m.body)
	}
	if !strings.HasPrefix(version.Coffee, "https://buymeacoffee.com/") {
		t.Fatal("Coffee-Link falsch")
	}
}

func TestChangelogShowsCoffeeLink(t *testing.T) {
	a := &App{w: 100, h: 40}
	m := a.changelogModal(nil)
	if !strings.Contains(strings.Join(m.lines, "\n"), version.Coffee) {
		t.Fatal("Kaffee-Link fehlt im Was-ist-neu-Dialog")
	}
	a.modal = m
	a.modalKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if a.modal == nil {
		t.Fatal("Dialog darf bei unbekannter Taste nicht schließen")
	}
}

// Nach einem Update öffnet sich die Kaffee-Seite genau einmal, sonst nie.
func TestCoffeeOpensOnceAfterUpdate(t *testing.T) {
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	a, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	old, oldV := openCoffee, version.Version
	openCoffee = func() { n++ }
	defer func() { openCoffee, version.Version = old, oldV }()

	version.Version = "1.0.5"
	a.showWhatsNew(true) // Ersteinrichtung: nicht öffnen
	if n != 0 {
		t.Fatal("bei der Ersteinrichtung darf sich der Browser nicht öffnen")
	}
	_ = st.UpdateSettings(func(s *domain.Settings) { s.LastVersion = "1.0.4" })
	a.showWhatsNew(false) // Update 1.0.4 → 1.0.5
	a.showWhatsNew(false) // zweiter Start derselben Version
	if n != 1 {
		t.Fatalf("Kaffee-Seite %d-mal geöffnet, erwartet 1", n)
	}
}
