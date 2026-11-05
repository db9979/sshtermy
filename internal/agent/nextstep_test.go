package agent

import (
	"context"
	"encoding/json"
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
	st, err := a.NextStep(context.Background(), "Docker installieren", nil, nil, nil, Context{OS: "Ubuntu"})
	if err != nil || st.Command != "docker --version" {
		t.Fatalf("%+v %v", st, err)
	}
	if len(*calls) != 2 || (*calls)[0] < 2048 {
		t.Fatalf("Aufrufe/Max-Tokens: %v", *calls)
	}
}

func TestNextStepShowsAnswer(t *testing.T) {
	a, _ := fakeQwen(t, "Ich würde docker --version ausführen.")
	_, err := a.NextStep(context.Background(), "x", nil, nil, nil, Context{})
	if err == nil || !strings.Contains(err.Error(), "docker --version ausführen") {
		t.Fatal(err)
	}
}
