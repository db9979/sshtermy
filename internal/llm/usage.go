package llm

import (
	"net/url"
	"strings"

	"github.com/db9979/sshtermy/internal/domain"
)

// KI-Verbrauch: Tokens je Anfrage, so wie der Server sie meldet (OpenAI
// usage.prompt_tokens/completion_tokens, Claude input_tokens/output_tokens
// samt Prompt-Cache). Nennt der Server nichts (Streams, manche lokalen
// Server), schätzt sshTermy grob mit vier Zeichen je Token und markiert das.
// Gezählt wird hier nur; aufbewahrt (je Tag, nur lokal) und angezeigt wird
// in der Oberfläche über OnUsage.

// Usage: eine erfolgreiche Anfrage.
type Usage struct {
	Task     string
	Model    string
	Endpoint string // Host des Endpunkts, z. B. api.anthropic.com
	In, Out  int
	Estimate bool // geschätzt (Server nannte keine Zahlen)
	// Price*: Preis je 1 Mio. Tokens aus der Einstellung dieses Endpunkts
	// (0 = unbekannt, z. B. eigener Server).
	PriceIn, PriceOut float64
}

// Cost: geschätzte Kosten der Anfrage in der Währung der Preis-Einstellung.
func (u Usage) Cost() float64 {
	return (float64(u.In)*u.PriceIn + float64(u.Out)*u.PriceOut) / 1e6
}

// OnUsage wird nach jeder erfolgreichen Anfrage aufgerufen (aus der
// Goroutine der Anfrage, also selbst für Gleichzeitigkeit sorgen).
var OnUsage func(Usage)

// estTokens: grobe Schätzung für Text ohne Zählung vom Server.
func estTokens(n int) int { return (n + 3) / 4 }

func recordUsage(task string, cfg domain.LLMConfig, msgs []Message, r Result) {
	f := OnUsage
	if f == nil {
		return
	}
	u := Usage{Task: task, Model: cfg.Model, Endpoint: endpointHost(cfg), In: r.In, Out: r.Out,
		PriceIn: cfg.PriceIn, PriceOut: cfg.PriceOut}
	if u.In == 0 {
		n := 0
		for _, m := range msgs {
			n += len(m.Content)
		}
		u.In, u.Estimate = estTokens(n), true
	}
	if r.In == 0 && r.Out == 0 && r.Content != "" {
		u.Out, u.Estimate = estTokens(len(r.Content)), true
	}
	f(u)
}

func endpointHost(cfg domain.LLMConfig) string {
	b := BaseURL(cfg)
	if pu, err := url.Parse(b); err == nil && pu.Host != "" {
		return pu.Host
	}
	return strings.TrimSpace(b)
}
