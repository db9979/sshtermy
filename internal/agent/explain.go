package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/llm"
)

// ---------------------------------------------------------------- Fehler erkennen

// Typische Fehlermeldungen von Linux-Werkzeugen, PowerShell/cmd und
// Netzwerkgeräten (Aruba, Cisco, HPE). Geprüft werden nur die letzten Zeilen
// der Ausgabe eines Befehls, damit z. B. ein grep nach "error" in Logdateien
// nicht als Fehlschlag zählt (die Treffer stehen dann mitten in der Ausgabe;
// ganz verhindern lässt sich das heuristisch nicht, daher ist es auch nur ein
// Angebot und keine Meldung).
var failPatterns = []struct {
	rx     *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`(?i)command not found|: not found$|is not recognized as (an internal|the name)|Befehl nicht gefunden`), "Befehl nicht gefunden"},
	{regexp.MustCompile(`(?i)no such file or directory|Datei oder Verzeichnis nicht gefunden|cannot find path`), "Datei oder Ordner fehlt"},
	{regexp.MustCompile(`(?i)permission denied|operation not permitted|Keine Berechtigung|Access is denied|must be run as root|are you root\?`), "keine Berechtigung"},
	{regexp.MustCompile(`(?i)^\s*% ?(invalid input|incomplete command|ambiguous command|unknown command|unrecognized command)|^\s*invalid input:|^\s*unknown command`), "Gerät kennt den Befehl nicht"},
	{regexp.MustCompile(`(?i)^(E|Err):\s|unable to locate package|could not resolve|temporary failure in name resolution`), "Paketverwaltung/Netz meldet Fehler"},
	{regexp.MustCompile(`(?i)^(fatal|error|fehler|critical)(\[[^\]]*\])?:|^\s*\[?error\]?\s|\bERROR\b:|^\S+: error:`), "Fehlermeldung"},
	{regexp.MustCompile(`(?i)Job for \S+ failed|failed to (start|connect|open|load)|\bfailed with\b|\bexit (code|status) [1-9]`), "Dienst/Befehl fehlgeschlagen"},
	{regexp.MustCompile(`(?i)^\s*Active: failed\b|status=[1-9]\d*/FAILURE|\[(emerg|crit|alert)\]|configuration file \S+ test failed`), "Dienst/Befehl fehlgeschlagen"},
	{regexp.MustCompile(`(?i)syntax error|unexpected token|invalid option|unrecognized option|unknown option|usage: `), "Aufruf falsch"},
	{regexp.MustCompile(`(?i)connection refused|connection timed out|no route to host|network is unreachable`), "Verbindung fehlgeschlagen"},
	{regexp.MustCompile(`Traceback \(most recent call last\)|Segmentation fault|core dumped|panic: `), "Programm abgestürzt"},
	{regexp.MustCompile(`(?i)CategoryInfo\s*:|FullyQualifiedErrorId`), "PowerShell-Fehler"},
}

// LooksFailed schätzt aus der Ausgabe eines Befehls, ob er fehlgeschlagen ist.
// Nur die letzten 12 Zeilen zählen.
func LooksFailed(output string) (bool, string) {
	lines := strings.Split(strings.TrimRight(output, "\n "), "\n")
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}
	for _, l := range lines {
		l = strings.TrimRight(l, " \r")
		for _, p := range failPatterns {
			if p.rx.MatchString(l) {
				return true, i18n.T(p.reason)
			}
		}
	}
	return false, ""
}

// ---------------------------------------------------------------- Fehler erklären

const sysExplain = "Ein Befehl in der Shell eines Servers oder Netzwerkgeräts ist fehlgeschlagen. Erkläre in der Sprache des " +
	"Admins (Deutsch, wenn unklar) kurz und konkret, als schlichter Text ohne Fettdruck und Überschriften: zuerst eine Zeile " +
	"„Warum: …“ mit 1 bis 3 Sätzen zur wahrscheinlichen Ursache (bezogen auf die Ausgabe), dann „Fix:“ mit 1 bis 3 Vorschlägen. Jeder Befehl steht in einem eigenen Codeblock " +
	"```sh … ``` mit genau einem Befehl, passend zum System. Sag deutlich, wenn ein Befehl Daten löscht, Rechte ändert " +
	"oder neu startet. Ist die Ursache unklar, nenne den Befehl, der sie am besten zeigt." + guard

// Failure beschreibt einen fehlgeschlagenen Befehl.
type Failure struct {
	Command string
	Output  string
	Exit    *int
	Reason  string // heuristischer Hinweis
}

// Explain fragt das Modell nach Ursache und Fix. Die Antwort ist Markdown mit
// Codeblöcken (wie im Frage-Modus), ohne Denkblöcke von Reasoning-Modellen.
func (a *Agent) Explain(ctx context.Context, f Failure, c Context) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Fehlgeschlagener Befehl: %s\n", Redact(f.Command))
	if f.Exit != nil {
		fmt.Fprintf(&b, "Exit-Code: %d\n", *f.Exit)
	}
	if f.Reason != "" {
		fmt.Fprintf(&b, "Hinweis: %s\n", f.Reason)
	}
	// Die Ausgabe des Befehls ersetzt den Bildschirm im Material (genauer).
	if out := strings.TrimSpace(f.Output); out != "" {
		c.Screen = "$ " + f.Command + "\n" + tail(out, 6000)
	}
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: a.sys(sysExplain)},
		{Role: "user", Content: material(c) + "\n\n" + b.String()},
	}, llm.Opts{MaxTokens: goalTokens, Temperature: 0.2, NoThink: true, Task: TaskExplain})
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(Clean(stripThink(res.Content)))
	if out == "" {
		return "", unreadable(res)
	}
	return out, nil
}

// ---------------------------------------------------------------- Zusammenfassung

const sysSummary = "Fasse ein Protokoll von Shell-Sitzungen für einen Admin zusammen (Deutsch). Antworte in Markdown mit " +
	"höchstens 8 Stichpunkten („- “, ohne Fettdruck und Überschriften): was wurde geprüft, geändert, installiert oder neu gestartet, was ist fehlgeschlagen und " +
	"was ist noch offen. Nenne konkrete Dienste, Pakete, Dateien und Hosts. Erfinde nichts, was nicht im Protokoll steht." + guard

// Summarize fasst ein oder mehrere Sitzungsprotokolle zusammen (transcript
// kommt aus sessionlog.Log.Transcript).
func (a *Agent) Summarize(ctx context.Context, transcript string) (string, error) {
	body := strings.NewReplacer("<<<", "‹‹‹", ">>>", "›››").Replace(Redact(transcript))
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: a.sys(sysSummary)},
		{Role: "user", Content: "Protokoll:\n<<<\n" + body + "\n>>>"},
	}, llm.Opts{MaxTokens: goalTokens, Temperature: 0.2, NoThink: true, Task: TaskSummary})
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(Clean(stripThink(res.Content)))
	if out == "" {
		return "", unreadable(res)
	}
	return out, nil
}

// VisibleAnswer blendet Denkblöcke in (gestreamten) Antworten aus. Während
// das Modell noch denkt, ist das Ergebnis leer.
func VisibleAnswer(s string) string {
	if !strings.Contains(s, "<think") && !strings.Contains(s, "</think") &&
		!strings.Contains(s, "<reason") && !strings.Contains(s, "</reason") {
		return s
	}
	return stripThink(s)
}
