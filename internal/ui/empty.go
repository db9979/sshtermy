package ui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Leere Zustände: Statt einer leeren Fläche sagt sshTermy, was hier
// hingehört und was man als Nächstes tun kann. Die Tasten sind anklickbar.

type emptyStep struct{ key, text string }

// emptyState zeichnet Symbol, Überschrift, einen erklärenden Satz und die
// nächsten Schritte, links ausgerichtet mit Einzug, höchstens w breit.
func (a *App) emptyState(icon, title, text string, steps []emptyStep, w int) string {
	s := a.s
	var b strings.Builder
	b.WriteString("\n  " + s.Accent.Render(icon+"  "+title) + "\n")
	if text != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(s.T.Muted).PaddingLeft(2).Width(max(16, w-2)).Render(text) + "\n")
	}
	if len(steps) > 0 {
		b.WriteString("\n")
	}
	kw := 0
	for _, st := range steps {
		kw = max(kw, lipgloss.Width(st.key))
	}
	for _, st := range steps {
		key := s.Key.Render(fmt.Sprintf("%-*s", kw, st.key))
		line := "  " + key + " " + s.Text.Render(trunc(st.text, max(6, w-kw-7)))
		b.WriteString(a.keyZone(st.key, line) + "\n")
	}
	return b.String()
}

// mgrEmpty: Manager ohne eine einzige Verbindung.
func (a *App) mgrEmpty(w int) string {
	return a.emptyState("≈", i18n.T("Noch keine Verbindungen"),
		i18n.T("Hier stehen gleich deine Server, Switches und Firewalls, mit Farbe und Online-Punkt."),
		[]emptyStep{
			{"n", i18n.T("neue Verbindung anlegen")},
			{a.keys.label(actImport), i18n.T("importieren (PuTTY, MobaXterm …)")},
			{a.keys.label(actScan), i18n.T("im Netz nach SSH-Geräten suchen")},
			{a.keys.label(actFolderNew), i18n.T("Ordner anlegen")},
		}, w)
}

// mgrNoMatch: Suche oder Tag-Filter ohne Treffer.
func (a *App) mgrNoMatch(w int) string {
	q := strings.TrimSpace(a.mgr.search.Value())
	title := i18n.T("Nichts gefunden")
	if q != "" {
		title = i18n.F("Nichts gefunden für „%s“", trunc(q, max(6, w-26)))
	} else if a.mgr.tag != "" {
		title = i18n.F("Nichts mit #%s", trunc(a.mgr.tag, max(6, w-20)))
	}
	steps := []emptyStep{{"Esc", i18n.T("Filter aufheben")}}
	if a.mgr.tag != "" {
		steps = append(steps, emptyStep{a.keys.label(actTagFilter), i18n.T("anderen Tag wählen")})
	}
	if !a.mgr.searching { // beim Tippen in der Suche wäre „n“ ein Buchstabe
		steps = append(steps, emptyStep{"n", i18n.T("neue Verbindung anlegen")})
	}
	return a.emptyState("/", title, i18n.T("Gesucht wird in Alias, Host, Benutzer, Ordner und Tags."), steps, w)
}

// mgrLegend: rechts im leeren Manager erklärt, was Punkte, Symbole und
// Farbstreifen in der Liste bedeuten.
func (a *App) mgrLegend(w int) string {
	s := a.s
	t := s.T
	var b strings.Builder
	b.WriteString("\n " + s.PanelTitle.Render(i18n.T("So liest du die Liste")) + "\n")
	item := func(g string, c color.Color, text string) {
		b.WriteString("   " + glyph(g, c, nil) + "  " + s.Text.Render(trunc(text, max(6, w-7))) + "\n")
	}
	item("◉", t.OK, i18n.T("verbunden"))
	item("●", t.OK, i18n.T("erreichbar"))
	item("●", t.Danger, i18n.T("nicht erreichbar"))
	item("○", t.Muted, i18n.T("noch nicht geprüft oder hinter Jump-Host"))
	b.WriteString("\n")
	for _, d := range []devClass{devLinux, devWindows, devMac, devSwitch, devRouter, devFirewall} {
		item(devGlyph(d), a.devColor(d), devName(d))
	}
	b.WriteString("\n")
	item("▌", accentColor(t, "red"), i18n.T("Produktiv (Ordner, Tag oder Alias mit „prod“)"))
	item("▌", accentColor(t, "green"), i18n.T("Test (test, dev, staging)"))
	item("▌", accentColor(t, "blue"), i18n.T("Labor (lab, labor)"))
	b.WriteString(lipgloss.NewStyle().Foreground(t.Muted).PaddingLeft(3).Width(max(16, w-3)).Render(i18n.T("Eigene Farbe: Verbindung bearbeiten (e) → Farbe.")) + "\n")
	return b.String()
}
