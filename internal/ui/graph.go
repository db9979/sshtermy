package ui

// Graphen aus Braille-Punkten: jede Zelle trägt 2 × 4 Punkte, also zwei
// Messwerte nebeneinander und vier Höhenstufen je Zeile. Das ist doppelt so
// fein in der Breite und viermal so fein in der Höhe wie Blockbalken und
// steht in jeder Schrift mit Braille (Cascadia hat alle 256 Zeichen).

import (
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// brailleBits: Punkt (Spalte, Zeile) → Bit im Braille-Zeichen.
var brailleBits = [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}

// brailleGraph: Werte 0 … 1 als Fläche, w Spalten breit, rows Zeilen hoch.
// Die neuesten Werte stehen rechts; fehlt etwas, bleibt links Platz.
func brailleGraph(vals []float64, w, rows int) []string {
	if w <= 0 || rows <= 0 {
		return nil
	}
	n := 2 * w
	if len(vals) > n {
		vals = vals[len(vals)-n:]
	}
	off := n - len(vals)
	dots := rows * 4
	cells := make([][]rune, rows)
	for r := range cells {
		cells[r] = make([]rune, w)
	}
	for i, v := range vals {
		if math.IsNaN(v) || v < 0 {
			continue
		}
		v = math.Min(1, v)
		h := int(math.Round(v * float64(dots)))
		if h == 0 && v > 0 {
			h = 1 // kleine Werte nicht verschlucken
		}
		x := off + i
		for d := 0; d < h; d++ {
			y := dots - 1 - d // von unten
			cells[y/4][x/2] |= brailleBits[y%4][x%2]
		}
	}
	out := make([]string, rows)
	for r, row := range cells {
		var b strings.Builder
		for _, c := range row {
			b.WriteRune(0x2800 + c)
		}
		out[r] = b.String()
	}
	return out
}

// graphBlock: Graph mit Farbverlauf von unten (lo) nach oben (hi).
func graphBlock(vals []float64, w, rows int, lo, hi color.Color) string {
	lines := brailleGraph(vals, w, rows)
	for i, l := range lines {
		k := 1.0
		if rows > 1 {
			k = float64(rows-1-i) / float64(rows-1)
		}
		lines[i] = lipgloss.NewStyle().Foreground(mixColor(lo, hi, k)).Render(l)
	}
	return strings.Join(lines, "\n")
}

// scaleTo: Werte auf 0 … 1 bezogen auf das Maximum (mindestens floor).
func scaleTo(vals []float64, floor float64) []float64 {
	hi := floor
	for _, v := range vals {
		hi = math.Max(hi, v)
	}
	out := make([]float64, len(vals))
	for i, v := range vals {
		out[i] = v / hi
	}
	return out
}
