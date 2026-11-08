package ui

// Broadcast als Kacheln: jeder Host in einem eigenen Rahmen, der Zustand
// steht oben im Rahmen und färbt ihn (läuft = pulsiert gelb, fertig = grün,
// Fehler = rot). Ohne Zoom zeigt jede Kachel das Ende der Ausgabe, mit Zoom
// (z, Doppelklick) eine Kachel groß zum Blättern.

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/broadcast"
	"github.com/db9979/sshtermy/internal/i18n"
)

const (
	bcTileMinW = 40
	bcTileMinH = 8
)

// bcGrid: Spalten und Zeilen für n Kacheln auf w × h.
func bcGrid(n, w, h int, zoom bool) (cols, rows int) {
	if zoom || n <= 1 {
		return 1, 1
	}
	maxC := max(1, (w+1)/(bcTileMinW+1))
	maxR := max(1, h/bcTileMinH)
	if n <= min(3, maxC) {
		return n, 1 // zwei oder drei nebeneinander
	}
	cols = min(maxC, int(math.Ceil(math.Sqrt(float64(n)))))
	rows = min(maxR, (n+cols-1)/cols)
	return cols, max(1, rows)
}

func intersperse(xs []string, sep string) []string {
	if len(xs) < 2 {
		return xs
	}
	out := make([]string, 0, 2*len(xs)-1)
	for i, x := range xs {
		if i > 0 {
			out = append(out, sep)
		}
		out = append(out, x)
	}
	return out
}

func bcSummaryText(ok, failed, other int) string {
	parts := []string{fmt.Sprintf(i18n.T("%d ok"), ok)}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T("%d Fehler"), failed))
	}
	if other > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T("%d abgebrochen"), other))
	}
	return strings.Join(parts, " · ")
}

// bcTile: eine Kachel; zweiter Wert = Zeilen der Ausgabe (fürs Blättern).
func (a *App) bcTile(i, w, h, bodyH int) (string, int) {
	b, s := a.bc, a.s
	r := b.results[i]
	nsteps := len(b.steps)
	if i < len(b.per) && len(b.per[i]) > 0 {
		nsteps = len(b.per[i])
	}
	icon, txt, st := a.bcStatus(r, nsteps)
	now := time.Now()
	var border color.Color = s.T.Border
	switch {
	case r.State == broadcast.Running || r.State == broadcast.Connecting:
		border = mixColor(s.T.Border, s.T.Warn, 0.35+0.45*pulse(now, 1200*time.Millisecond))
		if motionLevel() > 0 {
			icon = spinGlyph(now)
		}
	case r.State == broadcast.Done && r.Exit == 0:
		border = s.T.OK
	case r.Finished() && r.State != broadcast.Canceled:
		border = s.T.Danger
	}
	if i == b.sel {
		border = mixColor(border, s.T.Text, 0.35)
	}
	inner := w - 2
	out := strings.ReplaceAll(strings.ReplaceAll(r.Output, "\t", "    "), "\r", "")
	if r.State == broadcast.Failed && r.Err != "" {
		out = strings.TrimRight(out, "\n")
		if out != "" {
			out += "\n\n"
		}
		out += i18n.T("Fehler: ") + r.Err
	} else if out == "" && r.Finished() {
		out = i18n.T("(keine Ausgabe)")
	}
	wl := strings.Split(strings.TrimRight(ansi.Hardwrap(ansi.Strip(out), inner-1, true), "\n"), "\n")
	from := max(0, len(wl)-bodyH) // ohne Zoom: das Ende zeigen
	if b.zoom {
		b.scroll = clamp(b.scroll, 0, max(0, len(wl)-bodyH))
		from = b.scroll
	}
	var body []string
	for j := from; j < len(wl) && j < from+bodyH; j++ {
		line := wl[j]
		ls := s.Text
		if strings.HasPrefix(line, i18n.T("Fehler: ")) || strings.HasPrefix(line, "E: ") {
			ls = s.Danger
		}
		body = append(body, " "+ls.Render(line))
	}
	for len(body) < bodyH {
		body = append(body, "")
	}
	bs := lipgloss.NewStyle().Foreground(border)
	tc, bc := frameCorners()
	name := " " + trunc(r.Alias, max(4, inner/2)) + " "
	stat := " " + st.Render(icon+" "+trunc(txt, max(4, inner-lipgloss.Width(name)-6))) + " "
	fill := max(1, inner-lipgloss.Width(name)-lipgloss.Width(stat)-1)
	top := bs.Render(string(tc[0])+"─") + s.Text.Bold(true).Render(name) + bs.Render(strings.Repeat("─", fill)) + stat + bs.Render(string(tc[1]))
	if lipgloss.Width(top) > w {
		top = ansi.Truncate(top, w-1, "") + bs.Render(string(tc[1]))
	}
	var tb strings.Builder
	tb.WriteString(padRight(top, w) + "\n")
	side := bs.Render("│")
	for _, l := range body {
		l = ansi.Truncate(l, inner, "")
		tb.WriteString(side + l + strings.Repeat(" ", max(0, inner-ansi.StringWidth(l))) + side + "\n")
	}
	tb.WriteString(bs.Render(string(bc[0]) + strings.Repeat("─", inner) + string(bc[1])))
	i2 := i
	tile := a.z.mark(tb.String(), zoneAction{
		block: true,
		click: func(_ tea.Mouse, double bool) tea.Cmd {
			b.sel = i2
			if double {
				b.zoom = !b.zoom
			}
			return nil
		},
		wheel: func(up bool) tea.Cmd {
			if up {
				b.scroll = max(0, b.scroll-3)
			} else {
				b.scroll += 3
			}
			return nil
		},
	})
	return tile, len(wl)
}
