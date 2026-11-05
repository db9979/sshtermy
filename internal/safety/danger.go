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
	r(`\brm\s+(-[A-Za-z-]+\s+)*-[A-Za-z]*[rRf]`, "löscht Dateien"),
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
	if kind == "windows" {
		rules = windowsRules
	}
	var out []Finding
	for _, ru := range rules {
		if ru.rx.MatchString(cmd) {
			out = append(out, Finding{i18n.T(ru.reason)})
		}
	}
	return out
}

// Elevated: läuft der Befehl mit erhöhten Rechten?
func Elevated(cmd string) bool {
	c := strings.TrimSpace(cmd)
	return strings.HasPrefix(c, "sudo ") || strings.Contains(c, "| sudo ") || strings.HasPrefix(c, "doas ") ||
		strings.Contains(strings.ToLower(c), "-verb runas")
}
