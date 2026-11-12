package termemu

import (
	"image/color"

	uv "github.com/charmbracelet/ultraviolet"

	"github.com/db9979/sshtermy/internal/highlight"
)

// HL: Farbregeln beim Zeichnen. Set liefert die Bereiche pro Zeile,
// Colors die Farbe des Themes je Bedeutung (Index = highlight.Color).
type HL struct {
	Set    *highlight.Set
	Colors [8]color.Color
}

// columnsOf: wie columns, für eine schon geholte Zeile.
func (hl *HL) columnsOf(line uv.Line) []color.Color {
	if hl == nil || hl.Set == nil {
		return nil
	}
	w := len(line)
	text, cols := cellsText(line)
	spans := hl.Set.Spans(text)
	if len(spans) == 0 {
		return nil
	}
	out := make([]color.Color, w)
	for _, sp := range spans {
		c := hl.Colors[sp.Color]
		if c == nil || sp.Start >= len(cols) {
			continue
		}
		from := cols[sp.Start]
		to := cols[min(sp.End, len(cols))-1]
		for x := from; x <= to && x < w; x++ {
			out[x] = c
		}
		// breites letztes Zeichen: auch die zweite Zelle
		if to+1 < w && line[to].Width > 1 {
			out[to+1] = c
		}
	}
	return out
}
