// Package llm ist ein schlanker Client für Sprachmodelle ohne SDK, nur
// net/http. Er spricht zwei Protokolle:
//
//   - OpenAI-kompatibel (/v1/chat/completions): der Spark mit vLLM/SGLang,
//     Ollama, OpenAI, LiteLLM, LocalAI, LM Studio …
//   - Anthropic Messages (/v1/messages) für Claude.
//
// Ein Client kennt den eingestellten Endpunkt und die Ersatz-Anbieter
// (domain.LLMConfig.Fallbacks). Antwortet der Endpunkt nicht, springt der
// nächste ein (siehe chain.go).
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/debuglog"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

type Message struct {
	Role    string `json:"role"` // system | user | assistant
	Content string `json:"content"`
}

type request struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
	// OpenAI selbst: neuere Modelle kennen nur noch max_completion_tokens.
	MaxCompletionTokens int      `json:"max_completion_tokens,omitempty"`
	Temperature         *float64 `json:"temperature,omitempty"`
	Stream              bool     `json:"stream,omitempty"`
	Stop                []string `json:"stop,omitempty"`
	// vLLM/SGLang: Qwen3-Thinking per Chat-Template abschalten.
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
	// gpt-oss (vLLM, Ollama), OpenAI o-/gpt-5-Modelle, Ollama-Denkmodelle:
	// wie lange das Modell vor der Antwort nachdenkt.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	// JSON-Modus (Opts.JSON): der Server erzwingt genau ein JSON-Objekt
	// (OpenAI, vLLM, SGLang, Ollama).
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}

// msgOut ist eine Antwortnachricht. Reasoning-Modelle (Qwen3, DeepSeek-R1)
// liefern bei vLLM/SGLang mit Reasoning-Parser die Gedanken getrennt in
// reasoning_content (ältere Versionen: reasoning); content ist dann nur die
// eigentliche Antwort und kann leer sein, wenn das Token-Limit vorher endet.
type msgOut struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
	Reasoning        string `json:"reasoning"`
}

type response struct {
	Choices []struct {
		Message      msgOut `json:"message"`
		Delta        msgOut `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// conn ist ein einzelner Endpunkt (Hauptendpunkt oder ein Ersatz).
type conn struct {
	cfg  domain.LLMConfig
	http *http.Client
	idx  int // 0 = eingestellter Endpunkt, 1… = Ersatz-Anbieter
}

// quirks: optionale Felder, die ein Endpunkt schon einmal abgelehnt hat
// (Schlüssel Adresse|Modell|Feld). Spart bei jeder weiteren Anfrage den
// zweiten Versuch.
var quirks sync.Map

func (c *conn) quirkKey(field string) string {
	return c.cfg.BaseURL + "|" + c.cfg.Model + "|" + field
}
func (c *conn) quirk(field string) bool { _, ok := quirks.Load(c.quirkKey(field)); return ok }
func (c *conn) setQuirk(field string)   { quirks.Store(c.quirkKey(field), true) }

// Client hält den Endpunkt samt Ersatz-Anbietern.
type Client struct {
	cfg   domain.LLMConfig
	conns []*conn
}

const maxBody = 2 << 20 // Antworten über 2 MiB werden abgeschnitten (Schutz vor Speicherfluten)

// New prüft den Hauptendpunkt; Ersatz-Anbieter mit unvollständigen Angaben
// werden still übergangen (sie sollen den Copiloten nicht abschalten).
func New(cfg domain.LLMConfig) (*Client, error) {
	if err := Check(cfg); err != nil {
		return nil, err
	}
	c := &Client{cfg: cfg}
	for i, e := range cfg.Chain() {
		if i > 0 && Check(e) != nil {
			continue
		}
		c.conns = append(c.conns, &conn{cfg: e, http: httpClient(e), idx: i})
	}
	return c, nil
}

// Check: Modell gesetzt, Base-URL gültig, Cloud-Dienste mit API-Key; ein
// API-Key geht nie unverschlüsselt (http) an einen fremden Host, außer
// AllowInsecure ist bewusst gesetzt.
func Check(cfg domain.LLMConfig) error {
	if cfg.Model == "" {
		return errors.New(i18n.T("Modellname fehlt"))
	}
	return CheckEndpoint(cfg)
}

// CheckEndpoint prüft nur Base-URL und Key-Transport (für die Modell-Liste,
// bei der noch kein Modell gewählt sein muss).
func CheckEndpoint(cfg domain.LLMConfig) error {
	u, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New(i18n.T("Base-URL ungültig (http(s)://host[:port]/v1)"))
	}
	if cfg.Provider.NeedsKey() && !cfg.APIKey.IsSet() {
		return fmt.Errorf(i18n.T("%s braucht einen API-Key"), cfg.Provider.Label())
	}
	if u.Scheme == "http" && cfg.APIKey.IsSet() && !cfg.AllowInsecure && !isLocal(u.Hostname()) {
		return errors.New(i18n.T("API-Key würde unverschlüsselt (http) übertragen; https nutzen oder 'unsicher erlauben' setzen"))
	}
	return nil
}

func isLocal(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

func baseURL(cfg domain.LLMConfig) string {
	b := strings.TrimRight(cfg.BaseURL, "/")
	b = strings.TrimSuffix(b, "/chat/completions")
	return strings.TrimSuffix(b, "/messages")
}

func (c *conn) endpoint() string {
	if c.cfg.Provider == domain.ProviderAnthropic {
		return baseURL(c.cfg) + "/messages"
	}
	return baseURL(c.cfg) + "/chat/completions"
}

// transport: kurzer Verbindungsaufbau, damit ein ausgeschalteter Spark
// schnell als „nicht erreichbar“ gilt und der Ersatz übernimmt, statt erst
// nach dem vollen Timeout.
var transport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 6 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	TLSHandshakeTimeout:   10 * time.Second,
	MaxIdleConns:          16,
	IdleConnTimeout:       90 * time.Second,
	ExpectContinueTimeout: time.Second,
	ForceAttemptHTTP2:     true,
}

func httpClient(cfg domain.LLMConfig) *http.Client {
	t := cfg.TimeoutSec
	if t <= 0 {
		t = 60
	}
	return &http.Client{Timeout: time.Duration(t) * time.Second, Transport: transport, CheckRedirect: sameHostRedirect}
}

// sameHostRedirect: Weiterleitungen nur zum selben Host und nie von https
// auf http. Go entfernt beim Hostwechsel nur „Authorization“, nicht den
// API-Schlüssel im Kopf „x-api-key“ (Anthropic).
func sameHostRedirect(req *http.Request, via []*http.Request) error {
	first := via[0].URL
	if len(via) >= 5 || req.URL.Host != first.Host || (first.Scheme == "https" && req.URL.Scheme != "https") {
		return fmt.Errorf(i18n.T("KI-Endpunkt leitet nach %s weiter, abgelehnt"), req.URL.Host)
	}
	return nil
}

func setAuth(req *http.Request, cfg domain.LLMConfig) {
	req.Header.Set("User-Agent", "sshtermy")
	if cfg.Provider == domain.ProviderAnthropic {
		req.Header.Set("anthropic-version", "2023-06-01")
		if cfg.APIKey.IsSet() {
			req.Header.Set("x-api-key", cfg.APIKey.Reveal())
		}
		return
	}
	if cfg.APIKey.IsSet() {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey.Reveal())
	}
}

// Models fragt GET {base}/models ab (OpenAI-Format {"data":[{"id":…}]}; so
// liefern es auch vLLM, SGLang, LiteLLM, LocalAI, LM Studio, Ollama und die
// Anthropic-API).
func Models(ctx context.Context, cfg domain.LLMConfig) ([]string, error) {
	if err := CheckEndpoint(cfg); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL(cfg)+"/models?limit=1000", nil)
	if err != nil {
		return nil, err
	}
	setAuth(req, cfg)
	resp, err := httpClient(cfg).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	var r struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct { // Ollama /api/tags-Form, falls ein Proxy sie durchreicht
			Name string `json:"name"`
		} `json:"models"`
		Error json.RawMessage `json:"error"`
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf(i18n.T("HTTP %d: API-Key fehlt oder ist falsch"), resp.StatusCode)
	}
	if json.Unmarshal(raw, &r) != nil {
		return nil, fmt.Errorf(i18n.T("HTTP %d, keine JSON-Antwort von /models"), resp.StatusCode)
	}
	if m := errMessage(r.Error); m != "" {
		return nil, fmt.Errorf("%s", m)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf(i18n.T("HTTP %d von /models"), resp.StatusCode)
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range r.Data {
		if m.ID != "" && !seen[m.ID] {
			seen[m.ID] = true
			out = append(out, m.ID)
		}
	}
	for _, m := range r.Models {
		if m.Name != "" && !seen[m.Name] {
			seen[m.Name] = true
			out = append(out, m.Name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// errMessage liest {"message": …} (OpenAI) oder einen reinen Text.
func errMessage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var e struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Message != "" {
		return e.Message
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

// TestResult beschreibt einen Verbindungstest.
type TestResult struct {
	Models    []string
	ModelsErr error
	Reply     string
	Latency   time.Duration
	ChatErr   error
}

// Test lädt die Modell-Liste und schickt eine winzige Chat-Anfrage an das
// eingestellte Modell. Beides wird getrennt gemeldet, weil manche Server
// /models nicht anbieten, Chat aber funktioniert. Getestet wird nur dieser
// eine Endpunkt, nicht die Ersatz-Anbieter.
func Test(ctx context.Context, cfg domain.LLMConfig) TestResult {
	cfg.Fallbacks = nil
	var r TestResult
	r.Models, r.ModelsErr = Models(ctx, cfg)
	if cfg.Model == "" {
		r.ChatErr = errors.New(i18n.T("kein Modell gewählt"))
		return r
	}
	if err := Check(cfg); err != nil {
		r.ChatErr = err
		return r
	}
	c := &conn{cfg: cfg, http: httpClient(cfg)}
	t0 := time.Now()
	res, err := c.completeResult(ctx, []Message{{Role: "user", Content: "Antworte nur mit: OK"}}, Opts{MaxTokens: 16, Temperature: 0.01})
	r.Reply, r.ChatErr = res.Content, err
	r.Latency = time.Since(t0)
	return r
}

// Opts überschreibt pro Aufruf Tokens und Temperatur.
type Opts struct {
	MaxTokens   int
	Temperature float64
	// NoThink bittet Reasoning-Modelle, nicht lange nachzudenken (für kurze
	// JSON-Antworten): Qwen über chat_template_kwargs.enable_thinking=false
	// (vLLM, SGLang), gpt-oss und OpenAI-Denkmodelle über
	// reasoning_effort=low, Ollama-Denkmodelle über reasoning_effort=none.
	// Lehnt der Server die Felder ab, wird ohne wiederholt.
	NoThink bool
	// Fast: Aufgabe, für die das schnelle Modell (LLMConfig.FastModel) reicht
	// (Ghost-Text, Folgebefehle, Begleiter). Gilt nur für den eingestellten
	// Endpunkt; Ersatz-Anbieter nehmen ihr eigenes Modell.
	Fast bool
	// Task benennt die Aufgabe für die Antwortzeiten im KI-Status
	// (TaskStats), z. B. "ghost" oder "agent".
	Task string
	// JSON: die Antwort ist genau ein JSON-Objekt (Ziel-, Agent-, Plan-Modus).
	// OpenAI-kompatible Server bekommen response_format json_object, damit
	// kein Text davor/danach steht. Kennt der Server das Feld nicht, wird
	// ohne wiederholt (und für den Endpunkt gemerkt).
	JSON bool
}

func noThinkKwargs(model string) map[string]any {
	if strings.Contains(strings.ToLower(model), "qwen") {
		return map[string]any{"enable_thinking": false}
	}
	return nil
}

// noThinkEffort: reasoning_effort für kurze Antworten ("" = Feld weglassen).
// gpt-oss lässt sich nicht ganz abschalten, „low“ ist das Minimum; die
// OpenAI-Denkmodelle kennen je nach Generation „minimal“ oder „none“, „low“
// verstehen alle.
func noThinkEffort(p domain.LLMProvider, model string) string {
	m := strings.ToLower(model)
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	switch {
	case strings.Contains(m, "gpt-oss"):
		return "low"
	case p == domain.ProviderOpenAI && (strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4") || strings.HasPrefix(m, "gpt-5")):
		return "low"
	case p == domain.ProviderOllama && (strings.HasPrefix(m, "qwen3") || strings.HasPrefix(m, "deepseek-r1") || strings.HasPrefix(m, "magistral") || strings.HasPrefix(m, "phi4-reasoning")):
		return "none"
	}
	return ""
}

// Result ist die erste Antwort mit Zusatzinfos.
type Result struct {
	Content      string
	Reasoning    string // getrennt gelieferte Gedanken (nur zur Diagnose)
	FinishReason string // "stop", "length", …
}

// Truncated: das Modell wurde vom Token-Limit abgeschnitten.
func (r Result) Truncated() bool { return r.FinishReason == "length" }

// Complete schickt eine Anfrage und liefert den Text der ersten Antwort.
func (c *Client) Complete(ctx context.Context, msgs []Message, o Opts) (string, error) {
	r, err := c.CompleteResult(ctx, msgs, o)
	return r.Content, err
}

func (c *conn) params(o Opts) (int, float64) {
	mt, temp := c.cfg.MaxTokens, c.cfg.Temperature
	if o.MaxTokens > 0 {
		mt = o.MaxTokens
	}
	if o.Temperature > 0 {
		temp = o.Temperature
	}
	return mt, temp
}

// openAIBody baut die Anfrage im OpenAI-Format.
func (c *conn) openAIBody(msgs []Message, o Opts, stream bool) request {
	mt, temp := c.params(o)
	body := request{Model: c.cfg.Model, Messages: msgs, Temperature: &temp, Stream: stream}
	if c.cfg.Provider == domain.ProviderOpenAI {
		body.MaxCompletionTokens = mt
	} else {
		body.MaxTokens = mt
	}
	if o.NoThink {
		body.ChatTemplateKwargs = noThinkKwargs(c.cfg.Model)
		body.ReasoningEffort = noThinkEffort(c.cfg.Provider, c.cfg.Model)
	}
	return body
}

// rejects: der Server lehnt ein Feld der Anfrage ab (HTTP 400 mit Hinweis).
func rejects(status int, raw []byte, field string) bool {
	return status == http.StatusBadRequest && bytes.Contains(bytes.ToLower(raw), []byte(field))
}

func (c *conn) completeResult(ctx context.Context, msgs []Message, o Opts) (Result, error) {
	if c.cfg.Provider == domain.ProviderAnthropic {
		return c.anthropicComplete(ctx, msgs, o)
	}
	body := c.openAIBody(msgs, o, false)
	if o.JSON && !c.quirk("response_format") {
		body.ResponseFormat = map[string]string{"type": "json_object"}
	}
	if c.quirk("temperature") {
		body.Temperature = nil
	}
	status, raw, err := c.post(ctx, body)
	if err == nil && body.ResponseFormat != nil && rejects(status, raw, "response_format") {
		c.setQuirk("response_format")
		body.ResponseFormat = nil
		status, raw, err = c.post(ctx, body)
	}
	if err == nil && status == http.StatusBadRequest && (body.ChatTemplateKwargs != nil || body.ReasoningEffort != "") {
		body.ChatTemplateKwargs, body.ReasoningEffort = nil, "" // Server kennt die Felder nicht
		status, raw, err = c.post(ctx, body)
	}
	if err == nil && body.Temperature != nil && rejects(status, raw, "temperature") {
		c.setQuirk("temperature")
		body.Temperature = nil // Reasoning-Modelle bei OpenAI erlauben nur den Standard
		status, raw, err = c.post(ctx, body)
	}
	if err != nil {
		return Result{}, err
	}
	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return Result{}, fmt.Errorf(i18n.T("LLM: HTTP %d, keine JSON-Antwort"), status)
	}
	if r.Error != nil {
		return Result{}, fmt.Errorf("LLM: %s", r.Error.Message)
	}
	if status/100 != 2 || len(r.Choices) == 0 {
		return Result{}, fmt.Errorf("LLM: HTTP %d", status)
	}
	ch := r.Choices[0]
	reasoning := ch.Message.ReasoningContent
	if reasoning == "" {
		reasoning = ch.Message.Reasoning
	}
	return Result{Content: ch.Message.Content, Reasoning: reasoning, FinishReason: ch.FinishReason}, nil
}

func (c *conn) do(ctx context.Context, body any, stream bool) (*http.Response, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	setAuth(req, c.cfg)
	cl := c.http
	if stream {
		req.Header.Set("Accept", "text/event-stream")
		cp := *c.http
		cp.Timeout = 0 // Streams dürfen länger dauern; Abbruch über ctx
		cl = &cp
	}
	remember(c.cfg, req.URL.Host, raw)
	t0 := time.Now()
	resp, err := cl.Do(req)
	if debuglog.On() {
		st := 0
		if resp != nil {
			st = resp.StatusCode
		}
		debuglog.Printf("ki", "%s %s Modell=%s Stream=%t %d Bytes → HTTP %d nach %s %v", c.cfg.Provider, req.URL.Host, c.cfg.Model, stream, len(raw), st, time.Since(t0).Round(time.Millisecond), orNil(err))
	}
	return resp, err
}

func orNil(err error) any {
	if err == nil {
		return ""
	}
	return err
}

func (c *conn) post(ctx context.Context, body any) (int, []byte, error) {
	resp, err := c.do(ctx, body, false)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	return resp.StatusCode, raw, err
}

// openStream startet einen Stream; nil-Fehler heißt: Kopfzeilen mit 2xx da.
func (c *conn) openStream(ctx context.Context, msgs []Message, o Opts) (*http.Response, error) {
	var body any
	var ab *anthropicRequest
	if c.cfg.Provider == domain.ProviderAnthropic {
		b := c.anthropicBody(msgs, o)
		b.Stream = true
		ab, body = &b, &b
	} else {
		b := c.openAIBody(msgs, o, true)
		b.ChatTemplateKwargs, b.ReasoningEffort = nil, ""
		if c.quirk("temperature") {
			b.Temperature = nil
		}
		body = b
	}
	resp, err := c.do(ctx, body, true)
	if err != nil {
		return nil, err
	}
	// Anthropic: abgelehntes optionales Feld weglassen und neu starten
	for i := 0; i < 3 && ab != nil && resp.StatusCode == http.StatusBadRequest; i++ {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		if !c.anthropicRetry(resp.StatusCode, raw, ab) {
			resp = &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(bytes.NewReader(raw))}
			break
		}
		if resp, err = c.do(ctx, body, true); err != nil {
			return nil, err
		}
	}
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		if m := apiError(raw); m != "" {
			return nil, fmt.Errorf("LLM: HTTP %d: %s", resp.StatusCode, m)
		}
		return nil, fmt.Errorf("LLM: HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

// apiError: Fehlermeldung aus einer JSON-Antwort (OpenAI und Anthropic
// haben beide {"error":{"message":…}}).
func apiError(raw []byte) string {
	var r struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return ""
	}
	return errMessage(r.Error)
}

// readStream liest Server-Sent Events beider Formate.
func (c *conn) readStream(resp *http.Response, onDelta func(string)) error {
	defer resp.Body.Close()
	sc := bufio.NewScanner(io.LimitReader(resp.Body, maxBody))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return nil
		}
		if c.cfg.Provider == domain.ProviderAnthropic {
			var ev anthropicEvent
			if json.Unmarshal([]byte(data), &ev) != nil {
				continue
			}
			switch ev.Type {
			case "content_block_delta":
				if ev.Delta.Text != "" {
					onDelta(ev.Delta.Text)
				}
			case "message_stop":
				return nil
			case "error":
				if ev.Error != nil {
					return fmt.Errorf("LLM: %s", ev.Error.Message)
				}
			}
			continue
		}
		var r response
		if json.Unmarshal([]byte(data), &r) == nil && len(r.Choices) > 0 {
			onDelta(r.Choices[0].Delta.Content)
		}
	}
	return sc.Err()
}
