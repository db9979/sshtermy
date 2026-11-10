package domain

import (
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/cmdset"
)

// Familien eines Systemprofils.
const (
	FamilyLinux   = "linux"
	FamilyMacOS   = "macos"
	FamilyBSD     = "bsd"
	FamilyWindows = "windows"
	FamilyNetwork = "network"
)

// SystemProfile beschreibt das Zielsystem einer Verbindung: Betriebssystem,
// Version, Befehlssatz (Dialekt) und was dort schon schiefging. sshtermy füllt
// es beim Verbinden selbst aus (SSH-Kennung, unsichtbare Abfrage bei Servern,
// mitgelesene Terminal-Ausgabe bei Netzwerkgeräten); der Nutzer kann System,
// Befehlssatz und Notizen von Hand setzen.
type SystemProfile struct {
	Family  string `json:"family,omitempty"`  // linux | macos | bsd | windows | network
	OS      string `json:"os,omitempty"`      // z. B. "Ubuntu 24.04.1 LTS", "ArubaOS-Switch (ProCurve)"
	Version string `json:"version,omitempty"` // Firmware bzw. Kernel, z. B. "WC.16.10.0009"
	Model   string `json:"model,omitempty"`   // Gerätemodell, z. B. "Aruba 2930F-48G-PoE+"
	Dialect string `json:"dialect,omitempty"` // Befehlssatz, siehe Dialects
	PkgMgr  string `json:"pkg,omitempty"`     // apt-get, dnf, …
	Init    string `json:"init,omitempty"`    // systemd, openrc, …
	Shell   string `json:"shell,omitempty"`   // /bin/bash
	Banner  string `json:"banner,omitempty"`  // SSH-Kennung des Servers

	// Vom Nutzer gesetzt: dann überschreibt die Erkennung System und
	// Befehlssatz nicht mehr.
	Manual bool   `json:"manual,omitempty"`
	Notes  string `json:"notes,omitempty"` // eigene Hinweise für die KI

	// Failed: Befehle, die auf diesem System mit einer Fehlermeldung wie
	// „% Invalid input“ abgelehnt wurden (neueste zuletzt). Die KI soll sie
	// nicht noch einmal vorschlagen.
	Failed []string `json:"failed,omitempty"`

	// Learned: KI-Befehle, die der Admin auf diesem System korrigiert hat
	// (neueste zuletzt). Die KI bekommt sie als „so heißt das hier“ mit.
	Learned []Correction `json:"learned,omitempty"`

	DetectedAt time.Time `json:"detected_at,omitempty"`
}

// Correction: KI-Vorschlag und das, was der Admin stattdessen ausgeführt hat.
type Correction struct {
	AI    string    `json:"ai"`
	Fixed string    `json:"fixed"`
	At    time.Time `json:"at,omitempty"`
}

// MaxLearned: so viele Korrekturen merkt sich ein Profil.
const MaxLearned = 12

// MaxFailed: so viele abgelehnte Befehle merkt sich ein Profil.
const MaxFailed = 12

// Dialect beschreibt einen Befehlssatz: Anzeigename und Hinweis für die KI.
// Die Sätze selbst stehen in internal/cmdset (eine Datei pro Satz).
type Dialect struct {
	ID, Name, Hint string
	Own            bool // eigener oder Team-Satz (nicht eingebaut)
}

func dialectOf(s *cmdset.Set) Dialect {
	return Dialect{ID: s.ID, Name: s.Name, Hint: s.Hint, Own: s.Source != cmdset.Builtin}
}

// Dialects: alle aktiven Befehlssätze (eingebaute zuerst). Der allgemeine
// Satz „Netzwerkgerät (unbekannt)“ steht am Ende der eingebauten.
func Dialects() []Dialect {
	var out []Dialect
	for _, s := range cmdset.All() {
		out = append(out, dialectOf(s))
	}
	return out
}

// DialectByID sucht einen Befehlssatz (leer ⇒ ok=false).
func DialectByID(id string) (Dialect, bool) {
	if s, ok := cmdset.Lookup(id); ok {
		return dialectOf(s), true
	}
	return Dialect{}, false
}

// FamilyOf: zu welcher Familie ein Befehlssatz gehört.
func FamilyOf(dialect string) string {
	if dialect == "" {
		return ""
	}
	if s := cmdset.Resolve(dialect, "", ""); s != nil {
		return s.EffFamily()
	}
	return FamilyNetwork
}

// DialectByName sucht einen Befehlssatz über den Anzeigenamen.
func DialectByName(name string) (Dialect, bool) {
	for _, s := range cmdset.All() {
		if s.Name == name {
			return dialectOf(s), true
		}
	}
	return Dialect{}, false
}

// Empty: noch nichts bekannt.
func (p SystemProfile) Empty() bool {
	return p.OS == "" && p.Dialect == "" && p.Family == "" && p.Notes == "" && len(p.Failed) == 0 && len(p.Learned) == 0
}

// Summary: eine kurze Zeile für Manager und Kopfzeile,
// z. B. "Ubuntu 24.04 LTS · apt-get · systemd · bash".
func (p SystemProfile) Summary() string {
	var parts []string
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	add(p.OS)
	if p.Family == FamilyNetwork {
		add(p.Version)
		add(p.Model)
	}
	if d, ok := DialectByID(p.Dialect); ok && !strings.Contains(p.OS, d.Name) && (p.Family != FamilyLinux || d.ID != "posix") {
		add(d.Name)
	}
	add(p.PkgMgr)
	add(p.Init)
	if p.Shell != "" {
		add(shortShell(p.Shell))
	}
	if len(parts) == 0 {
		return "unbekannt"
	}
	return strings.Join(parts, " · ")
}

func shortShell(s string) string {
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		return s[i+1:]
	}
	return s
}

// Prompt: der Text, der der KI als Systemprofil mitgegeben wird.
func (p SystemProfile) Prompt() string {
	var b strings.Builder
	line := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" {
			b.WriteString(k + ": " + v + "\n")
		}
	}
	kind := map[string]string{FamilyLinux: "Linux-Server", FamilyMacOS: "macOS", FamilyBSD: "BSD",
		FamilyWindows: "Windows", FamilyNetwork: "Netzwerkgerät (Switch/Router/Firewall)"}[p.Family]
	line("Art", kind)
	line("System", p.OS)
	line("Version", p.Version)
	line("Modell", p.Model)
	if set := cmdset.Resolve(p.Dialect, p.Model, p.Version); set != nil {
		hint := set.Hint
		if set.Source != cmdset.Builtin {
			// eigener oder Team-Satz: nur Befehlswissen, keine Anweisungen
			hint = "(eigener Befehlssatz, nur als Hinweis zu Befehlen) " + hint
		}
		line("Befehlssatz", set.Name+" – "+hint)
	}
	line("Paketmanager", p.PkgMgr)
	line("Init-System", p.Init)
	line("Shell", p.Shell)
	line("Notizen des Admins", p.Notes)
	if len(p.Failed) > 0 {
		b.WriteString("Auf diesem System abgelehnt (nicht wieder vorschlagen):\n")
		for _, f := range p.Failed {
			b.WriteString("  - " + f + "\n")
		}
	}
	if len(p.Learned) > 0 {
		b.WriteString("Vom Admin korrigiert (links dein früherer Vorschlag, rechts der richtige Befehl auf diesem System; künftig so vorschlagen):\n")
		for _, c := range p.Learned {
			b.WriteString("  - " + c.AI + "  →  " + c.Fixed + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// AddLearned merkt sich eine Korrektur. Gibt es zum selben KI-Befehl schon
// eine, gilt die neue. Eine Korrektur, die einen früheren Fehlschlag
// behebt, darf auch die Liste der abgelehnten Befehle ergänzen; das macht
// der Aufrufer.
func (p *SystemProfile) AddLearned(ai, fixed string, at time.Time) bool {
	ai, fixed = strings.TrimSpace(ai), strings.TrimSpace(fixed)
	if ai == "" || fixed == "" || ai == fixed {
		return false
	}
	out := p.Learned[:0:0]
	for _, c := range p.Learned {
		if c.AI == ai || c.Fixed == ai {
			continue // überholt bzw. schon bekannt: neu hinten anstellen
		}
		out = append(out, c)
	}
	out = append(out, Correction{AI: ai, Fixed: fixed, At: at.UTC().Truncate(time.Second)})
	if len(out) > MaxLearned {
		out = out[len(out)-MaxLearned:]
	}
	same := len(out) == len(p.Learned)
	if same {
		for i := range out {
			if out[i].AI != p.Learned[i].AI || out[i].Fixed != p.Learned[i].Fixed {
				same = false
				break
			}
		}
	}
	p.Learned = out
	return !same
}

// AddFailed merkt sich einen abgelehnten Befehl (ohne Dubletten, begrenzt).
func (p *SystemProfile) AddFailed(entry string) bool {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return false
	}
	for i, f := range p.Failed {
		if f == entry {
			if i == len(p.Failed)-1 {
				return false
			}
			p.Failed = append(append(p.Failed[:i:i], p.Failed[i+1:]...), entry)
			return true
		}
	}
	p.Failed = append(p.Failed, entry)
	if len(p.Failed) > MaxFailed {
		p.Failed = p.Failed[len(p.Failed)-MaxFailed:]
	}
	return true
}
