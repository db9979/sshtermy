package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/llm"
)

// Ghost-Text: Hausregeln im Systemteil, fester Teil vorn und das Getippte
// am Ende (Prefix-Cache), nur die letzten Bildschirmzeilen, schnelles Modell.
func TestCompletePromptShape(t *testing.T) {
	var got struct {
		Model    string        `json:"model"`
		Messages []llm.Message `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"zeile\":\"systemctl status nginx\"}"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	c, err := llm.New(domain.LLMConfig{BaseURL: srv.URL + "/v1", Model: "gross", FastModel: "klein"})
	if err != nil {
		t.Fatal(err)
	}
	var screen strings.Builder
	for i := 1; i <= 80; i++ {
		fmt.Fprintf(&screen, "zeile%02d\n", i)
	}
	a := &Agent{C: c, Rules: "nie reboot vorschlagen"}
	rest, err := a.Complete(context.Background(), "systemctl st", Context{Host: "web1", Screen: screen.String()})
	if err != nil || rest != "atus nginx" {
		t.Fatalf("%q %v", rest, err)
	}
	if got.Model != "klein" {
		t.Errorf("Ghost-Text ging an %q statt an das schnelle Modell", got.Model)
	}
	sysMsg, user := got.Messages[0].Content, got.Messages[1].Content
	if !strings.Contains(sysMsg, "Hausregeln") || !strings.HasSuffix(sysMsg, "nie reboot vorschlagen") {
		t.Errorf("Hausregeln fehlen im Systemteil: %q", sysMsg)
	}
	if !strings.HasSuffix(user, "Bisher getippt: systemctl st") || !strings.HasPrefix(user, "<<<") {
		t.Errorf("Reihenfolge falsch: %q", user)
	}
	if strings.Contains(user, "zeile55") || !strings.Contains(user, "zeile56") || !strings.Contains(user, "zeile80") {
		t.Errorf("nicht die letzten %d Zeilen: %q", ghostLines, user)
	}
}

func TestLastLines(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"a\nb\nc\n", 2, "b\nc\n"},
		{"a\nb\nc", 2, "b\nc"},
		{"a\nb", 5, "a\nb"},
		{"", 3, ""},
		{"a\nb", 0, "a\nb"},
	}
	for _, c := range cases {
		if got := lastLines(c.in, c.n); got != c.want {
			t.Errorf("lastLines(%q, %d) = %q, erwartet %q", c.in, c.n, got, c.want)
		}
	}
}

func TestSysWithoutRules(t *testing.T) {
	a := &Agent{}
	if a.sys("x") != sys("x") {
		t.Fatal("ohne Hausregeln darf sich der Prompt nicht ändern")
	}
}
