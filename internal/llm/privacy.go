package llm

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// KI-Datenschutz: welche Endpunkte dürfen Daten bekommen, und was ging
// zuletzt hinaus (zum Nachsehen in der Oberfläche).

// Datenschutz-Stufen (LLMConfig.Privacy und Tags an Verbindungen).
const (
	PrivacyAll   = ""      // alle eingerichteten Anbieter
	PrivacyLocal = "lokal" // nur Endpunkte im eigenen Netz
	PrivacyOff   = "aus"   // keine KI
)

// Stricter: die strengere von zwei Stufen.
func Stricter(a, b string) string {
	rank := map[string]int{PrivacyAll: 0, PrivacyLocal: 1, PrivacyOff: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// ErrAIOff: für diese Verbindung (oder überhaupt) ist die KI aus.
var ErrAIOff = errors.New("KI aus")

// IsLocalEndpoint: liegt der Endpunkt im eigenen Netz? Loopback, private und
// link-lokale Adressen, Namen ohne Punkt (dgx-spark) und die üblichen
// internen Endungen. Kein DNS-Nachschlagen: ein öffentlicher Name zählt
// immer als außerhalb, auch wenn er gerade auf eine interne Adresse zeigt.
func IsLocalEndpoint(baseURL string) bool {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	h := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if h == "" {
		return false
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	if h == "localhost" || !strings.Contains(h, ".") {
		return true
	}
	for _, s := range []string{".local", ".lan", ".internal", ".intranet", ".home.arpa", ".localdomain", ".localhost"} {
		if strings.HasSuffix(h, s) {
			return true
		}
	}
	return false
}

// NewWithPrivacy wie New, behält aber nur Endpunkte, die die Stufe erlaubt.
func NewWithPrivacy(cfg domain.LLMConfig, level string) (*Client, error) {
	if level == PrivacyOff {
		return nil, ErrAIOff
	}
	c, err := New(cfg)
	if err != nil || level != PrivacyLocal {
		return c, err
	}
	var keep []*conn
	for _, k := range c.conns {
		if IsLocalEndpoint(baseURL(k.cfg)) {
			keep = append(keep, k)
		}
	}
	if len(keep) == 0 {
		return nil, errors.New(i18n.T("Nur lokale KI erlaubt, aber kein Endpunkt im eigenen Netz eingerichtet (z. B. Ollama auf localhost)"))
	}
	c.conns = keep
	return c, nil
}

// ---------------------------------------------------------------- letzte Anfrage

// Request: was zuletzt an eine KI ging (nur im Speicher, nie auf Platte).
type Request struct {
	At       time.Time
	Provider domain.LLMProvider
	Host     string
	Model    string
	Messages []Message
}

var last struct {
	mu  sync.Mutex
	req *Request
}

// LastRequest: die letzte Anfrage oder nil.
func LastRequest() *Request {
	last.mu.Lock()
	defer last.mu.Unlock()
	if last.req == nil {
		return nil
	}
	r := *last.req
	r.Messages = append([]Message(nil), r.Messages...)
	return &r
}

// remember: Nachrichten aus dem JSON-Körper (OpenAI und Anthropic haben
// beide "messages", Anthropic zusätzlich "system").
func remember(cfg domain.LLMConfig, host string, raw []byte) {
	var b struct {
		System   any       `json:"system"`
		Messages []Message `json:"messages"`
	}
	if json.Unmarshal(raw, &b) != nil {
		return
	}
	msgs := b.Messages
	if s, ok := b.System.(string); ok && s != "" {
		msgs = append([]Message{{Role: "system", Content: s}}, msgs...)
	}
	last.mu.Lock()
	last.req = &Request{At: time.Now(), Provider: cfg.Provider, Host: host, Model: cfg.Model, Messages: msgs}
	last.mu.Unlock()
}
