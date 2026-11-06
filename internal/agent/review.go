package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/db9979/sshtermy/internal/llm"
)

// ---------------------------------------------------------------- Runbook prüfen

const sysReview = "Du prüfst, ob eine Befehlsfolge (Runbook) auf dem beschriebenen System so funktioniert. " +
	"Achte auf Befehlssatz, Paketverwaltung, Init-System und Geräte-CLI (Systemprofil). " +
	`Antworte nur mit einem JSON-Objekt {"schritte": [{"passt": true|false, "befehl": "…", "grund": "…"}], "fazit": "…"}: ` +
	"genau ein Element pro Schritt, in derselben Reihenfolge. passt = true, wenn der Schritt dort unverändert läuft " +
	"(dann befehl leer). Sonst befehl = die Entsprechung für dieses System (eine Zeile, höchstens 300 Zeichen; " +
	"leer, wenn es keine gibt) und grund = was nicht passt (eine Zeile, höchstens 160 Zeichen). " +
	"fazit: ein Satz, höchstens 160 Zeichen. Bewerte nicht, ob ein Schritt gefährlich ist, das prüfen feste Regeln." + guard

// StepReview: Urteil zu einem Schritt.
type StepReview struct {
	OK      bool
	Adapted Suggestion // nur wenn !OK: Entsprechung für das System (Command leer = keine)
	Reason  string
}

// Review: Urteil zu einem Runbook auf einem System.
type Review struct {
	Steps   []StepReview
	Summary string
}

// Fits: läuft jeder Schritt unverändert?
func (r Review) Fits() bool {
	for _, s := range r.Steps {
		if !s.OK {
			return false
		}
	}
	return true
}

// Changed: Zahl der Schritte, die angepasst werden müssten.
func (r Review) Changed() int {
	n := 0
	for _, s := range r.Steps {
		if !s.OK {
			n++
		}
	}
	return n
}

// Apply: die Schritte mit den Vorschlägen der KI. Schritte ohne Entsprechung
// fallen weg; ok = false, wenn dabei etwas fehlen würde.
func (r Review) Apply(steps []string) (out []string, ok bool) {
	ok = true
	for i, s := range steps {
		if i >= len(r.Steps) || r.Steps[i].OK {
			out = append(out, s)
			continue
		}
		if c := r.Steps[i].Adapted.Command; c != "" {
			out = append(out, c)
		} else {
			ok = false
		}
	}
	return out, ok
}

// ErrReviewMismatch: die Antwort passt nicht zur Zahl der Schritte.
var ErrReviewMismatch = errors.New("Antwort passt nicht zu den Schritten")

// Review fragt, ob steps auf dem System in c laufen, und schlägt für die
// übrigen eine Entsprechung vor. Die Gefahrenregeln gelten für jeden
// Vorschlag (Adapted.Danger).
func (a *Agent) Review(ctx context.Context, steps []string, c Context) (Review, error) {
	var b strings.Builder
	for i, s := range steps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, Redact(s))
	}
	out, err := a.C.Complete(ctx, []llm.Message{
		{Role: "system", Content: sys(sysReview)},
		{Role: "user", Content: "Schritte:\n" + strings.NewReplacer("<<<", "‹‹‹", ">>>", "›››").Replace(b.String()) + "\n" + material(c)},
	}, llm.Opts{MaxTokens: followTokens, Temperature: 0.1, NoThink: true})
	if err != nil {
		return Review{}, err
	}
	return parseReview(out, len(steps), c.kind())
}

func parseReview(out string, n int, kind string) (Review, error) {
	var v struct {
		Schritte []struct {
			Passt  bool   `json:"passt"`
			Befehl string `json:"befehl"`
			Grund  string `json:"grund"`
		} `json:"schritte"`
		Fazit string `json:"fazit"`
	}
	if !strictJSON(out, &v) {
		return Review{}, ErrUnreadable
	}
	if len(v.Schritte) != n {
		return Review{}, ErrReviewMismatch
	}
	var r Review
	r.Summary, _ = oneLine(v.Fazit, maxReason)
	for _, s := range v.Schritte {
		reason, _ := oneLine(s.Grund, maxReason)
		sr := StepReview{OK: s.Passt, Reason: reason}
		if !s.Passt {
			cmd, ok := oneLine(s.Befehl, maxCmd)
			if !ok {
				return Review{}, ErrUnreadable
			}
			if cmd != "" {
				sr.Adapted = annotate(cmd, reason, kind)
			}
		}
		r.Steps = append(r.Steps, sr)
	}
	return r, nil
}
