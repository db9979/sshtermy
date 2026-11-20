// Package clickzone: klickbare Bereiche im fertigen Bild (Maus), aus
// internal/ui ausgelagert (erster Schritt der Aufteilung).
package clickzone

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
// sucht Map.Scan die Marker, merkt sich die Rechtecke und entfernt die
// Marker wieder. So bleibt jede Ansicht frei in ihrem Layout, und die
// Mausbehandlung muss keine Positionen nachrechnen.

const Base = 71000

type Action struct {
	Click func(m tea.Mouse, double bool) tea.Cmd // nil = nicht klickbar
	Menu  func(m tea.Mouse) tea.Cmd              // Rechts- oder Mittelklick, nil = keiner
	Wheel func(up bool) tea.Cmd                  // nil = Rad geht weiter nach außen
	Block bool                                   // rechteckiger Block (Box), sonst Fließtext
	Row   int                                    // Manager: Zeile+1 als Ziel beim Ziehen (0 = keins); Dateien: Eintrag+1
	Pane  int                                    // Dateien: Seite+1 (0 = keine)
	Tab   int                                    // Tab-Leiste: Tab+1 als Ziel beim Ziehen (0 = keins)
	Tip   func() string                          // Karte beim Verweilen der Maus (motion.go)
}

type Rect struct {
	X0, Y0, X1, Y1 int // inklusive
	id             int
	block          bool
}

func (r Rect) Contains(x, y int) bool {
	if y < r.Y0 || y > r.Y1 {
		return false
	}
	if r.Y0 == r.Y1 || r.block {
		return x >= r.X0 && x <= r.X1
	}
	// mehrzeilig: wie Fließtext (erste Zeile ab x0, letzte bis x1)
	switch y {
	case r.Y0:
		return x >= r.X0
	case r.Y1:
		return x <= r.X1
	}
	return true
}

type Map struct {
	acts  []Action
	rects []Rect
}

func (z *Map) Reset() { z.acts, z.rects = z.acts[:0], z.rects[:0] }

// ClearRects vergisst nur die Rechtecke (Overlay wird neu vermessen).
func (z *Map) ClearRects() { z.rects = z.rects[:0] }

func Marker(id int) string { return "\x1b[" + strconv.Itoa(Base+id) + "z" }

// Mark umschließt s mit einer Zone.
func (z *Map) Mark(s string, act Action) string {
	if s == "" {
		return s
	}
	id := len(z.acts)
	z.acts = append(z.acts, act)
	m := Marker(id)
	return m + s + m
}

func (z *Map) Click(s string, f func(m tea.Mouse, double bool) tea.Cmd) string {
	return z.Mark(s, Action{Click: f})
}

// Scan entfernt die Marker aus s und merkt sich die Rechtecke, verschoben
// um ox/oy (für Overlays).
func (z *Map) Scan(s string, ox, oy int) string {
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
			if v, err := strconv.Atoi(seq[2 : len(seq)-1]); err == nil && v >= Base && v-Base < len(z.acts) {
				id := v - Base
				if st, ok := open[id]; ok {
					delete(open, id)
					if x > st[0] || y > st[1] {
						z.rects = append(z.rects, Rect{st[0] + ox, st[1] + oy, x - 1 + ox, y + oy, id, z.acts[id].Block})
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
		z.rects = append(z.rects, Rect{st[0] + ox, st[1] + oy, 1 << 20, y + oy, id, z.acts[id].Block})
	}
	return out.String()
}

// At: die innerste (zuletzt geöffnete, also kleinste) Zone unter x/y, die
// die gewünschte Aktion hat.
func (z *Map) At(x, y int, wheel bool) *Action {
	var best *Action
	bestArea := math.MaxInt
	for _, r := range z.rects {
		if !r.Contains(x, y) {
			continue
		}
		act := &z.acts[r.id]
		if (wheel && act.Wheel == nil) || (!wheel && act.Click == nil) {
			continue
		}
		area := (r.Y1-r.Y0+1)*4096 + (r.X1 - r.X0)
		if area <= bestArea {
			best, bestArea = act, area
		}
	}
	return best
}

// TipAt: die innerste Zone unter x/y mit Hover-Karte.
func (z *Map) TipAt(x, y int) (*Action, Rect) {
	var best *Action
	var br Rect
	bestArea := math.MaxInt
	for _, r := range z.rects {
		if !r.Contains(x, y) || z.acts[r.id].Tip == nil {
			continue
		}
		if area := (r.Y1-r.Y0+1)*4096 + (r.X1 - r.X0); area <= bestArea {
			best, br, bestArea = &z.acts[r.id], r, area
		}
	}
	return best, br
}

// AtMenu: die innerste Zone unter x/y mit Rechtsklick-Aktion (Tabs).
func (z *Map) AtMenu(x, y int) *Action {
	var best *Action
	bestArea := math.MaxInt
	for _, r := range z.rects {
		if !r.Contains(x, y) || z.acts[r.id].Menu == nil {
			continue
		}
		if area := (r.Y1-r.Y0+1)*4096 + (r.X1 - r.X0); area <= bestArea {
			best, bestArea = &z.acts[r.id], area
		}
	}
	return best
}

// RowAt: Zeile (row-1) unter x/y für das Ziehen im Manager, sonst -1.
func (z *Map) RowAt(x, y int) int {
	for _, r := range z.rects {
		if y >= r.Y0 && y <= r.Y1 && z.acts[r.id].Row > 0 && z.acts[r.id].Pane == 0 { // nur die Zeile zählt
			_ = x
			return z.acts[r.id].Row - 1
		}
	}
	return -1
}

// TabAt: Tab (tab-1) unter x/y für das Ziehen in der Tab-Leiste, sonst -1.
func (z *Map) TabAt(x, y int) int {
	for _, r := range z.rects {
		if z.acts[r.id].Tab > 0 && r.Contains(x, y) {
			return z.acts[r.id].Tab - 1
		}
	}
	return -1
}

// FileAt: Seite und Eintrag des Dateibrowsers unter x/y (je -1 = keiner).
func (z *Map) FileAt(x, y int) (side, row int) {
	side, row = -1, -1
	bestArea := math.MaxInt
	for _, r := range z.rects {
		act := z.acts[r.id]
		if act.Pane == 0 || !r.Contains(x, y) {
			continue
		}
		area := (r.Y1-r.Y0+1)*4096 + (r.X1 - r.X0)
		if area <= bestArea {
			bestArea, side, row = area, act.Pane-1, act.Row-1
		}
	}
	return side, row
}
