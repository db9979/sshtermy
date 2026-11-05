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
}

var urlCreds = regexp.MustCompile(`(?i)(://[^:/\s]+:)[^@/\s]+@`) // user:pass@ in URLs

func Redact(s string) string {
	s = urlCreds.ReplaceAllString(s, "${1}[ENTFERNT]@")
	for _, rx := range redactions {
		s = rx.ReplaceAllString(s, "[ENTFERNT]")
	}
	return s
}

// LooksLikeSecretPrompt: steht der Cursor hinter einer Passwortabfrage? Dann
// gibt es weder Ghost-Text noch Bildschirm ans Modell.
var secretPrompt = regexp.MustCompile(`(?i)(password|passwort|passphrase|kennwort|pin|token|otp|verification code)[^\n]{0,40}:\s*$`)

func LooksLikeSecretPrompt(lastLine string) bool { return secretPrompt.MatchString(lastLine) }
