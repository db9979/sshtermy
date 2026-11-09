package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

// Qwen: enable_thinking=false wird mitgeschickt; lehnt der Server das Feld ab
// (HTTP 400), kommt die Anfrage ohne erneut.
func TestNoThinkFallback(t *testing.T) {
	var seen []bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		_, has := req["chat_template_kwargs"]
		seen = append(seen, has)
		if has {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"unknown field chat_template_kwargs"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"},"finish_reason":"length"}]}`))
	}))
	defer srv.Close()
	c, _ := New(domain.LLMConfig{BaseURL: srv.URL + "/v1", Model: "Qwen3-32B"})
	r, err := c.CompleteResult(context.Background(), []Message{{Role: "user", Content: "x"}}, Opts{NoThink: true})
	if err != nil || r.Content != "{}" || !r.Truncated() || len(seen) != 2 || !seen[0] || seen[1] {
		t.Fatalf("%+v %v %v", r, err, seen)
	}
	seen = nil
	c, _ = New(domain.LLMConfig{BaseURL: srv.URL + "/v1", Model: "llama3.1"})
	if _, err := c.CompleteResult(context.Background(), nil, Opts{NoThink: true}); err != nil || len(seen) != 1 || seen[0] {
		t.Fatalf("llama: %v %v", err, seen)
	}
}

func TestOwnEndpoint(t *testing.T) {
	own := []string{"llm.firma.de", "https://ki.example.org/v1"}
	cases := map[string]bool{
		"http://localhost:11434/v1":     true,
		"https://llm.firma.de/v1":       true,
		"https://a.llm.firma.de/v1":     true,
		"https://ki.example.org":        true,
		"https://evilllm.firma.de.x.io": false,
		"https://api.openai.com/v1":     false,
	}
	for u, want := range cases {
		if got := IsOwnEndpoint(u, own); got != want {
			t.Errorf("%s: %v, erwartet %v", u, got, want)
		}
	}
	c, err := NewWithPrivacy(domain.LLMConfig{BaseURL: "https://llm.firma.de/v1", Model: "m", OwnHosts: own}, PrivacyLocal)
	if err != nil || !c.AllOwn() {
		t.Fatalf("eigener Server bei „nur eigene KI“ abgelehnt: %v", err)
	}
	if _, err := NewWithPrivacy(domain.LLMConfig{BaseURL: "https://api.openai.com/v1", Model: "m", OwnHosts: own}, PrivacyLocal); err == nil {
		t.Fatal("fremder Anbieter bei „nur eigene KI“ erlaubt")
	}
}
