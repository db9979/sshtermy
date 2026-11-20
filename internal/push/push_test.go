package push

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSend(t *testing.T) {
	var got *http.Request
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, body = r, nil
		body, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()
	ctx := context.Background()
	m := Message{Title: "sshTermy · Prüfung", Body: "web2: Platte voll", Urgent: true}

	if err := Send(ctx, Config{Kind: Ntfy, URL: srv.URL + "/thema", Token: "tk"}, m); err != nil {
		t.Fatal(err)
	}
	title, _ := new(mime.WordDecoder).DecodeHeader(got.Header.Get("Title"))
	if got.URL.Path != "/thema" || string(body) != m.Body || title != m.Title || got.Header.Get("Priority") != "high" || got.Header.Get("Authorization") != "Bearer tk" {
		t.Errorf("ntfy: %s %q %q %v", got.URL.Path, body, title, got.Header)
	}

	if err := Send(ctx, Config{Kind: Gotify, URL: srv.URL + "/", Token: "app"}, m); err != nil {
		t.Fatal(err)
	}
	var g map[string]any
	_ = json.Unmarshal(body, &g)
	if got.URL.Path != "/message" || got.Header.Get("X-Gotify-Key") != "app" || g["message"] != m.Body || g["priority"] != float64(8) {
		t.Errorf("gotify: %s %v", got.URL.Path, g)
	}

	if err := Send(ctx, Config{Kind: Webhook, URL: srv.URL + "/hook"}, m); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(body, &g)
	if g["title"] != m.Title || g["source"] != "sshTermy" || got.Header.Get("Authorization") != "" {
		t.Errorf("webhook: %v", g)
	}

	for _, c := range []Config{{Kind: Ntfy, URL: "https://ntfy.sh/"}, {Kind: Gotify, URL: "https://g.example"}, {Kind: "x", URL: "https://a"}, {Kind: Webhook, URL: "ftp://a"}} {
		if c.Check() == nil {
			t.Errorf("%+v sollte nicht gehen", c)
		}
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", http.StatusForbidden) }))
	defer bad.Close()
	if err := Send(ctx, Config{Kind: Webhook, URL: bad.URL}, m); err == nil {
		t.Error("403 nicht gemeldet")
	}
}
