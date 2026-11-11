package ui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Markieren im Seitenpanel (Ziel/Agent/Plan, KI fragen, Begleiter)
//
// Die Maus gehört sshTermy, also kann das Terminalfenster dort nichts
// markieren. Deshalb markiert sshTermy selbst, wie im Terminal: Ziehen
// markiert, Loslassen kopiert (Zwischenablage + OSC 52), Doppelklick = Wort,
// Dreifachklick = Zeile, Rechtsklick kopiert die Markierung. Über den Rand
// ziehen blättert mit. Anklickbares (Hinweise, Antwortvorschläge) bleibt
// anklickbar; Ziehen fängt dann daneben an.

// pselPos: Stelle im Panelinhalt (Zeile des ganzen Inhalts, Spalte in Zellen).
type pselPos struct{ line, col int }

func (p pselPos) before(q pselPos) bool {
	return p.line < q.line || (p.line == q.line && p.col < q.col)
}

type panelSel struct {
	a, b     pselPos
	dragging bool
}

// norm: Anfang vor Ende.
func (p panelSel) norm() (pselPos, pselPos) {
	if p.b.before(p.a) {
		return p.b, p.a
	}
	return p.a, p.b
}

func (p panelSel) empty() bool { return p.a == p.b }

// selectablePanel: in welchen Panels markiert sshTermy selbst?
func selectablePanel(k panelKind) bool {
	return k == panelGoal || k == panelAsk || k == panelCompanion
}

// panelTextPos: Stelle im Panelinhalt unter (x, y). ok = false außerhalb der
// Textfläche (Rahmen, unter dem Inhalt). Mit clampIt wird auf die Fläche
// begrenzt (Ziehen über den Rand); dy sagt dann, ob oben (-1) oder unten (+1)
// hinausgezogen wurde.
func (a *App) panelTextPos(s *session, x, y int, clampIt bool) (p pselPos, dy int, ok bool) {
	r := a.termRect(s)
	pw := a.panelShown(s)
	x0, y0 := r.x+r.w-pw+2, r.y+1 // Rahmen + Innenabstand
	w := pw - 4
	vis := s.pscroll.vis
	if s.pscroll.total <= vis || vis <= 0 {
		vis = s.pscroll.total
	}
	if w <= 0 || vis <= 0 {
		return p, 0, false
	}
	row, col := y-y0, x-x0
	if row < 0 {
		dy = -1
	} else if row >= vis {
		dy = 1
	}
	if !clampIt && (dy != 0 || col < 0 || col >= w) {
		return p, 0, false
	}
	row, col = clamp(row, 0, vis-1), clamp(col, 0, w)
	return pselPos{line: s.pscroll.start + row, col: col}, dy, true
}

// panelSelMouse: Markieren im Panel. ok = false: Ereignis gehört nicht hierher.
func (a *App) panelSelMouse(s *session, msg tea.MouseMsg, clicks int) (tea.Cmd, bool) {
	if a.scr != scrTerminal || s == nil || a.modal != nil || a.pal != nil || a.pick != nil ||
		!selectablePanel(s.panel) || s.files.open || (s.panel == panelHost && s.host.live.wide) {
		return nil, false
	}
	if a.panelCollapsed(s) && !s.panelFocus {
		return nil, false // nur die schmale Leiste ist zu sehen
	}
	m := msg.Mouse()
	_, isClick := msg.(tea.MouseClickMsg)
	_, isRelease := msg.(tea.MouseReleaseMsg)
	_, isMotion := msg.(tea.MouseMotionMsg)

	if sel := s.psel; sel != nil && sel.dragging {
		switch {
		case isMotion:
			p, dy, _ := a.panelTextPos(s, m.X, m.Y, true)
			if dy != 0 && s.scrollPanel(dy) { // über den Rand: mitblättern
				p.line += dy
			}
			sel.b = p
			return nil, true
		case isRelease:
			sel.dragging = false
			if sel.empty() {
				s.psel = nil
				return nil, true
			}
			return a.copyPanelSel(s), true
		}
		return nil, true
	}

	if !isClick || !a.inPanel(s, m.X, m.Y) {
		return nil, false
	}
	switch m.Button {
	case tea.MouseRight:
		if s.psel != nil && !s.psel.empty() {
			cmd := a.copyPanelSel(s)
			s.psel = nil
			return cmd, true
		}
		return nil, false
	case tea.MouseLeft:
	default:
		return nil, false
	}
	// Anklickbares bleibt anklickbar (Hinweise, Vorschläge, Runbooks); nur
	// große Blöcke (KI-Karten) lassen sich zusätzlich markieren.
	if act := a.z.at(m.X, m.Y, false); act != nil && !act.block {
		s.psel = nil
		return nil, false
	}
	p, _, ok := a.panelTextPos(s, m.X, m.Y, false)
	if !ok {
		s.psel = nil
		return nil, false
	}
	s.panelFocus = true
	switch {
	case clicks == 2:
		s.psel = a.panelWordAt(s, p)
		return a.copyPanelSel(s), true
	case clicks >= 3:
		s.psel = &panelSel{a: pselPos{p.line, 0}, b: pselPos{p.line, 1 << 20}}
		return a.copyPanelSel(s), true
	}
	s.psel = &panelSel{a: p, b: p, dragging: true}
	// Klick auf eine KI-Karte wählt sie weiterhin aus.
	if act := a.z.at(m.X, m.Y, false); act != nil {
		return act.click(m, false), true
	}
	return nil, true
}

// panelWordAt: Wort unter p (Leerzeichen trennen).
func (a *App) panelWordAt(s *session, p pselPos) *panelSel {
	if p.line < 0 || p.line >= len(s.pplain) {
		return nil
	}
	cells := []rune{}
	for _, r := range s.pplain[p.line] {
		cells = append(cells, r)
		for i := 1; i < ansi.StringWidth(string(r)); i++ {
			cells = append(cells, 0) // zweite Zelle breiter Zeichen
		}
	}
	space := func(i int) bool { return i >= len(cells) || (cells[i] != 0 && unicode.IsSpace(cells[i])) }
	if space(p.col) {
		return nil
	}
	l, r := p.col, p.col
	for l > 0 && !space(l-1) {
		l--
	}
	for r+1 < len(cells) && !space(r+1) {
		r++
	}
	return &panelSel{a: pselPos{p.line, l}, b: pselPos{p.line, r + 1}}
}

// panelSelText: markierter Text, Zeilen ohne Leerzeichen am Ende.
func (s *session) panelSelText() string {
	if s.psel == nil {
		return ""
	}
	a, b := s.psel.norm()
	var out []string
	for l := max(0, a.line); l <= b.line && l < len(s.pplain); l++ {
		line := s.pplain[l]
		from, to := 0, ansi.StringWidth(line)
		if l == a.line {
			from = a.col
		}
		if l == b.line {
			to = min(to, b.col)
		}
		t := ""
		if to > from {
			t = ansi.Cut(line, from, to)
		}
		out = append(out, strings.TrimRight(t, " "))
	}
	return strings.Trim(strings.Join(out, "\n"), "\n")
}

// copyPanelSel kopiert die Markierung im Panel (Betriebssystem + OSC 52).
func (a *App) copyPanelSel(s *session) tea.Cmd {
	text := s.panelSelText()
	if strings.TrimSpace(text) == "" {
		return nil
	}
	go func() { _ = clipboard.WriteAll(text) }()
	if lines := strings.Count(text, "\n") + 1; lines > 1 {
		a.flash(i18n.T("Kopiert: ") + plural(lines, i18n.T("Zeile"), i18n.T("Zeilen")))
	} else {
		n := utf8.RuneCountInString(text)
		a.flash(i18n.T("Kopiert: ") + plural(n, i18n.T("Zeichen"), i18n.T("Zeichen")))
	}
	return tea.SetClipboard(text)
}

// highlightPanelSel hebt die Markierung in den sichtbaren Panelzeilen hervor.
// start: Inhaltszeile der ersten sichtbaren Zeile, w: Breite der Textfläche.
func (a *App) highlightPanelSel(s *session, lines []string, start, w int) {
	if s.psel == nil || s.psel.empty() {
		return
	}
	p, q := s.psel.norm()
	hl := a.s.Selected
	for i := range lines {
		l := start + i
		if l < p.line || l > q.line {
			continue
		}
		from, to := 0, w
		if l == p.line {
			from = p.col
		}
		if l == q.line {
			to = min(w, q.col)
		}
		if to <= from {
			continue
		}
		line := lines[i]
		plain := ansi.Strip(ansi.Cut(line, from, to))
		lines[i] = ansi.Cut(line, 0, from) + hl.Render(plain) + ansi.Cut(line, to, max(w, ansi.StringWidth(line)))
	}
}
