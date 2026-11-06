// Package recording nimmt eine Terminal-Sitzung im asciicast-v2-Format auf
// (https://docs.asciinema.org/manual/asciicast/v2/). Die Datei lässt sich mit
// „asciinema play datei.cast“ abspielen, auf asciinema.org hochladen oder mit
// dem asciinema-player in eine Doku einbetten.
//
// Aufgenommen wird nur, was der Server schickt (die Ausgabe), nie die
// Tastendrücke: Passwörter, die die Gegenstelle nicht anzeigt, landen damit
// auch nicht in der Aufnahme.
package recording

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// MaxBytes: größte Aufnahme (Ausgabe); danach wird nicht mehr mitgeschrieben.
const MaxBytes = 64 << 20

// Header ist die erste Zeile einer .cast-Datei.
type Header struct {
	Version   int               `json:"version"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	Timestamp int64             `json:"timestamp,omitempty"`
	Title     string            `json:"title,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

// Recorder sammelt Ausgabe mit Zeitstempeln. Sicher für mehrere Goroutinen.
type Recorder struct {
	mu        sync.Mutex
	start     time.Time
	head      Header
	events    bytes.Buffer
	size      int
	rest      []byte // unvollständiges UTF-8-Zeichen vom Ende des letzten Blocks
	truncated bool
	now       func() time.Time
}

// New beginnt eine Aufnahme mit der aktuellen Terminalgröße.
func New(width, height int, title string) *Recorder {
	return newAt(width, height, title, time.Now)
}

func newAt(width, height int, title string, now func() time.Time) *Recorder {
	t := now()
	return &Recorder{start: t, now: now, head: Header{Version: 2, Width: max(1, width), Height: max(1, height),
		Timestamp: t.Unix(), Title: title, Env: map[string]string{"TERM": "xterm-256color"}}}
}

// Output nimmt Ausgabe des Servers auf.
func (r *Recorder) Output(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.truncated || len(p) == 0 {
		return
	}
	buf := append(r.rest, p...)
	r.rest = nil
	// unvollständiges Zeichen am Ende fürs nächste Mal aufheben
	if cut := incompleteTail(buf); cut > 0 {
		r.rest = append([]byte(nil), buf[len(buf)-cut:]...)
		buf = buf[:len(buf)-cut]
	}
	if len(buf) == 0 {
		return
	}
	if r.size+len(buf) > MaxBytes {
		r.truncated = true
		return
	}
	r.size += len(buf)
	r.event("o", strings.ToValidUTF8(string(buf), "�"))
}

// Resize merkt eine neue Terminalgröße (Ereignis „r“).
func (r *Recorder) Resize(width, height int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.truncated {
		return
	}
	r.event("r", fmt.Sprintf("%dx%d", width, height))
}

// Marker setzt eine Sprungmarke (asciinema-player zeigt sie an).
func (r *Recorder) Marker(label string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.event("m", label)
}

func (r *Recorder) event(kind, data string) {
	t := r.now().Sub(r.start).Seconds()
	b, _ := json.Marshal([]any{roundT(t), kind, data})
	r.events.Write(b)
	r.events.WriteByte('\n')
}

func roundT(t float64) float64 { return float64(int64(t*1e6+0.5)) / 1e6 }

// Duration: Länge der Aufnahme bis jetzt.
func (r *Recorder) Duration() time.Duration { return r.now().Sub(r.start) }

// Size: aufgenommene Bytes Ausgabe.
func (r *Recorder) Size() int { r.mu.Lock(); defer r.mu.Unlock(); return r.size }

// Truncated: Größengrenze erreicht, der Rest fehlt.
func (r *Recorder) Truncated() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.truncated }

// Cast liefert die .cast-Datei (Header + Ereignisse).
func (r *Recorder) Cast() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, _ := json.Marshal(r.head)
	out := make([]byte, 0, len(h)+1+r.events.Len())
	out = append(append(out, h...), '\n')
	return append(out, r.events.Bytes()...)
}

// incompleteTail: wie viele Bytes am Ende ein angefangenes UTF-8-Zeichen sind.
func incompleteTail(b []byte) int {
	for i := 1; i <= 3 && i <= len(b); i++ {
		c := b[len(b)-i]
		if c < 0x80 {
			return 0 // ASCII: nichts offen
		}
		if utf8.RuneStart(c) {
			if !utf8.FullRune(b[len(b)-i:]) {
				return i
			}
			return 0
		}
	}
	return 0
}

// ---------------------------------------------------------------- Erzeugen

// Builder baut eine Aufnahme aus bekannten Ereignissen nach (z. B. aus einem
// Sitzungsprotokoll, das nur Befehle und Ausgaben kennt).
type Builder struct {
	head   Header
	t      float64
	events bytes.Buffer
}

// NewBuilder beginnt eine nachgebaute Aufnahme.
func NewBuilder(width, height int, title string, started time.Time) *Builder {
	return &Builder{head: Header{Version: 2, Width: max(1, width), Height: max(1, height), Timestamp: started.Unix(),
		Title: title, Env: map[string]string{"TERM": "xterm-256color"}}}
}

// Wait schiebt die Zeit vor.
func (b *Builder) Wait(d time.Duration) {
	if d > 0 {
		b.t += d.Seconds()
	}
}

// Out schreibt Ausgabe zum aktuellen Zeitpunkt.
func (b *Builder) Out(s string) {
	if s == "" {
		return
	}
	x, _ := json.Marshal([]any{roundT(b.t), "o", strings.ToValidUTF8(s, "�")})
	b.events.Write(x)
	b.events.WriteByte('\n')
}

// Marker setzt eine Sprungmarke.
func (b *Builder) Marker(label string) {
	x, _ := json.Marshal([]any{roundT(b.t), "m", label})
	b.events.Write(x)
	b.events.WriteByte('\n')
}

// Type tippt Text Zeichen für Zeichen (sieht beim Abspielen wie getippt aus).
func (b *Builder) Type(s string, perChar time.Duration) {
	for _, r := range s {
		b.Out(string(r))
		b.Wait(perChar)
	}
}

// Cast liefert die .cast-Datei.
func (b *Builder) Cast() []byte {
	h, _ := json.Marshal(b.head)
	return append(append(h, '\n'), b.events.Bytes()...)
}

// ---------------------------------------------------------------- Lesen

// Event: ein Eintrag einer .cast-Datei.
type Event struct {
	Time float64
	Kind string
	Data string
}

// Parse liest eine .cast-Datei (v2), z. B. für Tests oder eine Vorschau.
func Parse(data []byte) (Header, []Event, error) {
	var h Header
	lines := bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n"))
	if len(lines) == 0 || json.Unmarshal(lines[0], &h) != nil || h.Version != 2 {
		return h, nil, fmt.Errorf("kein asciicast v2")
	}
	var ev []Event
	for i, l := range lines[1:] {
		var raw []any
		if err := json.Unmarshal(l, &raw); err != nil || len(raw) != 3 {
			return h, ev, fmt.Errorf("Zeile %d: kaputtes Ereignis", i+2)
		}
		t, _ := raw[0].(float64)
		k, _ := raw[1].(string)
		d, _ := raw[2].(string)
		ev = append(ev, Event{t, k, d})
	}
	return h, ev, nil
}
