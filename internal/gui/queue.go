package gui

import (
	"io"
	"sync"
)

// queueWriter reicht Bytes in eigener Goroutine weiter, in Reihenfolge, und
// blockiert den Aufrufer nie.
//
// Warum: Mausereignisse laufen durch den Emulator (SendMouse), der dabei
// seine Sperre hält, bis die Bytes abgenommen sind. Hängt die Gegenseite
// gerade, weil bubbletea ein Bild in denselben Emulator schreibt (braucht
// dieselbe Sperre), stünde alles still: Schnelles Mausrad in langen Listen
// hat das Fenster so eingefroren. Tasten gehen denselben Weg, damit sie
// in Reihenfolge mit Maus und Einfügen bleiben.
type queueWriter struct {
	mu   sync.Mutex
	cond *sync.Cond
	buf  []byte
	err  error
	dst  io.Writer
}

func newQueueWriter(dst io.Writer) *queueWriter {
	q := &queueWriter{dst: dst}
	q.cond = sync.NewCond(&q.mu)
	go q.loop()
	return q
}

func (q *queueWriter) Write(p []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return 0, q.err
	}
	q.buf = append(q.buf, p...)
	q.cond.Signal()
	return len(p), nil
}

func (q *queueWriter) loop() {
	for {
		q.mu.Lock()
		for len(q.buf) == 0 {
			q.cond.Wait()
		}
		b := q.buf
		q.buf = nil
		q.mu.Unlock()
		if _, err := q.dst.Write(b); err != nil {
			q.mu.Lock()
			q.err = err
			q.buf = nil
			q.mu.Unlock()
			return
		}
	}
}
