package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Geteilte Ansicht (Strg+T)
//
// Zwei Sitzungen nebeneinander oder untereinander, oder vier im Raster.
// Jeder Bereich hat eine eigene Titelzeile; der aktive ist blau. Strg+Pfeil
// wechselt den Bereich, ein Klick in einen Bereich auch. Tasten, Vorschläge,
// Panels und die Suche gelten immer für den aktiven Bereich.
//
// Die Sitzungen bleiben Tabs: F7/F8 holt eine verdeckte Sitzung in den
// aktiven Bereich, eine neue Verbindung landet in einem leeren Bereich.

type splitMode int

const (
	splitOff  splitMode = iota
	splitCols           // zwei nebeneinander
	splitRows           // zwei untereinander
	splitGrid           // vier im Raster
)

// grid: Spalten und Zeilen der Aufteilung.
func (m splitMode) grid() (cols, rows int) {
	switch m {
	case splitCols:
		return 2, 1
	case splitRows:
		return 1, 2
	case splitGrid:
		return 2, 2
	}
	return 1, 1
}

func (m splitMode) slots() int { c, r := m.grid(); return c * r }

func (m splitMode) label() string {
	return map[splitMode]string{
		splitOff:  i18n.T("eine Sitzung"),
		splitCols: i18n.T("zwei nebeneinander"),
		splitRows: i18n.T("zwei untereinander"),
		splitGrid: i18n.T("vier Bereiche"),
	}[m]
}

type rect struct{ x, y, w, h int }

func (r rect) has(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

// areaRect: Fläche zwischen Sitzungszeile und Vorschlagszeile.
func (a *App) areaRect() rect {
	return rect{0, topRows, a.w, max(5, a.h-topRows-bottomRows)}
}

// splitLen teilt total in n Stücke mit sep Trennbreite dazwischen.
func splitLen(total, n, sep int) []int {
	avail := max(n, total-sep*(n-1))
	out := make([]int, n)
	for i := range out {
		out[i] = avail / n
		if i < avail%n {
			out[i]++
		}
	}
	return out
}

// paneRects: Bereiche (mit Titelzeile) in Lesereihenfolge.
func (a *App) paneRects() []rect {
	ar := a.areaRect()
	c, r := a.split.grid()
	ws, hs := splitLen(ar.w, c, 1), splitLen(ar.h, r, 0)
	var out []rect
	y := ar.y
	for j := 0; j < r; j++ {
		x := ar.x
		for i := 0; i < c; i++ {
			out = append(out, rect{x, y, ws[i], hs[j]})
			x += ws[i] + 1
		}
		y += hs[j]
	}
	return out
}

// slotOf: Bereich der Sitzung oder -1.
func (a *App) slotOf(s *session) int {
	if s == nil || a.split == splitOff {
		return -1
	}
	for i, id := range a.panes {
		if id == s.id {
			return i
		}
	}
	return -1
}

// termRect: Fläche des Terminals (samt Seitenpanel) für eine Sitzung.
func (a *App) termRect(s *session) rect {
	if j := a.slotOf(s); j >= 0 {
		if rs := a.paneRects(); j < len(rs) {
			r := rs[j]
			return rect{r.x, r.y + 1, r.w, max(3, r.h-1)}
		}
	}
	return a.areaRect()
}

func (a *App) resizeAll() {
	for _, s := range a.sessions {
		a.resize(s)
	}
}

func (a *App) indexOf(s *session) int {
	for i, x := range a.sessions {
		if x == s {
			return i
		}
	}
	return -1
}

// focusSession macht Sitzung i aktiv. In der geteilten Ansicht kommt sie in
// einen leeren Bereich oder ersetzt die bisher aktive.
func (a *App) focusSession(i int) {
	if i < 0 || i >= len(a.sessions) {
		return
	}
	prev := a.current()
	a.cur = i
	if a.split != splitOff {
		a.prunePanes()
		s := a.sessions[i]
		if a.slotOf(s) < 0 {
			placed := false
			for j, id := range a.panes {
				if id == 0 {
					a.panes[j], placed = s.id, true
					break
				}
			}
			if !placed {
				j := a.slotOf(prev)
				a.panes[max(0, j)] = s.id
			}
		}
	}
	a.resizeAll()
}

// prunePanes leert Bereiche geschlossener Tabs.
func (a *App) prunePanes() {
	for j, id := range a.panes {
		if id != 0 && a.byID(id) == nil {
			a.panes[j] = 0
		}
	}
}

// fixFocus nach dem Schließen eines Tabs: Die aktive Sitzung soll sichtbar
// sein, frei gewordene Bereiche bekommen verdeckte Sitzungen.
func (a *App) fixFocus() {
	if a.split == splitOff {
		a.resizeAll()
		return
	}
	a.prunePanes()
	for _, s := range a.sessions {
		if a.slotOf(s) >= 0 {
			continue
		}
		for j, id := range a.panes {
			if id == 0 {
				a.panes[j] = s.id
				break
			}
		}
	}
	if a.slotOf(a.current()) < 0 {
		for _, id := range a.panes {
			if s := a.byID(id); s != nil {
				a.cur = a.indexOf(s)
				break
			}
		}
	}
	a.resizeAll()
}

// setSplit stellt die Aufteilung um.
func (a *App) setSplit(m splitMode) {
	if m != splitOff {
		c, r := m.grid()
		if ar := a.areaRect(); ar.w/c < 30 || ar.h/r < 6 {
			a.flash(i18n.T("Fenster zu klein für „") + m.label() + "“")
			return
		}
	}
	old := a.panes
	a.split = m
	a.panes = nil
	if m != splitOff {
		n := m.slots()
		seen := map[int]bool{}
		var ids []int
		add := func(id int) {
			if id != 0 && !seen[id] && a.byID(id) != nil {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		for _, id := range old {
			add(id)
		}
		cur := a.current()
		if len(a.sessions) <= n || cur == nil {
			for _, x := range a.sessions { // passt alles: Reihenfolge der Tabs
				add(x.id)
			}
		} else {
			add(cur.id)
			for k := range a.sessions {
				add(a.sessions[(a.cur+k)%len(a.sessions)].id)
			}
		}
		if cur != nil {
			if i := indexInt(ids, cur.id); i >= n {
				ids[n-1] = cur.id
			}
		}
		a.panes = make([]int, n)
		copy(a.panes, ids)
	}
	a.resizeAll()
	a.flash(i18n.T("Ansicht: ") + m.label())
}

func indexInt(xs []int, v int) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return -1
}

// cycleSplit: Strg+T schaltet weiter: eine → nebeneinander → untereinander →
// vier → eine.
func (a *App) cycleSplit() {
	next := (a.split + 1) % 4
	for next != splitOff {
		c, r := next.grid()
		if ar := a.areaRect(); ar.w/c >= 30 && ar.h/r >= 6 {
			break
		}
		next = (next + 1) % 4
	}
	a.setSplit(next)
}

// movePane: Strg+Pfeil wechselt in den Nachbarbereich (leere werden
// übersprungen, am Rand geht es auf der anderen Seite weiter).
func (a *App) movePane(dir string) {
	c, r := a.split.grid()
	j := a.slotOf(a.current())
	if j < 0 {
		return
	}
	col, row := j%c, j/c
	for step := 0; step < max(c, r); step++ {
		switch dir {
		case "left":
			col = (col - 1 + c) % c
		case "right":
			col = (col + 1) % c
		case "up":
			row = (row - 1 + r) % r
		case "down":
			row = (row + 1) % r
		}
		k := row*c + col
		if k == j || k >= len(a.panes) {
			return
		}
		if s := a.byID(a.panes[k]); s != nil {
			a.focusSession(a.indexOf(s))
			return
		}
	}
}

func paneDir(key string) (string, bool) {
	switch key {
	case "ctrl+left", "ctrl+right", "ctrl+up", "ctrl+down":
		return strings.TrimPrefix(key, "ctrl+"), true
	}
	return "", false
}

// paneAt: Sitzung im Bereich unter (x, y), sonst nil.
func (a *App) paneAt(x, y int) *session {
	if a.split == splitOff {
		return nil
	}
	for j, r := range a.paneRects() {
		if r.has(x, y) && j < len(a.panes) {
			return a.byID(a.panes[j])
		}
	}
	return nil
}

// ---------------------------------------------------------------- Ansicht

func (a *App) viewSplit(focus *session) (string, *tea.Cursor) {
	st := a.s
	rects := a.paneRects()
	c, r := a.split.grid()
	blocks := make([]string, len(rects))
	var cur *tea.Cursor
	for j, rc := range rects {
		var ps *session
		if j < len(a.panes) {
			ps = a.byID(a.panes[j])
		}
		title := a.paneTitle(ps, rc.w, ps != nil && ps == focus)
		var body string
		if ps == nil {
			body = lipgloss.Place(rc.w, rc.h-1, lipgloss.Center, lipgloss.Center,
				st.Muted.Render(i18n.T("Leerer Bereich"))+"\n\n"+st.Muted.Render(i18n.T("Neue Verbindung landet hier:"))+"\n"+
					a.keyZone("Strg+Space", st.Muted.Render(i18n.T("Strg+Space")))+st.Muted.Render(" "+i18n.T("oder")+" ")+
					a.keyZone(a.keys.label(actManager), st.Muted.Render(a.keys.label(actManager)))+st.Muted.Render(" → ⏎"))
		} else {
			b, cc := a.paneBody(ps, ps == focus)
			body = b
			if ps == focus {
				cur = cc
			}
		}
		blocks[j] = title + "\n" + fitBlock(body, rc.w, rc.h-1)
	}
	var rows []string
	for j := 0; j < r; j++ {
		var parts []string
		for i := 0; i < c; i++ {
			k := j*c + i
			if i > 0 {
				h := rects[k].h
				parts = append(parts, st.Muted.Foreground(st.T.Border).Render(strings.TrimSuffix(strings.Repeat("│\n", h), "\n")))
			}
			parts = append(parts, blocks[k])
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, parts...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...), cur
}

// paneTitle: eine Zeile über jedem Bereich, anklickbar.
func (a *App) paneTitle(s *session, w int, focused bool) string {
	st := a.s
	bar := lipgloss.NewStyle().Background(st.T.Surface).Foreground(st.T.Muted)
	if focused {
		bar = lipgloss.NewStyle().Background(st.T.Accent).Foreground(st.T.Bg).Bold(true)
	}
	if s == nil {
		return bar.Width(w).MaxWidth(w).Render(i18n.T(" ○ leer"))
	}
	// Akzentfarbe der Verbindung: aktiver Bereich ganz in der Farbe,
	// die anderen mit Farbstreifen (accent.go)
	col := a.accentOf(s.conn)
	if col != nil && focused {
		bar = bar.Background(col)
	}
	dot := map[sessState]string{stConnecting: "◌", stConnected: "●", stClosed: "○"}[s.state]
	left := " " + dot + " " + s.conn.Alias + "  " + s.conn.Username + "@" + s.conn.Host
	var marks []string
	if s.search.open {
		marks = append(marks, i18n.T("/ Suche"))
	}
	if s.goal.active {
		marks = append(marks, i18n.T("◎ Ziel"))
	}
	if s.panel != panelNone {
		marks = append(marks, i18n.T("▐ Panel"))
	}
	if focused && len(marks) == 0 && w >= 60 {
		marks = append(marks, i18n.T("Strg+Pfeil wechselt"))
	}
	right := strings.Join(marks, " · ")
	if right != "" {
		right += " "
	}
	left = ansi.Truncate(left, max(0, w-ansi.StringWidth(right)-1), "…")
	line := left + strings.Repeat(" ", max(1, w-ansi.StringWidth(left)-ansi.StringWidth(right))) + right
	if col != nil && !focused {
		line = glyph("▌", col, st.T.Surface) + bar.Render(ansi.Truncate(strings.TrimPrefix(line, " "), w-1, ""))
	} else {
		line = bar.Render(ansi.Truncate(line, w, ""))
	}
	return a.z.click(line, func(tea.Mouse, bool) tea.Cmd {
		if i := a.indexOf(s); i >= 0 {
			a.focusSession(i)
		}
		return nil
	})
}

func padRight(s string, w int) string {
	if d := w - ansi.StringWidth(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// fitBlock schneidet/füllt einen Block auf genau w×h Zellen (ohne
// lipgloss-Umbruch, der ANSI-Zeilen zerreißen würde).
func fitBlock(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	for i, l := range lines {
		if lw := ansi.StringWidth(l); lw > w {
			lines[i] = ansi.Truncate(l, w, "")
		} else if lw < w {
			lines[i] = l + strings.Repeat(" ", w-lw)
		}
	}
	return strings.Join(lines, "\n")
}
