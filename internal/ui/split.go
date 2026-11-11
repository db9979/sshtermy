package ui

import (
	"math"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Geteilte Ansicht (Strg+T)
//
// Mehrere Sitzungen gleichzeitig: zwei nebeneinander oder untereinander,
// einer groß mit zwei daneben, drei nebeneinander, vier oder sechs im Raster.
// Strg+T öffnet die Auswahl (layout.go). Jeder Bereich hat eine eigene
// Titelzeile; der aktive ist farbig. Strg+Pfeil wechselt den Bereich, ein
// Klick auch.
//
//   - Größe: Trennlinie mit der Maus ziehen (Doppelklick = gleich groß),
//     Alt+⇧+Pfeil verschiebt die Linie neben dem aktiven Bereich, zwischen
//     Zeilen zieht man am ↕ rechts in der Titelzeile.
//   - Zoom (Alt+Z, Doppelklick auf den Titel): der aktive Bereich bekommt
//     vorübergehend das ganze Fenster, die anderen laufen weiter.
//   - Tauschen: Titelzeile auf einen anderen Bereich ziehen.
//
// Die Sitzungen bleiben Tabs: F7/F8 holt eine verdeckte Sitzung in den
// aktiven Bereich, eine neue Verbindung landet in einem leeren Bereich.

type splitMode int

const (
	splitOff   splitMode = iota
	splitCols            // zwei nebeneinander
	splitRows            // zwei untereinander
	splitGrid            // vier im Raster
	splitMain            // einer groß links, zwei rechts übereinander
	splitCols3           // drei nebeneinander
	splitGrid6           // sechs im Raster (3 × 2)
)

// layoutOrder: Reihenfolge in der Auswahl (Tasten 1 … 7) und für Strg+T im
// Auswahlfenster.
var layoutOrder = []splitMode{splitOff, splitCols, splitRows, splitMain, splitCols3, splitGrid, splitGrid6}

// Kleinste Bereiche beim Ändern der Größe (Spalten / Zeilen samt Titel).
const (
	minPaneW = 20
	minPaneH = 4
)

// grid: Spalten und Zeilen der Aufteilung (bei „groß + zwei“ gelten die
// Zeilen nur für die rechte Spalte).
func (m splitMode) grid() (cols, rows int) {
	switch m {
	case splitCols:
		return 2, 1
	case splitRows:
		return 1, 2
	case splitGrid, splitMain:
		return 2, 2
	case splitCols3:
		return 3, 1
	case splitGrid6:
		return 3, 2
	}
	return 1, 1
}

func (m splitMode) slots() int {
	if m == splitMain {
		return 3
	}
	c, r := m.grid()
	return c * r
}

func (m splitMode) label() string {
	return map[splitMode]string{
		splitOff:   i18n.T("eine Sitzung"),
		splitCols:  i18n.T("zwei nebeneinander"),
		splitRows:  i18n.T("zwei untereinander"),
		splitGrid:  i18n.T("vier Bereiche"),
		splitMain:  i18n.T("einer groß + zwei"),
		splitCols3: i18n.T("drei nebeneinander"),
		splitGrid6: i18n.T("sechs Bereiche"),
	}[m]
}

var splitIDs = map[splitMode]string{splitOff: "", splitCols: "cols", splitRows: "rows", splitGrid: "grid", splitMain: "main", splitCols3: "cols3", splitGrid6: "grid6"}

// id: Name der Aufteilung zum Speichern (Arbeitsplätze, Neustart).
func (m splitMode) id() string { return splitIDs[m] }

func splitByID(id string) (splitMode, bool) {
	for m, s := range splitIDs {
		if s == id {
			return m, true
		}
	}
	return splitOff, false
}

// fits: passt die Aufteilung ins Fenster (je Bereich 30 Spalten, 6 Zeilen)?
func (a *App) fits(m splitMode) bool {
	if m == splitOff {
		return true
	}
	c, r := m.grid()
	ar := a.areaRect()
	return ar.w/c >= 30 && ar.h/r >= 6
}

type rect struct{ x, y, w, h int }

func (r rect) has(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

// areaRect: Fläche zwischen Sitzungszeile und Vorschlagszeile.
func (a *App) areaRect() rect {
	return rect{0, topRows, a.w, max(5, a.h-topRows-bottomRows-a.companionRows()-a.vgRows())}
}

// splitLen teilt total in n gleiche Stücke mit sep Trennbreite dazwischen.
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

// fracLens teilt total nach den Anteilen fr (Summe 1); passen die Anteile
// nicht (falsche Anzahl, zu klein), gleich groß.
func fracLens(total, sep int, fr []float64, n int) []int {
	if len(fr) != n || n < 2 {
		return splitLen(total, n, sep)
	}
	avail := total - sep*(n-1)
	out := make([]int, n)
	cum, prev := 0.0, 0
	for i := range out {
		cum += fr[i]
		b := int(math.Round(float64(avail) * cum))
		if i == n-1 {
			b = avail
		}
		out[i] = b - prev
		prev = b
		if out[i] < 1 {
			return splitLen(total, n, sep)
		}
	}
	return out
}

// lens: Spaltenbreiten und Zeilenhöhen der aktuellen Aufteilung.
func (a *App) lens() (ws, hs []int) {
	ar := a.areaRect()
	c, r := a.split.grid()
	return fracLens(ar.w, 1, a.fracX, c), fracLens(ar.h, 0, a.fracY, r)
}

// paneRects: Bereiche (mit Titelzeile) in Lesereihenfolge.
func (a *App) paneRects() []rect {
	ar := a.areaRect()
	c, r := a.split.grid()
	ws, hs := a.lens()
	if a.split == splitMain {
		x2 := ar.x + ws[0] + 1
		return []rect{{ar.x, ar.y, ws[0], ar.h}, {x2, ar.y, ws[1], hs[0]}, {x2, ar.y + hs[0], ws[1], hs[1]}}
	}
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

// cellOf: Spalte und Zeile von Bereich j (bei „groß + zwei“ steht der
// große Bereich in Zeile 0, reicht aber über beide).
func (a *App) cellOf(j int) (col, row int) {
	if a.split == splitMain {
		switch j {
		case 0:
			return 0, 0
		case 1:
			return 1, 0
		}
		return 1, 1
	}
	c, _ := a.split.grid()
	return j % c, j / c
}

// hasAbove: liegt über Bereich j ein anderer (Titel trägt dann den ↕-Griff)?
func (a *App) hasAbove(j int) bool {
	_, row := a.cellOf(j)
	return row > 0
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
	if a.zoom && a.split != splitOff && s != nil && s == a.current() && a.slotOf(s) >= 0 {
		ar := a.areaRect()
		return rect{ar.x, ar.y + 1, ar.w, max(3, ar.h-1)}
	}
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
	a.sessions[i].unseen = false
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

// setSplit stellt die Aufteilung um. Die Größen gelten je Aufteilung neu
// (gleich groß), Zoom endet.
func (a *App) setSplit(m splitMode) {
	if !a.fits(m) {
		a.flash(i18n.T("Fenster zu klein für „") + m.label() + i18n.T("“"))
		return
	}
	old := a.panes
	if m != a.split {
		a.fracX, a.fracY = nil, nil
	}
	a.split, a.zoom = m, false
	if m == splitOff {
		a.syncIn = false
	}
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

// cycleSplit schaltet zur nächsten Aufteilung, die ins Fenster passt.
func (a *App) cycleSplit() {
	k := 0
	for i, m := range layoutOrder {
		if m == a.split {
			k = i
		}
	}
	for step := 1; step <= len(layoutOrder); step++ {
		next := layoutOrder[(k+step)%len(layoutOrder)]
		if a.fits(next) {
			a.setSplit(next)
			return
		}
	}
}

// movePane: Strg+Pfeil wechselt in den nächsten Bereich in dieser Richtung
// (leere werden übersprungen, am Rand geht es auf der anderen Seite weiter).
func (a *App) movePane(dir string) {
	j := a.slotOf(a.current())
	rs := a.paneRects()
	if j < 0 || j >= len(rs) {
		return
	}
	cx, cy := rs[j].x+rs[j].w/2, rs[j].y+rs[j].h/2
	best, bestD := -1, 0
	wrap, wrapD := -1, 0
	for k, r := range rs {
		if k == j || k >= len(a.panes) || a.byID(a.panes[k]) == nil {
			continue
		}
		// nur Bereiche in derselben Zeile bzw. Spalte (überlappend)
		fr := rs[j]
		if dir == "left" || dir == "right" {
			if r.y >= fr.y+fr.h || fr.y >= r.y+r.h {
				continue
			}
		} else if r.x >= fr.x+fr.w || fr.x >= r.x+r.w {
			continue
		}
		kx, ky := r.x+r.w/2, r.y+r.h/2
		var along, across int
		switch dir {
		case "left":
			along, across = cx-kx, abs(ky-cy)
		case "right":
			along, across = kx-cx, abs(ky-cy)
		case "up":
			along, across = cy-ky, abs(kx-cx)
		case "down":
			along, across = ky-cy, abs(kx-cx)
		}
		d := abs(along) + 2*across
		if along > 0 {
			if best < 0 || d < bestD {
				best, bestD = k, d
			}
		} else if along < 0 {
			// am Rand weiter: der am weitesten entfernte auf der anderen Seite
			w := -abs(along)*4 + across
			if wrap < 0 || w < wrapD {
				wrap, wrapD = k, w
			}
		}
	}
	if best < 0 {
		best = wrap
	}
	if best >= 0 {
		a.focusSession(a.indexOf(a.byID(a.panes[best])))
	}
}

func paneDir(key string) (string, bool) {
	switch key {
	case "ctrl+left", "ctrl+right", "ctrl+up", "ctrl+down":
		return strings.TrimPrefix(key, "ctrl+"), true
	}
	return "", false
}

// paneAt: Sitzung im Bereich unter (x, y), sonst nil. Im Zoom ist nur der
// aktive Bereich zu sehen.
func (a *App) paneAt(x, y int) *session {
	if a.split == splitOff || a.zoom {
		return nil
	}
	if j := a.slotAt(x, y); j >= 0 {
		return a.byID(a.panes[j])
	}
	return nil
}

// slotAt: Bereich unter (x, y) oder -1.
func (a *App) slotAt(x, y int) int {
	if a.split == splitOff {
		return -1
	}
	for j, r := range a.paneRects() {
		if r.has(x, y) && j < len(a.panes) {
			return j
		}
	}
	return -1
}

// ---------------------------------------------------------------- Größe

// moveBoundary verschiebt die Grenze i (zwischen Spalte/Zeile i und i+1)
// um d Zellen. Kein Bereich wird kleiner als minPaneW × minPaneH.
func (a *App) moveBoundary(vertical bool, i, d int) bool {
	ar := a.areaRect()
	c, r := a.split.grid()
	n, total, sep, mn, fr := c, ar.w, 1, minPaneW, &a.fracX
	if !vertical {
		n, total, sep, mn, fr = r, ar.h, 0, minPaneH, &a.fracY
	}
	if i < 0 || i >= n-1 {
		return false
	}
	ls := fracLens(total, sep, *fr, n)
	d = clamp(d, -(ls[i] - mn), ls[i+1]-mn)
	if d == 0 {
		return false
	}
	ls[i] += d
	ls[i+1] -= d
	avail := float64(total - sep*(n-1))
	out := make([]float64, n)
	for k := range ls {
		out[k] = float64(ls[k]) / avail
	}
	*fr = out
	a.resizeAll()
	return true
}

// boundaryAt: Bildschirmposition der Grenze i (x der Trennlinie bzw. y der
// Titelzeile darunter).
func (a *App) boundaryAt(vertical bool, i int) int {
	ar := a.areaRect()
	ws, hs := a.lens()
	if vertical {
		x := ar.x
		for k := 0; k <= i && k < len(ws); k++ {
			x += ws[k]
		}
		return x + i
	}
	y := ar.y
	for k := 0; k <= i && k < len(hs); k++ {
		y += hs[k]
	}
	return y
}

// resizeFocused: Alt+⇧+Pfeil verschiebt die Linie neben dem aktiven
// Bereich in Pfeilrichtung (rechts/unten, am Rand die linke/obere).
func (a *App) resizeFocused(dir string) {
	if a.split == splitOff || a.zoom {
		return
	}
	j := a.slotOf(a.current())
	if j < 0 {
		return
	}
	c, r := a.split.grid()
	col, row := a.cellOf(j)
	vertical := dir == "left" || dir == "right"
	pos, n, step := col, c, 2
	if !vertical {
		pos, n, step = row, r, 1
		if a.split == splitMain && j == 0 {
			pos = 0
		}
	}
	if n < 2 {
		a.flash(i18n.T("In dieser Aufteilung gibt es keine Linie in diese Richtung"))
		return
	}
	i := pos
	if i >= n-1 {
		i = n - 2
	}
	if dir == "left" || dir == "up" {
		step = -step
	}
	a.moveBoundary(vertical, i, step)
}

// equalize: alle Bereiche wieder gleich groß.
func (a *App) equalize() {
	a.fracX, a.fracY = nil, nil
	a.resizeAll()
	a.flash(i18n.T("Alle Bereiche gleich groß"))
}

// toggleZoom: aktiven Bereich aufs ganze Fenster und zurück.
func (a *App) toggleZoom() {
	if a.split == splitOff {
		a.flash(i18n.T("Zoom gibt es nur in der geteilten Ansicht (Strg+T)"))
		return
	}
	a.zoomGlide(!a.zoom)
	a.zoom = !a.zoom
	a.resizeAll()
	if a.zoom {
		a.flash(i18n.T("Zoom: ") + a.keys.label(actZoom) + i18n.T(" holt die anderen Bereiche zurück"))
	}
}

// swapPanes tauscht den Inhalt zweier Bereiche.
func (a *App) swapPanes(i, j int) {
	if i == j || i < 0 || j < 0 || i >= len(a.panes) || j >= len(a.panes) {
		return
	}
	a.panes[i], a.panes[j] = a.panes[j], a.panes[i]
	a.resizeAll()
}

// ---------------------------------------------------------------- Maus

type splitDragKind int

const (
	dragSepV  splitDragKind = iota + 1 // senkrechte Trennlinie
	dragSepH                           // ↕-Griff in einer Titelzeile
	dragTitle                          // Titelzeile: Bereich tauschen
)

type splitDrag struct {
	kind  splitDragKind
	idx   int // Grenze bzw. Bereich, aus dem gezogen wird
	x, y  int // Startpunkt
	moved bool
}

// sepAt: senkrechte Trennlinie unter (x, y)?
func (a *App) sepAt(x, y int) (int, bool) {
	ar := a.areaRect()
	if !ar.has(x, y) || a.zoom {
		return 0, false
	}
	c, _ := a.split.grid()
	for i := 0; i < c-1; i++ {
		if x == a.boundaryAt(true, i) {
			return i, true
		}
	}
	return 0, false
}

// titleAt: Titelzeile von Bereich j unter (x, y)? grip: auf dem ↕-Griff.
func (a *App) titleAt(x, y int) (j int, grip bool) {
	if a.zoom {
		return -1, false
	}
	for k, r := range a.paneRects() {
		if y == r.y && x >= r.x && x < r.x+r.w && k < len(a.panes) {
			return k, a.hasAbove(k) && x >= r.x+r.w-gripW
		}
	}
	return -1, false
}

const gripW = 3 // „ ↕ “ am rechten Ende der Titelzeile

// splitMouse: Ziehen an Trennlinien und Titelzeilen der geteilten Ansicht.
func (a *App) splitMouse(msg tea.MouseMsg, clicks int) (tea.Cmd, bool) {
	if a.scr != scrTerminal || a.split == splitOff || a.modal != nil || a.pal != nil || a.pick != nil {
		a.sdrag = nil
		return nil, false
	}
	m := msg.Mouse()
	switch msg.(type) {
	case tea.MouseClickMsg:
		if m.Button != tea.MouseLeft {
			return nil, false
		}
		if a.zoom {
			if ar := a.areaRect(); m.Y == ar.y && clicks >= 2 {
				a.toggleZoom()
				return nil, true
			}
			return nil, false
		}
		if i, ok := a.sepAt(m.X, m.Y); ok {
			if clicks >= 2 {
				a.equalize()
				return nil, true
			}
			a.sdrag = &splitDrag{kind: dragSepV, idx: i, x: m.X, y: m.Y}
			return nil, true
		}
		j, grip := a.titleAt(m.X, m.Y)
		if j < 0 {
			return nil, false
		}
		if s := a.byID(a.panes[j]); s != nil {
			a.focusSession(a.indexOf(s))
		}
		if grip {
			_, row := a.cellOf(j)
			a.sdrag = &splitDrag{kind: dragSepH, idx: row - 1, x: m.X, y: m.Y}
			return nil, true
		}
		if clicks >= 2 && a.byID(a.panes[j]) != nil {
			a.toggleZoom()
			return nil, true
		}
		a.sdrag = &splitDrag{kind: dragTitle, idx: j, x: m.X, y: m.Y}
		return nil, true
	case tea.MouseMotionMsg:
		d := a.sdrag
		if d == nil {
			return nil, false
		}
		switch d.kind {
		case dragSepV:
			a.moveBoundary(true, d.idx, m.X-a.boundaryAt(true, d.idx))
		case dragSepH:
			a.moveBoundary(false, d.idx, m.Y-a.boundaryAt(false, d.idx))
		case dragTitle:
			if abs(m.X-d.x) > 1 || m.Y != d.y {
				d.moved = true
			}
			a.dropOn = 0
			if k := a.slotAt(m.X, m.Y); d.moved && k >= 0 && k != d.idx {
				a.dropOn = k + 1
			}
		}
		d.moved = true
		return nil, true
	case tea.MouseReleaseMsg:
		d := a.sdrag
		if d == nil {
			return nil, false
		}
		if d.kind == dragTitle && a.dropOn > 0 {
			a.swapPanes(d.idx, a.dropOn-1)
		}
		a.sdrag, a.dropOn = nil, 0
		return nil, true
	}
	return nil, false
}

// ---------------------------------------------------------------- Ansicht

func (a *App) viewSplit(focus *session) (string, *tea.Cursor) {
	st := a.s
	ar := a.areaRect()
	now := time.Now()
	if a.zoom {
		b, cc := a.paneBody(focus, true)
		out := a.paneTitle(focus, ar.w, true, false) + "\n" + fitBlock(b, ar.w, ar.h-1)
		if gl := a.glideLayers(ar, now); gl != nil {
			out = lipgloss.NewCompositor(append([]*lipgloss.Layer{lipgloss.NewLayer(out)}, gl...)...).Render()
		}
		return out, cc
	}
	dim := a.st != nil && a.st.Settings().DimPanes
	rects := a.paneRects()
	layers := []*lipgloss.Layer{lipgloss.NewLayer(fitBlock("", ar.w, ar.h))}
	var cur *tea.Cursor
	for j, rc := range rects {
		var ps *session
		if j < len(a.panes) {
			ps = a.byID(a.panes[j])
		}
		title := a.paneTitle(ps, rc.w, ps != nil && ps == focus, a.hasAbove(j))
		if a.dropOn == j+1 {
			title = lipgloss.NewStyle().Background(st.T.Warn).Foreground(st.T.Bg).Bold(true).Render(
				padRight(ansi.Truncate(i18n.T(" ↔ hier ablegen: tauscht mit ")+orDefault(paneName(ps), i18n.T("leerem Bereich")), rc.w, "…"), rc.w))
		}
		if d := a.sdrag; d != nil && d.kind == dragSepH && a.hasAbove(j) {
			if _, row := a.cellOf(j); row == d.idx+1 {
				_, hs := a.lens()
				p := hs[d.idx] * 100 / max(1, hs[d.idx]+hs[d.idx+1])
				lbl := " ▲ " + strconv.Itoa(p) + " % ─ " + strconv.Itoa(100-p) + " % ▾ "
				title = lipgloss.NewStyle().Background(st.T.Accent).Foreground(st.T.Bg).Bold(true).Render(
					padRight(strings.Repeat(" ", max(0, (rc.w-ansi.StringWidth(lbl))/2))+lbl, rc.w))
			}
		}
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
			} else if dim {
				body = fadeANSI(body, st.T, 0.45)
			}
		}
		layers = append(layers, lipgloss.NewLayer(title+"\n"+fitBlock(body, rc.w, rc.h-1)).X(rc.x-ar.x).Y(rc.y-ar.y))
	}
	// senkrechte Trennlinien (beim Ziehen farbig, mit Anteilen)
	fj := a.slotOf(focus)
	c, _ := a.split.grid()
	ws, _ := a.lens()
	for i := 0; i < c-1; i++ {
		x := a.boundaryAt(true, i) - ar.x
		col, ch := st.T.Border, "│"
		drag := a.sdrag != nil && a.sdrag.kind == dragSepV && a.sdrag.idx == i
		if drag {
			col, ch = st.T.Accent, "█"
		}
		line := lipgloss.NewStyle().Foreground(col).Render(strings.TrimSuffix(strings.Repeat(ch+"\n", ar.h), "\n"))
		if !drag && fj >= 0 && fj < len(rects) {
			// Linie direkt neben dem aktiven Bereich leuchtet
			fr := rects[fj]
			if bx := fr.x - ar.x; bx == x+1 || bx+fr.w == x {
				line = a.sepLine(ar.h, fr.y-ar.y, fr.y-ar.y+fr.h)
			}
		}
		layers = append(layers, lipgloss.NewLayer(line).X(x).Y(0).Z(1))
		if drag {
			p := ws[i] * 100 / max(1, ws[i]+ws[i+1])
			lbl := lipgloss.NewStyle().Background(st.T.Accent).Foreground(st.T.Bg).Bold(true).
				Render(" ◂ " + strconv.Itoa(p) + " % │ " + strconv.Itoa(100-p) + " % ▸ ")
			lx := clamp(x-lipgloss.Width(lbl)/2, 0, max(0, ar.w-lipgloss.Width(lbl)))
			layers = append(layers, lipgloss.NewLayer(lbl).X(lx).Y(ar.h/2).Z(2))
		}
	}
	layers = append(layers, a.glideLayers(ar, now)...)
	return lipgloss.NewCompositor(layers...).Render(), cur
}

func paneName(s *session) string {
	if s == nil {
		return ""
	}
	return s.tabName()
}

// paneTitle: eine Zeile über jedem Bereich (Klick wählt, Ziehen tauscht,
// Doppelklick zoomt; grip: ↕ rechts zum Ändern der Höhe).
func (a *App) paneTitle(s *session, w int, focused, grip bool) string {
	st := a.s
	bar := lipgloss.NewStyle().Background(st.T.Surface).Foreground(st.T.Muted)
	if focused {
		bar = lipgloss.NewStyle().Background(st.T.Accent).Foreground(st.T.Bg).Bold(true)
	}
	gw := 0
	if grip && w >= 30 {
		gw = gripW
	}
	if s == nil {
		line := bar.Render(padRight(ansi.Truncate(i18n.T(" ○ leer"), w-gw, ""), w-gw))
		if gw > 0 {
			line += bar.Render(" ↕ ")
		}
		return line
	}
	// Akzentfarbe der Verbindung: aktiver Bereich ganz in der Farbe,
	// die anderen mit Farbstreifen (accent.go)
	col := a.accentOf(s.conn)
	if col != nil && focused {
		bar = bar.Background(col)
	}
	dot := map[sessState]string{stConnecting: "◌", stConnected: "●", stClosed: "○", stReconnect: "◌"}[s.state]
	left := " " + dot + " " + strconv.Itoa(a.winNo(s)) + " · " + s.conn.Alias + "  " + s.conn.Username + "@" + s.conn.Host
	var marks []string
	if a.syncIn && s.state == stConnected && !s.detached() {
		marks = append(marks, i18n.T("◈ alle"))
	}
	if s.search.open {
		marks = append(marks, i18n.T("/ Suche"))
	}
	if s.goal.active {
		marks = append(marks, i18n.T("◎ Ziel"))
	}
	if a.vg.member(s.id) != nil {
		marks = append(marks, i18n.T("◆ Vorgang"))
	}
	if s.panel != panelNone {
		marks = append(marks, i18n.T("▐ Panel"))
	}
	if focused && a.zoom {
		n := 0
		for _, id := range a.panes {
			if id != 0 && id != s.id && a.byID(id) != nil {
				n++
			}
		}
		marks = append(marks, i18n.F("■ Zoom · %d weitere verdeckt · %s zurück", n, a.keys.label(actZoom)))
	}
	if focused && len(marks) == 0 && w >= 60 {
		marks = append(marks, i18n.T("Strg+Pfeil wechselt"))
	}
	right := strings.Join(marks, " · ")
	if right != "" {
		right += " "
	}
	inner := w - gw
	left = ansi.Truncate(left, max(0, inner-ansi.StringWidth(right)-1), "…")
	line := left + strings.Repeat(" ", max(1, inner-ansi.StringWidth(left)-ansi.StringWidth(right))) + right
	if col != nil && !focused {
		line = glyph("▌", col, st.T.Surface) + bar.Render(ansi.Truncate(strings.TrimPrefix(line, " "), inner-1, ""))
	} else {
		line = bar.Render(ansi.Truncate(line, inner, ""))
	}
	if gw > 0 {
		line += bar.Render(" ↕ ")
	}
	return line
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
