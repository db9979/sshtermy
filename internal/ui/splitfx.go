package ui

// Geteilte Ansicht in Bewegung: Die Trennlinien neben dem aktiven Bereich
// leuchten in der Akzentfarbe. Wechselt der Fokus, gleitet ein Rahmen vom
// alten zum neuen Bereich; beim Zoom (Alt+Z) wächst er vom Bereich aufs
// ganze Fenster und schrumpft beim Zurückgehen wieder. Der Rahmen liegt nur
// gut eine Fünftelsekunde über dem Inhalt, Tasten gehen sofort durch.

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

const glideDur = 220 * time.Millisecond

type splitFx struct {
	from, to rect
	at       time.Time
	slot     int // zuletzt aktiver Bereich (-1 = keiner)
	seen     bool
}

// glideTo startet den Rahmen von from nach to.
func (a *App) glideTo(from, to rect) {
	if motionLevel() == 0 || from == to || from.w == 0 || to.w == 0 {
		return
	}
	a.sfx.from, a.sfx.to, a.sfx.at = from, to, time.Now()
}

// gliding: läuft der Rahmen noch?
func (a *App) gliding(now time.Time) bool {
	return !a.sfx.at.IsZero() && now.Sub(a.sfx.at) < glideDur && motionLevel() > 0
}

// trackFocusGlide: Fokuswechsel zwischen Bereichen erkennen.
func (a *App) trackFocusGlide(focus *session) {
	j := a.slotOf(focus)
	rs := a.paneRects()
	if a.sfx.seen && j >= 0 && a.sfx.slot >= 0 && j != a.sfx.slot && j < len(rs) && a.sfx.slot < len(rs) && !a.zoom {
		a.glideTo(rs[a.sfx.slot], rs[j])
	}
	a.sfx.slot, a.sfx.seen = j, true
}

// zoomGlide: Rahmen für Alt+Z (vor dem Umschalten aufrufen).
func (a *App) zoomGlide(zoomIn bool) {
	j := a.slotOf(a.current())
	rs := a.paneRects()
	if j < 0 || j >= len(rs) {
		return
	}
	full := a.areaRect()
	if zoomIn {
		a.glideTo(rs[j], full)
	} else {
		a.glideTo(full, rs[j])
	}
}

// glideLayers: der wandernde Rahmen als Ebenen (Koordinaten relativ zur
// Fläche ar).
func (a *App) glideLayers(ar rect, now time.Time) []*lipgloss.Layer {
	if !a.gliding(now) {
		return nil
	}
	k := easeOut(float64(now.Sub(a.sfx.at)) / float64(glideDur))
	lerp := func(x, y int) int { return x + int(float64(y-x)*k+0.5) }
	f, t := a.sfx.from, a.sfx.to
	r := rect{lerp(f.x, t.x) - ar.x, lerp(f.y, t.y) - ar.y, lerp(f.w, t.w), lerp(f.h, t.h)}
	r.w, r.h = max(2, min(r.w, ar.w-r.x)), max(2, min(r.h, ar.h-r.y))
	if r.x < 0 || r.y < 0 {
		return nil
	}
	tc, bc := frameCorners()
	st := lipgloss.NewStyle().Foreground(mixColor(a.s.T.Accent, a.s.T.AI, k))
	hz := strings.Repeat("─", max(0, r.w-2))
	top := st.Render(string(tc[0]) + hz + string(tc[1]))
	bot := st.Render(string(bc[0]) + hz + string(bc[1]))
	side := st.Render(strings.TrimSuffix(strings.Repeat("│\n", max(0, r.h-2)), "\n"))
	ls := []*lipgloss.Layer{
		lipgloss.NewLayer(top).X(r.x).Y(r.y).Z(5),
		lipgloss.NewLayer(bot).X(r.x).Y(r.y + r.h - 1).Z(5),
	}
	if r.h > 2 {
		ls = append(ls, lipgloss.NewLayer(side).X(r.x).Y(r.y+1).Z(5), lipgloss.NewLayer(side).X(r.x+r.w-1).Y(r.y+1).Z(5))
	}
	return ls
}

// sepLine: senkrechte Trennlinie; neben dem aktiven Bereich (Zeilen y0 …
// y1-1) leuchtet sie.
func (a *App) sepLine(h, y0, y1 int) string {
	st := a.s
	dim := lipgloss.NewStyle().Foreground(st.T.Border)
	glow := lipgloss.NewStyle().Foreground(st.T.Accent)
	var b strings.Builder
	for y := 0; y < h; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		if y >= y0 && y < y1 {
			b.WriteString(glow.Render("┃"))
		} else {
			b.WriteString(dim.Render("│"))
		}
	}
	return b.String()
}
