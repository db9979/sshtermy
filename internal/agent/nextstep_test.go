package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/llm"
)

// fakeQwen antwortet wie ein Qwen3-Reasoning-Modell hinter vLLM: erst Prosa
// mit Gedanken ohne JSON, nach der Korrektur das saubere Objekt.
func fakeQwen(t *testing.T, replies ...string) (*Agent, *[]int) {
	var maxTok []int
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MaxTokens int `json:"max_tokens"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		maxTok = append(maxTok, req.MaxTokens)
		txt := replies[min(n, len(replies)-1)]
		n++
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"message":       map[string]any{"role": "assistant", "content": txt, "reasoning_content": "denke…"},
			"finish_reason": "stop",
		}}})
	}))
	t.Cleanup(srv.Close)
	c, err := llm.New(domain.LLMConfig{BaseURL: srv.URL + "/v1", Model: "qwen3"})
	if err != nil {
		t.Fatal(err)
	}
	return &Agent{C: c}, &maxTok
}

func TestNextStepRetry(t *testing.T) {
	a, calls := fakeQwen(t, "Okay, ich denke, man sollte Docker prüfen.\n</think>\nIch würde docker --version ausführen.",
		`{"befehl":"docker --version","grund":"prüfen"}`)
	st, err := a.NextStep(context.Background(), "Docker installieren", nil, nil, nil, "", Context{OS: "Ubuntu"})
	if err != nil || st.Command != "docker --version" {
		t.Fatalf("%+v %v", st, err)
	}
	if len(*calls) != 2 || (*calls)[0] < 2048 {
		t.Fatalf("Aufrufe/Max-Tokens: %v", *calls)
	}
}

func TestNextStepShowsAnswer(t *testing.T) {
	a, _ := fakeQwen(t, "Ich würde docker --version ausführen.")
	_, err := a.NextStep(context.Background(), "x", nil, nil, nil, "", Context{})
	if err == nil || !strings.Contains(err.Error(), "docker --version ausführen") {
		t.Fatal(err)
	}
}

// Gitea-Fall: das Modell packt einen Konfigurationsabschnitt als Heredoc in
// den Befehl. Die Nachfrage nennt den Fehler und bietet "datei" an; die
// korrigierte Antwort legt eine Datei an.
func TestNextStepHeredocRetry(t *testing.T) {
	heredoc := `{"befehl": "docker cp /dev/stdin gitea:/data/gitea/conf/app.ini << 'EOF'\n[migrations]\nALLOW_LOCALNETWORKS = true\n\n[mirror]\nENABLED = true\nEOF", "grund": "Abschnitte eintragen"}`
	var prompts []string
	n := 0
	replies := []string{heredoc, `{"datei": {"name": "gitea-extra.ini", "inhalt": "[migrations]\nALLOW_LOCALNETWORKS = true\n"}, "grund": "Abschnitt als Datei"}`}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []llm.Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		prompts = append(prompts, req.Messages[len(req.Messages)-1].Content)
		txt := replies[min(n, len(replies)-1)]
		n++
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"message": map[string]any{"role": "assistant", "content": txt}, "finish_reason": "stop",
		}}})
	}))
	t.Cleanup(srv.Close)
	c, err := llm.New(domain.LLMConfig{BaseURL: srv.URL + "/v1", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	a := &Agent{C: c}
	st, err := a.NextStep(context.Background(), "Gitea app.ini ergänzen", nil, nil, nil, "~/.sshtermy-agent/ab12", Context{OS: "Ubuntu"})
	if err != nil || st.File == nil || st.File.Name != "gitea-extra.ini" {
		t.Fatalf("%+v %v", st, err)
	}
	if !strings.Contains(prompts[0], "~/.sshtermy-agent/ab12") {
		t.Fatalf("Arbeitsordner fehlt im Prompt: %s", prompts[0])
	}
	if len(prompts) != 2 || !strings.Contains(prompts[1], "Zeilenumbrüche") || !strings.Contains(prompts[1], `"datei"`) {
		t.Fatalf("Nachfrage nennt den Fehler nicht: %q", prompts)
	}

	// Ohne Arbeitsordner: kein "datei" im Hinweis, und bleibt das Modell
	// stur, sagt die Meldung warum.
	n, prompts, replies = 0, nil, []string{heredoc}
	_, err = a.NextStep(context.Background(), "Gitea app.ini ergänzen", nil, nil, nil, "", Context{OS: "Ubuntu"})
	if err == nil || !strings.Contains(err.Error(), "mehrere Zeilen") || !strings.Contains(err.Error(), "docker cp") {
		t.Fatal(err)
	}
	if strings.Contains(prompts[1], `{"datei"`) || !strings.Contains(prompts[1], "printf") {
		t.Fatalf("Hinweis ohne Arbeitsordner: %s", prompts[1])
	}
	// Datei ohne Arbeitsordner wird nicht angenommen.
	if st, err := parseAuto(replies[0], "unix", false); err == nil && st.File != nil {
		t.Fatal("Datei ohne Arbeitsordner angenommen")
	}
}

func TestRepairJSON(t *testing.T) {
	var v map[string]any
	if !strictJSON("{\"befehl\": \"ls -la\",\n \"grund\": \"Zeile 1\nZeile 2\",}", &v) || v["befehl"] != "ls -la" {
		t.Fatalf("%v", v)
	}
	// roher Umbruch im Befehl: lesbar, aber als mehrzeilig abgelehnt
	_, err := parseStep("{\"befehl\": \"cat <<EOF\nx\nEOF\"}", "unix")
	var se *StepError
	if !errors.As(err, &se) || !errors.Is(err, ErrUnreadable) {
		t.Fatal(err)
	}
	if _, err := parseStep(`{"befehl": "`+strings.Repeat("a", 400)+`"}`, "unix"); err == nil || !strings.Contains(err.Error(), "zu lang") {
		t.Fatal(err)
	}
}
