package agent

import "regexp"

// Redact entfernt offensichtliche Geheimnisse, bevor Text an ein Sprachmodell
// geht (auch ein lokales Modell kann Anfragen protokollieren). Das ist eine
// Heuristik, kein Versprechen: deshalb sendet sshtermy den Bildschirm nur,
// wenn "Bildschirm mitschicken" an ist, und nie während einer Passwortabfrage.
var redactions = []*regexp.Regexp{
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(-----END [A-Z ]*PRIVATE KEY-----|$)`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),                             // AWS Access Key
	regexp.MustCompile(`\b(ghp|gho|ghu|ghs|github_pat)_[A-Za-z0-9_]{20,}`), // GitHub
	regexp.MustCompile(`\bsk-[A-Za-z0-9_\-]{20,}`),                         // OpenAI-artige Keys
	regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9\-]{10,}`),                  // Slack
	regexp.MustCompile(`(?i)\b(bearer)\s+[A-Za-z0-9._\-]{16,}`),
	regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|token|api[_-]?key)\s*[=:]\s*\S+`),
	regexp.MustCompile(`(?i)(--?(password|passwd|pass)[= ]|sshpass -p ?)\S+`), // mysql --password x, sshpass -p x
	// Umgebungsvariablen: MYSQL_ROOT_PASSWORD=…, DB_PASS=…, AWS_SECRET_ACCESS_KEY=…
	regexp.MustCompile(`(?i)\b[a-z0-9_]*(password|passwd|secret|token|api_?key|access_?key|private_?key|credentials?|_pass)\s*[=:]\s*\S+`),
	// Passwort-Hashes (/etc/shadow, enable secret 5 …)
	regexp.MustCompile(`\$(1|2[aby]|5|6|y|gy|7|md5|sha1)\$[^\s:]{8,}`),
}

// keepPrefix: nur der Wert hinter dem Schlüsselwort geht weg.
var keepPrefix = []*regexp.Regexp{
	regexp.MustCompile(`(\s(?:-u|--user)\s+[^:\s]+:)\S+`),                    // curl -u admin:pw
	regexp.MustCompile(`(?i)(\bmysql(?:dump|admin)?\b[^\n]*?\s-p)[^\s-]\S*`), // mysql -pGeheim
	regexp.MustCompile(`(?i)(\bsnmp-server\s+community\s+)\S+`),              // Cisco/HP/Aruba
	regexp.MustCompile(`(?i)(\bplaintext\s+)\S+`),                            // Aruba: … plaintext geheim
	regexp.MustCompile(`(?im)^(\s*(?:username\s+\S+\s+(?:privilege\s+\d+\s+)?|enable\s+)(?:password|secret)\s+(?:(?:\d|manager|operator|sha1|sha256)\s+)*)\S+`),
	regexp.MustCompile(`(?im)^(\s*(?:password|secret|key)\s+(?:\d|manager|operator)\s+(?:\S+\s+)??)\S+`), // password 7 …, password manager …
	regexp.MustCompile(`(?im)^(\s+password\s+)\S+`), // eingerückt (line vty)
}

var urlCreds = regexp.MustCompile(`(?i)(://[^:/\s]+:)[^@/\s]+@`) // user:pass@ in URLs

func Redact(s string) string {
	s = urlCreds.ReplaceAllString(s, "${1}[ENTFERNT]@")
	for _, rx := range keepPrefix {
		s = rx.ReplaceAllString(s, "${1}[ENTFERNT]")
	}
	for _, rx := range redactions {
		s = rx.ReplaceAllString(s, "[ENTFERNT]")
	}
	return s
}

// LooksLikeSecretPrompt: steht der Cursor hinter einer Passwortabfrage? Dann
// gibt es weder Ghost-Text noch Bildschirm ans Modell.
var secretPrompt = regexp.MustCompile(`(?i)(password|passwort|passphrase|kennwort|pin|token|otp|verification code)[^\n]{0,40}:\s*$`)

func LooksLikeSecretPrompt(lastLine string) bool { return secretPrompt.MatchString(lastLine) }
