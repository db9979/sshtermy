package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
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
