package ui

import (
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Klickbare Bereiche (Zonen)
//
// Beim Zeichnen wird anklickbarer Text mit zwei unsichtbaren Markern
// umschlossen (CSI <n> z, eine Sequenz, die kein Terminal kennt und die
// lipgloss als Breite 0 behandelt). Nach dem Zusammensetzen eines Bildes
// sucht scanZones die Marker, merkt sich die Rechtecke und entfernt die
// Marker wieder. So bleibt jede Ansicht frei in ihrem Layout, und die
// Mausbehandlung muss keine Positionen nachrechnen.

const zoneBase = 71000

type zoneAction struct {
	click func(m tea.Mouse, double bool) tea.Cmd // nil = nicht klickbar
	menu  func(m tea.Mouse) tea.Cmd              // Rechts- oder Mittelklick, nil = keiner
	wheel func(up bool) tea.Cmd                  // nil = Rad geht weiter nach außen
	block bool                                   // rechteckiger Block (Box), sonst Fließtext
	row   int                                    // Manager: Zeile+1 als Ziel beim Ziehen (0 = keins); Dateien: Eintrag+1
	pane  int                                    // Dateien: Seite+1 (0 = keine)
}

type zoneRect struct {
	x0, y0, x1, y1 int // inklusive
	id             int
	block          bool
}

func (r zoneRect) contains(x, y int) bool {
	if y < r.y0 || y > r.y1 {
		return false
	}
	if r.y0 == r.y1 || r.block {
		return x >= r.x0 && x <= r.x1
	}
	// mehrzeilig: wie Fließtext (erste Zeile ab x0, letzte bis x1)
	switch y {
	case r.y0:
		return x >= r.x0
	case r.y1:
		return x <= r.x1
	}
	return true
}

type zones struct {
	acts  []zoneAction
	rects []zoneRect
}

func (z *zones) reset() { z.acts, z.rects = z.acts[:0], z.rects[:0] }

func zoneMarker(id int) string { return "\x1b[" + strconv.Itoa(zoneBase+id) + "z" }

// mark umschließt s mit einer Zone.
func (z *zones) mark(s string, act zoneAction) string {
	if s == "" {
		return s
	}
	id := len(z.acts)
	z.acts = append(z.acts, act)
	m := zoneMarker(id)
	return m + s + m
}

func (z *zones) click(s string, f func(m tea.Mouse, double bool) tea.Cmd) string {
	return z.mark(s, zoneAction{click: f})
}

// scan entfernt die Marker aus s und merkt sich die Rechtecke, verschoben
// um ox/oy (für Overlays).
func (z *zones) scan(s string, ox, oy int) string {
	if !strings.Contains(s, "\x1b[") {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	open := map[int][2]int{}
	x, y := 0, 0
	state := byte(0)
	for len(s) > 0 {
		seq, width, n, newState := ansi.DecodeSequence(s, state, nil)
		state = newState
		if n <= 0 {
			break
		}
		if seq == "\n" {
			x, y = 0, y+1
			out.WriteString(seq)
			s = s[n:]
			continue
		}
		if width == 0 && len(seq) > 3 && seq[0] == 0x1b && seq[1] == '[' && seq[len(seq)-1] == 'z' {
			if v, err := strconv.Atoi(seq[2 : len(seq)-1]); err == nil && v >= zoneBase && v-zoneBase < len(z.acts) {
				id := v - zoneBase
				if st, ok := open[id]; ok {
					delete(open, id)
					if x > st[0] || y > st[1] {
						z.rects = append(z.rects, zoneRect{st[0] + ox, st[1] + oy, x - 1 + ox, y + oy, id, z.acts[id].block})
					}
				} else {
					open[id] = [2]int{x, y}
				}
				s = s[n:]
				continue
			}
		}
		out.WriteString(seq)
		x += width
		s = s[n:]
	}
	for id, st := range open { // Ende abgeschnitten: bis zum Bildende
		z.rects = append(z.rects, zoneRect{st[0] + ox, st[1] + oy, 1 << 20, y + oy, id, z.acts[id].block})
	}
	return out.String()
}

// at: die innerste (zuletzt geöffnete, also kleinste) Zone unter x/y, die
// die gewünschte Aktion hat.
func (z *zones) at(x, y int, wheel bool) *zoneAction {
	var best *zoneAction
	bestArea := math.MaxInt
	for _, r := range z.rects {
		if !r.contains(x, y) {
			continue
		}
		act := &z.acts[r.id]
		if (wheel && act.wheel == nil) || (!wheel && act.click == nil) {
			continue
		}
		area := (r.y1-r.y0+1)*4096 + (r.x1 - r.x0)
		if area <= bestArea {
			best, bestArea = act, area
		}
	}
	return best
}

// atMenu: die innerste Zone unter x/y mit Rechtsklick-Aktion (Tabs).
func (z *zones) atMenu(x, y int) *zoneAction {
	var best *zoneAction
	bestArea := math.MaxInt
	for _, r := range z.rects {
		if !r.contains(x, y) || z.acts[r.id].menu == nil {
			continue
		}
		if area := (r.y1-r.y0+1)*4096 + (r.x1 - r.x0); area <= bestArea {
			best, bestArea = &z.acts[r.id], area
		}
	}
	return best
}

// rowAt: Zeile (row-1) unter x/y für das Ziehen im Manager, sonst -1.
func (z *zones) rowAt(x, y int) int {
	for _, r := range z.rects {
		if y >= r.y0 && y <= r.y1 && z.acts[r.id].row > 0 && z.acts[r.id].pane == 0 { // nur die Zeile zählt
			_ = x
			return z.acts[r.id].row - 1
		}
	}
	return -1
}

// fileAt: Seite und Eintrag des Dateibrowsers unter x/y (je -1 = keiner).
func (z *zones) fileAt(x, y int) (side, row int) {
	side, row = -1, -1
	bestArea := math.MaxInt
	for _, r := range z.rects {
		act := z.acts[r.id]
		if act.pane == 0 || !r.contains(x, y) {
			continue
		}
		area := (r.y1-r.y0+1)*4096 + (r.x1 - r.x0)
		if area <= bestArea {
			bestArea, side, row = area, act.pane-1, act.row-1
		}
	}
	return side, row
}
