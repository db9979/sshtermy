// Package safe fängt Paniken in Hintergrundarbeit ab, damit ein Fehler an
// einer Stelle (KI-Antwort auslesen, Geräteausgabe auswerten, Dateiansicht …)
// nicht das ganze Programm mit allen offenen Verbindungen beendet.
//
// Ohne Handler (Tests, Unterbefehle) wird die Panik unverändert
// weitergereicht: Fehler fallen dort weiterhin sofort auf. Erst die
// Oberfläche setzt mit SetHandler einen Handler, der einen Absturzbericht
// schreibt und eine Meldung zeigt.
//
// Häufen sich die Paniken (Sturm: mehr als StormN in StormWindow), ist der
// Zustand vermutlich kaputt: dann wird wie früher weitergereicht und das
// Programm endet mit dem gewohnten Absturzbericht.
package safe

import (
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// Caught beschreibt eine abgefangene Panik.
type Caught struct {
	Where string // Stelle, z. B. „ui.Update“ oder „sshx/client.go:464“
	Value any    // Wert aus panic(…)
	Stack []byte
	At    time.Time
}

// Raw: Text wie bei einer echten Panik (für den Absturzbericht).
func (c Caught) Raw() string {
	return fmt.Sprintf("panic: %v [recovered in %s]\n\n%s", c.Value, c.Where, c.Stack)
}

const (
	StormN      = 5
	StormWindow = time.Minute
)

var (
	handler atomic.Pointer[func(Caught)]
	count   atomic.Int64

	mu     sync.Mutex
	recent []time.Time
)

// SetHandler legt fest, was mit abgefangenen Paniken passiert. nil schaltet
// das Abfangen ab (Paniken laufen dann wie gewohnt durch).
func SetHandler(fn func(Caught)) {
	if fn == nil {
		handler.Store(nil)
		return
	}
	handler.Store(&fn)
}

// Active: ist ein Handler gesetzt?
func Active() bool { return handler.Load() != nil }

// Count: so viele Paniken wurden seit dem Start abgefangen.
func Count() int64 { return count.Load() }

// Recover gehört direkt hinter defer: `defer safe.Recover("wo")`.
func Recover(where string) {
	if r := recover(); r != nil {
		handle(where, r)
	}
}

// Go startet fn nebenher mit Absturzschutz.
func Go(where string, fn func()) {
	go func() {
		defer Recover(where)
		fn()
	}()
}

// Run führt fn aus; false, wenn dabei eine Panik abgefangen wurde.
func Run(where string, fn func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			handle(where, r)
			ok = false
		}
	}()
	fn()
	return true
}

func handle(where string, r any) {
	h := handler.Load()
	if h == nil || storm(time.Now()) {
		panic(r)
	}
	count.Add(1)
	(*h)(Caught{Where: where, Value: r, Stack: debug.Stack(), At: time.Now()})
}

// storm merkt sich den Zeitpunkt und meldet, ob es zu viele waren.
func storm(now time.Time) bool {
	mu.Lock()
	defer mu.Unlock()
	keep := recent[:0]
	for _, t := range recent {
		if now.Sub(t) < StormWindow {
			keep = append(keep, t)
		}
	}
	recent = append(keep, now)
	return len(recent) > StormN
}

// Reset: nur für Tests (Zähler, Sturm-Fenster, Handler zurücksetzen).
func Reset() {
	mu.Lock()
	recent = nil
	mu.Unlock()
	count.Store(0)
	handler.Store(nil)
}
