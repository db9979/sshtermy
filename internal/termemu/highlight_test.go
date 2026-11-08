package termemu

import (
	"image/color"
	"io"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/highlight"
)

func TestRenderHighlight(t *testing.T) {
	set, err := highlight.New([]string{"rot boom"}, false)
	if err != nil {
		t.Fatal(err)
	}
	hl := &HL{Set: set}
	hl.Colors[highlight.Red] = color.RGBA{0xff, 0, 0, 0xff}
	tm := New(30, 3, io.Discard)
	// boom ohne eigene Farbe, dann boom in Grün vom Programm selbst
	_, _ = tm.Write([]byte("a boom b\r\n\x1b[32mboom\x1b[0m\r\n"))
	out := tm.ViewHL("", func(s string) string { return s }, hl)
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[0], "38;2;255;0;0") {
		t.Fatalf("Zeile 1 nicht eingefärbt: %q", lines[0])
	}
	if strings.Contains(lines[1], "38;2;255;0;0") {
		t.Fatalf("eigene Farbe des Programms überschrieben: %q", lines[1])
	}
	if plain := tm.View("", func(s string) string { return s }); strings.Contains(plain, "38;2;255;0;0") {
		t.Fatal("ohne Regeln eingefärbt")
	}
	// Vollbild (vim, htop): nie einfärben
	_, _ = tm.Write([]byte("\x1b[?1049hboom"))
	if out := tm.ViewHL("", func(s string) string { return s }, hl); strings.Contains(out, "38;2;255;0;0") {
		t.Fatal("Alt-Screen eingefärbt")
	}
}
