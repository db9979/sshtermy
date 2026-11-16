package recording

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Cast: geladene Aufnahme (asciicast v2 oder v3).
type Cast struct {
	Width, Height int
	Title         string
	Events        []Event
	raw           []float64 // Zeiten ohne Pausenkürzung
}

// Duration: Länge in Sekunden.
func (c *Cast) Duration() float64 {
	if len(c.Events) == 0 {
		return 0
	}
	return c.Events[len(c.Events)-1].Time
}

// Load liest eine .cast-Datei zum Abspielen (v2: absolute Zeiten, v3:
// Abstände; Kommentarzeilen und unbekannte Ereignisse werden übersprungen).
func Load(data []byte) (*Cast, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	if !sc.Scan() {
		return nil, errors.New("leere Datei")
	}
	var h struct {
		Version int      `json:"version"`
		Width   int      `json:"width"`
		Height  int      `json:"height"`
		Title   string   `json:"title"`
		Idle    *float64 `json:"idle_time_limit"`
		Term    struct {
			Cols int `json:"cols"`
			Rows int `json:"rows"`
		} `json:"term"`
	}
	if err := json.Unmarshal(sc.Bytes(), &h); err != nil {
		return nil, fmt.Errorf("keine asciinema-Aufnahme: %w", err)
	}
	if h.Version != 2 && h.Version != 3 {
		return nil, fmt.Errorf("asciicast-Version %d wird nicht unterstützt", h.Version)
	}
	c := &Cast{Width: h.Width, Height: h.Height, Title: h.Title}
	if h.Version == 3 {
		c.Width, c.Height = h.Term.Cols, h.Term.Rows
	}
	if c.Width < 1 || c.Height < 1 {
		c.Width, c.Height = 80, 24
	}
	var t float64
	for sc.Scan() {
		ln := bytes.TrimSpace(sc.Bytes())
		if len(ln) == 0 || ln[0] != '[' {
			continue // v3 erlaubt Kommentare (#)
		}
		var raw []json.RawMessage
		if json.Unmarshal(ln, &raw) != nil || len(raw) < 3 {
			continue
		}
		var at float64
		var kind, d string
		if json.Unmarshal(raw[0], &at) != nil || json.Unmarshal(raw[1], &kind) != nil || json.Unmarshal(raw[2], &d) != nil {
			continue
		}
		if h.Version == 3 {
			t += at
		} else {
			t = max(t, at) // nie rückwärts
		}
		c.Events = append(c.Events, Event{Time: t, Kind: kind, Data: d})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	c.raw = make([]float64, len(c.Events))
	for i, e := range c.Events {
		c.raw[i] = e.Time
	}
	if h.Idle != nil && *h.Idle > 0 {
		c.IdleLimit(*h.Idle)
	}
	return c, nil
}

// IdleLimit kürzt Pausen auf höchstens limit Sekunden (0 = Originalzeiten).
func (c *Cast) IdleLimit(limit float64) {
	var shift, prev float64
	for i := range c.Events {
		gap := c.raw[i] - prev
		if limit > 0 && gap > limit {
			shift += gap - limit
		}
		prev = c.raw[i]
		c.Events[i].Time = c.raw[i] - shift
	}
}

// Index: erstes Ereignis nach Zeitpunkt t.
func (c *Cast) Index(t float64) int {
	return sort.Search(len(c.Events), func(i int) bool { return c.Events[i].Time > t })
}

// Search: erstes Ausgabe-Ereignis ab from, in dessen Text (ohne
// Steuerzeichen) q vorkommt, Groß/klein egal. Treffer über eine
// Ereignisgrenze hinweg werden mitgefunden. -1 = keiner.
func (c *Cast) Search(q string, from int) int {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return -1
	}
	tail := ""
	for i := max(0, from); i < len(c.Events); i++ {
		e := c.Events[i]
		if e.Kind != "o" {
			continue
		}
		txt := strings.ToLower(ansi.Strip(e.Data))
		if strings.Contains(tail+txt, q) {
			return i
		}
		if len(txt) >= len(q) {
			tail = txt[len(txt)-len(q)+1:]
		} else {
			tail += txt
			if len(tail) > len(q) {
				tail = tail[len(tail)-len(q)+1:]
			}
		}
	}
	return -1
}

// NextMarker: nächste Marke nach Zeitpunkt t (-1 = keine).
func (c *Cast) NextMarker(t float64) int {
	for i := c.Index(t); i < len(c.Events); i++ {
		if c.Events[i].Kind == "m" {
			return i
		}
	}
	return -1
}
