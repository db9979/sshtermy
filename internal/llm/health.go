package llm

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

// Health merkt sich, wie die letzten Anfragen an den Endpunkt ausgegangen
// sind. Jede echte Anfrage (Vorschlag, Frage, Ziel-Modus) zählt mit, so muss
// die Statusanzeige nur selten selbst nachfragen.

// Observation ist das Ergebnis einer Anfrage.
type Observation struct {
	At      time.Time
	Model   string
	BaseURL string
	Latency time.Duration // Dauer der Anfrage
	Err     error         // nil = Antwort erhalten
	Probe   bool          // Hintergrundprüfung statt echter Anfrage
	// Quick: Latency misst nur die Reaktionszeit (Prüfung mit einem Token
	// oder Beginn eines Streams), nicht das Schreiben einer langen Antwort.
	Quick bool
}

var health struct {
	mu   sync.Mutex
	last Observation
	ok   Observation // letzte erfolgreiche mit gemessener Reaktionszeit (Quick)
	n    uint64      // zählt jede Meldung, damit die Anzeige Änderungen erkennt
}

// observe meldet ein Ergebnis. Vom Nutzer abgebrochene Anfragen sagen nichts
// über den Endpunkt und werden übergangen.
func observe(o Observation) {
	if o.Err != nil && errors.Is(o.Err, context.Canceled) {
		return
	}
	if o.At.IsZero() {
		o.At = time.Now()
	}
	health.mu.Lock()
	defer health.mu.Unlock()
	health.last = o
	if o.Err == nil && o.Quick {
		health.ok = o
	}
	health.n++
}

// LastHealth liefert die letzte Meldung, die letzte erfolgreiche mit
// gemessener Reaktionszeit und einen
// Zähler, der bei jeder Meldung wächst.
func LastHealth() (last, ok Observation, n uint64) {
	health.mu.Lock()
	defer health.mu.Unlock()
	return health.last, health.ok, health.n
}

func (c *Client) observe(t0 time.Time, err error, quick bool) {
	observe(Observation{At: time.Now(), Model: c.cfg.Model, BaseURL: baseURL(c.cfg),
		Latency: time.Since(t0), Err: err, Quick: quick})
}

// BaseURL: die Basisadresse so, wie sie in Meldungen steht (zum Vergleich).
func BaseURL(cfg domain.LLMConfig) string { return baseURL(cfg) }

// Probe schickt die kleinstmögliche Chat-Anfrage (ein Token) und meldet das
// Ergebnis an die Statusanzeige.
func Probe(ctx context.Context, cfg domain.LLMConfig) error {
	c, err := New(cfg)
	if err != nil {
		observe(Observation{Model: cfg.Model, BaseURL: baseURL(cfg), Err: err, Probe: true})
		return err
	}
	t0 := time.Now()
	_, err = c.completeResult(ctx, []Message{{Role: "user", Content: "Antworte nur mit: OK"}}, Opts{MaxTokens: 1, Temperature: 0.01})
	observe(Observation{At: time.Now(), Model: cfg.Model, BaseURL: baseURL(cfg),
		Latency: time.Since(t0), Err: err, Probe: true, Quick: true})
	return err
}
