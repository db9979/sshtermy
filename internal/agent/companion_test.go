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

// fakeSeen: wie fakeQwen, merkt sich aber den Text der Anfragen.
func fakeSeen(t *testing.T, reply string) (*Agent, *[]string) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, m := range req.Messages {
			seen = append(seen, m.Content)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"message": map[string]any{"role": "assistant", "content": reply}, "finish_reason": "stop",
		}}})
	}))
	t.Cleanup(srv.Close)
	c, err := llm.New(domain.LLMConfig{BaseURL: srv.URL + "/v1", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	return &Agent{C: c}, &seen
}

func TestCompanionComment(t *testing.T) {
	a, seen := fakeSeen(t, `{"kommentar":"Tippfehler in Zeile 14: proxy_pas heißt proxy_pass.",
		"befehle":[{"befehl":"sudo nginx -t","grund":"prüfen"},{"befehl":"rm -rf /etc/nginx","grund":"x"},{"befehl":"a\nb"}],
		"merken":"Seite heißt shop"}`)
	ev := CompanionEvent{Kind: EventFail, Command: "mysql --password=geheim123 -e x", Output: "nginx: [emerg] unknown directive"}
	r, err := a.Comment(context.Background(), ev, Context{OS: "Ubuntu", Screen: "token=abcdef0123456789"})
	if err != nil || r.Quiet || !strings.Contains(r.Text, "proxy_pass") {
		t.Fatalf("%+v %v", r, err)
	}
	if len(r.Commands) != 2 || len(r.Commands[1].Danger) == 0 {
		t.Fatalf("Befehle: %+v", r.Commands)
	}
	if r.Fact != "Seite heißt shop" || r.Tokens <= 0 {
		t.Fatalf("Fakt/Tokens: %+v", r)
	}
	all := strings.Join(*seen, "\n")
	if strings.Contains(all, "geheim123") || !strings.Contains(all, "Anlass: fehler") {
		t.Fatalf("Geheimnis gesendet oder Anlass fehlt:\n%s", all)
	}

	// Filter aus (nur eigene KI): Geheimnisse bleiben stehen
	a2, seen2 := fakeSeen(t, `{"still": true}`)
	r2, err := a2.Comment(context.Background(), ev, Context{Raw: true})
	if err != nil || !r2.Quiet {
		t.Fatalf("still nicht erkannt: %+v %v", r2, err)
	}
	if !strings.Contains(strings.Join(*seen2, "\n"), "geheim123") {
		t.Fatal("Raw entfernt trotzdem")
	}
}

func TestCompanionIntentAndRecap(t *testing.T) {
	a, _ := fakeSeen(t, `{"absicht":"HTTPS für shop.example.de einrichten","belege":"certbot, ufw",
		"schritte":[{"befehl":"sudo ufw allow 443/tcp","grund":"Firewall"},{"befehl":"sudo certbot --nginx -d shop.example.de"}]}`)
	in, err := a.GuessIntent(context.Background(), Context{Commands: []string{"sudo apt install certbot"}})
	if err != nil || in.Goal == "" || len(in.Steps) != 2 {
		t.Fatalf("%+v %v", in, err)
	}
	a2, _ := fakeSeen(t, `{"keine": true}`)
	if in, _ := a2.GuessIntent(context.Background(), Context{}); in.Goal != "" {
		t.Fatal("keine Absicht nicht erkannt")
	}
	a3, _ := fakeSeen(t, `{"zusammenfassung":["nginx repariert"],"merken":["Backend ist php8.3-fpm","password=xyz"],"runbook":"nginx reparieren"}`)
	r, err := a3.RecapSession(context.Background(), "$ sudo nginx -t\nok\n", Context{})
	if err != nil || len(r.Points) != 1 || len(r.Facts) != 1 || r.Runbook != "nginx reparieren" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestLooksFailedServices(t *testing.T) {
	for _, out := range []string{
		"× nginx.service\n     Active: failed (Result: exit-code) since Mon",
		"nginx: [emerg] unknown directive \"proxy_pas\" in /etc/nginx/sites-enabled/shop:14\nnginx: configuration file /etc/nginx/nginx.conf test failed",
	} {
		if ok, _ := LooksFailed(out); !ok {
			t.Errorf("nicht erkannt: %q", out)
		}
	}
	if ok, _ := LooksFailed("● nginx.service\n     Active: active (running)"); ok {
		t.Error("laufender Dienst als Fehler")
	}
}
