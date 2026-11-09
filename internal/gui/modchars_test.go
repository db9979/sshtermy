//go:build windows || darwin || linux

package gui

import (
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// Windows meldet bei Strg+Leertaste zusätzlich ein Leerzeichen als Text.
// Das darf nicht ankommen, sonst klappt der Manager den Ordner um, statt
// die Palette zu öffnen.
func TestModCharsDropsSpace(t *testing.T) {
	if got := modChars([]rune{' '}, uv.ModCtrl); len(got) != 0 {
		t.Errorf("Strg+Leertaste: %q, erwartet nichts", string(got))
	}
	if got := string(modChars([]rune{'@'}, uv.ModCtrl|uv.ModAlt)); got != "@" {
		t.Errorf("AltGr+Q: %q, erwartet @", got)
	}
	if got := string(modChars([]rune{' '}, 0)); got != " " {
		t.Errorf("Leertaste: %q, erwartet Leerzeichen", got)
	}
}
