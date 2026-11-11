package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/llm"
	"github.com/db9979/sshtermy/internal/safety"
)

// ---------------------------------------------------------------- Vorgang über mehrere Fenster
//
// Ein Vorgang verbindet mehrere offene Fenster (Tabs/Bereiche), z. B. Switch,
// Webserver und Datenbank. Die KI sieht alle zusammen und nennt zu jedem
// Schritt das Fenster, in das der Befehl gehört. Ausgeführt wird nie hier:
// die Oberfläche tippt den Befehl im Zielfenster vor, ⏎ drückt der Mensch.

// Window: ein Fenster im Vorgang, wie es die KI sieht.
type Window struct {
	N    int     // Fensternummer (bleibt, solange der Tab offen ist)
	Name string  // Alias der Verbindung
	Prod bool    // produktiv (Schutz an)
	Ctx  Context // Material: Profil, je nach Freigabe Befehle und Ausgabe
}

// Kind: Regelgruppe (unix, windows, network) des Fensters.
func (w Window) Kind() string { return w.Ctx.kind() }

// MultiStep: ein Schritt in einem bestimmten Fenster.
type MultiStep struct {
	Win     int    // Fensternummer; 0 = System ist nicht im Vorgang (siehe System)
	System  string // Name des fehlenden Systems (nur bei Win == 0)
	Command string // eine Zeile; {{name}} = gemerkter Wert aus einem früheren Schritt
	Reason  string
	Save    string // Wert aus der Ausgabe merken unter diesem Namen
	Pattern string // … mit diesem regulären Ausdruck (erste Gruppe)
	Undo    string // Rückweg für diesen Schritt (optional)

	Danger   []safety.Finding
	Elevated bool
	ReadOnly bool // ändert sicher nichts (darf im Agent allein laufen)
	Needs    []string
}

// Finding2: Befund aus einem Fenster.
type WinFinding struct {
	Win  int
	Text string
}

// MultiAnswer: Antwort der KI über alle Fenster.
type MultiAnswer struct {
	Summary  string
	Findings []WinFinding
	Steps    []MultiStep
	Missing  []WinFinding // Win = 0, Text = "system: grund"
	Tokens   int
}

const (
	maxMultiSteps = 20
	maxPattern    = 200
)

var (
	saveName    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	placeholder = regexp.MustCompile(`\{\{([a-z0-9][a-z0-9-]{0,31})\}\}`)
)

// Placeholders: Namen der {{…}}-Werte in cmd.
func Placeholders(cmd string) []string {
	var out []string
	for _, m := range placeholder.FindAllStringSubmatch(cmd, -1) {
		out = append(out, m[1])
	}
	return out
}

// FillPlaceholders setzt gemerkte Werte ein; fehlende bleiben stehen.
func FillPlaceholders(cmd string, vals map[string]string) string {
	return placeholder.ReplaceAllStringFunc(cmd, func(m string) string {
		if v, ok := vals[placeholder.FindStringSubmatch(m)[1]]; ok {
			return v
		}
		return m
	})
}

// Extract: Wert aus einer Ausgabe nach dem Muster eines Schritts. Ohne
// Gruppe zählt der ganze Treffer. ok = false, wenn nichts passt.
func (s MultiStep) Extract(out string) (string, bool) {
	if s.Pattern == "" {
		return "", false
	}
	rx, err := regexp.Compile(s.Pattern)
	if err != nil {
		return "", false
	}
	m := rx.FindStringSubmatch(out)
	if m == nil {
		return "", false
	}
	v := m[0]
	if len(m) > 1 {
		v = m[1]
	}
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 200 || strings.ContainsAny(v, "\r\n;&|`$<>") {
		return "", false // nichts, was einen eingesetzten Befehl verbiegen könnte
	}
	return v, true
}

const multiRules = " Regeln: fenster ist IMMER die Nummer eines Fensters aus dem Material; 0 nur für ein System, das in keinem Fenster offen ist " +
	`(dann zusätzlich "system": "Name"). Jeder befehl ist genau EINE Zeile, höchstens 300 Zeichen, nicht interaktiv, und MUSS im Befehlssatz ` +
	"genau des Fensters funktionieren, das du nennst (Systemprofil dieses Fensters): auf einem Switch keine Linux-Befehle, auf Linux keine Switch-Befehle. " +
	"Konfigurationsmodi auf Netzwerkgeräten: jede Zeile ein eigener Schritt (configure terminal, interface …, …, end). " +
	"Die Reihenfolge der schritte ist die Ausführungsreihenfolge über alle Fenster. Erst lesen, dann ändern, nach jeder Änderung prüfen. " +
	`Braucht ein späterer Schritt einen Wert aus der Ausgabe eines früheren (IP, MAC, Port, ID), gib beim früheren "merke": {"name": "web02-ip", "muster": "…"} ` +
	"an (name nur a-z, 0-9, -; muster ist ein regulärer Ausdruck, die erste Gruppe ist der Wert) und schreib im späteren befehl {{web02-ip}}. " +
	`"zurueck" ist optional der Befehl, der eine Änderung wieder rückgängig macht (gleiches Fenster). ` +
	"Erfinde keine Fensternummern und keine Ausgaben; was du nicht weißt, prüfst du mit einem lesenden Schritt."

const sysMulti = "Du hilfst einem Admin bei einem Vorgang, der mehrere Systeme betrifft. Jedes System ist ein eigenes Terminalfenster " +
	"mit einer Nummer. Du siehst alle Fenster des Vorgangs zusammen und sagst zu jedem Schritt, in welches Fenster der Befehl gehört. " +
	"Antworte nur mit GENAU EINEM JSON-Objekt, ohne Text davor oder danach: " +
	`{"antwort": "…", "befund": [{"fenster": 1, "text": "…"}], "schritte": [{"fenster": 1, "befehl": "…", "grund": "…", "merke": {"name": "…", "muster": "…"}, "zurueck": "…"}], ` +
	`"fehlt": [{"system": "…", "grund": "…"}]}. antwort: 1 bis 3 Sätze. befund: was du je Fenster siehst (nur Fenster mit Befund). ` +
	"schritte: höchstens 20, merke und zurueck nur wenn nötig. fehlt: Systeme, die für den Vorgang nötig sind, aber in keinem Fenster offen sind." +
	multiRules + guard

// MultiAsk: Frage oder Ziel über alle Fenster eines Vorgangs. goal ist das
// Ziel des Vorgangs (darf leer sein), q die Frage. plan = vollständigen Plan
// bis zum Ziel verlangen (Plan in Bahnen), sonst kurze Antwort mit Schritten.
func (a *Agent) MultiAsk(ctx context.Context, goal, q string, wins []Window, prev []Turn, plan bool) (MultiAnswer, error) {
	msg := multiMaterial(goal, wins)
	task := "Frage"
	if plan {
		task = "Plane den ganzen Vorgang bis zum Ziel, mit Prüfung am Ende und Rückweg je Änderung. Auftrag"
	}
	for _, t := range lastTurns(prev, 3) {
		msg += "\n\nFrühere Frage: " + Redact(t.Q) + "\nFrühere Antwort (gekürzt): " + tail(Redact(t.A), 800)
	}
	msg += "\n\n" + task + ": " + Redact(q)
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: a.sys(sysMulti)},
		{Role: "user", Content: msg},
	}, llm.Opts{MaxTokens: goalTokens, Temperature: 0.2, NoThink: true, Task: TaskAsk})
	tok := estTokens(sysMulti, msg, res.Content)
	if err != nil {
		return MultiAnswer{Tokens: tok}, err
	}
	ans, perr := ParseMulti(res.Content, wins)
	ans.Tokens = tok
	if perr != nil {
		return ans, unreadable(res, perr)
	}
	return ans, nil
}

func lastTurns(t []Turn, n int) []Turn {
	if len(t) > n {
		return t[len(t)-n:]
	}
	return t
}

// multiMaterial: alle Fenster als Material, jedes in eigenem Block.
func multiMaterial(goal string, wins []Window) string {
	var b strings.Builder
	if g := strings.TrimSpace(goal); g != "" {
		b.WriteString("Ziel des Vorgangs: " + Redact(g) + "\n\n")
	}
	if len(wins) > 0 && !wins[0].Ctx.Now.IsZero() {
		b.WriteString(Today(wins[0].Ctx.Now) + "\n\n")
	}
	for _, w := range wins {
		c := w.Ctx
		c.Now = time.Time{} // Datum steht schon oben
		prod := ""
		if w.Prod {
			prod = " · produktiv"
		}
		fmt.Fprintf(&b, "Fenster %d: %s%s\n%s\n\n", w.N, w.Name, prod, material(c))
	}
	return strings.TrimSpace(b.String())
}

// ParseMulti prüft die Antwort streng: Fenster müssen existieren, Befehle
// einzeilig, Muster gültig. Gefahr und „nur lesen“ bestimmt sshTermy nach
// dem System des jeweiligen Fensters, nicht die KI.
func ParseMulti(out string, wins []Window) (MultiAnswer, error) {
	var v struct {
		Antwort string `json:"antwort"`
		Befund  []struct {
			Fenster int    `json:"fenster"`
			Text    string `json:"text"`
		} `json:"befund"`
		Schritte []struct {
			Fenster int    `json:"fenster"`
			System  string `json:"system"`
			Befehl  string `json:"befehl"`
			Grund   string `json:"grund"`
			Merke   *struct {
				Name   string `json:"name"`
				Muster string `json:"muster"`
			} `json:"merke"`
			Zurueck string `json:"zurueck"`
		} `json:"schritte"`
		Fehlt []struct {
			System string `json:"system"`
			Grund  string `json:"grund"`
		} `json:"fehlt"`
	}
	if !strictJSON(out, &v) {
		return MultiAnswer{}, fmt.Errorf("kein JSON-Objekt")
	}
	byN := map[int]Window{}
	for _, w := range wins {
		byN[w.N] = w
	}
	var r MultiAnswer
	r.Summary = flatText(Clean(v.Antwort), maxSummary)
	for _, f := range v.Befund {
		if _, ok := byN[f.Fenster]; !ok {
			continue
		}
		if t := flatText(Clean(f.Text), maxShown); t != "" {
			r.Findings = append(r.Findings, WinFinding{f.Fenster, t})
		}
	}
	saved := map[string]bool{}
	for _, s := range v.Schritte {
		if len(r.Steps) >= maxMultiSteps {
			break
		}
		cmd, ok := oneLine(s.Befehl, maxCmd)
		if !ok || cmd == "" {
			return r, fmt.Errorf("befehl nicht einzeilig oder zu lang")
		}
		st := MultiStep{Win: s.Fenster, Command: cmd}
		st.Reason, _ = oneLine(flatText(Clean(s.Grund), maxShown), maxShown)
		kind := "unix"
		if w, ok := byN[s.Fenster]; ok {
			kind = w.Kind()
		} else {
			sys := flatText(Clean(s.System), 40)
			if sys == "" {
				return r, fmt.Errorf("fenster %d gibt es nicht", s.Fenster)
			}
			st.Win, st.System = 0, sys
		}
		if m := s.Merke; m != nil && m.Name != "" {
			name := strings.ToLower(strings.TrimSpace(m.Name))
			if !saveName.MatchString(name) || len(m.Muster) > maxPattern {
				return r, fmt.Errorf("merke ungültig")
			}
			if _, err := regexp.Compile(m.Muster); err != nil {
				return r, fmt.Errorf("muster ungültig: %v", err)
			}
			st.Save, st.Pattern = name, m.Muster
		}
		if u, ok := oneLine(s.Zurueck, maxCmd); ok {
			st.Undo = u
		}
		for _, p := range Placeholders(cmd) {
			if !saved[p] {
				st.Needs = append(st.Needs, p) // wird vor dem Vortippen erfragt
			}
		}
		// Gefahr nach dem System des Zielfensters, Platzhalter als Wort geprüft
		probe := placeholder.ReplaceAllString(cmd, "x")
		sg := annotate(probe, "", kind)
		st.Danger, st.Elevated = sg.Danger, sg.Elevated
		st.ReadOnly = st.Win != 0 && len(st.Danger) == 0 && !st.Elevated && safety.ReadOnly(probe, kind)
		if st.Save != "" {
			saved[st.Save] = true
		}
		r.Steps = append(r.Steps, st)
	}
	for _, f := range v.Fehlt {
		sys := flatText(Clean(f.System), 40)
		if sys == "" {
			continue
		}
		why := flatText(Clean(f.Grund), maxShown)
		r.Missing = append(r.Missing, WinFinding{0, strings.TrimSpace(sys + ": " + why)})
	}
	if r.Summary == "" && len(r.Steps) == 0 && len(r.Findings) == 0 {
		return r, fmt.Errorf("leere Antwort")
	}
	return r, nil
}

// ---------------------------------------------------------------- Begleiter über alle Fenster

const sysCross = "Du begleitest einen Admin bei einem Vorgang über mehrere Terminalfenster. In einem Fenster ist gerade ein Befehl " +
	"fehlgeschlagen. Prüfe NUR, ob die Ursache in einem ANDEREN Fenster des Vorgangs liegt oder dort behoben werden muss " +
	"(z. B. Firewall auf dem anderen System, Dienst dort nicht neu geladen, VLAN am Switch). Antworte nur mit einem JSON-Objekt: " +
	`{"still": true}, wenn kein Zusammenhang mit einem anderen Fenster klar erkennbar ist (das ist der Normalfall), sonst ` +
	`{"fenster": 3, "text": "…", "befehl": "…"}: fenster = Nummer des Fensters, in dem etwas zu tun ist; text = 1 bis 3 kurze Sätze, ` +
	"welcher Zusammenhang und was zu tun ist; befehl optional, eine Zeile, im Befehlssatz GENAU dieses Fensters." + guard

// CrossHint: Hinweis des Begleiters auf ein anderes Fenster.
type CrossHint struct {
	Win     int
	Text    string
	Command string
	Danger  []safety.Finding
	Tokens  int
}

// Cross fragt, ob ein Fehlschlag in Fenster failWin mit einem anderen
// Fenster zusammenhängt. still = kein Zusammenhang (keine Meldung).
func (a *Agent) Cross(ctx context.Context, failWin int, f Failure, goal string, wins []Window) (CrossHint, bool, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Fehlgeschlagen in Fenster %d: %s\n", failWin, Redact(f.Command))
	if f.Exit != nil {
		fmt.Fprintf(&b, "Exit-Code: %d\n", *f.Exit)
	}
	if f.Reason != "" {
		fmt.Fprintf(&b, "Hinweis: %s\n", f.Reason)
	}
	body := strings.NewReplacer("<<<", "‹‹‹", ">>>", "›››").Replace(Redact(tail(f.Output, 3000)))
	msg := multiMaterial(goal, wins) + "\n\n" + b.String() + "Ausgabe:\n<<<\n" + body + "\n>>>"
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: a.sys(sysCross)},
		{Role: "user", Content: msg},
	}, llm.Opts{MaxTokens: followTokens, Temperature: 0.1, NoThink: true, Task: TaskCompanion})
	h := CrossHint{Tokens: estTokens(sysCross, msg, res.Content)}
	if err != nil {
		return h, false, err
	}
	var v struct {
		Still   bool   `json:"still"`
		Fenster int    `json:"fenster"`
		Text    string `json:"text"`
		Befehl  string `json:"befehl"`
	}
	if !strictJSON(res.Content, &v) {
		return h, false, unreadable(res, nil)
	}
	if v.Still {
		return h, true, nil
	}
	var win *Window
	for i := range wins {
		if wins[i].N == v.Fenster {
			win = &wins[i]
		}
	}
	h.Text = flatText(Clean(v.Text), maxShown)
	if win == nil || win.N == failWin || h.Text == "" {
		return h, true, nil // kein anderes Fenster: nichts melden
	}
	h.Win = win.N
	if c, ok := oneLine(v.Befehl, maxCmd); ok && c != "" {
		h.Command = c
		h.Danger = safety.Check(c, win.Kind())
	}
	return h, false, nil
}

// ---------------------------------------------------------------- Rückblick über den Vorgang

// MultiEvent: ein ausgeführter Befehl im Vorgang.
type MultiEvent struct {
	At      time.Time
	Win     int
	Command string
	Failed  bool
	Note    string // z. B. Fehlergrund oder gemerkter Wert
}

// MultiRecap: Zusammenfassung über alle Fenster.
type MultiRecap struct {
	Points []string
	Undo   []MultiStep // Rückweg, in umgekehrter Reihenfolge
	Open   []string
	Tokens int
}

const sysMultiRecap = "Du fasst einen Vorgang über mehrere Terminalfenster für ein Change-Protokoll zusammen. " +
	"Antworte nur mit einem JSON-Objekt: " +
	`{"zusammenfassung": ["…"], "zurueck": [{"fenster": 1, "befehl": "…"}], "offen": ["…"]}. ` +
	"zusammenfassung: 2 bis 6 kurze Punkte, was sich auf welchem System geändert hat und was geprüft wurde. " +
	"zurueck: Befehle, die die Änderungen rückgängig machen, in der Reihenfolge, in der man sie ausführt (also umgekehrt), " +
	"jeder im Befehlssatz seines Fensters; leer, wenn nur gelesen wurde. offen: was noch zu tun bleibt (darf leer sein)." + guard

// RecapMulti fasst den Vorgang zusammen.
func (a *Agent) RecapMulti(ctx context.Context, goal string, evs []MultiEvent, wins []Window) (MultiRecap, error) {
	var b strings.Builder
	for _, e := range evs {
		st := "ok"
		if e.Failed {
			st = "FEHLER"
		}
		line := fmt.Sprintf("%s Fenster %d [%s] %s", e.At.Format("15:04"), e.Win, st, e.Command)
		if e.Note != "" {
			line += " (" + e.Note + ")"
		}
		b.WriteString(line + "\n")
	}
	body := strings.NewReplacer("<<<", "‹‹‹", ">>>", "›››").Replace(Redact(tail(b.String(), 12000)))
	for i := range wins {
		wins[i].Ctx.Screen, wins[i].Ctx.Commands = "", nil
	}
	msg := multiMaterial(goal, wins) + "\n\nAblauf:\n<<<\n" + body + "\n>>>"
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: a.sys(sysMultiRecap)},
		{Role: "user", Content: msg},
	}, llm.Opts{MaxTokens: goalTokens, Temperature: 0.2, NoThink: true, Task: TaskRecap})
	r := MultiRecap{Tokens: estTokens(sysMultiRecap, msg, res.Content)}
	if err != nil {
		return r, err
	}
	var v struct {
		Zusammenfassung []string `json:"zusammenfassung"`
		Zurueck         []struct {
			Fenster int    `json:"fenster"`
			Befehl  string `json:"befehl"`
		} `json:"zurueck"`
		Offen []string `json:"offen"`
	}
	if !strictJSON(res.Content, &v) {
		return r, unreadable(res, nil)
	}
	byN := map[int]Window{}
	for _, w := range wins {
		byN[w.N] = w
	}
	for _, p := range v.Zusammenfassung {
		if p = flatText(Clean(p), maxShown); p != "" && len(r.Points) < 8 {
			r.Points = append(r.Points, p)
		}
	}
	for _, u := range v.Zurueck {
		w, ok := byN[u.Fenster]
		c, ok2 := oneLine(u.Befehl, maxCmd)
		if !ok || !ok2 || c == "" || len(r.Undo) >= maxMultiSteps {
			continue
		}
		sg := annotate(c, "", w.Kind())
		r.Undo = append(r.Undo, MultiStep{Win: u.Fenster, Command: c, Danger: sg.Danger, Elevated: sg.Elevated})
	}
	for _, o := range v.Offen {
		if o = flatText(Clean(o), maxShown); o != "" && len(r.Open) < 6 {
			r.Open = append(r.Open, o)
		}
	}
	return r, nil
}
