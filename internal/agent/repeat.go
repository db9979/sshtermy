package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/llm"
	"github.com/db9979/sshtermy/internal/safety"
)

// ---------------------------------------------------------------- Wiederholungen

// Anlass (2026-11-12): Im Ziel-Modus scheiterte „cat /etc/netplan/*.yaml“ mit
// „keine Berechtigung“, „sudo cat …“ klappte, danach schlug das Modell
// sechsmal „cat /etc/netplan/90-NM-….yaml“ ohne sudo vor. Es sah weder die
// Ausgaben noch eine Regel gegen Wiederholungen. Jetzt prüft sshTermy selbst,
// ob ein Vorschlag einen schon gescheiterten Befehl wiederholt, fragt einmal
// mit konkretem Hinweis nach und setzt im Ziel-Modus bei fehlender
// Berechtigung zur Not selbst sudo davor.

var permRE = regexp.MustCompile(`(?i)permission denied|operation not permitted|Keine Berechtigung|Access is denied|must be run as root|are you root\?`)

// permDenied: ist der Schritt an fehlenden Rechten gescheitert?
func permDenied(h DoneStep) bool {
	if h.ExitHint == "" || h.Skipped || h.File != "" {
		return false
	}
	return permRE.MatchString(h.Output) || permRE.MatchString(h.ExitHint) ||
		strings.Contains(h.ExitHint, i18n.T("keine Berechtigung"))
}

func normCmd(s string) string { return strings.Join(strings.Fields(s), " ") }

// cmdPaths: absolute Pfade im Befehl (ohne Anführungszeichen und Satzzeichen).
func cmdPaths(cmd string) []string {
	var out []string
	for _, f := range strings.Fields(cmd) {
		f = strings.Trim(f, `"'();|&<>`)
		if i := strings.IndexByte(f, '='); i > 0 && !strings.HasPrefix(f, "/") {
			f = f[i+1:] // --file=/etc/x
		}
		if strings.HasPrefix(f, "/") && len(f) > 1 {
			out = append(out, path.Clean(f))
		}
	}
	return out
}

// program: erstes Wort des Befehls (ohne sudo davor).
func program(cmd string) string {
	f := strings.Fields(cmd)
	for len(f) > 0 && (f[0] == "sudo" || f[0] == "doas" || strings.HasPrefix(f[0], "-")) {
		f = f[1:]
	}
	if len(f) == 0 {
		return ""
	}
	return path.Base(f[0])
}

// samePlace: zeigen zwei Pfade auf dieselbe Datei, dasselbe Muster oder
// Dateien im selben (nicht zu allgemeinen) Ordner?
func samePlace(a, b string) bool {
	if a == b {
		return true
	}
	if ok, _ := path.Match(a, b); ok {
		return true
	}
	if ok, _ := path.Match(b, a); ok {
		return true
	}
	da, db := path.Dir(a), path.Dir(b)
	return da == db && strings.Count(da, "/") >= 2 // /etc/netplan ja, /etc nein
}

// repeatOf: wiederholt cmd einen gescheiterten Schritt? perm meldet, ob das
// an fehlenden Rechten lag (dann hilft sudo).
func repeatOf(cmd string, hist []DoneStep) (prev string, perm, ok bool) {
	n := normCmd(cmd)
	if n == "" {
		return "", false, false
	}
	elevated := safety.Elevated(cmd)
	for i := len(hist) - 1; i >= 0; i-- {
		h := hist[i]
		if h.ExitHint == "" || h.Skipped || h.File != "" {
			continue
		}
		p := permDenied(h)
		if normCmd(h.Command) == n {
			return h.Command, p, true
		}
		// Rechte fehlten: derselbe Befehl auf eine Datei daneben scheitert genauso
		if !p || elevated || safety.Elevated(h.Command) || program(h.Command) != program(cmd) {
			continue
		}
		for _, x := range cmdPaths(h.Command) {
			for _, y := range cmdPaths(cmd) {
				if samePlace(x, y) {
					return h.Command, true, true
				}
			}
		}
	}
	return "", false, false
}

// sudoWorked: hat schon ein Schritt mit sudo geklappt?
func sudoWorked(hist []DoneStep) bool {
	for _, h := range hist {
		if !h.Skipped && h.File == "" && h.ExitHint == "" && safety.Elevated(h.Command) {
			return true
		}
	}
	return false
}

func repeatFix(prev string, perm, sudoOK bool) string {
	s := fmt.Sprintf("Dein befehl wiederholt einen Schritt, der schon fehlgeschlagen ist (%s). Schlag ihn nicht noch einmal vor. ", short(prev, 160))
	switch {
	case perm && sudoOK:
		s += "Er scheiterte an fehlenden Rechten; sudo hat in diesem Lauf schon geklappt, also nimm sudo. "
	case perm:
		s += "Er scheiterte an fehlenden Rechten: nimm sudo oder einen anderen Weg. "
	default:
		s += "Lies die Ausgabe oben und versuche einen anderen Weg. "
	}
	return s + "Steht die nötige Information schon in einer Ausgabe oben, nutze sie, statt sie erneut zu lesen. " +
		`Ist das Ziel erreicht, antworte mit {"fertig": true, "grund": "…"}. Antworte jetzt NUR mit dem JSON-Objekt, ohne Text davor oder danach.`
}

// avoidRepeat fragt einmal nach, wenn st einen gescheiterten Befehl
// wiederholt. Bleibt das Modell dabei, setzt sshTermy bei fehlenden Rechten
// sudo davor (nur wenn withSudo, also im Ziel-Modus, wo der Mensch jeden
// Schritt bestätigt) oder markiert den Vorschlag im Grund.
func (a *Agent) avoidRepeat(ctx context.Context, msgs []llm.Message, o llm.Opts, st Step, hist []DoneStep, kind string, withSudo bool, parse func(string) (Step, error)) Step {
	if st.Command == "" || st.File != nil {
		return st
	}
	prev, perm, ok := repeatOf(st.Command, hist)
	if !ok {
		return st
	}
	sudoOK := sudoWorked(hist)
	js, _ := json.Marshal(map[string]string{"befehl": st.Command, "grund": st.Reason})
	msgs = append(append([]llm.Message(nil), msgs...),
		llm.Message{Role: "assistant", Content: string(js)},
		llm.Message{Role: "user", Content: repeatFix(prev, perm, sudoOK)})
	o.Temperature = 0.1
	if res, err := a.C.CompleteResult(ctx, msgs, o); err == nil {
		if st2, perr := parse(res.Content); perr == nil {
			if st2.Command == "" || st2.File != nil {
				return st2 // fertig, Frage oder Datei
			}
			if _, _, again := repeatOf(st2.Command, hist); !again {
				return st2
			}
			st = st2
			prev, perm, _ = repeatOf(st.Command, hist)
		}
	}
	if perm && withSudo && kind == "unix" && !safety.Elevated(st.Command) {
		cmd := "sudo " + strings.TrimSpace(st.Command)
		if len(cmd) <= maxCmd {
			reason := i18n.T("mit sudo, ohne ging es schon nicht")
			if st.Reason != "" {
				reason = st.Reason + " (" + reason + ")"
			}
			return Step{Suggestion: annotate(cmd, reason, kind), Plan: st.Plan}
		}
	}
	st.Reason = strings.TrimSpace(i18n.T("Achtung: so schon einmal fehlgeschlagen.") + " " + st.Reason)
	return st
}
