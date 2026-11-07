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
func (s Secret) IsSet() bool    { return s != "" }

// AuthMethod: wie sich sshtermy am Server anmeldet.
type AuthMethod string

const (
	AuthPassword AuthMethod = "password" // Passwort (verschlüsselt im Store)
	AuthKeyFile  AuthMethod = "keyfile"  // Private Key, als Pfad oder eingebettet (verschlüsselt)
	AuthAgent    AuthMethod = "agent"    // ssh-agent / Windows OpenSSH-Agent / Pageant-kompatibel
	// AuthAuto: wie das ssh-Kommando: erst Agent und Standard-Keys
	// (~/.ssh/id_*), dann Passwort-Abfrage. Für Schnellverbindungen.
	AuthAuto AuthMethod = "auto"
	// AuthKerberos: Anmeldung mit dem Kerberos-Ticket der Firmen-Domäne
	// (gssapi-with-mic): Windows mit Domänen-Konto, sonst nach kinit.
	AuthKerberos AuthMethod = "kerberos"
)

// DeviceKind: was am anderen Ende hängt. Netzwerkgeräte (Switches, Router,
// Firewalls) vertragen oft nur einen einzigen Kanal und keine Exec-Befehle;
// sshtermy verzichtet dort auf die unsichtbare Systemabfrage.
type DeviceKind string

const (
	DeviceAuto    DeviceKind = ""        // am Server-Banner erkennen, im Zweifel ausprobieren
	DeviceServer  DeviceKind = "server"  // Linux/macOS/Windows mit normaler Shell
	DeviceNetwork DeviceKind = "network" // Switch/Router (Aruba, Cisco, MikroTik …)
)

// Connection ist ein gespeichertes SSH-Ziel.
type Connection struct {
	ID       string     `json:"id"`
	Alias    string     `json:"alias"`
	Host     string     `json:"host"`
	Port     int        `json:"port"`
	Username string     `json:"username"`
	Auth     AuthMethod `json:"auth"`
	Group    string     `json:"group,omitempty"` // Ordner, verschachtelt mit „ / “, z. B. "Produktion / Web"
	Tags     []string   `json:"tags,omitempty"`
	Order    float64    `json:"order,omitempty"` // Reihenfolge im Ordner (0 = alphabetisch)

	Password      Secret `json:"password,omitempty"`       // AuthPassword
	KeyPath       string `json:"key_path,omitempty"`       // AuthKeyFile: Datei auf der Platte …
	KeyPEM        Secret `json:"key_pem,omitempty"`        // … oder eingebettet im Store
	KeyPassphrase Secret `json:"key_passphrase,omitempty"` // optional für verschlüsselte Keys

	// Keychain: Passwort bzw. Key-Passphrase liegen nicht im Store, sondern
	// im Schlüsselbund des Betriebssystems (macOS-Schlüsselbund, Windows-
	// Anmeldeinformationsverwaltung). Jeder Rechner hat seinen eigenen
	// Eintrag; der Sync überträgt nur diesen Schalter. Fehlt der Eintrag,
	// fragt sshTermy beim Verbinden und legt die Antwort dort ab.
	Keychain bool `json:"keychain,omitempty"`

	JumpHostID string `json:"jump_host_id,omitempty"` // ProxyJump über eine andere Connection
	// RelayID/RelaySystem: über einen Vermittler (eigener sshtermy
	// vermittler oder Speech on DGX Spark) zum sshd eines Systems, das selbst
	// die Verbindung zum Vermittler hält (Anker). Host ist dann nur Anzeige.
	RelayID     string   `json:"relay_id,omitempty"`
	RelaySystem string   `json:"relay_system,omitempty"`
	TunnelIDs   []string `json:"tunnel_ids,omitempty"`  // Tunnel, die mit dieser Verbindung starten
	KeepAlive   int      `json:"keepalive_s,omitempty"` // Sekunden, 0 = Standard (30)

	// ForwardAgent: den lokalen SSH-Agent an den Server weiterreichen (wie
	// ssh -A). Dort funktionieren dann git, ssh und scp mit den eigenen
	// Keys, ohne sie auf den Server zu kopieren. Nur für vertraute Server.
	ForwardAgent bool `json:"forward_agent,omitempty"`
	// ForwardX11: grafische Programme vom Server auf dem lokalen X-Server
	// zeigen (wie ssh -X). Braucht lokal einen X-Server (Linux-Desktop,
	// XQuartz, VcXsrv/X410/WSLg).
	ForwardX11 bool `json:"forward_x11,omitempty"`
	// CertPath: SSH-Zertifikat zum Key (von einer CA signiert, *-cert.pub).
	// Leer = neben dem Keyfile suchen (<key>-cert.pub, wie OpenSSH).
	CertPath string     `json:"cert_path,omitempty"`
	Device   DeviceKind `json:"device,omitempty"` // Server oder Netzwerkgerät, leer = automatisch

	// Persist: beim Verbinden immer in dieselbe tmux-/screen-Sitzung
	// ("auto", "tmux", "screen"; leer = aus). PersistName: Sitzungsname,
	// leer = "sshtermy". PersistView: nur zuschauen (tmux attach -r), z. B.
	// um einer laufenden Sitzung von einem zweiten Rechner aus zu folgen.
	Persist     string `json:"persist,omitempty"`
	PersistName string `json:"persist_name,omitempty"`
	PersistView bool   `json:"persist_view,omitempty"`

	// Profile: was für ein System dahinter steckt (automatisch erkannt,
	// vom Nutzer ergänzbar). Geht in jeden KI-Prompt, damit Befehle im
	// richtigen Dialekt kommen. Liegt wie alles andere verschlüsselt im Store.
	Profile *SystemProfile `json:"profile,omitempty"`

	// Color: Akzentfarbe in Tab, Statuszeile und Liste ("red", "green" …).
	// Leer = automatisch aus Ordner/Tags/Alias (prod → rot, test → grün,
	// labor → blau), "none" = keine Farbe.
	Color string `json:"color,omitempty"`

	// QuickCmds: Schnellbefehle (⇧F1 …). Leer = Vorbelegung je Gerätetyp.
	QuickCmds []string `json:"quick_cmds,omitempty"`

	// Protocol: SSH (leer), Telnet oder serielle Konsole (USB-Konsolenkabel).
	// Bei Telnet und seriell meldet sshTermy sich selbst an, wenn Benutzer
	// und Passwort hinterlegt sind (Auth/Keyfile/Jump-Host gelten nur für SSH).
	Protocol   Protocol `json:"protocol,omitempty"`
	SerialPort string   `json:"serial_port,omitempty"` // COM3, /dev/ttyUSB0 … leer = automatisch (einziges USB-Kabel)
	Baud       int      `json:"baud,omitempty"`        // 0 = 9600
	Frame      string   `json:"frame,omitempty"`       // Datenbits, Parität, Stoppbits, leer = "8N1"

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty"` // letzte Änderung (Sync: neuere gewinnt)
	LastUsed  time.Time `json:"last_used,omitempty"`
}

// DefaultPersistName: Sitzungsname, wenn bei „Feste Sitzung“ keiner steht.
const DefaultPersistName = "sshtermy"

// PersistSession: Name der festen Sitzung (leer, wenn aus).
func (c Connection) PersistSession() string {
	if c.Persist == "" {
		return ""
	}
	if c.PersistName != "" {
		return c.PersistName
	}
	return DefaultPersistName
}

// Rev ist der Zeitstempel, nach dem der Sync entscheidet (alte Einträge ohne
// UpdatedAt zählen mit ihrem Anlagedatum).
func (c Connection) Rev() time.Time {
	if c.UpdatedAt.IsZero() {
		return c.CreatedAt
	}
	return c.UpdatedAt
}

// Protocol: wie sshTermy die Gegenstelle erreicht.
type Protocol string

const (
	ProtoSSH    Protocol = ""       // Standard
	ProtoTelnet Protocol = "telnet" // unverschlüsselt, für ältere Switches
	ProtoSerial Protocol = "serial" // serielle Konsole (USB-Konsolenkabel)
)

// Raw: kein SSH, also keine Exec-Abfragen, kein SFTP und keine Tunnel.
func (p Protocol) Raw() bool { return p == ProtoTelnet || p == ProtoSerial }

// DefaultBaud: übliche Geschwindigkeit von Switch-Konsolen (Aruba, Cisco, HPE).
const DefaultBaud = 9600

// SerialBaud und SerialFrame liefern die Einstellung samt Standardwert.
func (c Connection) SerialBaud() int {
	if c.Baud <= 0 {
		return DefaultBaud
	}
	return c.Baud
}
func (c Connection) SerialFrame() string {
	if c.Frame == "" {
		return "8N1"
	}
	return c.Frame
}

// DefaultPort: 22 für SSH, 23 für Telnet.
func (c Connection) DefaultPort() int {
	if c.Protocol == ProtoTelnet {
		return 23
	}
	return 22
}

// Target: Anzeige des Ziels, z. B. „admin@10.0.0.1:22“ oder bei seriellen
// Verbindungen „COM3 · 9600 8N1“.
func (c Connection) Target() string {
	a := c.ShowAddr()
	if c.Protocol != ProtoSerial && c.Username != "" {
		a = c.Username + "@" + a
	}
	if c.Protocol == ProtoTelnet {
		a = "telnet://" + a
	}
	return a
}

// Addr liefert host:port (IPv6 korrekt geklammert), bei seriellen
// Verbindungen Schnittstelle und Geschwindigkeit.
func (c Connection) Addr() string {
	if c.ViaRelay() {
		// Name, unter dem known_hosts den Host-Key merkt (kein echter Host)
		return c.RelaySystem + "." + c.RelayID + ".vermittler:22"
	}
	if c.Protocol == ProtoSerial {
		port := c.SerialPort
		if port == "" {
			port = "USB"
		}
		return fmt.Sprintf("%s · %d %s", port, c.SerialBaud(), c.SerialFrame())
	}
	p := c.Port
	if p == 0 {
		p = c.DefaultPort()
	}
	if len(c.Host) > 0 && c.Host[0] != '[' && containsColon(c.Host) {
		return fmt.Sprintf("[%s]:%d", c.Host, p)
	}
	return fmt.Sprintf("%s:%d", c.Host, p)
}

// ShowAddr: Adresse für die Anzeige. Über einen Vermittler der Name des
// Systems mit „↔“ statt des internen Namens für known_hosts.
func (c Connection) ShowAddr() string {
	if c.ViaRelay() {
		return c.Host + " ↔"
	}
	return c.Addr()
}

// ViaRelay: Verbindung läuft über einen Vermittler.
func (c Connection) ViaRelay() bool {
	return c.RelayID != "" && c.RelaySystem != "" && !c.Protocol.Raw()
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

	WebScheme string `json:"web_scheme,omitempty"` // "http" | "https" → One-Click Web Service
	WebPath   string `json:"web_path,omitempty"`   // z. B. "/admin"
	AutoOpen  bool   `json:"auto_open,omitempty"`  // Browser nach Start öffnen
	AutoStart bool   `json:"auto_start,omitempty"` // mit der Verbindung starten

	UpdatedAt time.Time `json:"updated_at,omitempty"` // letzte Änderung (Sync)
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

// LLMProvider: welche Art von KI-Dienst hinter einem Endpunkt steckt.
type LLMProvider string

const (
	// ProviderCompat: jeder OpenAI-kompatible Server (Spark mit vLLM/SGLang,
	// LiteLLM, LocalAI, LM Studio …). Leer, damit alte Stores so bleiben.
	ProviderCompat    LLMProvider = ""
	ProviderOllama    LLMProvider = "ollama"
	ProviderOpenAI    LLMProvider = "openai"
	ProviderAnthropic LLMProvider = "anthropic" // Claude (Messages-API)
)

// Providers: Reihenfolge in der Auswahl.
var Providers = []LLMProvider{ProviderCompat, ProviderOllama, ProviderOpenAI, ProviderAnthropic}

// DefaultBaseURL: Adresse, die beim Wählen des Anbieters vorbelegt wird.
func (p LLMProvider) DefaultBaseURL() string {
	switch p {
	case ProviderOllama:
		return "http://localhost:11434/v1"
	case ProviderOpenAI:
		return "https://api.openai.com/v1"
	case ProviderAnthropic:
		return "https://api.anthropic.com/v1"
	}
	return ""
}

// DefaultModel: Modell, das beim Wählen des Anbieters vorbelegt wird.
func (p LLMProvider) DefaultModel() string {
	switch p {
	case ProviderOllama:
		return "qwen2.5-coder:7b"
	case ProviderOpenAI:
		return "gpt-4.1-mini"
	case ProviderAnthropic:
		return "claude-haiku-5-5"
	}
	return ""
}

// NeedsKey: Cloud-Dienste brauchen immer einen API-Key.
func (p LLMProvider) NeedsKey() bool { return p == ProviderOpenAI || p == ProviderAnthropic }

// Label: Name in der Oberfläche (deutsch, wird beim Anzeigen übersetzt).
func (p LLMProvider) Label() string {
	switch p {
	case ProviderOllama:
		return "Ollama"
	case ProviderOpenAI:
		return "OpenAI"
	case ProviderAnthropic:
		return "Claude"
	}
	return "Spark / kompatibel"
}

// LLMBackup ist ein Ersatz-Anbieter, der einspringt, wenn der eigentliche
// Endpunkt nicht antwortet.
type LLMBackup struct {
	Provider LLMProvider `json:"provider,omitempty"`
	BaseURL  string      `json:"base_url,omitempty"`
	APIKey   Secret      `json:"api_key,omitempty"`
	Model    string      `json:"model,omitempty"`
}

// LLMConfig: der eigentliche KI-Endpunkt (Standard: OpenAI-kompatibel, z. B.
// der Spark mit vLLM) plus Ersatz-Anbieter in fester Reihenfolge.
type LLMConfig struct {
	Provider    LLMProvider `json:"provider,omitempty"`
	BaseURL     string      `json:"base_url"` // z. B. http://dgx-spark:8000/v1
	APIKey      Secret      `json:"api_key,omitempty"`
	Model       string      `json:"model"`
	MaxTokens   int         `json:"max_tokens"`
	Temperature float64     `json:"temperature"`
	TimeoutSec  int         `json:"timeout_s"`

	// Feineinstellungen der Assistenz
	SuggestEnabled bool `json:"suggest_enabled"` // Ghost-Text beim Tippen
	SuggestDelayMS int  `json:"suggest_delay_ms"`
	ContextLines   int  `json:"context_lines"`  // letzte N Zeilen des Terminals
	SendScreen     bool `json:"send_screen"`    // Bildschirm überhaupt mitschicken
	GoalStepsMin   int  `json:"goal_steps_min"` // Ratenlimit Ziel-Modus pro Minute
	AllowInsecure  bool `json:"allow_insecure"` // http:// zu Nicht-Localhost erlauben

	// Fallbacks: der Reihe nach, wenn der Endpunkt oben nicht antwortet.
	Fallbacks []LLMBackup `json:"fallbacks,omitempty"`
}

// Chain: der Endpunkt und alle eingerichteten Ersatz-Anbieter, jeweils als
// vollständige Einstellung (Tokens, Temperatur, Timeout vom Hauptendpunkt).
func (c LLMConfig) Chain() []LLMConfig {
	main := c
	main.Fallbacks = nil
	out := []LLMConfig{main}
	for _, b := range c.Fallbacks {
		if b.Model == "" && b.BaseURL == "" {
			continue
		}
		e := main
		e.Provider, e.BaseURL, e.APIKey, e.Model = b.Provider, b.BaseURL, b.APIKey, b.Model
		if e.BaseURL == "" {
			e.BaseURL = b.Provider.DefaultBaseURL()
		}
		e.AllowInsecure = false
		out = append(out, e)
	}
	return out
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
	Theme        string `json:"theme"`                    // "nacht" | "hell" | "kontrast" | "kontrasthell" | "matrix" | "bernstein" | "neon" | "eigene"
	UseDPAPI     bool   `json:"use_dpapi"`                // Windows: Store-Schlüssel zusätzlich per DPAPI
	LockAfterMin int    `json:"lock_after_min"`           // 0 = nie automatisch sperren
	Language     string `json:"language"`                 // "de" | "en"
	NoMouse      bool   `json:"no_mouse,omitempty"`       // Maus nicht einfangen (Terminalfenster markiert selbst)
	NoSessionLog bool   `json:"no_session_log,omitempty"` // Sitzungen nicht mitschreiben
	NoPasteGuard bool   `json:"no_paste_guard,omitempty"` // mehrzeiliges Einfügen ohne Vorschau (Gefahrenregeln greifen trotzdem)
	DangerAll    bool   `json:"danger_all,omitempty"`     // Gefahren-Check beim Tippen auf allen Hosts, nicht nur Produktiv
	LastVersion  string `json:"last_version,omitempty"`   // zuletzt gestartete Version (für „Was ist neu“)
	// NoAutoReconnect: abgerissene Sitzungen nicht von selbst neu verbinden.
	NoAutoReconnect bool `json:"no_auto_reconnect,omitempty"`
	TourDone        bool `json:"tour_done,omitempty"`
	ExplainKeys     bool `json:"explain_keys,omitempty"` // Tastenzeile mit Erklärungen (⇧F9)
	// NoReachCheck: im Manager nicht prüfen, ob die Hosts erreichbar sind.
	NoReachCheck bool `json:"no_reach_check,omitempty"`
	// NoNotify: keine Meldung (Tab-Markierung, Einblendung), wenn ein langer
	// Befehl in einem verdeckten Tab fertig ist oder dort die Glocke läutet.
	NoNotify bool `json:"no_notify,omitempty"`
	// NoDesktopNotify: dafür keine Benachrichtigung des Betriebssystems,
	// wenn das sshTermy-Fenster nicht im Vordergrund ist.
	NoDesktopNotify bool `json:"no_desktop_notify,omitempty"`
	// NoOSC52: Programme auf dem Server (tmux, vim) dürfen nicht in die
	// lokale Zwischenablage schreiben.
	NoOSC52 bool `json:"no_osc52,omitempty"`
	// Monitor: Warnschwellen der Host-Übersicht (F6) und wie oft Server
	// automatisch nachgefragt werden.
	Monitor MonitorConfig `json:"monitor,omitempty"`

	// Colors: eigene Farben (Theme "eigene"), wird mit dem Sync übertragen.
	Colors *CustomColors `json:"colors,omitempty"`
	// Icons: Symbolstil der Menüs. "" = Text-Tags wie [+] (läuft mit jeder
	// Schrift), "uni", "min", "nerd" = Nerd-Font-Symbole. Wird mit dem Sync
	// übertragen; "nerd" greift aber nur, wo NerdFont nicht "nein" ist.
	Icons string `json:"icons,omitempty"`
	// NerdFont: hat das Terminal dieses Rechners eine Nerd Font? "" = nicht
	// gefragt (wer "nerd" selbst gewählt hat, hat sie), "ja" = Probe bestätigt,
	// "nein" = per Sync von einem anderen Rechner gekommen, hier noch nicht
	// bestätigt. Rein lokal, nie im Sync.
	NerdFont string `json:"nerd_font,omitempty"`
	// Intro: Startanimation "an" / "aus" ("" = nie umgestellt). Wird mit dem
	// Sync übertragen; die Oberfläche spiegelt sie in die Datei no-intro,
	// weil die Animation schon vor dem Entsperren läuft.
	Intro string `json:"intro,omitempty"`

	// DimPanes: in der geteilten Ansicht die nicht aktiven Bereiche abdunkeln.
	DimPanes bool `json:"dim_panes,omitempty"`
	// Workspaces: gespeicherte Arbeitsplätze (Aufteilung + Verbindungen),
	// LastLayout: Aufteilung beim letzten Beenden (Eintrag „Zuletzt“).
	// Arbeitsplätze gehen mit dem Sync, „Zuletzt“ bleibt auf dem Rechner.
	Workspaces []Workspace `json:"workspaces,omitempty"`
	LastLayout *Workspace  `json:"last_layout,omitempty"`

	// Keys: eigene Tastenkürzel, Aktion → Taste (bubbletea-Schreibweise,
	// z. B. "goal": "ctrl+g"). Fehlt eine Aktion, gilt der Standard.
	Keys map[string]string `json:"keys,omitempty"`
}

// Workspace: geteilte Ansicht zum Wiederöffnen. Layout ist der Name der
// Aufteilung ("cols", "rows", "grid", "main", "cols3", "grid6"), FracX/FracY
// die Anteile der Spalten/Zeilen (leer = gleich groß), Conns die Verbindung
// je Bereich ("" = leer). Folder statt Conns: alle Verbindungen im Ordner.
type Workspace struct {
	Name   string    `json:"name"`
	Layout string    `json:"layout"`
	FracX  []float64 `json:"frac_x,omitempty"`
	FracY  []float64 `json:"frac_y,omitempty"`
	Conns  []string  `json:"conns,omitempty"`
	Folder string    `json:"folder,omitempty"`
	Saved  time.Time `json:"saved"`
}

// IconsNow: der Symbolstil, der auf diesem Rechner wirklich gilt. Kam
// "nerd" per Sync und ist die Schrift hier nicht bestätigt, bleiben die
// Text-Tags.
func (s Settings) IconsNow() string {
	if s.Icons == "nerd" && s.NerdFont == "nein" {
		return ""
	}
	return s.Icons
}

// CustomColors: eigene Farben als "#rrggbb". Base ist das Theme, von dem
// sie ausgehen; leere Felder nehmen dessen Farbe.
type CustomColors struct {
	Base   string `json:"base,omitempty"`
	Bg     string `json:"bg,omitempty"`
	Text   string `json:"text,omitempty"`
	Muted  string `json:"muted,omitempty"`
	Border string `json:"border,omitempty"`
	Accent string `json:"accent,omitempty"`
	AI     string `json:"ai,omitempty"`
	OK     string `json:"ok,omitempty"`
	Warn   string `json:"warn,omitempty"`
	Danger string `json:"danger,omitempty"`
}

// MonitorConfig: Warnschwellen für CPU, RAM und Platte in Prozent (0 =
// Standard 90 %) und der Abstand der automatischen Abfrage bei Servern in
// Minuten (0 = Standard 5, negativ = aus). Netzwerkgeräte werden nie
// automatisch abgefragt (dort tippt sshTermy sichtbar am Prompt).
type MonitorConfig struct {
	CPU      int `json:"cpu,omitempty"`
	RAM      int `json:"ram,omitempty"`
	Disk     int `json:"disk,omitempty"`
	EveryMin int `json:"every_min,omitempty"`
}

// Standardwerte der Überwachung.
const (
	DefaultMonitorLimit = 90
	DefaultMonitorEvery = 5
)

// Limits: wirksame Schwellen (Standard für 0, begrenzt auf 1…100).
func (m MonitorConfig) Limits() (cpu, ram, disk int) {
	f := func(v int) int {
		if v <= 0 {
			return DefaultMonitorLimit
		}
		return min(v, 100)
	}
	return f(m.CPU), f(m.RAM), f(m.Disk)
}

// Every: Abstand der automatischen Abfrage (0 = aus).
func (m MonitorConfig) Every() time.Duration {
	switch {
	case m.EveryMin < 0:
		return 0
	case m.EveryMin == 0:
		return DefaultMonitorEvery * time.Minute
	}
	return time.Duration(m.EveryMin) * time.Minute
}

// SyncBackend: wohin die verschlüsselte Sync-Datei geschrieben wird.
type SyncBackend string

const (
	SyncOff    SyncBackend = ""       // kein Sync
	SyncFolder SyncBackend = "folder" // Ordner, den OneDrive/Dropbox/iCloud/Syncthing abgleicht
	SyncWebDAV SyncBackend = "webdav" // eigenes NAS (Synology, QNAP), Nextcloud, jeder WebDAV-Server
)

// SyncConfig ist rein lokal: sie wird nie in die Sync-Datei geschrieben.
// Key enthält den Sync-Schlüssel (nicht das Sync-Passwort); er liegt nur im
// lokalen, verschlüsselten Store und ist damit wie alles andere per
// Master-Passwort bzw. DPAPI geschützt.
type SyncConfig struct {
	Backend SyncBackend `json:"backend,omitempty"`
	Folder  string      `json:"folder,omitempty"` // SyncFolder

	Preset   string `json:"preset,omitempty"`  // "synology" | "qnap" | "nextcloud" | "andere"
	Address  string `json:"address,omitempty"` // Eingabe des Nutzers (Name, IP oder URL)
	URL      string `json:"url,omitempty"`     // gefundene WebDAV-Ordner-URL
	User     string `json:"user,omitempty"`
	Password Secret `json:"password,omitempty"`
	Remote   string `json:"remote,omitempty"`   // Ordner am Server, z. B. /home/sshtermy
	CertPin  string `json:"cert_pin,omitempty"` // SHA-256 des vertrauten (selbstsignierten) Zertifikats

	Key      Secret `json:"key,omitempty"` // Sync-Schlüssel (Base64), nicht das Passwort
	HostKeys bool   `json:"host_keys"`     // known_hosts mit abgleichen
	Auto     bool   `json:"auto"`          // beim Start, nach Änderungen und alle 5 Minuten
	// Archiv: Config-Sicherungen und Sitzungsprotokolle mit abgleichen
	// (eigener Unterordner neben der Sync-Datei, siehe syncx/archive.go).
	Archive     bool      `json:"archive,omitempty"`
	ArchiveDays int       `json:"archive_days,omitempty"` // Aufbewahrung im Archiv, 0 = 90 Tage
	LastSync    time.Time `json:"last_sync,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
}

// Enabled: ist Sync eingerichtet?
func (c SyncConfig) Enabled() bool { return c.Backend != SyncOff && c.Key.IsSet() }

// Data ist alles, was verschlüsselt auf der Platte liegt.
type Data struct {
	Version     int          `json:"version"`
	Connections []Connection `json:"connections"`
	Tunnels     []Tunnel     `json:"tunnels"`
	Runbooks    []Runbook    `json:"runbooks,omitempty"`
	Folders     []Folder     `json:"folders,omitempty"`
	LLM         LLMConfig    `json:"llm"`
	Settings    Settings     `json:"settings"`

	// Sync: Zeitstempel für die Zusammenführung pro Eintrag.
	LLMUpdatedAt      time.Time            `json:"llm_updated_at,omitempty"`
	SettingsUpdatedAt time.Time            `json:"settings_updated_at,omitempty"`
	Deleted           map[string]time.Time `json:"deleted,omitempty"` // ID → Löschzeit (Grabsteine)
	Sync              SyncConfig           `json:"sync,omitempty"`

	// Relays: gekoppelte Vermittler. Rein lokal wie Sync: jeder Rechner
	// koppelt sich selbst (eigener Geräteschlüssel), nie in der Sync-Datei.
	Relays []Relay `json:"relays,omitempty"`

	// LocalKeys: Schlüsseldatei pro Verbindung (ID → Pfad), nur für diesen
	// Rechner. Rein lokal wie Sync, geht nie in die Sync-Datei. Hat Vorrang
	// vor Connection.KeyPath, wenn der Key hier woanders liegt als auf dem
	// Rechner, der die Verbindung angelegt hat.
	LocalKeys map[string]string `json:"local_keys,omitempty"`

	// TeamLinks: Team-Ordner (veröffentlichen bzw. abonnieren). Rein lokal:
	// Pfade sind je Rechner anders, das Team-Passwort bleibt auf dem Gerät.
	TeamLinks []TeamLink `json:"team_links,omitempty"`
}

// Rollen eines Team-Ordners.
const (
	TeamPublish   = "pub" // dieser Rechner schreibt das Team-Paket bei Änderungen neu
	TeamSubscribe = "sub" // dieser Rechner liest es regelmäßig und übernimmt Änderungen
)

// TeamLink ist ein Team-Paket in einem gemeinsamen Ordner (Netzlaufwerk,
// OneDrive, Teams) oder unter einer https-/WebDAV-Adresse.
type TeamLink struct {
	ID       string `json:"id"`
	Role     string `json:"role"` // TeamPublish | TeamSubscribe
	Name     string `json:"name"`
	Path     string `json:"path"` // Datei oder https://[user:pw@]host/…/x.sshteam
	TeamPass Secret `json:"team_pass"`

	// Veröffentlichen: was ins Paket geht. Folder = ganzer Ordner (auch
	// später dazugekommene Verbindungen), IDs = einzeln gewählte.
	Folder    string   `json:"folder,omitempty"`
	IDs       []string `json:"ids,omitempty"`
	From      string   `json:"from,omitempty"`
	Users     bool     `json:"users,omitempty"`
	QuickCmds bool     `json:"quick,omitempty"`
	Runbooks  bool     `json:"runbooks,omitempty"`
	Tunnels   bool     `json:"tunnels,omitempty"`

	// Abonnieren: wohin und was schon da ist.
	Into        string   `json:"into,omitempty"` // Zielordner, z. B. „Team“
	DefaultUser string   `json:"user,omitempty"` // für Verbindungen ohne Benutzernamen
	Have        []string `json:"have,omitempty"` // IDs, die zuletzt aus dem Paket kamen
	Skip        []string `json:"skip,omitempty"` // in der Vorschau abgewählt: nie übernehmen

	LastSum string    `json:"last_sum,omitempty"` // Stand des zuletzt geschriebenen/gelesenen Pakets
	LastRun time.Time `json:"last_run,omitempty"` // letzte Änderung (geschrieben/übernommen)
	LastErr string    `json:"last_err,omitempty"`
}

// Relay ist ein gekoppelter Vermittler.
type Relay struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`         // Anzeigename (vom Vermittler, änderbar)
	Kind   string    `json:"kind"`         // "sshtermy" | "spark"
	Base   string    `json:"base"`         // https://host[:port][/pfad]
	FP     string    `json:"fp,omitempty"` // Pin des Zertifikats (SHA-256 hex), leer = System-CAs
	Key    Secret    `json:"key"`          // Geräteschlüssel aus der Kopplung
	Device string    `json:"device"`       // Name dieses Rechners beim Vermittler
	Since  time.Time `json:"since"`
}

// NewData: leerer Store mit Standardwerten.
func NewData() *Data {
	return &Data{Version: 1, LLM: DefaultLLM(), Settings: Settings{Theme: "nacht", UseDPAPI: true, Language: "de"}}
}

// Runbook ist ein gespeichertes Ziel zum Wiederverwenden. Steps sind die
// Befehle, die beim Speichern ausgeführt worden waren: Sie gehen dem Agenten
// als Vorlage mit, er passt sie an die aktuelle Lage an. Ausgeführt wird
// trotzdem jeder Schritt erst nach Bestätigung.
type Runbook struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Goal         string        `json:"goal"`
	Steps        []string      `json:"steps,omitempty"`
	Files        []RunbookFile `json:"files,omitempty"`         // Skripte aus dem Agent-Modus
	ConnectionID string        `json:"connection_id,omitempty"` // leer = für alle Verbindungen
	OS           string        `json:"os,omitempty"`            // System beim Speichern, nur als Hinweis
	CreatedAt    time.Time     `json:"created_at"`
	LastUsed     time.Time     `json:"last_used,omitempty"`
	Uses         int           `json:"uses,omitempty"`
	UpdatedAt    time.Time     `json:"updated_at,omitempty"` // letzte Änderung (Sync)
}

// RunbookFile: eine Datei, die der Agent-Modus angelegt hat. Pfade in den
// Schritten nennen den Arbeitsordner als AgentDirToken.
type RunbookFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// AgentDirToken steht in gespeicherten Schritten für den Arbeitsordner des
// Agenten (jeder Lauf hat einen eigenen).
const AgentDirToken = "<ARBEITSORDNER>"

// Rev: Zeitstempel für den Sync (alte Einträge zählen mit dem Anlagedatum).
func (r Runbook) Rev() time.Time {
	if r.UpdatedAt.IsZero() {
		return r.CreatedAt
	}
	return r.UpdatedAt
}
