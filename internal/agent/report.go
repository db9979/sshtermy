package agent

import (
	"context"
	"strings"

	"github.com/db9979/sshtermy/internal/llm"
)

// ---------------------------------------------------------------- Log-Diagnose

const sysDiagnose = "Du bekommst Log- oder Befehlsausgabe von einem Server oder Netzwerkgerät (journalctl, dmesg, syslog, " +
	"show logging, Anwendungslogs …). Werte sie für einen Admin aus, kurz und konkret, als schlichter Text ohne Fettdruck " +
	"und Überschriften. Zuerst eine Zeile „Lage:“ mit einem Satz, ob etwas Ernstes vorliegt. Dann „Auffällig:“ mit höchstens " +
	"6 Stichpunkten („- “), je mit Zeitpunkt oder Zeile, falls vorhanden, und was es bedeutet. Fasse Wiederholungen zusammen " +
	"(„47× …“). Dann „Ursache:“ mit der wahrscheinlichsten Erklärung und zuletzt „Prüfen:“ mit 1 bis 3 Befehlen, je in einem " +
	"eigenen Codeblock ```sh … ``` mit genau einem Befehl, die nur lesen. Erfinde nichts, was nicht in der Ausgabe steht; " +
	"ist alles unauffällig, sag das." + guard

// Diagnose wertet Log-Ausgabe aus (gestreamt wie im Frage-Modus). text ist
// die Markierung oder der Verlauf; er ersetzt im Material den Bildschirm.
func (a *Agent) Diagnose(ctx context.Context, text string, c Context, onDelta func(string)) error {
	c.Screen = tail(text, 24000)
	return a.C.Stream(ctx, []llm.Message{
		{Role: "system", Content: sys(sysDiagnose)},
		{Role: "user", Content: "Bitte diese Ausgabe auswerten.\n\n" + material(c)},
	}, llm.Opts{MaxTokens: 900, Temperature: 0.2}, func(d string) { onDelta(Clean(d)) })
}

// ---------------------------------------------------------------- Änderungsbericht

const sysReport = "Schreibe aus Protokollen von Shell-Sitzungen einen Änderungsbericht, den ein Admin direkt in ein Ticket " +
	"oder eine Doku kopieren kann (Markdown, Sprache des Admins, Deutsch wenn unklar). Aufbau: eine Zeile „Zusammenfassung:“ " +
	"mit einem Satz. Dann pro Host eine Zeile „### <Host>“ und darunter Stichpunkte („- “) nur mit dem, was geändert wurde " +
	"(Konfiguration, Pakete, Dienste, Dateien, Benutzer, Neustarts), je mit dem Befehl in `Backticks`. Reine Leseabfragen " +
	"(ls, cat, show, systemctl status …) nicht aufzählen. Fehlgeschlagenes kennzeichnen („fehlgeschlagen:“). Am Ende " +
	"„Offen:“ mit dem, was nicht abgeschlossen wirkt, sonst „Offen: nichts erkennbar“. Gab es keine Änderung, schreib genau " +
	"das. Erfinde nichts, was nicht im Protokoll steht; Zeitangaben nur aus dem Protokoll." + guard

// Report schreibt einen Änderungsbericht aus Sitzungsprotokollen
// (transcript wie bei Summarize).
func (a *Agent) Report(ctx context.Context, transcript string) (string, error) {
	body := strings.NewReplacer("<<<", "‹‹‹", ">>>", "›››").Replace(Redact(transcript))
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: sys(sysReport)},
		{Role: "user", Content: "Protokoll:\n<<<\n" + body + "\n>>>"},
	}, llm.Opts{MaxTokens: 1500, Temperature: 0.2, NoThink: true})
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(Clean(stripThink(res.Content)))
	if out == "" {
		return "", unreadable(res)
	}
	return out, nil
}
