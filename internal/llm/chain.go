package llm

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Ersatz-Anbieter: Antwortet der eingestellte Endpunkt (z. B. der Spark)
// nicht, versucht der Client der Reihe nach die Ersatz-Anbieter. Ein
// Endpunkt, der gerade ausgefallen ist, wird für downFor übersprungen, damit
// nicht jede Anfrage erst auf ihn wartet. Die Hintergrundprüfung der
// Statusanzeige (Probe) gilt immer dem Hauptendpunkt; antwortet er wieder,
// ist er sofort wieder erste Wahl.

const downFor = 5 * time.Minute

var down struct {
	mu    sync.Mutex
	until map[string]time.Time
}

func markDown(sig string) {
	down.mu.Lock()
	defer down.mu.Unlock()
	if down.until == nil {
		down.until = map[string]time.Time{}
	}
	down.until[sig] = time.Now().Add(downFor)
}

func markUp(sig string) {
	down.mu.Lock()
	defer down.mu.Unlock()
	delete(down.until, sig)
}

func isDown(sig string) bool {
	down.mu.Lock()
	defer down.mu.Unlock()
	return time.Now().Before(down.until[sig])
}

func (c *conn) sig() string { return Sig(c.cfg) }

// order: erreichbare Endpunkte in der eingestellten Reihenfolge, gerade
// ausgefallene als letzter Versuch dahinter.
func (c *Client) order() []*conn {
	if len(c.conns) <= 1 {
		return c.conns
	}
	var up, dn []*conn
	for _, k := range c.conns {
		if isDown(k.sig()) {
			dn = append(dn, k)
		} else {
			up = append(up, k)
		}
	}
	return append(up, dn...)
}

// failed: Fehler, nach dem sich ein Ersatz lohnt. Bricht der Aufrufer ab
// (Esc, neue Eingabe), wird nichts weiter versucht.
func failed(ctx context.Context, err error) bool {
	return err != nil && ctx.Err() == nil && !errors.Is(err, context.Canceled)
}

// fast: der eingestellte Endpunkt mit dem schnellen Modell, falls eins
// eingetragen ist und die Aufgabe es erlaubt (sonst nil). Antwortet es
// nicht, geht es mit der normalen Reihenfolge weiter.
func (c *Client) fast(o Opts) *conn {
	if !o.Fast || len(c.conns) == 0 || c.conns[0].idx != 0 {
		return nil
	}
	m := c.conns[0]
	if m.cfg.FastModel == "" || m.cfg.FastModel == m.cfg.Model || isDown(m.sig()) {
		return nil
	}
	k := *m
	k.cfg.Model = k.cfg.FastModel
	if isDown(k.sig()) { // eben ausgefallen: erst mal das Hauptmodell
		return nil
	}
	return &k
}

// CompleteResult wie Complete, aber mit finish_reason und Reasoning.
func (c *Client) CompleteResult(ctx context.Context, msgs []Message, o Opts) (Result, error) {
	var first error
	if k := c.fast(o); k != nil {
		t0 := time.Now()
		r, err := k.completeResult(ctx, msgs, o)
		k.observe(t0, err, false)
		if err == nil {
			recordTask(o.Task, k.cfg.Model, time.Since(t0))
			return r, nil
		}
		if !failed(ctx, err) {
			return r, err
		}
		markDown(k.sig())
		first = err
	}
	for _, k := range c.order() {
		t0 := time.Now()
		r, err := k.completeResult(ctx, msgs, o)
		k.observe(t0, err, false)
		if err == nil {
			markUp(k.sig())
			recordTask(o.Task, k.cfg.Model, time.Since(t0))
			return r, nil
		}
		if !failed(ctx, err) {
			return r, err
		}
		if len(c.conns) > 1 {
			markDown(k.sig())
		}
		if first == nil {
			first = err
		}
	}
	return Result{}, first
}

// Stream liefert die Antwort stückweise (Server-Sent Events), z. B. für den
// Frage-Modus, damit die Erklärung sofort zu lesen ist. Ein Ersatz springt
// nur ein, solange noch nichts angekommen ist.
func (c *Client) Stream(ctx context.Context, msgs []Message, o Opts, onDelta func(string)) error {
	var first error
	for _, k := range c.order() {
		t0 := time.Now()
		resp, err := k.openStream(ctx, msgs, o)
		k.observe(t0, err, true) // Kopfzeilen da: der Server antwortet
		if err == nil {
			markUp(k.sig())
			once := false
			return k.readStream(resp, func(d string) {
				if !once && d != "" {
					once = true
					recordTask(o.Task, k.cfg.Model, time.Since(t0))
				}
				onDelta(d)
			})
		}
		if !failed(ctx, err) {
			return err
		}
		if len(c.conns) > 1 {
			markDown(k.sig())
		}
		if first == nil {
			first = err
		}
	}
	return first
}
