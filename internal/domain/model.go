// Package domain enthält die reinen Datenmodelle von sshtermy. Kein Paket hier
// kennt SSH, HTTP, Krypto oder die Oberfläche (Clean Architecture: innerster Ring).
//
// Geheimnisse (Passwörter, Private Keys, Passphrasen, API-Keys) stehen nur in
// Feldern vom Typ Secret. Der gesamte Store wird beim Speichern als Ganzes mit
// AES-256-GCM verschlüsselt (siehe internal/vault); Secret sorgt zusätzlich dafür,
// dass ein Geheimnis nie versehentlich in Logs oder Fehlermeldungen landet.
package domain

import (
	"fmt"
	"time"
)

// Secret ist ein Geheimnis im Klartext, nur im Arbeitsspeicher. String() und
// GoString() geben nie den Inhalt aus, nur ob er gesetzt ist.
type Secret string

func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return "••••••"
}
func (s Secret) GoString() string { return `domain.Secret("••••••")` }

// Reveal gibt den Klartext zurück. Bewusst ein eigener Name, damit jede Stelle,
// die ein Geheimnis wirklich braucht, im Code leicht zu finden ist.
func (s Secret) Reveal() string { return string(s) }
func (s Secret) IsSet() bool     { return s != "" }

// AuthMethod: wie sich sshtermy am Server anmeldet.
type AuthMethod string

const (
	AuthPassword AuthMethod = "password" // Passwort (verschlüsselt im Store)
	AuthKeyFile  AuthMethod = "keyfile"  // Private Key, als Pfad oder eingebettet (verschlüsselt)
	AuthAgent    AuthMethod = "agent"    // ssh-agent / Windows OpenSSH-Agent / Pageant-kompatibel
)

// Connection ist ein gespeichertes SSH-Ziel.
type Connection struct {
	ID       string     `json:"id"`
	Alias    string     `json:"alias"`
	Host     string     `json:"host"`
	Port     int        `json:"port"`
	Username string     `json:"username"`
	Auth     AuthMethod `json:"auth"`
	Group    string     `json:"group,omitempty"` // z. B. "Produktion", "Homelab"
	Tags     []string   `json:"tags,omitempty"`

	Password      Secret `json:"password,omitempty"`       // AuthPassword
	KeyPath       string `json:"key_path,omitempty"`       // AuthKeyFile: Datei auf der Platte …
	KeyPEM        Secret `json:"key_pem,omitempty"`        // … oder eingebettet im Store
	KeyPassphrase Secret `json:"key_passphrase,omitempty"` // optional für verschlüsselte Keys

	JumpHostID string   `json:"jump_host_id,omitempty"` // ProxyJump über eine andere Connection
	TunnelIDs  []string `json:"tunnel_ids,omitempty"`   // Tunnel, die mit dieser Verbindung starten
	KeepAlive  int      `json:"keepalive_s,omitempty"`  // Sekunden, 0 = Standard (30)

	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used,omitempty"`
}

// Addr liefert host:port (IPv6 korrekt geklammert).
func (c Connection) Addr() string {
	p := c.Port
	if p == 0 {
		p = 22
	}
	if len(c.Host) > 0 && c.Host[0] != '[' && containsColon(c.Host) {
		return fmt.Sprintf("[%s]:%d", c.Host, p)
	}
	return fmt.Sprintf("%s:%d", c.Host, p)
}

func containsColon(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return true
		}
	}
	return false
}

// TunnelKind entspricht den OpenSSH-Schaltern -L, -R und -D.
type TunnelKind string

const (
	TunnelLocal   TunnelKind = "local"   // -L local_port:remote_host:remote_port
	TunnelRemote  TunnelKind = "remote"  // -R remote_port:local_host:local_port
	TunnelDynamic TunnelKind = "dynamic" // -D port (SOCKS5)
)

// Tunnel beschreibt ein Port-Forwarding. Für "Web-Service"-Profile (One-Click)
// sind WebScheme/WebPath gesetzt: dann zeigt sshtermy nach dem Start die lokale
// URL an und kann sie im Browser öffnen.
type Tunnel struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	ConnectionID string     `json:"connection_id"`
	Kind         TunnelKind `json:"kind"`

	BindHost string `json:"bind_host,omitempty"` // Standard 127.0.0.1 (nie 0.0.0.0 ohne Absicht)
	BindPort int    `json:"bind_port"`           // 0 = freien Port wählen (nur local/dynamic)

	TargetHost string `json:"target_host,omitempty"` // local: Ziel am Server; remote: Ziel lokal
	TargetPort int    `json:"target_port,omitempty"`

	WebScheme   string `json:"web_scheme,omitempty"` // "http" | "https" → One-Click Web Service
	WebPath     string `json:"web_path,omitempty"`   // z. B. "/admin"
	AutoOpen    bool   `json:"auto_open,omitempty"`  // Browser nach Start öffnen
	AutoStart   bool   `json:"auto_start,omitempty"` // mit der Verbindung starten
}

// Spec gibt die OpenSSH-Schreibweise zurück, z. B. "-L 8080:localhost:80".
func (t Tunnel) Spec() string {
	switch t.Kind {
	case TunnelLocal:
		return fmt.Sprintf("-L %d:%s:%d", t.BindPort, t.TargetHost, t.TargetPort)
	case TunnelRemote:
		return fmt.Sprintf("-R %d:%s:%d", t.BindPort, t.TargetHost, t.TargetPort)
	case TunnelDynamic:
		return fmt.Sprintf("-D %d", t.BindPort)
	}
	return "?"
}

// IsWebService: Tunnel mit lokal erreichbarer URL.
func (t Tunnel) IsWebService() bool {
	return t.WebScheme != "" && (t.Kind == TunnelLocal)
}

// LLMConfig: jeder OpenAI-kompatible Endpunkt (/v1/chat/completions):
// OpenAI, LocalAI, vLLM, SGLang, Ollama (/v1), LiteLLM, LM Studio …
type LLMConfig struct {
	BaseURL     string  `json:"base_url"` // z. B. http://dgx-spark:8000/v1
	APIKey      Secret  `json:"api_key,omitempty"`
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	TimeoutSec  int     `json:"timeout_s"`

	// Feineinstellungen der Assistenz
	SuggestEnabled bool `json:"suggest_enabled"` // Ghost-Text beim Tippen
	SuggestDelayMS int  `json:"suggest_delay_ms"`
	ContextLines   int  `json:"context_lines"`   // letzte N Zeilen des Terminals
	SendScreen     bool `json:"send_screen"`     // Bildschirm überhaupt mitschicken
	GoalStepsMin   int  `json:"goal_steps_min"`  // Ratenlimit Ziel-Modus pro Minute
	AllowInsecure  bool `json:"allow_insecure"`  // http:// zu Nicht-Localhost erlauben
}

// DefaultLLM: sinnvolle Standardwerte (lokales Modell, nichts verlässt das Netz).
func DefaultLLM() LLMConfig {
	return LLMConfig{
		BaseURL: "http://localhost:11434/v1", Model: "qwen2.5-coder:7b",
		MaxTokens: 400, Temperature: 0.2, TimeoutSec: 60,
		SuggestEnabled: true, SuggestDelayMS: 450, ContextLines: 80, SendScreen: true,
		GoalStepsMin: 6,
	}
}

// Settings: Oberflächen-Einstellungen.
type Settings struct {
	Theme        string `json:"theme"`         // "nacht" | "hell" | "kontrast"
	UseDPAPI     bool   `json:"use_dpapi"`     // Windows: Store-Schlüssel zusätzlich per DPAPI
	LockAfterMin int    `json:"lock_after_min"` // 0 = nie automatisch sperren
	Language     string `json:"language"`      // "de" | "en"
}

// Data ist alles, was verschlüsselt auf der Platte liegt.
type Data struct {
	Version     int          `json:"version"`
	Connections []Connection `json:"connections"`
	Tunnels     []Tunnel     `json:"tunnels"`
	LLM         LLMConfig    `json:"llm"`
	Settings    Settings     `json:"settings"`
}

// NewData: leerer Store mit Standardwerten.
func NewData() *Data {
	return &Data{Version: 1, LLM: DefaultLLM(), Settings: Settings{Theme: "nacht", UseDPAPI: true, Language: "de"}}
}
