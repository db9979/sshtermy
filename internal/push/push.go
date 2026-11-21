// Package push schickt kurze Meldungen aufs Handy: über ntfy (eigener
// Server oder ntfy.sh), Gotify oder einen beliebigen Webhook (JSON).
// sshTermy nutzt das, wenn niemand am Rechner ist: langer Befehl fertig,
// Zeitplan gelaufen, Logwache, Agent wartet auf Freigabe. Den Text hat der
// Aufrufer schon gekürzt und von Geheimnissen befreit.
package push

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Arten.
const (
	Ntfy    = "ntfy"
	Gotify  = "gotify"
	Webhook = "webhook"
)

// Config: Ziel der Meldungen.
type Config struct {
	Kind  string // Ntfy, Gotify, Webhook
	URL   string // ntfy: https://ntfy.sh/thema · Gotify: https://gotify.example · Webhook: volle Adresse
	Token string // ntfy: Zugangstoken (optional) · Gotify: App-Token · Webhook: Bearer (optional)
}

// Message: eine Meldung.
type Message struct {
	Title    string
	Body     string
	Urgent   bool   // Fehler, Befund, Freigabe nötig: höhere Priorität
	Click    string // optional: Adresse beim Antippen (ntfy)
	Category string // z. B. "befehl", "zeitplan", "freigabe" (Webhook)
}

// MinPublicTopic: so lang muss ein Thema auf ntfy.sh ohne Token sein.
const MinPublicTopic = 16

// RandomNtfyURL: ntfy.sh mit zufälligem, nicht erratbarem Thema.
func RandomNtfyURL() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "https://ntfy.sh/sshtermy-" + hex.EncodeToString(b)
}

// Client: in Tests austauschbar.
var Client = &http.Client{Timeout: 15 * time.Second}

// Check: Einstellungen plausibel? (beim Einrichten, mit Themenlänge)
func (c Config) Check() error { return c.check(true) }

func (c Config) check(setup bool) error {
	u, err := url.Parse(strings.TrimSpace(c.URL))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New(i18n.T("Adresse mit http:// oder https:// angeben"))
	}
	switch c.Kind {
	case Ntfy:
		topic := strings.Trim(u.Path, "/")
		if topic == "" {
			return errors.New(i18n.T("ntfy braucht ein Thema in der Adresse, z. B. https://ntfy.sh/mein-geheimes-thema"))
		}
		// Auf dem öffentlichen ntfy.sh liest jeder mit, der das Thema kennt.
		if setup && strings.EqualFold(u.Hostname(), "ntfy.sh") && c.Token == "" && len(topic) < MinPublicTopic {
			return fmt.Errorf(i18n.T("Auf ntfy.sh kann jeder ein kurzes Thema erraten und mitlesen: mindestens %d Zeichen, am besten zufällig"), MinPublicTopic)
		}
	case Gotify:
		if c.Token == "" {
			return errors.New(i18n.T("Gotify braucht ein App-Token"))
		}
	case Webhook:
	default:
		return errors.New(i18n.T("Art unbekannt"))
	}
	return nil
}

// Send schickt eine Meldung.
func Send(ctx context.Context, c Config, m Message) error {
	if err := c.check(false); err != nil { // alte, kurze Themen senden weiter
		return err
	}
	base := strings.TrimRight(strings.TrimSpace(c.URL), "/")
	var req *http.Request
	var err error
	switch c.Kind {
	case Ntfy:
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, base, strings.NewReader(m.Body))
		if err != nil {
			return err
		}
		// Kopfzeilen sind nur ASCII sicher: ntfy versteht RFC 2047
		req.Header.Set("Title", mimeWord(m.Title))
		req.Header.Set("Tags", "computer")
		if m.Urgent {
			req.Header.Set("Priority", "high")
			req.Header.Set("Tags", "warning,computer")
		}
		if m.Click != "" {
			req.Header.Set("Click", m.Click)
		}
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
	case Gotify:
		prio := 5
		if m.Urgent {
			prio = 8
		}
		b, _ := json.Marshal(map[string]any{"title": m.Title, "message": m.Body, "priority": prio})
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, base+"/message", bytes.NewReader(b))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Gotify-Key", c.Token)
	case Webhook:
		b, _ := json.Marshal(map[string]any{"source": "sshTermy", "title": m.Title, "message": m.Body, "urgent": m.Urgent, "category": m.Category,
			"text": m.Title + ": " + m.Body}) // „text“: Slack, Mattermost, Rocket.Chat
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, base, bytes.NewReader(b))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
	}
	req.Header.Set("User-Agent", "sshTermy")
	resp, err := Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

// mimeWord: Umlaute im Titel als =?UTF-8?B?…?= (RFC 2047).
func mimeWord(s string) string {
	for _, r := range s {
		if r > 126 || r < 32 {
			return mimeB(s)
		}
	}
	return s
}
