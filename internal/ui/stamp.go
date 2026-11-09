package ui

// stamp: ein kleines Feld (Toast, Tooltip, Leistungsanzeige) über das Bild
// legen, ohne den Compositor. Der zerlegt das ganze Bild in Zellen und setzt
// es neu zusammen, das kostete bei 200 × 55 Zellen mehrere Millisekunden je
// Bild. Hier werden nur die Zeilen angefasst, die das Feld überdeckt.

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const sgrReset = "\x1b[m"

// stamp legt box bei (x, y) über base; maxW begrenzt die Zeilenbreite.
func stamp(base, box string, x, y, maxW int) string {
	if box == "" {
		return base
	}
	lines := strings.Split(base, "\n")
	for i, bl := range strings.Split(box, "\n") {
		r := y + i
		if r < 0 || r >= len(lines) {
			continue
		}
		lines[r] = stampLine(lines[r], bl, max(0, x), maxW)
	}
	return strings.Join(lines, "\n")
}

// stampLine: bl ab Spalte x in die Zeile l schreiben.
func stampLine(l, bl string, x, maxW int) string {
	if maxW > 0 && x >= maxW {
		return l
	}
	bw := ansi.StringWidth(bl)
	if maxW > 0 && x+bw > maxW {
		bl = ansi.Truncate(bl, maxW-x, "")
		bw = ansi.StringWidth(bl)
	}
	lw := ansi.StringWidth(l)
	var b strings.Builder
	b.Grow(len(l) + len(bl) + 16)
	left := l
	if lw > x {
		left = ansi.Truncate(l, x, "") // ein breites Zeichen an der Kante fällt weg
	}
	b.WriteString(left)
	if pad := x - ansi.StringWidth(left); pad > 0 {
		if lw <= x {
			b.WriteString(sgrReset) // hinter dem Zeilenende: kein Hintergrund mitziehen
		}
		b.WriteString(strings.Repeat(" ", pad))
	}
	b.WriteString(sgrReset)
	b.WriteString(bl)
	b.WriteString(sgrReset)
	if end := x + bw; lw > end {
		// TruncateLeft behält alle Farbsequenzen, die Farbe stimmt also weiter
		right := ansi.TruncateLeft(l, end, "")
		want := lw - end
		rw := ansi.StringWidth(right)
		if rw > want { // breites Zeichen ragte unter das Feld
			right = ansi.TruncateLeft(l, end+1, "")
			rw = ansi.StringWidth(right)
		}
		b.WriteString(strings.Repeat(" ", max(0, want-rw)))
		b.WriteString(right)
	}
	return b.String()
}

// frameBlock: Inhalt auf genau w × h bringen (auffüllen, zu lange Zeilen
// umbrechen wie lipgloss, zu viele Zeilen abschneiden). Ist jede Zeile schon
// schmal genug – der Normalfall –, wird nur gemessen und aufgefüllt; ein
// lipgloss-Render mit Width/Height zerlegt dagegen jedes Zeichen.
func frameBlock(content string, w, h int) string {
	lines := strings.Split(content, "\n")
	widths := make([]int, len(lines))
	for i, l := range lines {
		widths[i] = ansi.StringWidth(l)
		if widths[i] > w {
			return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(content)
		}
	}
	var b strings.Builder
	b.Grow(len(content) + h*w/4)
	for i := 0; i < h; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i < len(lines) {
			b.WriteString(lines[i])
			if strings.IndexByte(lines[i], 0x1b) >= 0 {
				b.WriteString(sgrReset)
			}
			b.WriteString(strings.Repeat(" ", w-widths[i]))
		} else {
			b.WriteString(strings.Repeat(" ", w))
		}
	}
	return b.String()
}
