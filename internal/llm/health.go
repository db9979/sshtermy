package llm

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

// Health merkt sich, wie die letzten Anfragen an die Endpunkte ausgegangen
// sind. Jede echte Anfrage (Vorschlag, Frage, Ziel-Modus) zählt mit, so muss
// die Statusanzeige nur selten selbst nachfragen.

// Observation ist das Ergebnis einer Anfrage.
type Observation struct {
	At       time.Time
	Provider domain.LLMProvider
	Model    string
	BaseURL  string
	Index    int           // 0 = eingestellter Endpunkt, 1… = Ersatz-Anbieter
	Latency  time.Duration // Dauer der Anfrage
	Err      error         // nil = Antwort erhalten
	Probe    bool          // Hintergrundprüfung statt echter Anfrage
	// Quick: Latency misst nur die Reaktionszeit (Prüfung mit einem Token
	// oder Beginn eines Streams), nicht das Schreiben einer langen Antwort.
	Quick bool
}

type obsPair struct{ last, ok Observation }

var health struct {
	mu    sync.Mutex
	last  Observation
	ok    Observation // letzte erfolgreiche mit gemessener Reaktionszeit (Quick)
	n     uint64      // zählt jede Meldung, damit die Anzeige Änderungen erkennt
	bySig map[string]obsPair
}

// Sig kennzeichnet einen Endpunkt (Anbieter, Adresse, Modell).
func Sig(cfg domain.LLMConfig) string {
	return string(cfg.Provider) + "|" + baseURL(cfg) + "|" + cfg.Model
}

func obsSig(o Observation) string { return string(o.Provider) + "|" + o.BaseURL + "|" + o.Model }

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
	if health.bySig == nil {
		health.bySig = map[string]obsPair{}
	}
	p := health.bySig[obsSig(o)]
	p.last = o
	if o.Err == nil && o.Quick {
		health.ok = o
		p.ok = o
	}
	health.bySig[obsSig(o)] = p
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

// HealthOf: letzte Meldung und letzte gemessene Reaktionszeit für genau
// diesen Endpunkt (Ersatz-Anbieter eingeschlossen).
func HealthOf(cfg domain.LLMConfig) (last, ok Observation) {
	health.mu.Lock()
	defer health.mu.Unlock()
	p := health.bySig[Sig(cfg)]
	return p.last, p.ok
}

func (c *conn) observe(t0 time.Time, err error, quick bool) {
	observe(Observation{At: time.Now(), Provider: c.cfg.Provider, Model: c.cfg.Model, BaseURL: baseURL(c.cfg),
		Index: c.idx, Latency: time.Since(t0), Err: err, Quick: quick})
}

// BaseURL: die Basisadresse so, wie sie in Meldungen steht (zum Vergleich).
func BaseURL(cfg domain.LLMConfig) string { return baseURL(cfg) }

// Probe schickt die kleinstmögliche Chat-Anfrage (ein Token) an genau diesen
// Endpunkt (ohne Ersatz) und meldet das Ergebnis an die Statusanzeige.
// Antwortet er, ist er für echte Anfragen sofort wieder erste Wahl.
func Probe(ctx context.Context, cfg domain.LLMConfig) error {
	cfg.Fallbacks = nil
	o := Observation{Provider: cfg.Provider, Model: cfg.Model, BaseURL: baseURL(cfg), Probe: true}
	if err := Check(cfg); err != nil {
		o.Err = err
		observe(o)
		return err
	}
	c := &conn{cfg: cfg, http: httpClient(cfg)}
	t0 := time.Now()
	_, err := c.completeResult(ctx, []Message{{Role: "user", Content: "Antworte nur mit: OK"}}, Opts{MaxTokens: 1, Temperature: 0.01})
	o.At, o.Latency, o.Err, o.Quick = time.Now(), time.Since(t0), err, true
	observe(o)
	if err == nil {
		markUp(Sig(cfg))
	} else if ctx.Err() == nil {
		markDown(Sig(cfg))
	}
	return err
}
