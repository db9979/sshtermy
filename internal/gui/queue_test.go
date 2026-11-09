package gui

import (
	"bytes"
	"io"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"

	"github.com/db9979/sshtermy/internal/termemu"
)

// Mausrad, während das Programm gerade nichts liest: Der Emulator darf das
// Fenster nicht blockieren (früher eingefroren bei schnellem Rad in langen
// Listen).
func TestMouseWheelNeverBlocks(t *testing.T) {
	r, w := io.Pipe()
	q := newQueueWriter(w)
	term := termemu.New(80, 24, q)
	_, _ = term.Write([]byte("\x1b[?1000h\x1b[?1006h")) // Maus + SGR

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			term.SendMouse(uv.MouseWheelEvent{X: 3, Y: 4, Button: uv.MouseWheelDown})
			_, _ = term.Write([]byte("x")) // Bild schreiben braucht dieselbe Sperre
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Mausrad blockiert, obwohl niemand liest")
	}

	// danach kommt alles in Reihenfolge an
	want := bytes.Repeat([]byte("\x1b[<65;4;5M"), 50)
	got := make([]byte, len(want))
	if _, err := io.ReadFull(r, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("falsche Bytes: %q", got[:20])
	}
}
