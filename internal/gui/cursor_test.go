//go:build windows || darwin || linux

package gui

import (
	"image/color"
	"testing"
)

// Der Cursor darf auf hellem Grund nicht verschwinden: ohne gemeldete Farbe
// gilt die Schrift, eine zu helle Meldung wird ersetzt.
func TestCursorColorVisible(t *testing.T) {
	light := color.RGBA{0xf5, 0xf5, 0xf7, 0xff}
	text := color.RGBA{0x1f, 0x23, 0x35, 0xff}
	accent := color.RGBA{0x2e, 0x59, 0xc9, 0xff}
	if got := cursorColor(nil, text, light); got != text {
		t.Errorf("ohne OSC 12: %v", got)
	}
	if got := cursorColor(accent, text, light); got != accent {
		t.Errorf("Akzent: %v", got)
	}
	if got := cursorColor(color.RGBA{0xff, 0xff, 0xff, 0xff}, text, light); got != text {
		t.Errorf("Weiß auf Hell: %v", got)
	}
	if got := cursorColor(nil, defaultFg, defaultBg); lumaDiff(got, defaultBg) < 0.25 {
		t.Errorf("Dunkel: %v", got)
	}
}
