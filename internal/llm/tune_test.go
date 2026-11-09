package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestNoThinkEffort(t *testing.T) {
	cases := []struct {
		p    domain.LLMProvider
		m    string
		want string
	}{
		{domain.ProviderCompat, "openai/gpt-oss-120b", "low"},
		{domain.ProviderOllama, "gpt-oss:20b", "low"},
		{domain.ProviderOpenAI, "gpt-5-mini", "low"},
		{domain.ProviderOpenAI, "o4-mini", "low"},
		{domain.ProviderOpenAI, "gpt-4.1", ""},
		{domain.ProviderOllama, "qwen3:8b", "none"},
		{domain.ProviderOllama, "deepseek-r1:14b", "none"},
		{domain.ProviderOllama, "qwen2.5-coder:7b", ""},
		{domain.ProviderCompat, "Qwen3-32B", ""}, // dort chat_template_kwargs
	}
	for _, c := range cases {
		if got := noThinkEffort(c.p, c.m); got != c.want {
			t.Errorf("%s %s: %q, erwartet %q", c.p, c.m, got, c.want)
		}
	}
}

// reasoning_effort geht mit; lehnt der Server ab, kommt die Anfrage ohne.
func TestReasoningEffortFallback(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		e, _ := req["reasoning_effort"].(string)
		seen = append(seen, e)
		if e != "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"unsupported parameter reasoning_effort"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	c, _ := New(domain.LLMConfig{BaseURL: srv.URL + "/v1", Model: "gpt-oss-20b"})
	got, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}, Opts{NoThink: true})
	if err != nil || got != "ok" || len(seen) != 2 || seen[0] != "low" || seen[1] != "" {
		t.Fatalf("got=%q err=%v seen=%v", got, err, seen)
	}
}

// Das schnelle Modell übernimmt Aufgaben mit Fast; die übrigen und ein
// Ausfall des schnellen Modells gehen an das Hauptmodell. Die Antwortzeit
// landet bei der Aufgabe.
func TestFastModel(t *testing.T) {
	var models []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		models = append(models, req.Model)
		if req.Model == "kaputt" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"model not found"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + req.Model + `"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	cfg := domain.LLMConfig{BaseURL: srv.URL + "/v1", Model: "gross", FastModel: "klein"}
	markUp(Sig(cfg))
	c, _ := New(cfg)
	if got, _ := c.Complete(context.Background(), nil, Opts{Fast: true, Task: "test-fast"}); got != "klein" {
		t.Fatalf("Fast ging an %q", got)
	}
	if got, _ := c.Complete(context.Background(), nil, Opts{Task: "test-main"}); got != "gross" {
		t.Fatalf("ohne Fast ging an %q", got)
	}
	cfg.FastModel = "kaputt"
	c, _ = New(cfg)
	models = nil
	if got, err := c.Complete(context.Background(), nil, Opts{Fast: true}); err != nil || got != "gross" || len(models) != 2 {
		t.Fatalf("Ausfall des schnellen Modells: %q %v %v", got, err, models)
	}
	found := map[string]string{}
	for _, s := range TaskStats() {
		found[s.Task] = s.Model
	}
	if found["test-fast"] != "klein" || found["test-main"] != "gross" {
		t.Fatalf("TaskStats: %v", found)
	}
}

func TestFastModelDownSkipped(t *testing.T) {
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		hits[req.Model]++
		if req.Model == "weg" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	cfg := domain.LLMConfig{BaseURL: srv.URL + "/v1", Model: "haupt", FastModel: "weg"}
	c, _ := New(cfg)
	for range 3 {
		if _, err := c.Complete(context.Background(), nil, Opts{Fast: true}); err != nil {
			t.Fatal(err)
		}
	}
	if hits["weg"] != 1 || hits["haupt"] != 3 {
		t.Fatalf("ausgefallenes schnelles Modell nicht übersprungen: %v", hits)
	}
	fast := cfg
	fast.Model = "weg"
	markUp(Sig(fast))
}
