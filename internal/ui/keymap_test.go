package ui

import (
	"testing"
)

func TestKeyLabelRoundTrip(t *testing.T) {
	for _, k := range []string{"f2", "ctrl+g", "alt+1", "ctrl+space", "shift+enter", "ctrl+j", "i", "ctrl+shift+p"} {
		lbl := keyLabel(k)
		m, ok := keyFromLabel(lbl)
		if !ok || m.String() != k {
			t.Errorf("%s → %q → %q (%v)", k, lbl, m.String(), ok)
		}
	}
}

func TestKeymapOverride(t *testing.T) {
	km := newKeymap(map[string]string{"goal": "ctrl+g"})
	if !km.is(actGoal, "ctrl+g") || km.is(actGoal, "f2") || km.label(actGoal) != "Strg+G" {
		t.Fatal("Überschreiben wirkt nicht")
	}
	if !km.is(actPalette, "ctrl+@") {
		t.Fatal("Standard-Aliase verloren")
	}
	if o := km.overrides(); len(o) != 1 || o["goal"] != "ctrl+g" {
		t.Fatalf("overrides: %v", o)
	}
	for _, d := range actionDefs {
		for _, k := range d.def {
			if k == "alt+enter" {
				t.Fatal("Alt+⏎ darf nicht Standard sein (Windows Terminal Vollbild)")
			}
		}
	}
}

func TestKeymapConflicts(t *testing.T) {
	if c := newKeymap(nil).conflicts(); len(c) != 0 {
		t.Fatalf("Standardbelegung hat Konflikte: %v", c)
	}
	c := newKeymap(map[string]string{"goal": "f3"}).conflicts()
	if c[actGoal] != actAsk || c[actAsk] != actGoal {
		t.Fatalf("Konflikt F3 nicht erkannt: %v", c)
	}
	if reservedKey("alt+enter") == "" || reservedKey("ctrl+g") != "" {
		t.Fatal("reservedKey")
	}
}

func TestRelabelHelp(t *testing.T) {
	old := activeKeys
	defer func() { activeKeys = old }()
	activeKeys = newKeymap(map[string]string{"goal": "ctrl+g", "import": "I"})
	if relabelDefault("F2", hcTerminal) != "Strg+G" || relabelDefault("F3", hcTerminal) != "F3" {
		t.Fatal("Hilfe zeigt nicht die eigene Taste")
	}
	if relabelDefault("i", hcGoal) != "i" {
		t.Fatal("„i“ im Ziel-Modus darf nicht umbenannt werden")
	}
}

func TestThemes(t *testing.T) {
	if ThemeByName("hell").Name != "hell" || ThemeByName("").Name != "nacht" || ThemeByName("kontrast").Dark != true {
		t.Fatal("ThemeByName")
	}
	for _, th := range Themes {
		_ = NewStyles(th)
		if th.Bg == nil || th.Text == nil || th.Danger == nil {
			t.Fatalf("%s unvollständig", th.Name)
		}
	}
}
