package ui

import "testing"

func TestKeyFromLabel(t *testing.T) {
	for label, want := range map[string]string{
		"⏎": "enter", "Esc": "esc", "Tab": "tab", "F2": "f2", "F10": "f10", "n": "n", "/": "/", ".": ".", "~": "~",
		"Strg+S": "ctrl+s", "Strg+Space": "ctrl+space", "Strg+Q": "ctrl+q", "Strg+T": "ctrl+t", "←": "left",
	} {
		k, ok := keyFromLabel(label)
		if !ok || k.String() != want {
			t.Errorf("%q → %q (ok=%v), erwartet %q", label, k.String(), ok, want)
		}
	}
	for _, label := range []string{"↑↓", "1…9", "Alt+1…4", "Esc/F5"} {
		if _, ok := keyFromLabel(label); ok {
			t.Errorf("%q sollte nicht klickbar sein", label)
		}
	}
	if k, ok := keyFromKeyString("alt+2"); !ok || k.String() != "alt+2" {
		t.Errorf("alt+2 → %q", k.String())
	}
}
