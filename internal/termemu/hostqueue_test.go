package termemu

import (
	"bytes"
	"sync"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
)

// stuckWriter nimmt nichts an, bis release geschlossen wird (tote Verbindung).
type stuckWriter struct {
	release chan struct{}
	mu      sync.Mutex
	got     bytes.Buffer
}

func (w *stuckWriter) Write(p []byte) (int, error) {
	<-w.release
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.got.Write(p)
}

func (w *stuckWriter) String() string { w.mu.Lock(); defer w.mu.Unlock(); return w.got.String() }

// Nimmt der Server nichts an, dürfen Tasten, Text und das Lesen des
// Bildschirms nicht hängen bleiben; später kommt alles in Reihenfolge an.
func TestStuckHostDoesNotBlock(t *testing.T) {
	w := &stuckWriter{release: make(chan struct{})}
	tm := New(80, 24, w)
	defer tm.Close()
	done := make(chan struct{})
	go func() {
		for _, r := range "abcdefgh" {
			tm.SendKey(uv.KeyPressEvent{Code: r, Text: string(r)})
		}
		tm.SendText("XYZ")
		_, _ = tm.Write([]byte("\x1b[c")) // Abfrage, deren Antwort zum Server will
		tm.PlainLines(5)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Emulator hängt, weil der Server nichts annimmt")
	}
	close(w.release)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !bytes.HasPrefix([]byte(w.String()), []byte("abcdefghXYZ\x1b[")) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := w.String(); !bytes.HasPrefix([]byte(got), []byte("abcdefghXYZ\x1b[")) {
		t.Fatalf("Reihenfolge/Inhalt: %q", got)
	}
}

func TestHostQueueDropsWhenFull(t *testing.T) {
	w := &stuckWriter{release: make(chan struct{})}
	q := newHostQueue(w)
	defer close(w.release)
	defer q.close()
	chunk := make([]byte, 1<<20)
	for i := 0; i < 40; i++ {
		if _, err := q.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	q.mu.Lock()
	n := len(q.buf)
	q.mu.Unlock()
	if n > hostQueueMax {
		t.Fatalf("Warteschlange wächst über die Grenze: %d", n)
	}
}
