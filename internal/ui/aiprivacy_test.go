package ui

import (
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/llm"
)

// Tags an Verbindungen machen den KI-Datenschutz strenger, nie lockerer.
func TestAIPrivacyTagsAndSetting(t *testing.T) {
	a := newPaletteTestApp(t)
	cfg := domain.LLMConfig{Provider: domain.ProviderOpenAI, BaseURL: "https://api.openai.com/v1", APIKey: "sk-x", Model: "gpt", MaxTokens: 10,
		Fallbacks: []domain.LLMBackup{{BaseURL: "http://localhost:11434/v1", Model: "qwen"}}}
	if err := a.st.SetLLM(cfg); err != nil {
		t.Fatal(err)
	}
	kunde := domain.Connection{Alias: "kunde", Tags: []string{"KI-Lokal"}}
	geheim := domain.Connection{Alias: "geheim", Tags: []string{"no-ai"}}
	if a.aiLevel() != llm.PrivacyAll || a.aiLevel(kunde) != llm.PrivacyLocal || a.aiLevel(kunde, geheim) != llm.PrivacyOff {
		t.Fatalf("Stufen: %q %q %q", a.aiLevel(), a.aiLevel(kunde), a.aiLevel(kunde, geheim))
	}
	if _, err := a.agentClientFor(geheim); err == nil || !strings.Contains(err.Error(), "ki-aus") {
		t.Fatalf("ki-aus: %v", err)
	}
	if _, err := a.agentClientFor(kunde); err != nil {
		t.Fatalf("lokal mit Ollama-Ersatz: %v", err)
	}
	a.cyclePrivacy(1) // alle → lokal
	if a.st.LLM().Privacy != llm.PrivacyLocal || a.aiLevel() != llm.PrivacyLocal {
		t.Fatalf("Einstellung: %q", a.st.LLM().Privacy)
	}
	a.cyclePrivacy(1) // lokal → aus
	if _, err := a.agentClientFor(); err == nil || !strings.Contains(err.Error(), "ausgeschaltet") {
		t.Fatalf("aus: %v", err)
	}
	if m := a.lastRequestModal(); m == nil {
		t.Fatal("Ansicht fehlt")
	}
}
