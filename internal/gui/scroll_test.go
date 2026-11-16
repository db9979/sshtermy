//go:build windows || darwin || linux

package gui

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"

	"github.com/db9979/sshtermy/internal/termemu"
)

func rowText(cells []uv.Cell, w, y int) string {
	var b strings.Builder
	for _, c := range cells[y*w : (y+1)*w] {
		b.WriteString(c.Content)
	}
	return strings.TrimSpace(b.String())
}

// Zweites Fenster (abgetrennter Tab): Mausrad und ⇧Bild↑ blättern im
// eigenen Verlauf, Tippen führt zurück nach unten.
func TestAttachWindowScrolls(t *testing.T) {
	var sent bytes.Buffer
	term := termemu.New(20, 5, io.Discard)
	w := &window{opts: Options{attach: true}, term: term, in: &sent}
	for i := 0; i < 50; i++ {
		_, _ = term.Write([]byte(fmt.Sprintf("zeile %d\r\n", i)))
	}
	if !w.localWheel(false) {
		t.Fatal("Rad muss im Fenster blättern, wenn das Programm die Maus nicht will")
	}
	w.scrollWheel(true)
	if w.scroll != 3 {
		t.Fatalf("Rad: scroll=%d", w.scroll)
	}
	if !w.scrollKey(uv.KeyPgUp, uv.ModShift) || w.scroll != 7 {
		t.Fatalf("⇧Bild↑: scroll=%d", w.scroll)
	}
	if !w.scrollKey(uv.KeyHome, uv.ModShift) || w.scroll != term.ScrollbackLen() {
		t.Fatalf("⇧Pos1: scroll=%d", w.scroll)
	}
	cells, cw, _ := term.SnapshotAt(nil, w.scroll)
	if got := rowText(cells, cw, 0); got != "zeile 0" {
		t.Fatalf("oberste Zeile im Rückblick: %q", got)
	}
	// neue Ausgabe verschiebt die Ansicht nicht
	w.setScroll(10)
	_, _ = term.Write([]byte("neu 1\r\nneu 2\r\n"))
	w.followScroll()
	if w.scroll != 12 {
		t.Fatalf("Ansicht mitgewandert: scroll=%d", w.scroll)
	}
	if !w.scrollKey(uv.KeyEnd, uv.ModShift) || w.scroll != 0 {
		t.Fatalf("⇧Ende: scroll=%d", w.scroll)
	}
	if w.scrollKey(uv.KeyPgUp, 0) {
		t.Fatal("Bild↑ ohne ⇧ gehört dem Programm")
	}
	if sent.Len() != 0 {
		t.Fatalf("Blättern darf nichts an den Server schicken: %q", sent.String())
	}
	// Programm mit Maus (tmux, mc): Rad gehört ihm, ⇧ + Rad blättert
	_, _ = term.Write([]byte("\x1b[?1000h"))
	if w.localWheel(false) || !w.localWheel(true) {
		t.Fatal("Maus-Programm: nur ⇧ + Rad blättert")
	}
	// Hauptfenster: dort blättert sshTermy selbst
	main := &window{term: term, in: &sent}
	if main.localWheel(true) || main.scrollKey(uv.KeyPgUp, uv.ModShift) {
		t.Fatal("Hauptfenster darf nicht selbst blättern")
	}
}
