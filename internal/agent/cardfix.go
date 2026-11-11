package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/db9979/sshtermy/internal/llm"
)

// ---------------------------------------------------------------- Karte reparieren
//
// Ein Befehl aus einer Antwortkarte („KI fragen“) ist fehlgeschlagen. Der
// Begleiter liefert meist schon einen Fix; ist er aus oder still, fragt die
// Oberfläche hier gezielt nach genau einem korrigierten Befehl. Die Frage des
// Admins geht mit, damit der Fix dasselbe Ziel hat wie die Karte.

const sysCardFix = "Ein Admin hat dich etwas gefragt, und ein Befehl aus deiner Antwort ist fehlgeschlagen. Lies die " +
	"Fehlermeldung und gib genau einen korrigierten Befehl, der dasselbe erreicht. Antworte nur mit einem JSON-Objekt " +
	`{"befehl": "…", "grund": "…"}: befehl eine Zeile, höchstens 300 Zeichen; grund höchstens 160 Zeichen, was falsch war. ` +
	"Lässt sich der Fehler nicht durch einen anderen Befehl beheben (Rechte fehlen, Dienst nicht installiert, Netz weg), " +
	`antworte {"keiner": true, "grund": "…"}.` + guard

// CardFix: Ergebnis von FixCard. Command leer = kein Fix möglich.
type CardFix struct {
	Command Suggestion
	Reason  string
}

// FixCard fragt nach einem korrigierten Befehl für einen fehlgeschlagenen
// Kartenbefehl. question ist die Frage, auf die die Karte die Antwort war.
func (a *Agent) FixCard(ctx context.Context, question string, f Failure, c Context) (CardFix, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Frage des Admins: %s\n", Redact(question))
	fmt.Fprintf(&b, "Fehlgeschlagener Befehl aus deiner Antwort: %s\n", Redact(f.Command))
	if f.Exit != nil {
		fmt.Fprintf(&b, "Exit-Code: %d\n", *f.Exit)
	}
	if f.Reason != "" {
		fmt.Fprintf(&b, "Hinweis: %s\n", f.Reason)
	}
	if out := strings.TrimSpace(f.Output); out != "" {
		c.Screen = "$ " + f.Command + "\n" + tail(out, 3000)
	}
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: a.sys(sysCardFix)},
		{Role: "user", Content: material(c) + "\n\n" + b.String()},
	}, llm.Opts{MaxTokens: followTokens, Temperature: 0.1, NoThink: true, Fast: true, Task: TaskExplain})
	if err != nil {
		return CardFix{}, err
	}
	return parseCardFix(res, f.Command, c.kind())
}

func parseCardFix(res llm.Result, failed, kind string) (CardFix, error) {
	var v struct {
		Keiner bool   `json:"keiner"`
		Befehl string `json:"befehl"`
		Grund  string `json:"grund"`
	}
	if !strictJSON(res.Content, &v) {
		return CardFix{}, unreadable(res)
	}
	reason := flatText(Clean(v.Grund), maxReason)
	if v.Keiner {
		return CardFix{Reason: reason}, nil
	}
	cmd, ok := oneLine(v.Befehl, maxCmd)
	if !ok || cmd == "" || strings.Join(strings.Fields(cmd), " ") == strings.Join(strings.Fields(failed), " ") {
		return CardFix{Reason: reason}, nil // derselbe Befehl nochmal ist kein Fix
	}
	return CardFix{Command: annotate(cmd, reason, kind), Reason: reason}, nil
}
