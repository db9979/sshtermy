package agent

// Gegenprüfung: Nach einem Ziel oder Agent-Lauf prüft eine zweite,
// unabhängige KI-Anfrage (wenn eingerichtet ein anderer Anbieter), ob das
// Ziel wirklich erreicht ist. Sie sieht das Ziel, die Schritte mit Ausgaben
// und darf bis zu drei rein lesende Prüfbefehle vorschlagen; was nicht
// sicher nur liest (safety.ReadOnly), fällt weg. Danach urteilt sie mit den
// Ausgaben: bestätigt oder Zweifel, mit kurzem Befund.

import (
	"context"
	"fmt"
	"strings"

	"github.com/db9979/sshtermy/internal/llm"
	"github.com/db9979/sshtermy/internal/safety"
)

// TaskVerify: Gegenprüfung nach einem Ziel (KI-Status).
const TaskVerify = "verify"

// MaxVerifyChecks: so viele Prüfbefehle höchstens.
const MaxVerifyChecks = 3

const sysVerifyChecks = "Du bist eine zweite, unabhängige Prüferin. Ein anderer Assistent hat auf einem Server oder Netzwerkgerät " +
	"ein Ziel abgearbeitet und meldet „erreicht“. Glaub das nicht einfach. Schlag bis zu 3 kurze Befehle vor, die NUR LESEN " +
	"und zeigen, ob das Ziel wirklich erreicht ist (z. B. Dienststatus, Port offen, Datei enthält die Zeile, Version). " +
	"Nichts, was etwas ändert, neu startet, installiert oder schreibt. Antworte nur mit JSON " +
	`{"pruefungen": ["befehl", …]} (leere Liste, wenn die Ausgaben schon reichen).` + guard

const sysVerifyJudge = "Du bist eine zweite, unabhängige Prüferin. Beurteile anhand des Ziels, der Schritte und der Ausgaben deiner " +
	"Prüfbefehle, ob das Ziel wirklich erreicht ist. Sei streng: fehlt ein Beleg, ist das ein Zweifel. Antworte nur mit JSON " +
	`{"erreicht": true|false, "befund": "ein bis zwei kurze Sätze, was belegt ist oder was fehlt"}.` + guard

// Verdict: Ergebnis der Gegenprüfung.
type Verdict struct {
	OK      bool
	Finding string
}

// verifyMaterial: Ziel und Schritte (gekürzt, ohne Geheimnisse).
func verifyMaterial(goal, reason string, steps []DoneStep, c Context) string {
	var b strings.Builder
	b.WriteString(material(c) + "\n\n")
	fmt.Fprintf(&b, "<<<\nZiel: %s\nMeldung des Assistenten: %s\nSchritte:\n", oneLineText(goal), oneLineText(reason))
	from := max(0, len(steps)-15)
	for i, s := range steps[from:] {
		state := ""
		switch {
		case s.Skipped:
			state = " (ausgelassen)"
		case s.Exit != nil:
			state = fmt.Sprintf(" (Exit %d)", *s.Exit)
		case s.ExitHint != "":
			state = " (" + s.ExitHint + ")"
		}
		cmd := s.Command
		if s.File != "" {
			cmd = "Datei " + s.File + ": " + s.Command
		}
		fmt.Fprintf(&b, "%d. %s%s\n", from+i+1, cmd, state)
		if o := strings.TrimSpace(s.Output); o != "" {
			b.WriteString(indent(tailRunes(o, 600)) + "\n")
		}
	}
	b.WriteString(">>>")
	out := b.String()
	if !c.Raw {
		out = Redact(out)
	}
	return out
}

func oneLineText(s string) string { return strings.Join(strings.Fields(s), " ") }

func tailRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return "…" + string(r[len(r)-n:])
}

func indent(s string) string {
	return "   " + strings.ReplaceAll(s, "\n", "\n   ")
}

// VerifyChecks: rein lesende Prüfbefehle (höchstens MaxVerifyChecks).
func (a *Agent) VerifyChecks(ctx context.Context, goal, reason string, steps []DoneStep, c Context) ([]string, error) {
	out, err := a.C.Complete(ctx, []llm.Message{
		{Role: "system", Content: a.sys(sysVerifyChecks)},
		{Role: "user", Content: verifyMaterial(goal, reason, steps, c)},
	}, llm.Opts{MaxTokens: 300, Temperature: 0.1, NoThink: true, JSON: true, Task: TaskVerify})
	if err != nil {
		return nil, err
	}
	var v struct {
		P []string `json:"pruefungen"`
	}
	if !strictJSON(out, &v) {
		return nil, ErrUnreadable
	}
	kind := c.kind()
	var cmds []string
	for _, x := range v.P {
		line, ok := oneLine(x, maxCmd)
		if !ok || line == "" || !safety.ReadOnly(line, kind) {
			continue
		}
		cmds = append(cmds, line)
		if len(cmds) == MaxVerifyChecks {
			break
		}
	}
	return cmds, nil
}

// CheckResult: Ausgabe eines Prüfbefehls.
type CheckResult struct {
	Command string
	Output  string
	Exit    int
}

// VerifyJudge: Urteil mit den Ausgaben der Prüfbefehle.
func (a *Agent) VerifyJudge(ctx context.Context, goal, reason string, steps []DoneStep, checks []CheckResult, c Context) (Verdict, error) {
	var b strings.Builder
	b.WriteString(verifyMaterial(goal, reason, steps, c))
	if len(checks) == 0 {
		b.WriteString("\n\nKeine eigenen Prüfbefehle gelaufen: urteile nur anhand der Schritte.")
	} else {
		b.WriteString("\n\nDeine Prüfbefehle:\n<<<\n")
		for _, x := range checks {
			fmt.Fprintf(&b, "$ %s   (Exit %d)\n%s\n", x.Command, x.Exit, indent(tailRunes(strings.TrimSpace(x.Output), 1500)))
		}
		b.WriteString(">>>")
	}
	user := b.String()
	if !c.Raw {
		user = Redact(user)
	}
	out, err := a.C.Complete(ctx, []llm.Message{
		{Role: "system", Content: a.sys(sysVerifyJudge)},
		{Role: "user", Content: user},
	}, llm.Opts{MaxTokens: 300, Temperature: 0.1, NoThink: true, JSON: true, Task: TaskVerify})
	if err != nil {
		return Verdict{}, err
	}
	var v struct {
		OK *bool  `json:"erreicht"`
		F  string `json:"befund"`
	}
	if !strictJSON(out, &v) || v.OK == nil {
		return Verdict{}, ErrUnreadable
	}
	f, _ := oneLine(Clean(v.F), 400)
	return Verdict{OK: *v.OK, Finding: f}, nil
}
