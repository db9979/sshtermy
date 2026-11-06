package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/store"
)

func newPaletteTestApp(t *testing.T) *App {
	t.Helper()
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
	a.w, a.h = 120, 40
	a.openPalette()
	return a
}

func ansiStrip(s string) string { return ansi.Strip(s) }

func hasLabel(items []palItem, prefix string) (palItem, bool) {
	for _, it := range items {
		if strings.HasPrefix(it.label, prefix) {
			return it, true
		}
	}
	return palItem{}, false
}

// Schalter, Themes und Hilfe stehen nicht mehr in der Übersicht, sondern in
// den Untermenüs; getippter Text findet sie trotzdem.
func TestPaletteSubmenus(t *testing.T) {
	a := newPaletteTestApp(t)
	top := a.paletteItems()
	for _, gone := range []string{"Maus", "Aussehen: ", "Sprache / Language", "Tastenkürzel anzeigen", "Einführungstour"} {
		if _, ok := hasLabel(top, gone); ok {
			t.Errorf("%q gehört ins Untermenü: %v", gone, labels(top))
		}
	}
	set, ok := hasLabel(top, "Einstellungen ›")
	if !ok {
		t.Fatalf("Einstellungen fehlt: %v", labels(top))
	}
	if _, ok := hasLabel(top, "Hilfe & Info ›"); !ok {
		t.Fatalf("Hilfe & Info fehlt: %v", labels(top))
	}

	// Im Manager ohne Sitzung gibt es kein „KI fragen“: kein Tab-Hinweis.
	if strings.Contains(ansiStrip(a.viewPalette()), "Tab") {
		t.Error("Tab „KI fragen“ ohne Wirkung darf nicht angezeigt werden")
	}

	// Untermenü öffnen: Palette bleibt offen, Schalter zeigen ihren Zustand.
	a.palRun(set)
	if a.pal == nil || a.pal.menu != "settings" {
		t.Fatal("Einstellungen öffnen muss die Palette offen lassen")
	}
	items := a.paletteItems()
	mouse, ok := hasLabel(items, "Maus")
	if !ok || mouse.state != palOn {
		t.Fatalf("Maus-Schalter: %+v in %v", mouse, labels(items))
	}
	if _, ok := hasLabel(items, "Tastenkürzel anzeigen"); ok {
		t.Error("Hilfe gehört nicht in die Einstellungen")
	}
	a.palRun(mouse)
	if a.pal == nil {
		t.Fatal("Schalter muss die Palette offen lassen")
	}
	if mouse, _ = hasLabel(a.paletteItems(), "Maus"); mouse.state != palOff {
		t.Fatalf("nach dem Umschalten: %v", mouse.state)
	}
	if !strings.Contains(a.viewPalette(), "○ aus") {
		t.Error("Zustand „aus“ wird nicht angezeigt")
	}

	// Esc führt zurück zur Übersicht, ein zweites Esc schließt.
	a.paletteKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.pal == nil || a.pal.menu != "" {
		t.Fatal("Esc im Untermenü muss zurück zur Übersicht")
	}
	a.paletteKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.pal != nil {
		t.Fatal("Esc in der Übersicht schließt die Palette")
	}

	// Suche in der Übersicht findet Punkte aus den Untermenüs.
	a.openPalette()
	a.pal.in.SetValue("tour")
	it, ok := hasLabel(a.paletteItems(), "Einführungstour")
	if !ok || !strings.Contains(it.sub, "Hilfe") {
		t.Fatalf("Suche nach „tour“: %+v", it)
	}
	a.pal.in.SetValue("language")
	if _, ok := hasLabel(a.paletteItems(), "Sprache / Language"); !ok {
		t.Fatal("Suche nach „language“ findet die Sprache nicht")
	}
}
