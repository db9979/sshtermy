package vt

import (
	"math/rand"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// sshTermy: rotateLines muss genau dasselbe Ergebnis liefern wie das
// zellweise Umkopieren in Buffer.DeleteLineArea/InsertLineArea.
func TestRotateLinesMatchesCellCopy(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for round := 0; round < 500; round++ {
		w, h := 3+rng.Intn(10), 3+rng.Intn(10)
		fast, slow := NewScreen(w, h), NewScreen(w, h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := uv.Cell{Content: string(rune('a' + rng.Intn(26))), Width: 1}
				fast.buf.SetCell(x, y, &c)
				slow.buf.SetCell(x, y, &c)
			}
		}
		top := rng.Intn(h)
		bottom := top + 1 + rng.Intn(h-top)
		area := uv.Rect(0, top, w, bottom-top)
		fast.scroll, slow.scroll = area, area
		if rng.Intn(2) == 0 {
			bg := ansi.IndexedColor(rng.Intn(16))
			fast.cur.Pen.Bg, slow.cur.Pen.Bg = bg, bg
		}
		y := top + rng.Intn(bottom-top)
		n := 1 + rng.Intn(h+2)
		if rng.Intn(2) == 0 {
			if !fast.rotateLines(y, bottom, n) {
				t.Fatal("voller Bereich: schneller Weg erwartet")
			}
			slow.buf.DeleteLineArea(y, n, slow.blankCell(), area)
		} else {
			fast.rotateLines(y, bottom, -n)
			slow.buf.InsertLineArea(y, n, slow.blankCell(), area)
		}
		for yy := 0; yy < h; yy++ {
			for x := 0; x < w; x++ {
				a, b := fast.buf.CellAt(x, yy), slow.buf.CellAt(x, yy)
				if !a.Equal(b) || a.Style.Bg != b.Style.Bg {
					t.Fatalf("Runde %d: Zelle %d,%d: %+v statt %+v", round, x, yy, a, b)
				}
			}
		}
	}
}
