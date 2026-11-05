package agent

import "github.com/db9979/sshtermy/internal/i18n"

// sys hängt an einen Systemprompt die Sprache der Oberfläche an. Die Prompts
// selbst bleiben deutsch (sie sind erprobt); nur die Texte, die der Admin
// liest, sollen in seiner Sprache kommen.
func sys(prompt string) string {
	if i18n.English() {
		return prompt + " WICHTIG: Die Oberfläche ist englisch. Schreibe alle Texte für den Admin (grund, frage, Erklärungen, " +
			"Zusammenfassungen, „Why:“ statt „Warum:“) auf Englisch, außer die Frage des Admins ist erkennbar in einer anderen Sprache."
	}
	return prompt
}
