package safety

import (
	"github.com/db9979/sshtermy/internal/i18n"
)

// Sensitive: liest der Befehl Geheimnisse (Passwort-Hashes, private Schlüssel,
// Zugangsdaten, Umgebungsvariablen, Gerätekonfiguration mit Passwörtern)?
// Solche Befehle ändern nichts, ihre Ausgabe ginge aber an die KI (Agent,
// MCP-Client). Deshalb fragen sie nach, auch wenn sie nur lesen. Leer = nichts
// Bekanntes.
func Sensitive(cmd string) string {
	for _, ru := range sensitiveRules {
		if ru.rx.MatchString(cmd) {
			return i18n.T(ru.reason)
		}
	}
	return ""
}

var sensitiveRules = []rule{
	r(`/etc/(g?shadow|sudoers|master\.passwd|security/opasswd)\b|/etc/ssl/private\b|/etc/ssh/ssh_host_\w+_key(\s|$)`, "liest Passwort-Hashes oder Schlüssel des Systems"),
	r(`(^|[\s/'"=])\.ssh/(id_\w+|identity)(\s|$|['"])|\.ssh/[^\s]*\.(pem|key)\b|\bid_(rsa|dsa|ecdsa|ed25519)(\s|$)|\.(pem|key|p12|pfx|kdbx)(\s|$|['"])`, "liest einen privaten Schlüssel"),
	r(`(^|[\s/'"=])\.(env|pgpass|netrc|git-credentials|my\.cnf|vault-token)(\s|$|['"])|\.aws/credentials\b|\.kube/config\b|\.docker/config\.json\b|\bwp-config\.php\b|\bsecrets?\.(ya?ml|json|env)\b`, "liest Zugangsdaten"),
	r(`\bkubectl\s+(\S+\s+)*(get|describe)\s+(secret|secrets)\b|\bdocker\s+(container\s+)?inspect\b|\bpodman\s+inspect\b|/proc/\S*/environ\b|(^|[;&|]\s*)(sudo\s+)?(printenv|env|set)\s*($|[;&|])|\bGet-ChildItem\s+env:|\bgci\s+env:|\bdir\s+env:`, "zeigt Geheimnisse aus Umgebung oder Containern"),
	r(`^\s*(show|sh|display|dis)\s+(run(ning)?(-config)?|current-configuration|config(uration)?|startup-config|saved-configuration)\b|^\s*/export\b|^\s*show\s+full-configuration\b|^\s*cat\s+/tmp/system\.cfg\b`, "zeigt die Gerätekonfiguration (mit Passwörtern)"),
}
