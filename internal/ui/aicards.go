package ui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
)

// KI-Antworten als Karten: Befehle aus Codeblöcken stehen in eigenen,
// abgesetzten Kästen mit Syntaxfarben und Knöpfen (Einfügen, Ausführen,
// Kopieren). Die Erklärung drumherum lässt sich mit z einklappen, dann
// bleibt nur ihr erster Satz stehen.

type answerPart struct {
	code bool
	text string
	lang string // Sprache am Codeblock („sh“, „json“ …), leer = keine Angabe
}

// card: ein Codeblock aus der Antwort.
type card struct{ code, lang string }

// anyCodeBlock: alle Codeblöcke, gleich welche Sprache dran steht.
var anyCodeBlock = regexp.MustCompile("(?s)```([A-Za-z0-9_+-]*)[ \\t]*\\n?(.*?)```")

// splitAnswer zerlegt eine Antwort in Erklärungstext und Codeblöcke. Ein
// noch offener Codeblock (Antwort läuft noch) zählt schon als Block.
func splitAnswer(ans string) []answerPart {
	var out []answerPart
	addText := func(t string) {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, answerPart{text: t})
		}
	}
	rest := ans
	for {
		loc := anyCodeBlock.FindStringSubmatchIndex(rest)
		if loc == nil {
			break
		}
		addText(rest[:loc[0]])
		if c := cleanCode(rest[loc[4]:loc[5]]); c != "" {
			out = append(out, answerPart{code: true, text: c, lang: strings.ToLower(rest[loc[2]:loc[3]])})
		}
		rest = rest[loc[1]:]
	}
	if i := strings.Index(rest, "```"); i >= 0 { // offener Block beim Streamen
		addText(rest[:i])
		open := rest[i+3:]
		if nl := strings.IndexByte(open, '\n'); nl >= 0 {
			if c := cleanCode(open[nl+1:]); c != "" {
				out = append(out, answerPart{code: true, text: c, lang: strings.ToLower(strings.TrimSpace(open[:nl]))})
			}
		}
		return out
	}
	addText(rest)
	return out
}

// cleanCode: Leerzeilen am Rand und Prompt-Zeichen „$ “ weg.
func cleanCode(c string) string {
	lines := strings.Split(strings.TrimSpace(c), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimPrefix(strings.TrimRight(l, " \t\r"), "$ ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// answerCards: alle Codeblöcke einer Antwort, in Reihenfolge.
func answerCards(ans string) []card {
	var out []card
	for _, p := range splitAnswer(ans) {
		if p.code {
			out = append(out, card{p.text, p.lang})
		}
	}
	return out
}

// shell: Block ist Shell-Code (ohne Angabe gilt er als Shell).
func (c card) shell() bool {
	switch c.lang {
	case "", "sh", "bash", "shell", "zsh", "console", "terminal", "powershell", "ps1", "pwsh", "cmd", "bat":
		return true
	}
	return false
}

// runnable: einzeilige Shell-Befehle lassen sich einfügen und ausführen;
// mehrzeilige Skripte und Dateien (json, yaml …) nur kopieren.
func (c card) runnable() bool { return c.shell() && c.code != "" && !strings.Contains(c.code, "\n") }

// ---------------------------------------------------------------- Syntaxfarben

// highlightShell färbt eine Shell-Zeile: Programm, Optionen, Texte in
// Anführungszeichen, Variablen, Verkettungen und Kommentare. Es ist kein
// vollständiger Parser, nur eine Lesehilfe; der Text bleibt Zeichen für
// Zeichen gleich.
func highlightShell(line string, st Styles) string {
	cmdS := lipgloss.NewStyle().Foreground(st.T.OK).Bold(true)
	sudoS := lipgloss.NewStyle().Foreground(st.T.Warn).Bold(true)
	flagS := lipgloss.NewStyle().Foreground(st.T.Accent)
	strS := lipgloss.NewStyle().Foreground(st.T.Warn)
	varS := lipgloss.NewStyle().Foreground(st.T.AI)
	opS := lipgloss.NewStyle().Foreground(st.T.Accent).Bold(true)
	comS := lipgloss.NewStyle().Foreground(st.T.Muted).Italic(true)
	txtS := lipgloss.NewStyle().Foreground(st.T.Text)

	var b strings.Builder
	r := []rune(line)
	expectCmd := true
	for i := 0; i < len(r); {
		c := r[i]
		switch {
		case unicode.IsSpace(c):
			j := i
			for j < len(r) && unicode.IsSpace(r[j]) {
				j++
			}
			b.WriteString(string(r[i:j]))
			i = j
		case c == '#' && (i == 0 || unicode.IsSpace(r[i-1])):
			b.WriteString(comS.Render(string(r[i:])))
			i = len(r)
		case c == '\'' || c == '"':
			j := i + 1
			for j < len(r) && r[j] != c {
				if r[j] == '\\' && c == '"' {
					j++
				}
				j++
			}
			j = min(j+1, len(r))
			b.WriteString(strS.Render(string(r[i:j])))
			i = j
			expectCmd = false
		case c == '$':
			j := i + 1
			if j < len(r) && (r[j] == '{' || r[j] == '(') {
				end := map[rune]rune{'{': '}', '(': ')'}[r[j]]
				for j < len(r) && r[j] != end {
					j++
				}
				j = min(j+1, len(r))
			} else {
				for j < len(r) && (unicode.IsLetter(r[j]) || unicode.IsDigit(r[j]) || r[j] == '_' || r[j] == '?') {
					j++
				}
			}
			b.WriteString(varS.Render(string(r[i:j])))
			i = j
			expectCmd = false
		case strings.ContainsRune("|&;<>", c):
			j := i
			for j < len(r) && strings.ContainsRune("|&;<>", r[j]) {
				j++
			}
			op := string(r[i:j])
			b.WriteString(opS.Render(op))
			i = j
			if !strings.ContainsAny(op, "<>") {
				expectCmd = true // nach | && ; kommt ein neues Programm
			}
		default:
			j := i
			for j < len(r) && !unicode.IsSpace(r[j]) && !strings.ContainsRune("|&;<>'\"$", r[j]) {
				j++
			}
			if j == i { // einzelnes Sonderzeichen
				j = i + 1
			}
			w := string(r[i:j])
			switch {
			case expectCmd && (w == "sudo" || w == "doas" || w == "env" || w == "time" || w == "nohup"):
				b.WriteString(sudoS.Render(w)) // Programm kommt danach
			case expectCmd && strings.Contains(w, "=") && !strings.HasPrefix(w, "="):
				b.WriteString(varS.Render(w)) // VAR=wert vor dem Programm
			case expectCmd:
				b.WriteString(cmdS.Render(w))
				expectCmd = false
			case strings.HasPrefix(w, "-"):
				b.WriteString(flagS.Render(w))
			default:
				b.WriteString(txtS.Render(w))
			}
			i = j
		}
	}
	return b.String()
}

// ---------------------------------------------------------------- Ansicht

// viewAnswer zeichnet eine KI-Antwort als Erklärung plus Befehlskarten.
func (a *App) viewAnswer(b *strings.Builder, s *session, ans string, w int) {
	st := a.s
	q := &s.ask
	parts := splitAnswer(ans)
	cards := answerCards(ans)
	q.sel = clamp(q.sel, 0, max(0, len(cards)-1))
	kind := safety.Kind(a.remoteText(s))

	// Kopf der Erklärung: ein/ausklappen per Klick oder z
	hasText := false
	for _, p := range parts {
		if !p.code {
			hasText = true
			break
		}
	}
	if hasText && len(cards) > 0 {
		arrow, hint := "▾ ", i18n.T("einklappen")
		if q.collapsed {
			arrow, hint = "▸ ", i18n.T("ausklappen")
		}
		head := st.AI.Render(arrow+i18n.T("Erklärung")) + st.Muted.Render("  z "+hint)
		b.WriteString(a.z.click(head, func(tea.Mouse, bool) tea.Cmd {
			q.collapsed = !q.collapsed
			s.panelFocus = true
			return nil
		}) + "\n")
	}

	n, shownText := 0, false
	for _, p := range parts {
		if !p.code {
			if q.collapsed && len(cards) > 0 {
				if !shownText { // eingeklappt: nur der erste Satz
					b.WriteString(wrap(a.inlineCode(trunc(firstSentence(p.text), w*2)), w) + "\n\n")
				}
				shownText = true
				continue
			}
			shownText = true
			b.WriteString(wrap(a.inlineCode(p.text), w) + "\n\n")
			continue
		}
		i := n
		n++
		b.WriteString(a.commandCard(s, i, q.cardText(i, card{p.text, p.lang}), i == q.sel, q.armed == i, kind, w) + "\n")
	}
}

// inlineCode hebt `befehl` im Fließtext hervor (ohne die Backticks).
func (a *App) inlineCode(t string) string {
	parts := strings.Split(t, "`")
	var b strings.Builder
	for i, p := range parts {
		if i%2 == 1 && i < len(parts)-1 {
			b.WriteString(a.s.Code.Render(p))
		} else {
			if i%2 == 1 {
				p = "`" + p // ungerade Anzahl: Backtick stehen lassen
			}
			b.WriteString(a.s.Text.Render(p))
		}
	}
	return b.String()
}

// commandCard: ein Befehl in einem eigenen Kasten mit Knöpfen.
func (a *App) commandCard(s *session, i int, cd card, sel, armed bool, kind string, w int) string {
	st := a.s
	code := cd.code
	var danger []safety.Finding
	if cd.shell() {
		danger = safety.Check(code, kind)
	}
	fx := s.ask.fixes[i]
	open := fx != nil && !fx.adopted // fehlgeschlagen, Fix noch nicht übernommen
	border := st.T.Border
	if sel {
		border = st.T.AI
	}
	if open {
		border = st.T.Warn
	}
	if len(danger) > 0 {
		border = st.T.Danger
	}
	box := lipgloss.NewStyle().Border(frameBorder).BorderForeground(border).Padding(0, 1).Width(w)
	if len(danger) > 0 && sel {
		box = box.Border(lipgloss.ThickBorder())
	}
	inner := max(10, w-4)

	// Kopfzeile: Nummer, Art, Warnung
	num := st.Key.Render(fmt.Sprint(i + 1))
	label := st.Muted.Render(" " + i18n.T("Befehl"))
	n := strings.Count(code, "\n") + 1
	switch {
	case !cd.shell():
		label = st.Muted.Render(" " + strings.ToUpper(cd.lang) + " · " + plural(n, i18n.T("Zeile"), i18n.T("Zeilen")))
	case !cd.runnable():
		label = st.Muted.Render(" " + fmt.Sprintf(i18n.T("Skript · %d Zeilen"), n))
	}
	head := num + label
	switch {
	case fx != nil && fx.adopted && fx.like >= 0:
		head += "  " + st.OK.Render(fmt.Sprintf(i18n.T("✓ angepasst wie Karte %d"), fx.like+1))
	case fx != nil && fx.adopted:
		head += "  " + st.OK.Render("✓ "+i18n.T("korrigiert"))
	}
	if len(danger) > 0 {
		head += "  " + st.Danger.Render("▲ "+trunc(danger[0].Reason, max(8, inner-20)))
	}

	// Code mit Syntaxfarben, lange Zeilen umbrechen
	var lines []string
	for _, l := range strings.Split(code, "\n") {
		if cd.shell() {
			l = highlightShell(l, st)
		} else {
			l = st.Text.Render(l)
		}
		lines = append(lines, wrap(l, inner))
	}
	body := strings.Join(lines, "\n")
	if open {
		body += "\n" + a.cardFixView(fx, inner)
	}

	// Knöpfe (bei schmalem Panel nur die Tasten)
	type button struct {
		key, text string
		style     lipgloss.Style
		f         func() tea.Cmd
	}
	var bs []button
	if open && fx.fix.Command != "" {
		bs = append(bs, button{"u", i18n.T("Fix übernehmen"), st.Key, func() tea.Cmd { return a.adoptCardFix(s, i) }})
	}
	if cd.runnable() {
		bs = append(bs, button{"⏎", i18n.T("Einfügen"), st.Key, func() tea.Cmd { return a.cardInsert(s, i) }})
		run := button{"a", i18n.T("Ausführen"), st.Key, func() tea.Cmd { return a.cardRun(s, i) }}
		if armed {
			run.style, run.text = st.ChipDanger.Bold(true), i18n.T("sicher? nochmal a")
		}
		bs = append(bs, run)
	}
	bs = append(bs, button{"c", i18n.T("Kopieren"), st.Key, func() tea.Cmd { return a.cardCopy(s, i) }})
	render := func(withText bool) string {
		var out []string
		for _, bt := range bs {
			bt := bt
			txt := bt.style.Render(bt.key)
			if withText && (!armed || bt.key == "a") { // scharf: nur die Rückfrage zählt
				txt += lipgloss.NewStyle().Foreground(st.T.Muted).Render(" " + bt.text)
			}
			out = append(out, a.z.click(txt, func(tea.Mouse, bool) tea.Cmd {
				s.ask.sel = i
				return bt.f()
			}))
		}
		return strings.Join(out, " ")
	}
	bar := render(true)
	if lipgloss.Width(bar) > inner {
		bar = render(false)
	}
	if !sel {
		bar = "" // Knöpfe nur an der gewählten Karte, das hält die Liste ruhig
	}

	content := head + "\n" + body
	if bar != "" {
		content += "\n" + bar
	}
	card := box.Render(content)
	return a.z.mark(card, zoneAction{block: true, click: func(_ tea.Mouse, double bool) tea.Cmd {
		s.ask.sel, s.panelFocus = i, true
		if double && cd.runnable() {
			return a.cardInsert(s, i)
		}
		return nil
	}})
}

// ---------------------------------------------------------------- Aktionen

func (a *App) cardCode(s *session, i int) (card, bool) {
	cards := answerCards(agent.VisibleAnswer(s.ask.answer.String()))
	if i < 0 || i >= len(cards) {
		return card{}, false
	}
	return s.ask.cardText(i, cards[i]), true
}

// cardInsert schreibt den Befehl in die Eingabezeile; Enter drückt der Mensch.
func (a *App) cardInsert(s *session, i int) tea.Cmd {
	cd, ok := a.cardCode(s, i)
	if !ok || !cd.runnable() || s.term == nil {
		return nil
	}
	c := cd.code
	s.ask.armed = -1
	s.term.SendText(c)
	s.noteAI(c, true)
	if from := s.ask.learnSource(i); from != "" && s.aiPend != nil {
		s.aiPend.learnFrom = from // läuft er nach Enter, lernt das Profil daraus
	}
	s.panelFocus = false
	return nil
}

// cardRun führt den Befehl aus. Gefährliche Befehle erst beim zweiten Druck.
func (a *App) cardRun(s *session, i int) tea.Cmd {
	cd, ok := a.cardCode(s, i)
	if !ok || !cd.runnable() || s.term == nil || s.state != stConnected {
		return nil
	}
	c := cd.code
	if f := safety.Check(c, safety.Kind(a.remoteText(s))); len(f) > 0 && s.ask.armed != i {
		s.ask.armed = i
		a.flash(i18n.T("Vorsicht: ") + f[0].Reason + i18n.T(". Nochmal a zum Ausführen"))
		return nil
	}
	s.ask.armed = -1
	a.clearToasts()
	if why := a.aiIncomplete(s, c); why != "" {
		s.term.SendText(c) // nur einfügen: mit Enter bliebe die Shell am „>“ hängen
		s.panelFocus = false
		a.flash(i18n.F("Befehl unvollständig (%s): nur eingefügt, bitte ergänzen", why))
		return nil
	}
	a.beginCommand(s, c)
	if from := s.ask.learnSource(i); from != "" && s.run != nil {
		s.run.learnFrom = from
	}
	a.sendLine(s, c)
	s.noteAI(c, false)
	s.history = append(s.history, c)
	a.profileCommand(s, c)
	s.panelFocus = false
	return nil
}

func (a *App) cardCopy(s *session, i int) tea.Cmd {
	cd, ok := a.cardCode(s, i)
	if !ok {
		return nil
	}
	c := cd.code
	go func() { _ = clipboard.WriteAll(c) }()
	if !strings.Contains(c, "\n") {
		a.flash(i18n.T("Kopiert: ") + trunc(c, 50))
	} else {
		a.flash(i18n.T("Kopiert: ") + plural(strings.Count(c, "\n")+1, i18n.T("Zeile"), i18n.T("Zeilen")))
	}
	return tea.SetClipboard(c)
}

// askCardKey: Tasten im Fragen-Panel, wenn die Antwort den Fokus hat.
// Liefert ok=false, wenn die Taste nichts mit den Karten zu tun hat.
func (a *App) askCardKey(s *session, key string) (tea.Cmd, bool) {
	q := &s.ask
	cards := answerCards(agent.VisibleAnswer(q.answer.String()))
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		if i := int(key[0] - '1'); i < len(cards) {
			q.sel = i
			return a.cardInsert(s, i), true
		}
		return nil, true
	}
	switch key {
	case "z":
		q.collapsed = !q.collapsed
		return nil, true
	}
	if len(cards) == 0 {
		return nil, false
	}
	switch key {
	case "up", "k", "shift+tab":
		q.sel, q.armed = max(0, q.sel-1), -1
	case "down", "j", "tab":
		q.sel, q.armed = min(len(cards)-1, q.sel+1), -1
	case "enter":
		if !cards[clamp(q.sel, 0, len(cards)-1)].runnable() {
			return a.cardCopy(s, q.sel), true
		}
		return a.cardInsert(s, q.sel), true
	case "a":
		return a.cardRun(s, q.sel), true
	case "c":
		return a.cardCopy(s, q.sel), true
	case "u":
		return a.adoptCardFix(s, q.sel), true
	default:
		return nil, false
	}
	return nil, true
}
