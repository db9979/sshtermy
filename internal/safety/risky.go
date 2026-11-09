package safety

import "github.com/db9979/sshtermy/internal/i18n"

// Riskant, aber nicht gefährlich im Sinn von Check: Dinge, die meist ein
// Sicherheitsloch aufreißen oder später Ärger machen. Diese Regeln sperren
// nichts und färben nichts rot; nur der KI-Begleiter meldet sich dazu, bevor
// die Zeile abgeschickt ist.
var riskyUnix = []rule{
	r(`\bchmod\s+(-[A-Za-z]+\s+)*(0?777|a\+rwx|o\+w)\b`, "macht Dateien für alle Benutzer schreibbar"),
	r(`\b(ufw\s+disable|systemctl\s+(stop|disable)\s+(firewalld|ufw|apparmor)|setenforce\s+0|iptables\s+-F)\b`, "schaltet einen Schutz ab"),
	r(`\bPermitRootLogin\s+yes\b|\bPasswordAuthentication\s+yes\b`, "lockert die SSH-Anmeldung"),
	r(`\b(curl|wget)\b[^|;&]*\s(-k|--insecure|--no-check-certificate)\b`, "prüft das Zertifikat nicht"),
	r(`\bgit\s+push\b[^;&|]*\s(-f|--force)\b`, "überschreibt fremde Änderungen"),
	r(`\bkill\s+-9\s+1\b|\bkillall\s+-9\b`, "beendet Prozesse ohne Aufräumen"),
}

// Risky: Hinweise für den Begleiter (leer = nichts Auffälliges). Gefährliches
// (Check) gehört immer dazu.
func Risky(cmd, kind string) []Finding {
	out := Check(cmd, kind)
	if kind == "windows" || kind == Network {
		return out
	}
	for _, ru := range riskyUnix {
		if ru.rx.MatchString(cmd) {
			out = append(out, Finding{i18n.T(ru.reason)})
		}
	}
	return out
}
