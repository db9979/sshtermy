package termemu

import (
	"io"
	"sync"
	"time"
)

// hostQueueMax: so viel darf auf dem Weg zum Server warten. Mehr sammelt sich
// nur an, wenn die Gegenseite nichts mehr annimmt; dann wird verworfen.
const hostQueueMax = 16 << 20

// hostQueue entkoppelt den Emulator vom Schreibweg zum Server. Der Emulator
// schreibt Tasten und Antworten unter seiner Sperre in eine ungepufferte
// Leitung. Nimmt der Server nichts an (Verbindung tot, serielle Leitung ohne
// Gegenstelle), blieb früher der nächste Tastendruck mit gehaltener Sperre
// hängen und mit ihm die ganze Oberfläche. Jetzt landet alles sofort hier,
// ein eigener Läufer schreibt es in Ruhe weg.
type hostQueue struct {
	w    io.Writer
	mu   sync.Mutex
	cond *sync.Cond
	buf  []byte
	busy bool // der Läufer schreibt gerade
	done bool // geschlossen oder Schreibfehler: nichts mehr annehmen
}

func newHostQueue(w io.Writer) *hostQueue {
	q := &hostQueue{w: w}
	q.cond = sync.NewCond(&q.mu)
	go q.run()
	return q
}

// Write kehrt sofort zurück, die Reihenfolge bleibt erhalten.
func (q *hostQueue) Write(p []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.done {
		return 0, io.ErrClosedPipe
	}
	if len(q.buf)+len(p) > hostQueueMax {
		return len(p), nil // Gegenseite hängt seit Langem: verwerfen statt wachsen
	}
	q.buf = append(q.buf, p...)
	q.cond.Signal()
	return len(p), nil
}

func (q *hostQueue) run() {
	var out []byte
	for {
		q.mu.Lock()
		for len(q.buf) == 0 && !q.done {
			q.cond.Wait()
		}
		if len(q.buf) == 0 {
			q.mu.Unlock()
			return
		}
		out, q.buf = q.buf, out[:0]
		q.busy = true
		q.mu.Unlock()
		_, err := q.w.Write(out)
		q.mu.Lock()
		q.busy = false
		q.mu.Unlock()
		if err != nil {
			q.close()
			return
		}
	}
}

// close: Rest wird noch geschrieben, Neues nicht mehr angenommen.
func (q *hostQueue) close() {
	q.mu.Lock()
	q.done = true
	q.cond.Broadcast()
	q.mu.Unlock()
}

// flush wartet, bis alles beim Server ist (höchstens d). false = Zeit um.
func (q *hostQueue) flush(d time.Duration) bool {
	end := time.Now().Add(d)
	for {
		q.mu.Lock()
		idle := len(q.buf) == 0 && !q.busy
		q.mu.Unlock()
		if idle {
			return true
		}
		if time.Now().After(end) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
}
