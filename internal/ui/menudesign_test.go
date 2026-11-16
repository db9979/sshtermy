package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Menüfenster (2026-11-13): Reiter, Detailspalte, Pillen, Punkte, Fußzeile
// passend zum Eintrag, Aufleuchten beim Umschalten.
func TestMenuDesign(t *testing.T) {
	g := newGold(t, i18n.DE, 150, 44, true)
	g.session("web-01", termOut)
	a := g.a
	a.openPalette()
	a.palEnter("settings")

	// Reiter: nur Allgemein, Tab wechselt zu Aussehen.
	for _, it := range a.paletteItems() {
		if it.group != pgSetBehave {
			t.Fatalf("Reiter Allgemein zeigt %q", it.label)
		}
	}
	a.paletteKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if a.pal.tab != pgSetLook {
		t.Fatalf("Tab wechselt nicht den Reiter: %v", a.pal.tab)
	}
	out := ansi.Strip(a.View().Content)
	for _, want := range []string{"Allgemein", "Aussehen", "Sicherheit", "Einrichten", "Aussehen · 8", "Tab Reiter"} {
		if !strings.Contains(out, want) {
			t.Errorf("Ansicht ohne %q:\n%s", want, out)
		}
	}
	// Detailspalte: Hilfesatz und Vorschau zum Farbschema.
	if !strings.Contains(out, "26 Farbschemas") || !strings.Contains(out, "upstream timed out") {
		t.Errorf("Detailspalte fehlt:\n%s", out)
	}
	// Suche zeigt wieder alle Reiter.
	a.pal.in.SetValue("sperren")
	if _, ok := hasLabel(a.paletteItems(), "Automatisch sperren"); !ok {
		t.Error("Suche findet nichts aus anderen Reitern")
	}
	a.pal.in.SetValue("")

	// Fußzeile je Eintrag und Positionspunkte.
	items := a.paletteItems()
	for i, it := range items {
		if it.label == "Fensterstil" {
			a.pal.sel = i
			if it.pos[1] != len(framesValues) {
				t.Errorf("Fensterstil ohne Position: %v", it.pos)
			}
			if h := ansi.Strip(a.palHint(it, false)); !strings.ContainsAny(h, "●") || !strings.Contains(h, "○") {
				t.Errorf("Punkte fehlen: %q", h)
			}
		}
	}
	if k := a.palKeys(items); k[2][1] != "ändern" {
		t.Errorf("Fußzeile auf Wahlwert: %v", k)
	}
	before := a.st.Settings().Frames
	a.paletteKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if a.st.Settings().Frames == before {
		t.Error("→ schaltet nicht weiter")
	}
	if a.pal.mo.changed != "Fensterstil" {
		t.Errorf("Umschalten nicht gemerkt: %q", a.pal.mo.changed)
	}

	// Pillen: an/aus mit Fläche.
	if h := a.palHint(palItem{state: palOn}, false); !strings.Contains(h, "48;") {
		t.Errorf("an ohne Fläche: %q", h)
	}

	// Ordner zeigt in der Detailspalte seinen Inhalt.
	a.pal.tab = pgSetSetup
	a.pal.sel = 0
	out = ansi.Strip(a.View().Content)
	if !strings.Contains(out, "Darin:") {
		t.Errorf("Ordner ohne Inhalt in der Detailspalte:\n%s", out)
	}
	if k := a.palKeys(a.paletteItems()); k[2][1] != "öffnen" {
		t.Errorf("Fußzeile auf Ordner: %v", k)
	}

	// Schmales Fenster: keine Detailspalte, Hilfesatz unter der Auswahl.
	a.w = 100
	a.pal.tab = pgSetBehave
	a.pal.sel = 0
	out = ansi.Strip(a.View().Content)
	if !strings.Contains(out, "› Blendet Profi-Einträge") {
		t.Errorf("Hilfesatz unter der Auswahl fehlt:\n%s", out)
	}
}
