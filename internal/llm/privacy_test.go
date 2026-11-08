package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestIsLocalEndpoint(t *testing.T) {
	local := []string{"http://localhost:11434/v1", "http://127.0.0.1:8000/v1", "http://10.1.2.3/v1", "http://192.168.0.5:8000",
		"http://dgx-spark:8000/v1", "http://ki.lan/v1", "https://llm.firma.internal/v1", "http://[::1]:11434", "http://[fd00::5]/v1"}
	remote := []string{"https://api.openai.com/v1", "https://api.anthropic.com", "http://8.8.8.8/v1", "https://llm.example.com/v1", "kaputt"}
	for _, u := range local {
		if !IsLocalEndpoint(u) {
			t.Errorf("%s sollte lokal sein", u)
		}
	}
	for _, u := range remote {
		if IsLocalEndpoint(u) {
			t.Errorf("%s sollte nicht lokal sein", u)
		}
	}
	if Stricter(PrivacyAll, PrivacyLocal) != PrivacyLocal || Stricter(PrivacyOff, PrivacyLocal) != PrivacyOff || Stricter("", "") != "" {
		t.Error("Stricter")
	}
}

func TestNewWithPrivacy(t *testing.T) {
	cfg := domain.LLMConfig{Provider: domain.ProviderOpenAI, BaseURL: "https://api.openai.com/v1", APIKey: "sk-x", Model: "gpt",
		Fallbacks: []domain.LLMBackup{{BaseURL: "http://localhost:11434/v1", Model: "qwen"}}}
	if _, err := NewWithPrivacy(cfg, PrivacyOff); !errors.Is(err, ErrAIOff) {
		t.Fatalf("aus: %v", err)
	}
	c, err := NewWithPrivacy(cfg, PrivacyLocal)
	if err != nil || len(c.conns) != 1 || !strings.Contains(c.conns[0].cfg.BaseURL, "localhost") {
		t.Fatalf("lokal: %v", err)
	}
	cfg.Fallbacks = nil
	if _, err := NewWithPrivacy(cfg, PrivacyLocal); err == nil {
		t.Fatal("ohne lokalen Endpunkt muss es scheitern")
	}
	if c, err := NewWithPrivacy(cfg, PrivacyAll); err != nil || len(c.conns) != 1 {
		t.Fatalf("alle: %v", err)
	}
}

func TestLastRequestRemembered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	c, err := New(domain.LLMConfig{BaseURL: srv.URL + "/v1", Model: "m", MaxTokens: 10, TimeoutSec: 5})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CompleteResult(context.Background(), []Message{{Role: "system", Content: "sei knapp"}, {Role: "user", Content: "uptime?"}}, Opts{}); err != nil {
		t.Fatal(err)
	}
	r := LastRequest()
	if r == nil || r.Model != "m" || len(r.Messages) != 2 || r.Messages[1].Content != "uptime?" {
		t.Fatalf("letzte Anfrage: %+v", r)
	}
}
