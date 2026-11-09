package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/db9979/sshtermy/internal/llm"
)

// ---------------------------------------------------------------- Begleiter
//
// Der Begleiter liest im Terminal mit und meldet sich von selbst, aber nur,
// wenn es einen Anlass gibt. Ob es einen gibt, entscheiden feste Regeln in
// der Oberfläche (Fehler, Gefahrenregel, Wiederholung, Stufe); das Modell
// formuliert nur den Kommentar und darf jederzeit „still“ antworten. Wie
// überall gilt: Vorschläge werden nie selbst ausgeführt.

// Anlässe für einen Kommentar.
const (
	EventFail    = "fehler"       // Befehl fehlgeschlagen
	EventDanger  = "gefahr"       // getippte Zeile trifft eine Gefahrenregel, noch nicht abgeschickt
	EventRepeat  = "wiederholung" // gleicher Befehl mehrmals kurz hintereinander
	EventCommand = "befehl"       // gesprächig: jeder Befehl
)

// CompanionEvent: worum es geht.
type CompanionEvent struct {
	Kind    string
	Command string   // ausgeführter oder getippter Befehl
	Output  string   // Ausgabe (nur bei ausgeführten Befehlen)
	Exit    *int     // Exit-Code, wenn bekannt
	Reason  string   // Fehler- oder Gefahrenhinweis der festen Regeln
	Count   int      // Wiederholung: wie oft
	More    []string // gesprächig: weitere Befehle seit dem letzten Kommentar
}

// Remark: Antwort des Begleiters.
type Remark struct {
	Text     string
	Commands []Suggestion
	Fact     string // dauerhafte Tatsache über das System (zum Merken anbieten)
	Quiet    bool   // nichts Nützliches zu sagen
	Tokens   int    // geschätzt (Zeichen / 4), fürs Budget
}

const (
	maxRemark = 280
	maxFact   = 160
)

const sysCompanion = "Du bist der Begleiter eines Admins im Terminal eines Servers oder Netzwerkgeräts. Du liest mit und " +
	"meldest dich kurz, wenn du etwas Nützliches beitragen kannst: Ursache eines Fehlers, ein Risiko, einen besseren Weg, " +
	"einen Tippfehler. Unter „Anlass“ steht, warum du gefragt wirst. Antworte nur mit einem JSON-Objekt " +
	`{"kommentar": "…", "befehle": [{"befehl": "…", "grund": "…"}], "merken": "…"}: ` +
	"kommentar 1 bis 2 Sätze, höchstens 280 Zeichen, schlichter Text, konkret auf die Ausgabe bezogen; " +
	"befehle 0 bis 3 Vorschläge, je eine Zeile, höchstens 300 Zeichen, passend zum System; grund höchstens 160 Zeichen; " +
	"merken nur, wenn das Material sicher eine dauerhafte Tatsache über dieses System zeigt, die beim nächsten Mal hilft " +
	"(z. B. „Backend hinter nginx ist php8.3-fpm“), sonst weglassen. " +
	"Gibt es nichts, was dem Admin wirklich hilft (Befehl hat geklappt, alles normal), antworte {\"still\": true}. " +
	"Lieber still als belanglos; wiederhole nicht, was in der Ausgabe ohnehin steht." + guard

var eventText = map[string]string{
	EventFail:    "Der Befehl ist fehlgeschlagen. Erkläre kurz die wahrscheinliche Ursache und schlag den Fix vor.",
	EventDanger:  "Der Admin hat diese Zeile getippt, aber noch nicht abgeschickt; eine feste Gefahrenregel hat angeschlagen. Sag in einem Satz, was passieren würde, und schlag eine sicherere Variante vor, falls es eine gibt. Ist die Zeile so gewollt und vertretbar, antworte still.",
	EventRepeat:  "Der Admin führt denselben Befehl mehrmals kurz hintereinander aus. Schlag einen besseren Weg vor (z. B. watch, follow, Filter), falls es einen gibt.",
	EventCommand: "Kommentiere den letzten Befehl und seine Ausgabe, wenn es etwas Nützliches zu sagen gibt (Auffälligkeit, nächster sinnvoller Schritt).",
}

// Comment fragt das Modell nach einem Kommentar zu einem Anlass.
func (a *Agent) Comment(ctx context.Context, ev CompanionEvent, c Context) (Remark, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Anlass: %s\n%s\n", ev.Kind, eventText[ev.Kind])
	cmd := ev.Command
	if !c.Raw {
		cmd = Redact(cmd)
	}
	switch ev.Kind {
	case EventDanger:
		fmt.Fprintf(&b, "Getippte Zeile (nicht abgeschickt): %s\n", cmd)
	default:
		fmt.Fprintf(&b, "Befehl: %s\n", cmd)
	}
	if ev.Exit != nil {
		fmt.Fprintf(&b, "Exit-Code: %d\n", *ev.Exit)
	}
	if ev.Reason != "" {
		fmt.Fprintf(&b, "Hinweis der festen Regeln: %s\n", ev.Reason)
	}
	if ev.Count > 1 {
		fmt.Fprintf(&b, "Wiederholt: %d-mal\n", ev.Count)
	}
	if len(ev.More) > 0 {
		b.WriteString("Davor seit dem letzten Kommentar: " + strings.Join(ev.More, " · ") + "\n")
	}
	if out := strings.TrimSpace(ev.Output); out != "" {
		c.Screen = "$ " + ev.Command + "\n" + tail(out, 4000)
	}
	b.WriteString("\n" + material(c))
	msg := b.String()
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: sys(sysCompanion)},
		{Role: "user", Content: msg},
	}, llm.Opts{MaxTokens: followTokens, Temperature: 0.2, NoThink: true})
	r := Remark{Tokens: estTokens(sysCompanion, msg, res.Content)}
	if err != nil {
		return r, err
	}
	var v struct {
		Still     bool   `json:"still"`
		Kommentar string `json:"kommentar"`
		Befehle   []struct {
			Befehl string `json:"befehl"`
			Grund  string `json:"grund"`
		} `json:"befehle"`
		Merken string `json:"merken"`
	}
	if !strictJSON(res.Content, &v) {
		return r, unreadable(res)
	}
	text := flatText(Clean(v.Kommentar), maxRemark)
	if v.Still || text == "" {
		r.Quiet = true
		return r, nil
	}
	r.Text = text
	for _, x := range v.Befehle {
		if len(r.Commands) == 3 {
			break
		}
		cmd, ok := oneLine(x.Befehl, maxCmd)
		if !ok {
			continue
		}
		reason, _ := oneLine(x.Grund, maxReason)
		r.Commands = append(r.Commands, annotate(cmd, reason, c.kind()))
	}
	if f := flatText(Clean(v.Merken), maxFact); f != "" && (c.Raw || Redact(f) == f) {
		r.Fact = f
	}
	return r, nil
}

// ---------------------------------------------------------------- Absicht (Coach)

// Intent: was der Admin offenbar vorhat, und die fehlenden Schritte.
type Intent struct {
	Goal     string // „HTTPS für shop.example.de einrichten“
	Evidence string // woran erkannt
	Steps    []Suggestion
	Tokens   int
}

const sysIntent = "Du schaust einem Admin über die Schulter. Erkenne aus seinen letzten Befehlen, ob er erkennbar auf ein " +
	"konkretes Ziel hinarbeitet (z. B. „HTTPS für shop.example.de einrichten“, „Platte /dev/sdb einbinden“), und welche " +
	"Schritte dafür noch fehlen. Antworte nur mit einem JSON-Objekt " +
	`{"absicht": "…", "belege": "…", "schritte": [{"befehl": "…", "grund": "…"}]}: ` +
	"absicht ein kurzer Satzteil (höchstens 80 Zeichen, als Ziel formuliert), belege höchstens 160 Zeichen (woran du es " +
	"erkennst), schritte 1 bis 6 noch fehlende Befehle in Reihenfolge, je eine Zeile, passend zum System. " +
	"Ist kein klares Ziel erkennbar (nur Herumschauen, Routine) oder ist es schon erreicht, antworte {\"keine\": true}." + guard

// GuessIntent fragt das Modell nach der Absicht hinter den letzten Befehlen.
func (a *Agent) GuessIntent(ctx context.Context, c Context) (Intent, error) {
	msg := "Was hat der Admin vor?\n" + material(c)
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: sys(sysIntent)},
		{Role: "user", Content: msg},
	}, llm.Opts{MaxTokens: followTokens, Temperature: 0.2, NoThink: true})
	in := Intent{Tokens: estTokens(sysIntent, msg, res.Content)}
	if err != nil {
		return in, err
	}
	var v struct {
		Keine    bool   `json:"keine"`
		Absicht  string `json:"absicht"`
		Belege   string `json:"belege"`
		Schritte []struct {
			Befehl string `json:"befehl"`
			Grund  string `json:"grund"`
		} `json:"schritte"`
	}
	if !strictJSON(res.Content, &v) {
		return in, unreadable(res)
	}
	goal := flatText(Clean(v.Absicht), 80)
	if v.Keine || goal == "" {
		return in, nil
	}
	in.Goal, in.Evidence = goal, flatText(Clean(v.Belege), maxReason)
	for _, x := range v.Schritte {
		if len(in.Steps) == 6 {
			break
		}
		cmd, ok := oneLine(x.Befehl, maxCmd)
		if !ok {
			continue
		}
		reason, _ := oneLine(x.Grund, maxReason)
		in.Steps = append(in.Steps, annotate(cmd, reason, c.kind()))
	}
	if len(in.Steps) == 0 {
		in.Goal = "" // ohne fehlende Schritte gibt es nichts anzubieten
	}
	return in, nil
}

// ---------------------------------------------------------------- Rückblick (Coach)

// Recap: Rückblick auf eine Sitzung.
type Recap struct {
	Points  []string // was passiert ist
	Facts   []string // zum Merken für dieses System
	Runbook string   // Name, falls sich die Sitzung als Runbook eignet
	Tokens  int
}

const sysRecap = "Fasse eine Terminal-Sitzung eines Admins kurz zusammen. Antworte nur mit einem JSON-Objekt " +
	`{"zusammenfassung": ["…"], "merken": ["…"], "runbook": "…"}: ` +
	"zusammenfassung 1 bis 5 Stichpunkte (je höchstens 120 Zeichen): was geprüft, behoben, geändert wurde und was offen ist; " +
	"merken 0 bis 4 dauerhafte Tatsachen über dieses System, die beim nächsten Mal helfen (Dienste, Pfade, Besonderheiten; " +
	"nie Passwörter oder Schlüssel); runbook ein kurzer Name (höchstens 60 Zeichen), wenn die Sitzung eine wiederholbare " +
	"Aufgabe war (z. B. „nginx + HTTPS einrichten“), sonst leer. Erfinde nichts, was nicht im Material steht." + guard

// Summarize a session for the recap. log: Befehle mit gekürzter Ausgabe.
func (a *Agent) RecapSession(ctx context.Context, log string, c Context) (Recap, error) {
	c.Screen = ""
	body := log
	if !c.Raw {
		body = Redact(body)
	}
	body = strings.NewReplacer("<<<", "‹‹‹", ">>>", "›››").Replace(tail(body, 12000))
	msg := "Sitzung:\n<<<\n" + body + "\n>>>\n" + material(c)
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: sys(sysRecap)},
		{Role: "user", Content: msg},
	}, llm.Opts{MaxTokens: goalTokens, Temperature: 0.2, NoThink: true})
	r := Recap{Tokens: estTokens(sysRecap, msg, res.Content)}
	if err != nil {
		return r, err
	}
	var v struct {
		Zusammenfassung []string `json:"zusammenfassung"`
		Merken          []string `json:"merken"`
		Runbook         string   `json:"runbook"`
	}
	if !strictJSON(res.Content, &v) {
		return r, unreadable(res)
	}
	for _, p := range v.Zusammenfassung {
		if p = flatText(Clean(p), 120); p != "" && len(r.Points) < 5 {
			r.Points = append(r.Points, p)
		}
	}
	for _, f := range v.Merken {
		if f = flatText(Clean(f), maxFact); f != "" && len(r.Facts) < 4 && Redact(f) == f {
			r.Facts = append(r.Facts, f)
		}
	}
	r.Runbook = flatText(Clean(v.Runbook), 60)
	return r, nil
}

// estTokens: grobe Schätzung (etwa 4 Zeichen pro Token) für Budget und Anzeige.
func estTokens(parts ...string) int {
	n := 0
	for _, p := range parts {
		n += len([]rune(p))
	}
	return n/4 + 1
}
