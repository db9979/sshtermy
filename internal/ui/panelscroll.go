package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
)

// panelScroll: Blättern in der Seitenleiste (Ziel/Agent/Plan, KI fragen,
// Begleiter). Ohne Blättern zeigt das Panel das Ende (laufende Ausgabe), den
// Anfang (Plan) oder ab einer Marke (Abschlusszusammenfassung).
type panelScroll struct {
	pinned   bool // vom Menschen geblättert: start bleibt stehen
	start    int  // erste sichtbare Zeile
	def      int  // Startzeile ohne Blättern
	total    int  // Zeilen im letzten Bild
	vis      int  // sichtbare Zeilen
	mark     int  // Zeile, an der das Panel anfangen soll (-1 = keine), je Bild neu gesetzt
	markSeen int
}

// markPanel merkt die aktuelle Zeile als Anfang der Ansicht (z. B. die
// Zusammenfassung „✓ Ziel erreicht“), damit sie nicht oben abgeschnitten wird.
func markPanel(s *session, raw string) {
	n := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\n' {
			n++
		}
	}
	s.pscroll.mark = n
}

// scrollPanelLines schneidet die sichtbaren Zeilen aus und zeigt oben/unten,
// wie viel noch verborgen ist.
func (a *App) scrollPanelLines(s *session, lines []string, vis int, top bool, w int) []string {
	ps := &s.pscroll
	ps.total, ps.vis = len(lines), vis
	if vis <= 0 || len(lines) <= vis {
		ps.start, ps.def, ps.pinned = 0, 0, false
		return lines
	}
	maxStart := len(lines) - vis
	def := maxStart // Ende sichtbar halten
	switch {
	case top:
		def = 0 // Plan: Anfang mit den Schritten
	case ps.mark >= 0:
		def = min(max(0, ps.mark-1), maxStart) // eine Zeile davor: Platz für den ↑-Hinweis
	}
	start := def
	if ps.pinned {
		start = clamp(ps.start, 0, maxStart)
	}
	ps.start, ps.def = start, def
	out := append([]string(nil), lines[start:start+vis]...)
	if vis < 4 {
		return out
	}
	if start > 0 {
		out[0] = a.s.Muted.Render(trunc(fmt.Sprintf(i18n.T("↑ %d weitere Zeilen · Bild↑ oder Mausrad"), start+1), w))
	}
	if below := maxStart - start; below > 0 {
		out[vis-1] = a.s.Muted.Render(trunc(fmt.Sprintf(i18n.T("↓ %d weitere Zeilen · Bild↓ oder Mausrad"), below+1), w))
	}
	return out
}

// scrollPanel blättert um d Zeilen. false = hier gibt es nichts zu blättern
// (Panel passt ganz hinein oder blättert selbst).
func (s *session) scrollPanel(d int) bool {
	ps := &s.pscroll
	switch s.panel {
	case panelGoal, panelAsk, panelCompanion:
	default:
		return false
	}
	if ps.vis <= 0 || ps.total <= ps.vis {
		return false
	}
	ps.start = clamp(ps.start+d, 0, ps.total-ps.vis)
	ps.pinned = ps.start != ps.def // zurück an der Ausgangsstelle: wieder mitlaufen
	return true
}

// panelPage: Bild↑/Bild↓ blättern eine Seite (mit zwei Zeilen Überlappung).
func (s *session) panelPage(down bool) bool {
	d := max(1, s.pscroll.vis-2)
	if !down {
		d = -d
	}
	return s.scrollPanel(d)
}

// doneText: Ziel und Abschlusszusammenfassung als Klartext.
func (g *goalState) doneText() string {
	return i18n.T("Ziel: ") + g.goal + "\n\n" + g.doneReason
}

// copyDone kopiert Ziel und Zusammenfassung (c nach „✓ Ziel erreicht“).
func (a *App) copyDone(s *session) tea.Cmd {
	text := s.goal.doneText()
	go func() { _ = clipboard.WriteAll(text) }()
	a.flash(i18n.T("Kopiert: Zusammenfassung"))
	return tea.SetClipboard(text)
}

// readDone zeigt die Zusammenfassung groß und blätterbar (l).
func (a *App) readDone(s *session) {
	w := max(20, dialog{maxW: 200}.innerMax(a.w))
	lines := strings.Split(ansi.Wrap(s.goal.doneText(), w, ""), "\n")
	a.modal = textModal(i18n.T("✓ Ziel erreicht"), lines)
}
