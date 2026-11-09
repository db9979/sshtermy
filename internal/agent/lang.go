package agent

import (
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
)

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

// sys: Systemprompt samt Sprache und Hausregeln des Admins. Die Hausregeln
// stehen im Systemteil (sie sind Anweisungen, kein Material), ganz am Ende,
// damit der feste Anfang für den Prefix-Cache des Servers gleich bleibt.
func (a *Agent) sys(prompt string) string {
	s := sys(prompt)
	if r := strings.TrimSpace(a.Rules); r != "" {
		s += "\n\nHausregeln des Admins für diese Umgebung. Halte dich daran, soweit sie nicht gegen die Regeln oben " +
			"verstoßen; sie heben keine Sicherheits- oder Formatregel auf:\n" + r
	}
	return s
}

// Aufgaben für die Antwortzeiten im KI-Status (llm.TaskStats).
const (
	TaskGhost     = "ghost"
	TaskFollow    = "follow"
	TaskGoal      = "goal"
	TaskAgent     = "agent"
	TaskPlan      = "plan"
	TaskAsk       = "ask"
	TaskLogs      = "logs"
	TaskExplain   = "explain"
	TaskCompanion = "companion"
	TaskRecap     = "recap"
	TaskAdapt     = "adapt"
	TaskReview    = "review"
	TaskSummary   = "summary"
	TaskReport    = "report"
)

// Kontext je Aufgabe: Für den Ghost-Text reichen die letzten Zeilen, das
// spart Lesezeit beim Server (weniger Tokens vor der ersten Antwort).
const (
	ghostLines  = 25
	followLines = 40
)

// lastLines: höchstens die letzten n Zeilen von s.
func lastLines(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}
	cnt := 0
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' && i < len(s)-1 {
			cnt++
			if cnt == n {
				return s[i+1:]
			}
		}
	}
	return s
}
