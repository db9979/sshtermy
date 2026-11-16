package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func okServer(t *testing.T, reply string, hits *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + reply + `"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Fällt der Endpunkt aus, antwortet der Ersatz; danach wird der ausgefallene
// Endpunkt übersprungen, bis eine Prüfung ihn wieder erreicht.
func TestFallbackChain(t *testing.T) {
	var mainHits, fbHits int
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mainHits++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad.Close()
	fb := okServer(t, "vom Ersatz", &fbHits)
	cfg := domain.LLMConfig{BaseURL: bad.URL + "/v1", Model: "spark-modell", Fallbacks: []domain.LLMBackup{
		{Provider: domain.ProviderOllama, BaseURL: fb.URL + "/v1", Model: "qwen"},
		{Provider: domain.ProviderOpenAI, Model: "ohne-key"}, // unvollständig: wird übergangen
	}}
	markUp(Sig(cfg.Chain()[0]))
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.conns) != 2 {
		t.Fatalf("conns=%d, erwartet 2 (OpenAI ohne Key übergangen)", len(c.conns))
	}
	got, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}, Opts{})
	if err != nil || got != "vom Ersatz" || mainHits != 1 || fbHits != 1 {
		t.Fatalf("got=%q err=%v main=%d fb=%d", got, err, mainHits, fbHits)
	}
	if _, err := c.Complete(context.Background(), nil, Opts{}); err != nil || mainHits != 1 || fbHits != 2 {
		t.Fatalf("ausgefallener Endpunkt nicht übersprungen: main=%d fb=%d err=%v", mainHits, fbHits, err)
	}
	if last, _ := HealthOf(cfg.Chain()[1]); last.Err != nil || last.Index != 1 {
		t.Fatalf("Ersatz-Meldung: %+v", last)
	}
	if last, _ := HealthOf(cfg.Chain()[0]); last.Err == nil {
		t.Fatalf("Ausfall nicht gemeldet: %+v", last)
	}
	// Abbruch durch den Nutzer: kein Ersatz-Versuch
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := fbHits
	if _, err := c.Complete(ctx, nil, Opts{}); err == nil || fbHits != before {
		t.Fatalf("Abbruch: err=%v fb=%d→%d", err, before, fbHits)
	}
	markUp(Sig(cfg.Chain()[0]))
}

func TestStreamFallback(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer bad.Close()
	fb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hal\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\ndata: [DONE]\n"))
	}))
	defer fb.Close()
	cfg := domain.LLMConfig{BaseURL: bad.URL + "/v1", Model: "m2", Fallbacks: []domain.LLMBackup{{BaseURL: fb.URL + "/v1", Model: "m3"}}}
	c, _ := New(cfg)
	var b strings.Builder
	if err := c.Stream(context.Background(), nil, Opts{}, func(s string) { b.WriteString(s) }); err != nil || b.String() != "Hallo" {
		t.Fatalf("%v %q", err, b.String())
	}
	markUp(Sig(cfg.Chain()[0]))
}

// Claude: Messages-API mit x-api-key, System-Text getrennt, Rollen zusammengefasst.
func TestAnthropic(t *testing.T) {
	var req anthropicRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "sk-ant" || r.Header.Get("anthropic-version") == "" || r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`))
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Stream {
			_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"ls \"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"-la\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
			return
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ls -la"}],"stop_reason":"max_tokens"}`))
	}))
	defer srv.Close()
	cfg := domain.LLMConfig{Provider: domain.ProviderAnthropic, BaseURL: srv.URL + "/v1", APIKey: "sk-ant", Model: "claude-haiku-5-5", MaxTokens: 50}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.CompleteResult(context.Background(), []Message{
		{Role: "system", Content: "Du bist ein Shell-Assistent."},
		{Role: "user", Content: "a"}, {Role: "user", Content: "b"},
	}, Opts{})
	if err != nil || r.Content != "ls -la" || !r.Truncated() {
		t.Fatalf("%+v %v", r, err)
	}
	if len(req.System) != 1 || req.System[0].Text != "Du bist ein Shell-Assistent." || req.System[0].CacheControl["type"] != "ephemeral" {
		t.Fatalf("System ohne Prompt-Caching: %+v", req.System)
	}
	if req.Temperature != nil {
		t.Fatalf("temperature an %s geschickt (lehnt eigene Werte ab)", cfg.Model)
	}
	if len(req.Messages) != 1 || req.Messages[0].Content != "a\n\nb" || req.MaxTokens != 50 {
		t.Fatalf("Anfrage: %+v", req)
	}
	var b strings.Builder
	if err := c.Stream(context.Background(), []Message{{Role: "user", Content: "x"}}, Opts{}, func(s string) { b.WriteString(s) }); err != nil || b.String() != "ls -la" {
		t.Fatalf("Stream: %v %q", err, b.String())
	}
	cfg.APIKey = "falsch"
	c, _ = New(cfg)
	if _, err := c.Complete(context.Background(), nil, Opts{}); err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("Fehler: %v", err)
	}
	cfg.APIKey = ""
	if _, err := New(cfg); err == nil {
		t.Fatal("Claude ohne API-Key angenommen")
	}
}

// OpenAI selbst: max_completion_tokens statt max_tokens; lehnt das Modell die
// Temperatur ab, wird ohne wiederholt.
func TestOpenAIParams(t *testing.T) {
	var seen []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		seen = append(seen, m)
		if _, ok := m["temperature"]; ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Unsupported value: 'temperature' does not support 0.2"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	c, _ := New(domain.LLMConfig{Provider: domain.ProviderOpenAI, BaseURL: srv.URL + "/v1", APIKey: "sk", Model: "o4-mini", MaxTokens: 77, Temperature: 0.2})
	got, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}, Opts{})
	if err != nil || got != "ok" || len(seen) != 2 {
		t.Fatalf("%q %v %d", got, err, len(seen))
	}
	if _, ok := seen[0]["max_tokens"]; ok || seen[0]["max_completion_tokens"] != float64(77) {
		t.Fatalf("Tokens: %v", seen[0])
	}
}
