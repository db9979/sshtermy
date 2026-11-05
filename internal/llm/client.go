// Package llm ist ein schlanker Client für OpenAI-kompatible
// /v1/chat/completions-Endpunkte (OpenAI, LocalAI, vLLM, SGLang, Ollama,
// LiteLLM, LM Studio). Keine SDK-Abhängigkeit, nur net/http.
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
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

type Message struct {
	Role    string `json:"role"` // system | user | assistant
	Content string `json:"content"`
}

type request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature"`
	Stream      bool      `json:"stream,omitempty"`
	Stop        []string  `json:"stop,omitempty"`
}

type response struct {
	Choices []struct {
		Message Message `json:"message"`
		Delta   Message `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Client hält die Konfiguration und einen wiederverwendeten HTTP-Client.
type Client struct {
	cfg  domain.LLMConfig
	http *http.Client
}

const maxBody = 2 << 20 // Antworten über 2 MiB werden abgeschnitten (Schutz vor Speicherfluten)

func New(cfg domain.LLMConfig) (*Client, error) {
	if err := Check(cfg); err != nil {
		return nil, err
	}
	return &Client{cfg: cfg, http: httpClient(cfg)}, nil
}

// Check: Base-URL gültig; ein API-Key geht nie unverschlüsselt (http) an
// einen fremden Host, außer AllowInsecure ist bewusst gesetzt.
func Check(cfg domain.LLMConfig) error {
	if cfg.Model == "" {
		return errors.New("Modellname fehlt")
	}
	return CheckEndpoint(cfg)
}

// CheckEndpoint prüft nur Base-URL und Key-Transport (für die Modell-Liste,
// bei der noch kein Modell gewählt sein muss).
func CheckEndpoint(cfg domain.LLMConfig) error {
	u, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("Base-URL ungültig (http(s)://host[:port]/v1)")
	}
	if u.Scheme == "http" && cfg.APIKey.IsSet() && !cfg.AllowInsecure && !isLocal(u.Hostname()) {
		return errors.New("API-Key würde unverschlüsselt (http) übertragen; https nutzen oder 'unsicher erlauben' setzen")
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
	return strings.TrimSuffix(strings.TrimRight(cfg.BaseURL, "/"), "/chat/completions")
}

func (c *Client) endpoint() string { return baseURL(c.cfg) + "/chat/completions" }

func httpClient(cfg domain.LLMConfig) *http.Client {
	t := cfg.TimeoutSec
	if t <= 0 {
		t = 60
	}
	return &http.Client{Timeout: time.Duration(t) * time.Second}
}

// Models fragt GET {base}/models ab (OpenAI-Format {"data":[{"id":…}]}; so
// liefern es auch vLLM, SGLang, LiteLLM, LocalAI, LM Studio und Ollama).
func Models(ctx context.Context, cfg domain.LLMConfig) ([]string, error) {
	if err := CheckEndpoint(cfg); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL(cfg)+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sshtermy")
	if cfg.APIKey.IsSet() {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey.Reveal())
	}
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
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("HTTP %d: API-Key fehlt oder ist falsch", resp.StatusCode)
	}
	if json.Unmarshal(raw, &r) != nil {
		return nil, fmt.Errorf("HTTP %d, keine JSON-Antwort von /models", resp.StatusCode)
	}
	if r.Error != nil {
		return nil, fmt.Errorf("%s", r.Error.Message)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d von /models", resp.StatusCode)
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
// /models nicht anbieten, Chat aber funktioniert.
func Test(ctx context.Context, cfg domain.LLMConfig) TestResult {
	var r TestResult
	r.Models, r.ModelsErr = Models(ctx, cfg)
	if cfg.Model == "" {
		r.ChatErr = errors.New("kein Modell gewählt")
		return r
	}
	c, err := New(cfg)
	if err != nil {
		r.ChatErr = err
		return r
	}
	t0 := time.Now()
	r.Reply, r.ChatErr = c.Complete(ctx, []Message{{Role: "user", Content: "Antworte nur mit: OK"}}, Opts{MaxTokens: 16, Temperature: 0.01})
	r.Latency = time.Since(t0)
	return r
}

func (c *Client) newRequest(ctx context.Context, body request) (*http.Request, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "sshtermy")
	if c.cfg.APIKey.IsSet() {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey.Reveal())
	}
	return req, nil
}

// Opts überschreibt pro Aufruf Tokens und Temperatur.
type Opts struct {
	MaxTokens   int
	Temperature float64
}

// Complete schickt eine Anfrage und liefert den Text der ersten Antwort.
func (c *Client) Complete(ctx context.Context, msgs []Message, o Opts) (string, error) {
	mt, temp := c.cfg.MaxTokens, c.cfg.Temperature
	if o.MaxTokens > 0 {
		mt = o.MaxTokens
	}
	if o.Temperature > 0 {
		temp = o.Temperature
	}
	req, err := c.newRequest(ctx, request{Model: c.cfg.Model, Messages: msgs, MaxTokens: mt, Temperature: temp})
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", err
	}
	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("LLM: HTTP %d, keine JSON-Antwort", resp.StatusCode)
	}
	if r.Error != nil {
		return "", fmt.Errorf("LLM: %s", r.Error.Message)
	}
	if resp.StatusCode/100 != 2 || len(r.Choices) == 0 {
		return "", fmt.Errorf("LLM: HTTP %d", resp.StatusCode)
	}
	return r.Choices[0].Message.Content, nil
}

// Stream liefert die Antwort stückweise (Server-Sent Events), z. B. für den
// Frage-Modus, damit die Erklärung sofort zu lesen ist.
func (c *Client) Stream(ctx context.Context, msgs []Message, o Opts, onDelta func(string)) error {
	mt := c.cfg.MaxTokens
	if o.MaxTokens > 0 {
		mt = o.MaxTokens
	}
	req, err := c.newRequest(ctx, request{Model: c.cfg.Model, Messages: msgs, MaxTokens: mt,
		Temperature: c.cfg.Temperature, Stream: true})
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	cl := *c.http
	cl.Timeout = 0 // Streams dürfen länger dauern; Abbruch über ctx
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("LLM: HTTP %d", resp.StatusCode)
	}
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
		var r response
		if json.Unmarshal([]byte(data), &r) == nil && len(r.Choices) > 0 {
			onDelta(r.Choices[0].Delta.Content)
		}
	}
	return sc.Err()
}
