package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/db9979/sshtermy/internal/i18n"
)

// palArea: Einträge eines Bereichs der Palette.
func palArea(a *App, area string) []palItem {
	a.palEnter(area)
	defer a.palEnter("")
	return a.paletteItems()
}

// Menüs aufgeräumt (2026-11-11): KI und Suchen & Logs als eigene Bereiche,
// F3 und F5 in der Palette, Aufteilung als Durchschalt-Eintrag, keine
// Manager-Tasten im Terminal.
func TestPaletteAreasRegrouped(t *testing.T) {
	g := newGold(t, i18n.DE, 120, 40, true)
	g.session("web-01", termOut)
	a := g.a
	a.openPalette()

	ki := palArea(a, "ai")
	for _, want := range []string{"Ziel-Modus öffnen", "Agent-Modus", "KI fragen", "KI-Begleiter"} {
		if _, ok := hasLabel(ki, want); !ok {
			t.Errorf("Bereich KI ohne %q: %v", want, labels(ki))
		}
	}
	sess := palArea(a, "session")
	for _, want := range []string{"Dateien (SFTP)", "Host-Übersicht"} {
		if _, ok := hasLabel(sess, want); !ok {
			t.Errorf("Diese Sitzung ohne %q: %v", want, labels(sess))
		}
	}
	if _, ok := hasLabel(sess, "Ziel-Modus"); ok {
		t.Errorf("KI gehört nicht mehr zu „Diese Sitzung“: %v", labels(sess))
	}

	find := labels(palArea(a, "find"))
	want := []string{"Im Verlauf suchen", "In allen Tabs suchen", "Befehle dieses Tabs", "Fehlerzeilen im Verlauf zeigen", "Log-Blick", "Sitzungsprotokolle durchsuchen", "Ordner-Scan", "Logwache"}
	if len(find) < len(want) {
		t.Fatalf("Suchen & Logs: %v", find)
	}
	for i, w := range want {
		if !strings.HasPrefix(find[i], w) {
			t.Fatalf("Suchen & Logs, Platz %d: %q statt %q (%v)", i, find[i], w, find)
		}
	}

	tabs := palArea(a, "tabs")
	split, ok := hasLabel(tabs, "Aufteilung")
	if !ok {
		t.Fatalf("Tabs & Ansicht: %v", labels(tabs))
	}
	if _, ok := hasLabel(tabs, "Ansicht teilen:"); ok {
		t.Errorf("Einzelne Aufteilungen nur über die Suche: %v", labels(tabs))
	}
	for _, it := range tabs {
		if it.label == "Aufteilung" {
			split = it
		}
	}
	if split.cycle == nil {
		t.Fatal("Aufteilung muss sich durchschalten lassen")
	}
	split.cycle(1)
	if a.split == splitOff {
		t.Error("Durchschalten hat die Ansicht nicht geteilt")
	}

	// Manager-Tasten (m, f, n, l …) wirken im Terminal nicht: kein Hinweis.
	for _, area := range []string{"net", "conns", "find", "tabs"} {
		for _, it := range palArea(a, area) {
			switch it.hint {
			case "m", "f", "z", "n", "l", "c", "w", "i":
				t.Errorf("%s: %q zeigt die Manager-Taste %q im Terminal", area, it.label, it.hint)
			}
		}
	}
}

// Einstellungen: Gruppen Allgemein, Aussehen, Sicherheit, Einrichten; Logs,
// Netzwerkgeräte und Fehlersuche in eigenen Ordnern, Import/Export bei den
// Verbindungen.
func TestSettingsRegrouped(t *testing.T) {
	a := newPaletteTestApp(t)
	a.palEnter("settings")
	items := allSettings(a)
	order := []palGroup{pgSetBehave, pgSetLook, pgSetSafe, pgSetSetup}
	pos := 0
	for _, it := range items {
		for pos < len(order) && it.group != order[pos] {
			pos++
		}
		if pos == len(order) {
			t.Fatalf("Gruppen nicht in der Reihenfolge Allgemein, Aussehen, Sicherheit, Einrichten: %q", it.label)
		}
	}
	if items[0].label != i18n.T("Einfacher Modus: Profi-Funktionen ausblenden") {
		t.Errorf("Einfacher Modus gehört nach oben: %q", items[0].label)
	}
	for _, gone := range []string{"Sitzungsprotokoll schreiben", "Syslog", "Audit", "Befehlssätze", "Updates anbieten", "Debug", "Leistungsanzeige", "MCP", "Startanimation", "Import / Export"} {
		if _, ok := hasLabel(items, gone); ok {
			t.Errorf("%q gehört in einen Unterordner: %v", gone, labels(items))
		}
	}
	if len(items) > 28 {
		t.Errorf("Einstellungen zu lang (%d): %v", len(items), labels(items))
	}
	for menu, want := range map[string]string{
		"settings/logs": "Syslog-Empfang", "settings/net": "Befehlssätze", "settings/ki": "MCP-Zugriff",
		"help": "Updates anbieten", "help/debug": "Leistungsanzeige",
	} {
		a.palEnter(menu)
		if _, ok := hasLabel(a.paletteItems(), want); !ok {
			t.Errorf("%s ohne %q: %v", menu, want, labels(a.paletteItems()))
		}
	}
}

// Bewegung „aus“ schaltet auch die Startanimation ab, zurück schaltet sie an.
func TestMotionOffDisablesIntro(t *testing.T) {
	t.Setenv("SSHTERMY_NO_INTRO", "")
	a := newPaletteTestApp(t)
	prev := motionSetting
	t.Cleanup(func() { motionSetting = prev })
	for motionSetting != motionDezent {
		a.cycleMotion(1)
	}
	if !a.introOn() {
		t.Fatal("Startanimation sollte anfangs an sein")
	}
	a.cycleMotion(1) // → aus
	if motionSetting != motionAus || a.introOn() {
		t.Fatalf("Bewegung %q, Startanimation an: %v", motionSetting, a.introOn())
	}
	a.cycleMotion(1) // → voll
	if !a.introOn() {
		t.Fatal("Startanimation bleibt aus, obwohl Bewegung wieder an ist")
	}
}

// Rechtsklick auf eine Verbindung oder einen Ordner im Manager.
func TestManagerRightClick(t *testing.T) {
	g := newGold(t, i18n.DE, 120, 40, true)
	a := g.a
	rows := a.mgrRows()
	var conn, folder mgrRow
	for _, r := range rows {
		if r.isFolder() && folder.node == nil {
			folder = r
		}
		if !r.isFolder() && r.conn.Alias == "web-01" {
			conn = r
		}
	}
	m := a.mgrMenu(conn, 10, 5)
	var got []string
	for _, it := range m.items {
		got = append(got, it.label)
	}
	for _, want := range []string{"Verbinden", "Bearbeiten …", "In Ordner verschieben …", "Tunnel …", "Schnellbefehle …", "Löschen"} {
		if !strings.Contains(strings.Join(got, "|"), want) {
			t.Errorf("Menü ohne %q: %v", want, got)
		}
	}
	for _, it := range m.items {
		if strings.HasPrefix(it.label, "Markieren") {
			it.run()
		}
	}
	if !a.mgr.marked[conn.conn.ID] {
		t.Error("Markieren aus dem Menü wirkt nicht")
	}
	fm := a.mgrMenu(folder, 10, 5)
	if fm.title == "" || len(fm.items) < 4 {
		t.Fatalf("Ordner-Menü: %+v", fm.items)
	}
}

// Menüs aufräumen 15–16: Tab-Menü mit „Verbindung bearbeiten“ und „Daneben
// zeigen“, Trennlinien zählen nicht mit.
func TestTabMenuEditAndBeside(t *testing.T) {
	g := newGold(t, i18n.DE, 120, 40, true)
	web := g.session("web-01", termOut)
	db := g.session("db-01", termOut)
	a := g.a
	a.focusSession(a.indexOf(web))
	m := a.tabMenu(db, 5, 1, nil)
	var beside, edit menuItem
	for _, it := range m.items {
		switch it.label {
		case "Daneben zeigen (geteilt)":
			beside = it
		case "Verbindung bearbeiten …":
			edit = it
		}
	}
	if beside.run == nil || edit.run == nil {
		t.Fatalf("Tab-Menü: %+v", m.items)
	}
	if !edit.sep || !beside.sep {
		t.Error("Bearbeiten und Anordnen beginnen je eine Gruppe")
	}
	a.modal = m
	out := ansi.Strip(a.menuView(m))
	if !strings.Contains(out, "────") {
		t.Errorf("keine Trennlinie:\n%s", out)
	}
	if want := fmt.Sprintf("%d %s", len(m.items), m.items[len(m.items)-1].label); !strings.Contains(out, want) {
		t.Errorf("Trennlinien zählen mit, %q fehlt:\n%s", want, out)
	}
	beside.run()
	if a.split != splitCols || a.slotOf(db) < 0 || a.slotOf(web) < 0 || a.current() != web {
		t.Fatalf("Daneben zeigen: split=%v panes=%v aktiv=%s", a.split, a.panes, a.current().conn.Alias)
	}
	for _, it := range a.tabMenu(db, 5, 1, nil).items {
		if it.label == "Daneben zeigen (geteilt)" {
			t.Error("Sichtbarer Tab bietet „Daneben zeigen“ noch an")
		}
	}
	edit.run()
	if !a.returnToTerminal {
		t.Error("Bearbeiten vom Tab führt danach nicht zurück ins Terminal")
	}
}

// Menüs aufräumen 17–21: kurze Manager-Fußzeile, einheitliche Namen,
// Ordner-Scan nicht mehr unter „Häufig“.
func TestMenus1521Names(t *testing.T) {
	g := newGold(t, i18n.DE, 120, 40, true)
	a := g.a
	var foot []string
	for _, k := range a.managerKeys() {
		foot = append(foot, k[0]+" "+k[1])
	}
	for _, gone := range []string{"Broadcast", "s KI", "y Sync", "l Protokolle", "Über", "t Tunnel"} {
		if strings.Contains(strings.Join(foot, "|"), gone) {
			t.Errorf("Manager-Fußzeile zeigt noch %q: %v", gone, foot)
		}
	}
	a.openPalette()
	for _, it := range a.paletteItems() {
		if it.top && strings.HasPrefix(it.label, "Ordner-Scan") {
			t.Errorf("Ordner-Scan steht unter „Häufig“")
		}
	}
	if it, ok := hasLabel(palArea(a, "find"), "Logwache: gesammelte Meldungen ansehen"); !ok || it.cycle != nil {
		t.Errorf("Suchen & Logs: %v", labels(palArea(a, "find")))
	}
	a.palEnter("settings/logs")
	if it, ok := hasLabel(a.paletteItems(), "Logwache"); !ok || it.label != "Logwache" || it.cycle == nil {
		t.Errorf("Einstellung „Logwache ‹aus›“ fehlt: %v", labels(a.paletteItems()))
	}
	a.palEnter("settings/ki/begleiter")
	if it, ok := hasLabel(a.paletteItems(), "KI-Begleiter"); !ok || it.label != "KI-Begleiter" || it.cycle == nil {
		t.Errorf("Einstellung „KI-Begleiter ‹Aus›“ fehlt: %v", labels(a.paletteItems()))
	}
	a.palEnter("")
	g.session("web-01", termOut)
	it, ok := hasLabel(palArea(a, "ai"), "KI-Begleiter")
	if !ok || it.label != "KI-Begleiter" || it.cycle == nil {
		t.Fatalf("KI: %v", labels(palArea(a, "ai")))
	}
	before := a.compLevel()
	it.cycle(1)
	if a.compLevel() == before {
		t.Error("KI-Begleiter lässt sich nicht durchschalten")
	}
}

// allSettings: Einträge aller Reiter der Einstellungen, in Reihenfolge.
func allSettings(a *App) []palItem {
	keep := a.pal.tab
	defer func() { a.pal.tab = keep }()
	var out []palItem
	for _, g := range palTabGroups {
		a.pal.tab = g
		out = append(out, a.paletteItems()...)
	}
	return out
}

// settingsTabOf: öffnet den Reiter, in dem label steht.
func settingsTabOf(a *App, label string) {
	for _, it := range allSettings(a) {
		if it.label == label {
			a.pal.tab = it.group
			return
		}
	}
}
