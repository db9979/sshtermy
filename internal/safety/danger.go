// Package safety enthält feste Regeln, die gefährliche Befehle erkennen.
//
// Grundsatz (übernommen aus dem Fernzugang-Assistenten von speech-on-dgx-spark):
// Ob ein Befehl gefährlich ist, entscheiden diese Regeln, nie das Sprachmodell.
// Ein Treffer färbt den Vorschlag rot und verlangt eine zweite, ausdrückliche
// Bestätigung. Das Modell kann das weder abschalten noch "herunterstufen".
package safety

import (
	"regexp"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Finding: eine Regel, die zutrifft.
type Finding struct {
	Reason string
}

type rule struct {
	rx     *regexp.Regexp
	reason string
}

func r(p, reason string) rule { return rule{regexp.MustCompile(`(?i)` + p), reason} }

// Die Gründe stehen deutsch in den Regeln und werden bei Check übersetzt.

var unixRules = []rule{
	r(`\brm\s+(-[A-Za-z-]+\s+)*-[A-Za-z]*[rRf]|\brm\s+(\S+\s+)*--(recursive|force)\b`, "löscht Dateien"),
	r(`\bfind\b[^|;&]*\s(-delete\b|-exec(dir)?\s+(sudo\s+)?rm\b)`, "löscht Dateien"),
	r(`\|\s*(sudo\s+)?(ba|z|da|k)?sh(\s+-s?)?\s*($|[;&|])|\bbase64\s+(-d|--decode)\b[^;&]*\|\s*(sudo\s+)?\S*sh\b`, "führt unbekannten Text als Skript aus"),
	r(`\bdd\s`, "schreibt roh auf ein Gerät"),
	r(`\b(mkfs|wipefs|fdisk|sfdisk|parted|shred)\b`, "Datenträger formatieren oder löschen"),
	r(`\b(chmod|chown)\s+(\S+\s+)*-[A-Za-z]*R[A-Za-z]*\s+(\S+\s+)*/(\s|\*|$)`, "Rechte des ganzen Systems ändern"),
	r(`\b(curl|wget)\b[^|;&]*\|\s*(sudo\s+)?(ba|z|da|k)?sh\b`, "führt ein Skript aus dem Netz aus"),
	r(`>\s*/dev/(sd|hd|vd|xvd|nvme|mmcblk|disk)`, "überschreibt einen Datenträger"),
	r(`\b(shutdown|reboot|poweroff|halt)\b|\binit\s+[06]\b`, "fährt den Server herunter oder neu"),
	r(`\b(ip6?tables|nft)\s+(\S+\s+)*(-F\b|--flush\b|flush\b)|\bufw\s+(disable|reset)\b`, "leert oder deaktiviert die Firewall"),
	r(`\b(userdel|deluser|passwd|chpasswd|usermod)\b`, "ändert Benutzer oder Passwörter"),
	r(`:\(\)\s*\{`, "Fork-Bombe"),
	r(`\bgit\s+(reset\s+--hard|clean\s+-[A-Za-z]*f|push\s+(\S+\s+)*(-f\b|--force\b))`, "verwirft Änderungen"),
	r(`\b(apt|apt-get|dnf|yum|zypper|pacman)\s+(\S+\s+)*(remove|purge|autoremove|erase|-R)\b`, "entfernt Pakete"),
	r(`\bdocker\s+(system|volume|image|container)\s+prune\b|\bdocker\s+rm\s+-f`, "löscht Docker-Daten"),
	r(`\bmv\s+(\S+\s+)+/dev/null\b|>\s*/etc/(passwd|shadow|sudoers|ssh/sshd_config)\b`, "überschreibt Systemdateien"),
	r(`\bsystemctl\s+(stop|disable|mask)\s+(ssh|sshd)\b`, "sperrt dich aus (SSH-Dienst)"),
	r(`\bcrontab\s+-r\b`, "löscht alle Cron-Jobs"),
}

var windowsRules = []rule{
	r(`\b(Remove-Item|ri|rm|del|erase|rd|rmdir)\b[^|;&]*\s(-Recurse\b|-r\b|/s\b)`, "löscht Ordner mit allem darin"),
	r(`\b(Format-Volume|Clear-Disk|Initialize-Disk|diskpart|cipher\s+/w)\b|\bformat\s+[A-Za-z]:`, "Datenträger formatieren oder löschen"),
	r(`\b(Stop-Computer|Restart-Computer)\b|\bshutdown(\.exe)?\s+/[srp]`, "fährt den Rechner herunter oder neu"),
	r(`\b(Remove-LocalUser|Set-LocalUser|Disable-LocalUser)\b|\bnet\s+user\b[^|;&]*(/delete|/del\b)`, "ändert Benutzer oder Passwörter"),
	r(`\b(iex|Invoke-Expression)\b[^|;&]*\b(iwr|irm|Invoke-WebRequest|Invoke-RestMethod|DownloadString)\b|\b(iwr|irm|Invoke-WebRequest|Invoke-RestMethod)\b[^;&]*\|\s*(iex|Invoke-Expression)\b`, "führt ein Skript aus dem Netz aus"),
	r(`\bSet-ExecutionPolicy\s+(Unrestricted|Bypass)\b`, "schaltet den Skriptschutz ab"),
	r(`\b(Set-NetFirewallProfile[^|;&]*-Enabled\s+(False|0)|netsh\s+advfirewall\s+set\s+\S+\s+state\s+off)\b`, "schaltet die Firewall ab"),
	r(`\b(Remove-ItemProperty|reg\s+delete)\b[^|;&]*\bHKLM\b|\bbcdedit\b`, "ändert die Systemeinstellungen tief"),
	r(`\bvssadmin\s+delete\s+shadows\b|\bwbadmin\s+delete\b`, "löscht Sicherungen"),
}

// networkRules: Switches, Router, Firewalls (ArubaOS, Cisco, Comware, Junos,
// FortiOS, RouterOS). Die Regeln hängen am Zeilenanfang, damit etwa
// „show reload“ oder „no shutdown“ nicht anschlagen.
var networkRules = []rule{
	r(`^\s*(do\s+)?(reload|reboot|boot(\s+system)?)\b|^\s*(execute|exec)\s+(reboot|shutdown)\b|^\s*request\s+system\s+(reboot|halt|power-off)\b|^\s*/?system\s+(reboot|shutdown)\b`, "startet das Gerät neu"),
	r(`^\s*(do\s+)?(write\s+erase|erase\b|delete\b|format\b)|^\s*reset\s+saved-configuration\b|^\s*execute\s+(factoryreset|formatlogdisk)\b|^\s*request\s+system\s+zeroize\b|^\s*/?system\s+reset-configuration\b|\bfactory[- ]?reset\b|\bzeroize\b|\b(restore|set)-default\b`, "löscht Konfiguration oder Speicher des Geräts"),
	r(`^\s*(do\s+)?copy\s+(tftp|scp|sftp|ftp|https?|usb|flash|xmodem)\S*(\s+\S+)?\s+(startup-config|running-config)\b`, "überschreibt die Gerätekonfiguration"),
	r(`^\s*shut(down)?\s*$|^\s*(interface\s+\S+\s+)?disable\s*$`, "schaltet eine Schnittstelle ab"),
	r(`^\s*no\s+(vlan|interface|router|ip\s+route|spanning-tree|aaa|username|ip\s+address|ip\s+ssh|crypto)\b`, "entfernt Konfiguration (VLAN, Routing, Zugang)"),
	r(`^\s*clear\s+(config|crypto|counters\s+all|arp|ip\s+bgp\s+\*)`, "setzt Gerätezustand zurück"),
}

// Kind leitet die Regelgruppe aus dem Remote-OS-Text ab.
func Kind(osText string) string {
	if strings.Contains(strings.ToLower(osText), "windows") {
		return "windows"
	}
	return "unix"
}

// Check liefert alle zutreffenden Regeln (leer = keine bekannte Gefahr).
// Auch "sudo" allein macht einen Befehl nicht gefährlich; aber jeder
// Befehl mit sudo wird in der Oberfläche zusätzlich markiert (Elevated).
func Check(cmd, kind string) []Finding {
	rules := unixRules
	switch kind {
	case "windows":
		rules = windowsRules
	case Network:
		rules = networkRules
	}
	var out []Finding
	for _, ru := range rules {
		if ru.rx.MatchString(cmd) {
			out = append(out, Finding{i18n.T(ru.reason)})
		}
	}
	return out
}

// Network ist die Regelgruppe für Switches und Router.
const Network = "network"

// KindFor wie Kind, kennt aber Netzwerkgeräte (Switch, Router, Firewall).
func KindFor(osText string, netDevice bool) string {
	if netDevice {
		return Network
	}
	return Kind(osText)
}

// Kinds: alle Regelgruppen (für Stellen, an denen das Zielsystem noch
// nicht feststeht, z. B. ein Runbook vor dem Verbinden).
var Kinds = []string{"unix", "windows", Network}

// Elevated: läuft der Befehl mit erhöhten Rechten? Auch hinter && ; | oder
// in $( … ), sonst liefe „cd /etc && sudo …“ im Agent-Modus ohne Rückfrage.
func Elevated(cmd string) bool {
	return elevatedRE.MatchString(cmd) || strings.Contains(strings.ToLower(cmd), "-verb runas")
}

var elevatedRE = regexp.MustCompile(`(^|[;&|({` + "`" + `]|\$\(|\b(then|do|else|exec|xargs|nohup|env|time)\s)\s*(sudo|doas|pkexec|run0|su)(\s|$)`)

// InlineCode: startet der Befehl Code, der direkt in der Zeile steht
// (python -c, perl -e, bash -c, eval …)? Darin können die Gefahrenregeln
// nicht hineinsehen; der Agent-Modus fragt in der Stufe „Normal“ deshalb nach.
func InlineCode(cmd string) bool { return inlineRE.MatchString(cmd) }

var inlineRE = regexp.MustCompile(`(?i)\b(python[0-9.]*|perl|ruby|node|nodejs|php|pwsh|powershell)(\.exe)?\s+(-\S+\s+)*-(c|e|E|r|command|encodedcommand)\b` +
	`|\b(ba|z|da|k|fi)?sh\s+(-\S+\s+)*-[a-z]*c\b|(^|[;&|(]\s*)eval\b|\|\s*(python[0-9.]*|perl|ruby|node|php)(\s+-)?\s*($|[;&|])`)
