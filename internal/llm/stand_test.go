package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestAnthropicModelRules(t *testing.T) {
	for _, c := range []struct {
		model    string
		sampling bool
		thinking string // "disabled", "effort" oder ""
	}{
		{"claude-3-5-sonnet-20241022", true, ""},
		{"claude-haiku-4-5", true, "disabled"},
		{"claude-sonnet-4-6", true, "disabled"},
		{"claude-opus-4-8", false, "disabled"},
		{"claude-haiku-5-5", false, "disabled"},
		{"claude-sonnet-5-5", false, "effort"},
		{"claude-opus-5-5", false, "effort"},
		{"claude-fable-5-2", false, "effort"},
		{"mein-proxy-modell", false, ""},
	} {
		if got := anthropicSampling(c.model); got != c.sampling {
			t.Errorf("%s: temperature %v, erwartet %v", c.model, got, c.sampling)
		}
		th, out := anthropicNoThink(c.model)
		got := ""
		switch {
		case th != nil && th["type"] == "disabled":
			got = "disabled"
		case out != nil && out["effort"] == "low":
			got = "effort"
		}
		if got != c.thinking {
			t.Errorf("%s: NoThink %q, erwartet %q", c.model, got, c.thinking)
		}
	}
}

// Lehnt der Server thinking ab (Proxy, unbekanntes Modell), wird ohne
// wiederholt und das für den Endpunkt gemerkt.
func TestAnthropicThinkingRejectedOnce(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req anthropicRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Thinking != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"thinking: unknown field"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`))
	}))
	defer srv.Close()
	c, err := New(domain.LLMConfig{Provider: domain.ProviderAnthropic, BaseURL: srv.URL + "/v1", APIKey: "k", Model: "claude-haiku-5-5"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if r, err := c.CompleteResult(context.Background(), []Message{{Role: "user", Content: "x"}}, Opts{NoThink: true}); err != nil || r.Content != "ok" {
			t.Fatalf("%+v %v", r, err)
		}
	}
	if n := calls.Load(); n != 3 {
		t.Fatalf("%d Anfragen, erwartet 3 (einmal abgelehnt, dann gemerkt)", n)
	}
}

func TestAnthropicRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":[],"stop_reason":"refusal"}`))
	}))
	defer srv.Close()
	c, _ := New(domain.LLMConfig{Provider: domain.ProviderAnthropic, BaseURL: srv.URL + "/v1", APIKey: "k", Model: "claude-opus-5-5"})
	if _, err := c.CompleteResult(context.Background(), []Message{{Role: "user", Content: "x"}}, Opts{}); err == nil || !strings.Contains(err.Error(), "abgelehnt") {
		t.Fatalf("Ablehnung nicht gemeldet: %v", err)
	}
}

// JSON-Modus: response_format json_object; kennt der Server es nicht, ohne.
func TestOpenAIJSONMode(t *testing.T) {
	for _, supported := range []bool{true, false} {
		var calls atomic.Int32
		var last request
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			last = request{}
			_ = json.NewDecoder(r.Body).Decode(&last)
			if last.ResponseFormat != nil && !supported {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"response_format is not supported"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"befehl\":\"ls\"}"},"finish_reason":"stop"}]}`))
		}))
		cfg := domain.LLMConfig{Provider: domain.ProviderCompat, BaseURL: srv.URL + "/v1", Model: "m-" + map[bool]string{true: "ja", false: "nein"}[supported]}
		c, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		r, err := c.CompleteResult(context.Background(), []Message{{Role: "user", Content: "JSON bitte"}}, Opts{JSON: true})
		srv.Close()
		if err != nil || r.Content != `{"befehl":"ls"}` {
			t.Fatalf("%v: %+v %v", supported, r, err)
		}
		if supported && (calls.Load() != 1 || last.ResponseFormat["type"] != "json_object") {
			t.Fatalf("JSON-Modus nicht geschickt: %d %+v", calls.Load(), last.ResponseFormat)
		}
		if !supported && calls.Load() != 2 {
			t.Fatalf("ohne response_format nicht wiederholt: %d", calls.Load())
		}
	}
}
