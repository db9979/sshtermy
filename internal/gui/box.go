package gui

import (
	"image"
	"image/color"
	"math"
)

// Rahmen- und Blockzeichen zeichnet das Fenster selbst statt aus der Schrift:
// So stoßen Linien lückenlos aneinander (Rahmen von Panels, Tabellen) und die
// Halbblöcke der Bildvorschau füllen die Zelle genau, egal welche Schriftgröße.

// Gewicht je Richtung: oben, rechts, unten, links. 1 dünn, 2 dick, 3 doppelt.
var boxLines = map[rune][4]uint8{}

func init() {
	add := func(chars string, w uint8, dirs [][4]uint8) {
		i := 0
		for _, r := range chars {
			d := dirs[i]
			for k := range d {
				d[k] *= w
			}
			boxLines[r] = d
			i++
		}
	}
	shapes := [][4]uint8{
		{0, 1, 0, 1}, {1, 0, 1, 0}, // ─ │
		{0, 1, 1, 0}, {0, 0, 1, 1}, {1, 1, 0, 0}, {1, 0, 0, 1}, // ┌ ┐ └ ┘
		{1, 1, 1, 0}, {1, 0, 1, 1}, {0, 1, 1, 1}, {1, 1, 0, 1}, {1, 1, 1, 1}, // ├ ┤ ┬ ┴ ┼
	}
	add("─│┌┐└┘├┤┬┴┼", 1, shapes)
	add("━┃┏┓┗┛┣┫┳┻╋", 2, shapes)
	add("═║╔╗╚╝╠╣╦╩╬", 3, shapes)
	halves := [][4]uint8{{0, 0, 0, 1}, {1, 0, 0, 0}, {0, 1, 0, 0}, {0, 0, 1, 0}}
	add("╴╵╶╷", 1, halves)
	add("╸╹╺╻", 2, halves)
	// gestrichelte Linien: durchgezogen ist in einer Zelle gut lesbar
	add("┄┈╌", 1, [][4]uint8{{0, 1, 0, 1}, {0, 1, 0, 1}, {0, 1, 0, 1}})
	add("┆┊╎", 1, [][4]uint8{{1, 0, 1, 0}, {1, 0, 1, 0}, {1, 0, 1, 0}})
	add("┅┉╍", 2, [][4]uint8{{0, 1, 0, 1}, {0, 1, 0, 1}, {0, 1, 0, 1}})
	add("┇┋╏", 2, [][4]uint8{{1, 0, 1, 0}, {1, 0, 1, 0}, {1, 0, 1, 0}})
}

// boxGlyph liefert die Maske für ein Rahmen-/Blockzeichen oder nil, wenn die
// Schrift es zeichnen soll.
func boxGlyph(r rune, w, h int) *image.Alpha {
	if w <= 0 || h <= 0 {
		return nil
	}
	img := image.NewAlpha(image.Rect(0, 0, w, h))
	fill := func(x0, y0, x1, y1 int, a uint8) {
		for y := max(y0, 0); y < min(y1, h); y++ {
			for x := max(x0, 0); x < min(x1, w); x++ {
				img.SetAlpha(x, y, color.Alpha{a})
			}
		}
	}
	// Blöcke U+2580–U+259F
	if r >= 0x2580 && r <= 0x259F {
		hw, hh := w/2, h/2
		eighthH := func(n int) int { return int(math.Round(float64(h) * float64(n) / 8)) }
		eighthW := func(n int) int { return int(math.Round(float64(w) * float64(n) / 8)) }
		quad := func(ul, ur, ll, lr bool) {
			if ul {
				fill(0, 0, hw, hh, 255)
			}
			if ur {
				fill(hw, 0, w, hh, 255)
			}
			if ll {
				fill(0, hh, hw, h, 255)
			}
			if lr {
				fill(hw, hh, w, h, 255)
			}
		}
		switch {
		case r == 0x2580:
			fill(0, 0, w, hh, 255)
		case r >= 0x2581 && r <= 0x2588: // ▁…█
			fill(0, h-eighthH(int(r-0x2580)), w, h, 255)
		case r >= 0x2589 && r <= 0x258F: // ▉…▏
			fill(0, 0, eighthW(int(0x2590-r)), h, 255)
		case r == 0x2590:
			fill(hw, 0, w, h, 255)
		case r >= 0x2591 && r <= 0x2593: // ░▒▓
			fill(0, 0, w, h, uint8(64*(r-0x2590)))
		case r == 0x2594:
			fill(0, 0, w, eighthH(1), 255)
		case r == 0x2595:
			fill(w-eighthW(1), 0, w, h, 255)
		case r == 0x2596:
			quad(false, false, true, false)
		case r == 0x2597:
			quad(false, false, false, true)
		case r == 0x2598:
			quad(true, false, false, false)
		case r == 0x2599:
			quad(true, false, true, true)
		case r == 0x259A:
			quad(true, false, false, true)
		case r == 0x259B:
			quad(true, true, true, false)
		case r == 0x259C:
			quad(true, true, false, true)
		case r == 0x259D:
			quad(false, true, false, false)
		case r == 0x259E:
			quad(false, true, true, false)
		case r == 0x259F:
			quad(false, true, true, true)
		}
		return img
	}

	t := max(1, int(math.Round(float64(w)/9)))
	cx, cy := w/2, h/2
	// abgerundete Ecken ╭╮╯╰ (lipgloss RoundedBorder)
	if r >= 0x256D && r <= 0x2570 {
		roundCorner(img, r, w, h, t)
		return img
	}
	d, ok := boxLines[r]
	if !ok {
		return nil
	}
	seg := func(dir int, thick, off int) {
		lo, hi := -thick/2+off, thick-thick/2+off
		ext := thick - thick/2 // bis über die Mitte, damit Ecken geschlossen sind
		switch dir {
		case 0:
			fill(cx+lo, 0, cx+hi, cy+ext, 255)
		case 1:
			fill(cx-ext+1, cy+lo, w, cy+hi, 255)
		case 2:
			fill(cx+lo, cy-ext+1, cx+hi, h, 255)
		case 3:
			fill(0, cy+lo, cx+ext, cy+hi, 255)
		}
	}
	for dir, wt := range d {
		switch wt {
		case 1:
			seg(dir, t, 0)
		case 2:
			seg(dir, 2*t, 0)
		case 3:
			seg(dir, t, -t)
			seg(dir, t, t)
		}
	}
	return img
}

// roundCorner zeichnet einen Viertelkreis mit Kantenglättung und die geraden
// Stücke bis zum Zellrand.
func roundCorner(img *image.Alpha, r rune, w, h, t int) {
	cx, cy := float64(w)/2, float64(h)/2
	rad := math.Min(cx, cy)
	// Mittelpunkt des Kreises und Richtung der beiden Arme
	var mx, my float64
	var right, down bool
	switch r {
	case '╭':
		mx, my, right, down = cx+rad, cy+rad, true, true
	case '╮':
		mx, my, right, down = cx-rad, cy+rad, false, true
	case '╯':
		mx, my, right, down = cx-rad, cy-rad, false, false
	case '╰':
		mx, my, right, down = cx+rad, cy-rad, true, false
	}
	half := float64(t) / 2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			var dist float64
			inX := (right && px >= mx) || (!right && px <= mx)
			inY := (down && py >= my) || (!down && py <= my)
			switch {
			case inX && !inY: // waagerechter Arm
				dist = math.Abs(py - cy)
			case inY && !inX: // senkrechter Arm
				dist = math.Abs(px - cx)
			case !inX && !inY: // Bogen
				dist = math.Abs(math.Hypot(px-mx, py-my) - rad)
			default:
				continue
			}
			a := half + 0.5 - dist
			if a <= 0 {
				continue
			}
			if a > 1 {
				a = 1
			}
			img.SetAlpha(x, y, color.Alpha{uint8(a * 255)})
		}
	}
}
