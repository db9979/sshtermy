package domain

import (
	"strings"
	"time"
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
type Dialect struct {
	ID, Name, Hint string
}

// Dialects: bekannte Befehlssätze. Die Hinweise sind bewusst knapp und nennen
// typische Befehle, damit ein Modell nicht in Linux- oder Cisco-Syntax
// zurückfällt.
var Dialects = []Dialect{
	{"posix", "Linux/Unix-Shell", "POSIX-Shell (bash/sh) mit GNU-Werkzeugen."},
	{"macos", "macOS", "macOS: zsh, BSD-Werkzeuge (z. B. sed -i '', kein systemctl; launchctl, brew)."},
	{"bsd", "BSD", "BSD: sh/csh, BSD-Werkzeuge, service/rc.d statt systemctl, pkg."},
	{"powershell", "Windows PowerShell", "Windows mit PowerShell: Get-Service, Get-Process, Get-ChildItem, Get-NetIPAddress; keine Unix-Befehle."},
	{"cmd", "Windows cmd.exe", "Windows-Eingabeaufforderung (cmd.exe): dir, type, ipconfig, sc query, findstr; keine Unix-Befehle."},
	{"procurve", "ArubaOS-Switch (ProCurve)", "HPE/Aruba ProCurve-CLI, kein Linux: show running-config, show interfaces brief, show vlans, show mac-address, show lldp info remote-device, show system, show version; Blättern aus mit no page; Konfiguration mit configure … exit, speichern mit write memory."},
	{"arubaos-cx", "ArubaOS-CX", "ArubaOS-CX-CLI, kein Linux: show running-config, show interface brief, show vlan, show mac-address-table, show lldp neighbor-info, show version, show system; Blättern aus mit no page; Konfiguration mit configure terminal … end, speichern mit write memory."},
	{"cisco-ios", "Cisco IOS / IOS-XE", "Cisco-IOS-CLI, kein Linux: show running-config, show ip interface brief, show interfaces status, show vlan brief, show mac address-table, show cdp neighbors, show version; Blättern aus mit terminal length 0; Konfiguration mit configure terminal … end, speichern mit write memory."},
	{"junos", "Juniper Junos", "Junos-CLI: show configuration | display set, show interfaces terse, show version, show chassis hardware; Konfiguration mit configure, set …, commit."},
	{"comware", "HPE Comware / H3C", "Comware-CLI: display current-configuration, display interface brief, display vlan, display version; Konfiguration mit system-view … quit, speichern mit save."},
	{"huawei-vrp", "Huawei VRP", "Huawei-VRP-CLI: display current-configuration, display interface brief, display version; Konfiguration mit system-view … quit, speichern mit save."},
	{"routeros", "MikroTik RouterOS", "RouterOS-CLI: /system resource print, /interface print, /ip address print, /ip route print, /export; keine Linux-Befehle."},
	{"fortios", "FortiOS", "FortiOS-CLI: get system status, show full-configuration, get system interface, diagnose …; Konfiguration mit config … edit … set … next … end."},
	{"unifi", "UniFi-Switch (Ubiquiti)", "Ubiquiti-UniFi-Switch: Anmeldung landet in einer BusyBox-Linux-Shell (ash), keine Cisco-Syntax. Die Konfiguration kommt vom UniFi-Controller (Network-App) und wird beim nächsten Provisionieren überschrieben; dauerhaft also dort ändern. Shell: info (Modell, Firmware, Controller-Status), swctrl port show, swctrl poe show, swctrl mac show, cat /tmp/system.cfg (laufende Konfiguration), logread, set-inform http://<controller>:8080/inform. Ältere Modelle (US-…) haben zusätzlich eine EdgeSwitch-/FASTPATH-CLI über cli bzw. telnet localhost (Prompt (UBNT) >): enable, show running-config, show interfaces status all, show vlan brief, show mac-addr-table, show lldp remote-device all, zurück mit exit. syswrapper.sh restore-default setzt das Gerät zurück."},
	{"network", "Netzwerkgerät (unbekannt)", "Netzwerkgerät mit eigener CLI, kein Linux. Erst mit Befehlen wie show version oder ? herausfinden, was es kann."},
}

// DialectByID sucht einen Befehlssatz (leer ⇒ ok=false).
func DialectByID(id string) (Dialect, bool) {
	for _, d := range Dialects {
		if d.ID == id {
			return d, true
		}
	}
	return Dialect{}, false
}

// FamilyOf: zu welcher Familie ein Befehlssatz gehört.
func FamilyOf(dialect string) string {
	switch dialect {
	case "posix":
		return FamilyLinux
	case "macos":
		return FamilyMacOS
	case "bsd":
		return FamilyBSD
	case "powershell", "cmd":
		return FamilyWindows
	case "":
		return ""
	}
	return FamilyNetwork
}

// DialectByName sucht einen Befehlssatz über den Anzeigenamen.
func DialectByName(name string) (Dialect, bool) {
	for _, d := range Dialects {
		if d.Name == name {
			return d, true
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
	if d, ok := DialectByID(p.Dialect); ok && !strings.Contains(p.OS, d.Name) && p.Family != FamilyLinux {
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
	if d, ok := DialectByID(p.Dialect); ok {
		line("Befehlssatz", d.Name+" – "+d.Hint)
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
