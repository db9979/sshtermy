package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Claude über die Anthropic Messages-API (POST /v1/messages). Unterschiede
// zum OpenAI-Format: System-Text steht getrennt, max_tokens ist Pflicht, die
// Rollen müssen sich abwechseln und mit „user“ beginnen.

type anthropicRequest struct {
	Model string `json:"model"`
	// System als Block mit cache_control: der lange, gleichbleibende
	// Systemtext wird bei Anthropic zwischengespeichert (Prompt-Caching),
	// Folgeanfragen sind schneller und billiger.
	System        []anthropicBlock `json:"system,omitempty"`
	Messages      []Message        `json:"messages"`
	MaxTokens     int              `json:"max_tokens"`
	Temperature   *float64         `json:"temperature,omitempty"`
	Stream        bool             `json:"stream,omitempty"`
	StopSequences []string         `json:"stop_sequences,omitempty"`
	// Thinking/OutputConfig: kurze Antworten ohne langes Nachdenken (NoThink)
	Thinking     map[string]any `json:"thinking,omitempty"`
	OutputConfig map[string]any `json:"output_config,omitempty"`
}

type anthropicBlock struct {
	Type         string         `json:"type"`
	Text         string         `json:"text"`
	CacheControl map[string]any `json:"cache_control,omitempty"`
}

var claudeVersion = regexp.MustCompile(`claude-(?:[a-z]+-)?(\d+)(?:[-.](\d+))?`)

// claudeGen: Generation des Modells (z. B. 4.6 → 4, 6; claude-3-5-sonnet → 3, 5).
// ok=false bei unbekanntem Namen.
func claudeGen(model string) (major, minor int, ok bool) {
	m := claudeVersion.FindStringSubmatch(strings.ToLower(model))
	if m == nil {
		return 0, 0, false
	}
	major, _ = strconv.Atoi(m[1])
	if m[2] != "" && len(m[2]) <= 2 { // kein Datum (20250101)
		minor, _ = strconv.Atoi(m[2])
	}
	return major, minor, true
}

// anthropicSampling: nimmt das Modell temperature an? Ab Claude 4.7 lehnen
// die Modelle eigene Werte ab (HTTP 400); weglassen geht immer.
func anthropicSampling(model string) bool {
	major, minor, ok := claudeGen(model)
	return ok && (major < 4 || major == 4 && minor <= 6)
}

// anthropicNoThink: wie ein Modell kurz antwortet. Neuere Modelle denken von
// sich aus nach und verbrauchen dabei die (knappen) Tokens der Antwort. Wo
// thinking sich nicht abschalten lässt (Opus 5.5, Sonnet 5.5, Fable), hilft
// die geringste Mühe (effort low).
func anthropicNoThink(model string) (thinking, output map[string]any) {
	m := strings.ToLower(model)
	major, minor, ok := claudeGen(m)
	switch {
	case strings.Contains(m, "fable") || strings.Contains(m, "mythos"):
		return nil, map[string]any{"effort": "low"}
	case ok && major >= 5 && minor >= 5 && !strings.Contains(m, "haiku"):
		return nil, map[string]any{"effort": "low"}
	case ok && (major > 4 || major == 4 && minor >= 5):
		return map[string]any{"type": "disabled"}, nil
	}
	return nil, nil // ältere Modelle denken nur auf Wunsch
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type anthropicEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Text string `json:"text"`
	} `json:"delta"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// anthropicMessages trennt System-Text ab und fasst aufeinanderfolgende
// Nachrichten gleicher Rolle zusammen.
func anthropicMessages(msgs []Message) (string, []Message) {
	var sys []string
	var out []Message
	for _, m := range msgs {
		if m.Role == "system" {
			sys = append(sys, m.Content)
			continue
		}
		role := m.Role
		if role != "assistant" {
			role = "user"
		}
		if n := len(out); n > 0 && out[n-1].Role == role {
			out[n-1].Content += "\n\n" + m.Content
			continue
		}
		out = append(out, Message{Role: role, Content: m.Content})
	}
	if len(out) == 0 || out[0].Role != "user" {
		out = append([]Message{{Role: "user", Content: "…"}}, out...)
	}
	return strings.Join(sys, "\n\n"), out
}

func (c *conn) anthropicBody(msgs []Message, o Opts) anthropicRequest {
	mt, temp := c.params(o)
	if mt <= 0 {
		mt = 1024
	}
	sys, m := anthropicMessages(msgs)
	r := anthropicRequest{Model: c.cfg.Model, Messages: m, MaxTokens: mt}
	if sys != "" {
		r.System = []anthropicBlock{{Type: "text", Text: sys, CacheControl: map[string]any{"type": "ephemeral"}}}
	}
	if anthropicSampling(c.cfg.Model) && !c.quirk("temperature") {
		r.Temperature = &temp
	}
	if o.NoThink && !c.quirk("thinking") {
		r.Thinking, r.OutputConfig = anthropicNoThink(c.cfg.Model)
	}
	return r
}

// anthropicRetry: hat der Server ein optionales Feld abgelehnt (Proxy,
// älteres Modell), wird es für diesen Endpunkt gemerkt und ohne wiederholt.
func (c *conn) anthropicRetry(status int, raw []byte, body *anthropicRequest) bool {
	switch {
	case body.Temperature != nil && rejects(status, raw, "temperature"):
		c.setQuirk("temperature")
		body.Temperature = nil
	case (body.Thinking != nil || body.OutputConfig != nil) && (rejects(status, raw, "thinking") || rejects(status, raw, "effort") || rejects(status, raw, "output_config")):
		c.setQuirk("thinking")
		body.Thinking, body.OutputConfig = nil, nil
	case len(body.System) > 0 && body.System[0].CacheControl != nil && rejects(status, raw, "cache_control"):
		body.System[0].CacheControl = nil
	default:
		return false
	}
	return true
}

func (c *conn) anthropicComplete(ctx context.Context, msgs []Message, o Opts) (Result, error) {
	body := c.anthropicBody(msgs, o)
	status, raw, err := c.post(ctx, body)
	for i := 0; i < 3 && err == nil && c.anthropicRetry(status, raw, &body); i++ {
		status, raw, err = c.post(ctx, body)
	}
	if err != nil {
		return Result{}, err
	}
	var r anthropicResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return Result{}, fmt.Errorf(i18n.T("LLM: HTTP %d, keine JSON-Antwort"), status)
	}
	if r.Error != nil {
		return Result{}, fmt.Errorf("LLM: HTTP %d: %s", status, r.Error.Message)
	}
	if status/100 != 2 {
		return Result{}, fmt.Errorf("LLM: HTTP %d", status)
	}
	if status == http.StatusOK && len(r.Content) == 0 && r.StopReason == "" {
		return Result{}, fmt.Errorf(i18n.T("LLM: HTTP %d, keine JSON-Antwort"), status)
	}
	var b strings.Builder
	for _, p := range r.Content {
		if p.Type == "text" {
			b.WriteString(p.Text)
		}
	}
	finish := r.StopReason
	if finish == "refusal" && b.Len() == 0 {
		return Result{}, errors.New(i18n.T("LLM: Claude hat die Anfrage abgelehnt (Sicherheitsfilter). Anders formulieren oder ein anderes Modell wählen."))
	}
	switch finish {
	case "max_tokens":
		finish = "length"
	case "end_turn", "stop_sequence":
		finish = "stop"
	}
	return Result{Content: b.String(), FinishReason: finish}, nil
}
