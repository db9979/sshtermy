package agent

// Satz wird zum Befehl (Idee aus lxask): Der Admin tippt am Prompt in
// eigenen Worten, was er will, und bekommt genau eine Befehlszeile zurück,
// die die Oberfläche statt des Satzes einfügt (nicht ausführt). Dazu
// bessert RecheckCommand einen Befehl nach, gegen den die Prüfung
// (internal/cmdcheck) Einwände hat: Programm fehlt, Option steht nicht in
// der man-Seite des Hosts, Vorgabe aus der Frage nicht eingehalten.
//
// Die kleine Offline-KI (Agent.Small, das lxask-Modell) bekommt dafür den
// schlichten Prompt, auf den sie trainiert ist, und antwortet mit einem
// Codeblock statt JSON.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/db9979/sshtermy/internal/llm"
)

const sysWish = "Ein Admin beschreibt in eigenen Worten, was er auf dem System tun will, auf dem er gerade angemeldet ist. " +
	"Schreib genau eine Befehlszeile, die das tut, passend zu System und Shell. Nimm die Dateinamen, Pfade und Werte, " +
	"die er nennt; wo keine genannt sind, einen schlichten Platzhalter wie pfad/zur/datei. Hält er etwas aus " +
	"(„ohne lsof“, „kein sudo“), halte dich daran. Antworte nur mit JSON " +
	`{"befehl": "…"}; geht es nicht mit einem Befehl, {"befehl": "", "grund": "…"} mit höchstens 120 Zeichen.` + guard

const sysRecheck = "Ein Befehl, den du für einen Admin geschrieben hast, wurde geprüft, ohne ihn auszuführen: gegen die " +
	"man-Seiten des Systems und gegen die Frage. Die Prüfung hat Einwände. Schreib einen verbesserten Befehl, der dasselbe " +
	"erreicht und die Einwände ausräumt. Ist ein Programm nicht installiert, nimm ein anderes, das es sicher gibt. " +
	`Antworte nur mit JSON {"befehl": "…"} (eine Zeile, höchstens 300 Zeichen).` + guard

// So spricht die kleine Offline-KI (wie lxask selbst, englisch trainiert).
const sysSmall = "You are a Linux command-line assistant. Reply with one command line that does what the user asks, " +
	"inside a ```bash code block, and nothing else. Use the file names, paths and values the user gave; where none are " +
	"given, use a plain placeholder such as path/to/file."

// ErrNoCommand: Die KI hat keinen Befehl geliefert (Grund im Text).
var ErrNoCommand = errors.New("kein Befehl")

var codeBlock = regexp.MustCompile("(?s)```[A-Za-z0-9_-]*[ \\t]*\\n?(.*?)```")

// smallCommand liest den Befehl aus der Antwort der kleinen KI.
func smallCommand(s string) string {
	s = strings.TrimSpace(s)
	if m := codeBlock.FindStringSubmatch(s); m != nil {
		s = m[1]
	} else if i := strings.Index(s, "```"); i >= 0 { // nicht geschlossen
		s = s[i+3:]
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			s = s[nl+1:]
		}
	}
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "$ "))
		if l != "" && !strings.HasPrefix(l, "#") {
			c, _ := oneLine(l, maxCmd)
			return c
		}
	}
	return ""
}

// WishCommand macht aus einem Satz eine Befehlszeile.
func (a *Agent) WishCommand(ctx context.Context, wish string, c Context) (string, error) {
	wish = strings.TrimSpace(wish)
	if wish == "" {
		return "", ErrNoCommand
	}
	if a.Small {
		return a.smallAsk(ctx, wish, nil)
	}
	c.Screen = lastLines(c.Screen, followLines)
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: a.sys(sysWish)},
		{Role: "user", Content: material(c) + "\n\nBitte des Admins: " + Redact(wish)},
	}, llm.Opts{MaxTokens: followTokens, Temperature: 0.1, NoThink: true, Fast: true, Task: TaskWish})
	if err != nil {
		return "", err
	}
	return parseOneCommand(res)
}

// RecheckCommand bessert einen Befehl nach den Einwänden der Prüfung nach.
func (a *Agent) RecheckCommand(ctx context.Context, question, cmd string, issues []string, c Context) (string, error) {
	if a.Small {
		// Die kleine KI kennt keine Einwände: neue Stichprobe mit Hinweis
		return a.smallAsk(ctx, question, issues)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Frage des Admins: %s\nDein Befehl: %s\nEinwände der Prüfung:\n", Redact(question), Redact(cmd))
	for _, is := range issues {
		fmt.Fprintf(&b, "- %s\n", is)
	}
	c.Screen = lastLines(c.Screen, ghostLines)
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: a.sys(sysRecheck)},
		{Role: "user", Content: material(c) + "\n\n" + b.String()},
	}, llm.Opts{MaxTokens: followTokens, Temperature: 0.3, NoThink: true, Fast: true, Task: TaskRecheck})
	if err != nil {
		return "", err
	}
	out, err := parseOneCommand(res)
	if err == nil && strings.Join(strings.Fields(out), " ") == strings.Join(strings.Fields(cmd), " ") {
		return "", ErrNoCommand
	}
	return out, err
}

// smallAsk: eine Bitte an die kleine KI; avoid sind Einwände gegen den
// letzten Versuch (als „Notes“, so wie lxask Doku-Notizen mitgibt).
func (a *Agent) smallAsk(ctx context.Context, wish string, avoid []string) (string, error) {
	user := "Question: " + Redact(wish)
	if len(avoid) > 0 {
		user = "Reference notes:\n- avoid: " + strings.Join(avoid, "\n- avoid: ") + "\n\n" + user
	}
	temp := 0.1
	if len(avoid) > 0 {
		temp = 0.7 // eine andere Stichprobe, wie lxask bei Einwänden
	}
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: sysSmall},
		{Role: "user", Content: user},
	}, llm.Opts{MaxTokens: 200, Temperature: temp, NoThink: true, Task: TaskWish})
	if err != nil {
		return "", err
	}
	if cmd := smallCommand(res.Content); cmd != "" {
		return cmd, nil
	}
	return "", ErrNoCommand
}

func parseOneCommand(res llm.Result) (string, error) {
	var v struct {
		Befehl string `json:"befehl"`
		Grund  string `json:"grund"`
	}
	if !strictJSON(res.Content, &v) {
		// manche Modelle antworten trotzdem mit einem Codeblock
		if cmd := smallCommand(res.Content); cmd != "" && !strings.HasPrefix(cmd, "{") {
			return cmd, nil
		}
		return "", unreadable(res, nil)
	}
	cmd, ok := oneLine(v.Befehl, maxCmd)
	if !ok || cmd == "" {
		if g := flatText(Clean(v.Grund), maxReason); g != "" {
			return "", fmt.Errorf("%w: %s", ErrNoCommand, g)
		}
		return "", ErrNoCommand
	}
	return cmd, nil
}
