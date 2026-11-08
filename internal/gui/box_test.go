package gui

import "testing"

// Linien müssen an den Zellrändern ankommen, sonst entstehen Lücken in Rahmen.
func TestBoxGlyphEdges(t *testing.T) {
	const w, h = 9, 19
	cases := map[rune][4]bool{ // oben, rechts, unten, links erreicht
		'─': {false, true, false, true},
		'│': {true, false, true, false},
		'┌': {false, true, true, false},
		'┼': {true, true, true, true},
		'╭': {false, true, true, false},
		'╯': {true, false, false, true},
		'═': {false, true, false, true},
	}
	for r, want := range cases {
		img := boxGlyph(r, w, h)
		if img == nil {
			t.Fatalf("%c: kein Bild", r)
		}
		any := func(x0, y0, x1, y1 int) bool {
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					if img.AlphaAt(x, y).A > 100 {
						return true
					}
				}
			}
			return false
		}
		got := [4]bool{any(0, 0, w, 1), any(w-1, 0, w, h), any(0, h-1, w, h), any(0, 0, 1, h)}
		if got != want {
			t.Errorf("%c: Ränder %v, erwartet %v", r, got, want)
		}
	}
	if boxGlyph('a', w, h) != nil {
		t.Error("normale Buchstaben gehören der Schrift")
	}
	if img := boxGlyph('█', w, h); img.AlphaAt(0, 0).A != 255 || img.AlphaAt(w-1, h-1).A != 255 {
		t.Error("█ muss die Zelle ganz füllen")
	}
}
