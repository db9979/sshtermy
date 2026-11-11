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
	if _, ok := palAskItem(a.paletteItems()); ok || strings.Contains(ansiStrip(a.viewPalette()), "KI fragen") {
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

// Kompaktes Einstellungsmenü: Durchschalt-Einträge, Unterordner, Suche
// findet Einzelwerte.
func TestPaletteSettingsCompact(t *testing.T) {
	a := newPaletteTestApp(t)
	a.palEnter("settings")
	items := a.paletteItems()
	if _, ok := hasLabel(items, "Aussehen: "); ok {
		t.Fatal("Einzelwerte nur bei der Suche")
	}
	sel := func(label string) {
		for i, it := range a.paletteItems() {
			if it.label == label {
				a.pal.sel = i
				return
			}
		}
		t.Fatalf("%q fehlt", label)
	}
	sel("Automatisch sperren bei Leerlauf")
	a.paletteKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if a.st.Settings().LockAfterMin != 5 {
		t.Fatalf("→: %d", a.st.Settings().LockAfterMin)
	}
	a.paletteKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	a.paletteKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if a.st.Settings().LockAfterMin != 60 || a.pal == nil || a.pal.menu != "settings" {
		t.Fatalf("← rundherum: %d", a.st.Settings().LockAfterMin)
	}
	sel("Meldung, wenn lange Befehle fertig sind")
	old := desktopSend
	desktopSend = func(string, string) {}
	t.Cleanup(func() { desktopSend = old })
	a.paletteKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.notifyLevel() != 0 || !a.st.Settings().NoNotify {
		t.Fatalf("Meldung nach ⏎: %d", a.notifyLevel())
	}
	// Unterordner: → öffnet, Esc geht eine Ebene hoch.
	sel("KI")
	a.paletteKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if a.pal.menu != "settings/ki" {
		t.Fatalf("Ordner: %q", a.pal.menu)
	}
	if _, ok := hasLabel(a.paletteItems(), "KI-Ersatz-Anbieter"); !ok {
		t.Fatal("KI-Ordner leer")
	}
	a.paletteKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.pal == nil || a.pal.menu != "settings" {
		t.Fatal("Esc im Unterordner muss zu den Einstellungen")
	}
	// Suche: Einzelwerte und Einträge aus Unterordnern.
	a.pal.in.SetValue("matrix")
	if _, ok := hasLabel(a.paletteItems(), "Aussehen: Matrix"); !ok {
		t.Fatal("Suche findet das Farbschema nicht")
	}
	a.pal.in.SetValue("syslog")
	if it, ok := hasLabel(a.paletteItems(), "Syslog-Empfang"); !ok || !strings.Contains(it.sub, "Protokolle & Logs") {
		t.Fatalf("Suche im Unterordner: %+v", it)
	}
	// Import/Export sind Aktionen im Bereich „Verbindungen & Daten“.
	a.palEnter("")
	a.pal.in.SetValue("csv")
	if it, ok := hasLabel(a.paletteItems(), "Verbindungen als CSV"); !ok || it.group != pgAction {
		t.Fatalf("CSV-Export: %+v", it)
	}
}

// Übersicht ohne Text: häufige Aktionen und Ordner je Bereich statt einer
// langen Liste; Ordner öffnen mit →, Esc führt zurück, Suche findet alles.
func TestPaletteAreas(t *testing.T) {
	a := newPaletteTestApp(t)
	a.scr = scrManager
	top := a.paletteItems()
	if _, ok := hasLabel(top, "Team-Paket übernehmen"); ok {
		t.Fatalf("Team-Aktionen gehören in den Ordner: %v", labels(top))
	}
	if it, ok := hasLabel(top, "Neue Verbindung"); !ok || it.group != pgTop {
		t.Fatalf("Neue Verbindung fehlt unter Häufig: %v", labels(top))
	}
	for i, it := range top {
		if strings.HasPrefix(it.label, "Team & Teilen ›") {
			a.pal.sel = i
		}
	}
	a.paletteKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if a.pal.menu != "team" {
		t.Fatalf("→ öffnet den Ordner nicht: %q", a.pal.menu)
	}
	if _, ok := hasLabel(a.paletteItems(), "Team-Paket übernehmen"); !ok {
		t.Fatalf("Ordner Team: %v", labels(a.paletteItems()))
	}
	if !strings.Contains(ansiStrip(a.viewPalette()), "Befehlspalette › Team & Teilen") {
		t.Error("Titel zeigt den Ordner nicht")
	}
	a.paletteKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.pal == nil || a.pal.menu != "" {
		t.Fatal("Esc im Ordner muss zur Übersicht")
	}
	a.pal.in.SetValue("topologie")
	if _, ok := hasLabel(a.paletteItems(), "Netzwerk-Topologie"); !ok {
		t.Fatal("Suche findet Einträge aus Ordnern nicht")
	}
}
