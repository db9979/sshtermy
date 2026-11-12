package agent

import (
	"fmt"
	"strings"
)

// Was der Mensch der KI außerhalb einer Rückfrage mitgibt, steht mit in den
// Antworten (answers), ist aber markiert, damit der Prompt es richtig
// benennt: eine Anweisung (Taste ? im Ziel-Modus), ein Hinweis von sshTermy
// (z. B. Verbindung war weg) oder ein früheres Ziel (Folgeziel).
const (
	markInstruct = "\x01"
	markNotice   = "\x02"
	markPrior    = "\x03"
)

// Instruction: Anweisung des Menschen mitten im Lauf.
func Instruction(x string) string { return markInstruct + strings.TrimSpace(x) }

// Notice: Hinweis von sshTermy an die KI.
func Notice(x string) string { return markNotice + strings.TrimSpace(x) }

// PriorGoal: ein früheres Ziel in derselben Sitzung (Folgeziel).
// steps = so viele der bisherigen Schritte gehören zu ihm.
func PriorGoal(goal, result string, steps int) string {
	result = strings.TrimSpace(result)
	if r := []rune(result); len(r) > 600 {
		result = string(r[:600]) + " …"
	}
	return markPrior + fmt.Sprint(steps) + "\n" + strings.Join(strings.Fields(goal), " ") + "\n" + result
}

// IsInstruction: für die Oberfläche (Zeitleiste).
func IsInstruction(x string) (string, bool) {
	if strings.HasPrefix(x, markInstruct) {
		return x[len(markInstruct):], true
	}
	return "", false
}

// answerLine formatiert eine Antwort für den Prompt. def ist die Zeile für
// eine normale Antwort auf eine Rückfrage.
func answerLine(x, def string) string {
	switch {
	case strings.HasPrefix(x, markInstruct):
		return fmt.Sprintf("Anweisung des Menschen (gilt ab jetzt, geht deinem bisherigen Plan vor): %s\n", Redact(x[len(markInstruct):]))
	case strings.HasPrefix(x, markNotice):
		return fmt.Sprintf("Hinweis von sshTermy: %s\n", Redact(x[len(markNotice):]))
	case strings.HasPrefix(x, markPrior):
		n, rest, _ := strings.Cut(x[len(markPrior):], "\n")
		goal, res, _ := strings.Cut(rest, "\n")
		s := fmt.Sprintf("Früheres Ziel in dieser Sitzung (erledigt; die bisherigen Schritte 1 bis %s gehören dazu, das aktuelle Ziel baut darauf auf): %s\n", n, Redact(goal))
		if res != "" {
			s += "Ergebnis damals:\n" + block(Redact(res)) + "\n"
		}
		return s
	}
	return fmt.Sprintf(def, Redact(x))
}

// IsPriorGoal: Eintrag aus PriorGoal (wird beim nächsten Folgeziel mitgenommen).
func IsPriorGoal(x string) bool { return strings.HasPrefix(x, markPrior) }
