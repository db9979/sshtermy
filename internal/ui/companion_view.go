package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
)

// ---------------------------------------------------------------- Begleiter-Zeile (A)

// companionRows: Zeilen, die die Begleiter-Zeile unter dem Terminal belegt.
// Fest, solange der Begleiter an ist, damit das Terminal nicht bei jeder
// Bemerkung die Größe ändert. Ab 30 Zeilen zwei (Bemerkung umbricht), in
// kleinen Fenstern (uConsole) eine.
func (a *App) companionRows() int {
	if a.compLevel() == domain.CompanionOff {
		return 0
	}
	if a.h >= 30 {
		return 2
	}
	return 1
}

func (a *App) companionLine(s *session) string {
	rows := a.companionRows()
	if rows == 0 {
		return ""
	}
	st := a.s
	c := &s.comp
	head := st.AI.Bold(true).Render(" ◆ " + i18n.T("Begleiter") + " ")
	var body string
	switch {
	case c.remark != nil:
		e := c.remark
		switch {
		case e.kind == agent.EventDanger:
			body = st.Warn.Render(i18n.T("vor ⏎: ")) + st.Text.Render(e.text)
		case e.kind == compKindNote:
			body = st.Muted.Render(e.text)
		default:
			body = st.Text.Render(e.text)
		}
		if len(e.cmds) > 0 {
			body += st.Muted.Render("  · " + a.keys.label(actChip1) + "…")
		}
	case c.busy:
		body = a.shimmer(i18n.T("liest mit …"), st.T.Muted, st.T.AI, time.Now())
	case c.pausedWhy != "":
		body = st.Muted.Render(c.pausedWhy)
	default:
		u := &a.compUse
		body = st.Muted.Render(fmt.Sprintf(i18n.T("%s · hört zu · %s heute"), companionLevelLabel(a.compLevel()),
			plural(u.requests, i18n.T("Anfrage"), i18n.T("Anfragen"))))
	}
	tail := st.Muted.Render("  " + a.keys.label(actCompanionBar) + i18n.T(" Leiste"))
	hw := lipgloss.Width(head)
	room := max(10, a.w-hw-lipgloss.Width(tail))
	lines := strings.Split(lipgloss.NewStyle().Width(room).Render(body), "\n")
	if len(lines) > rows {
		lines = lines[:rows]
		last := lines[rows-1]
		lines[rows-1] = ansi.Truncate(last, max(1, room-1), "") + "…"
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	out := make([]string, rows)
	for i, l := range lines {
		l = strings.TrimRight(l, " ")
		pre := head
		if i > 0 {
			pre = strings.Repeat(" ", hw)
		}
		l = pre + l
		end := ""
		if i == 0 {
			end = tail
		}
		l += strings.Repeat(" ", max(0, a.w-lipgloss.Width(l)-lipgloss.Width(end))) + end
		out[i] = ansi.Truncate(l, a.w, "")
	}
	return a.z.click(strings.Join(out, "\n"), func(tea.Mouse, bool) tea.Cmd { return a.toggleCompanionBar(s) })
}

// toggleCompanionBar: Leiste ein/aus (Taste, Klick).
func (a *App) toggleCompanionBar(s *session) tea.Cmd {
	if s == nil {
		return nil
	}
	if a.compLevel() == domain.CompanionOff && s.panel != panelCompanion {
		a.flash(fmt.Sprintf(i18n.T("Begleiter ist aus · %s schaltet ihn ein"), a.keys.label(actCompanion)))
	}
	if s.panel == panelCompanion && s.panelFocus {
		s.panel, s.panelFocus = panelNone, false
	} else {
		s.panel, s.panelFocus = panelCompanion, true
		s.comp.sel, s.comp.armed = max(0, len(s.comp.choices())-1), -1
	}
	a.resize(s)
	return a.panelOpened(s)
}

// ---------------------------------------------------------------- Leiste (B)

type compChoice struct{ entry, cmd int }

// choices: alle Vorschläge im Verlauf (älteste zuerst).
func (c *companionState) choices() []compChoice {
	var out []compChoice
	for i, e := range c.feed {
		for j := range e.cmds {
			out = append(out, compChoice{i, j})
		}
	}
	return out
}

func (c *companionState) chosen() (*compEntry, *agent.Suggestion) {
	ch := c.choices()
	if len(ch) == 0 {
		return nil, nil
	}
	x := ch[clamp(c.sel, 0, len(ch)-1)]
	e := &c.feed[x.entry]
	return e, &e.cmds[x.cmd]
}

// lastEntry: Eintrag, auf den sich m/l beziehen (gewählter Vorschlag oder neuester).
func (c *companionState) lastEntry() *compEntry {
	if e, _ := c.chosen(); e != nil {
		return e
	}
	for i := len(c.feed) - 1; i >= 0; i-- {
		if c.feed[i].kind != compKindNote {
			return &c.feed[i]
		}
	}
	return nil
}

func (a *App) companionPanelKeys(s *session) [][2]string {
	c := &s.comp
	var k [][2]string
	if c.intent != nil {
		k = append(k, [2]string{"p", i18n.T("Plan")}, [2]string{"g", i18n.T("Agent übernimmt")}, [2]string{"n", i18n.T("nein")})
	}
	if _, sg := c.chosen(); sg != nil {
		k = append(k, [2]string{"⏎", i18n.T("einfügen")}, [2]string{"a", i18n.T("ausführen")}, [2]string{"c", i18n.T("kopieren")}, [2]string{"↑↓", i18n.T("wählen")})
	}
	if e := c.lastEntry(); e != nil && e.fact != "" && !e.factSaved {
		k = append(k, [2]string{"l", i18n.T("merken")})
	}
	if e := c.lastEntry(); e != nil && e.topic != "" {
		k = append(k, [2]string{"m", i18n.T("Thema stumm")})
	}
	return append(k, [2]string{"s", i18n.T("Stufe")}, [2]string{"?", i18n.T("gesendet")}, [2]string{"Esc", "Terminal"}, [2]string{a.keys.label(actCompanionBar), i18n.T("schließen")})
}

func (a *App) companionKey(s *session, key string) tea.Cmd {
	c := &s.comp
	n := len(c.choices())
	switch key {
	case "up", "k", "shift+tab":
		c.sel, c.armed = max(0, c.sel-1), -1
	case "down", "j", "tab":
		c.sel, c.armed = min(max(0, n-1), c.sel+1), -1
	case "enter", "i":
		if _, sg := c.chosen(); sg != nil && s.term != nil {
			if s.chipReplace || (c.remark != nil && c.remark.kind == agent.EventDanger) {
				s.term.SendText("\x15")
			}
			s.term.SendText(sg.Command)
			s.noteAI(sg.Command, true)
			s.panelFocus = false
		}
	case "a":
		return a.companionRun(s)
	case "c":
		if _, sg := c.chosen(); sg != nil {
			cmd := sg.Command
			go func() { _ = clipboard.WriteAll(cmd) }()
			a.flash(i18n.T("Kopiert: ") + trunc(cmd, 50))
			return tea.SetClipboard(cmd)
		}
	case "m":
		if e := c.lastEntry(); e != nil && e.topic != "" {
			c.mute(e.topic)
			if c.remark == e {
				c.remark = nil
			}
			a.flash(i18n.T("Begleiter: zu diesem Thema still (bis zum Schließen des Tabs)"))
		}
	case "l":
		if e := c.lastEntry(); e != nil && e.fact != "" && !e.factSaved {
			a.companionRemember(s, e.fact)
			e.factSaved = true
		}
	case "s":
		return a.cycleCompanion(1)
	case "?":
		a.modal = a.lastRequestModal()
	case "p", "g":
		if c.intent != nil {
			return a.companionTakeIntent(s, key == "p")
		}
	case "n", "x":
		if c.intent != nil {
			a.companionDropIntent(s, key == "x")
		}
	case "r":
		return a.openRecap(s, false, nil)
	}
	return nil
}

// companionRun führt den gewählten Vorschlag aus; gefährliche erst beim zweiten a.
func (a *App) companionRun(s *session) tea.Cmd {
	c := &s.comp
	_, sg := c.chosen()
	if sg == nil || s.term == nil || s.state != stConnected {
		return nil
	}
	cmd := sg.Command
	if f := safety.Check(cmd, a.safetyKind(s)); len(f) > 0 && c.armed != c.sel {
		c.armed = c.sel
		a.flash(i18n.T("Vorsicht: ") + f[0].Reason + i18n.T(". Nochmal a zum Ausführen"))
		return nil
	}
	c.armed = -1
	if typed, ok := s.term.TypedInput(); ok && typed != "" {
		s.term.SendText("\x15") // getippte Zeile ersetzen
	}
	a.clearToasts()
	if why := a.aiIncomplete(s, cmd); why != "" {
		s.term.SendText(cmd) // nur einfügen: mit Enter bliebe die Shell am „>“ hängen
		a.flash(i18n.F("Befehl unvollständig (%s): nur eingefügt, bitte ergänzen", why))
		return nil
	}
	a.beginCommand(s, cmd)
	a.sendLine(s, cmd)
	s.noteAI(cmd, false)
	s.history = append(s.history, cmd)
	a.profileCommand(s, cmd)
	s.chips, s.chipReplace = nil, false
	s.panelFocus = false
	return nil
}

func (a *App) viewCompanion(b *strings.Builder, s *session, w int) {
	st := a.s
	c := &s.comp
	level := a.compLevel()
	b.WriteString(st.PanelTitle.Render("◆ "+i18n.T("Begleiter")) + st.Muted.Render(" · "+s.conn.Alias) + "  " +
		st.Badge.Render(" "+companionLevelLabel(level)+" ") + "\n")
	if c.pausedWhy != "" && level != domain.CompanionOff {
		b.WriteString(st.Muted.Render(c.pausedWhy) + "\n")
	}
	b.WriteString("\n")
	if level == domain.CompanionOff {
		b.WriteString(wrap(st.Muted.Render(fmt.Sprintf(i18n.T("Der Begleiter ist aus. %s oder s schaltet durch: Leise (nur bei Fehler, Gefahr, Wiederholung), Gesprächig (jeder Befehl), Coach (erkennt, was du vorhast, Rückblick beim Schließen)."),
			a.keys.label(actCompanion))), w) + "\n")
	}
	if len(c.feed) == 0 && level != domain.CompanionOff {
		b.WriteString(wrap(st.Muted.Render(i18n.T("Noch nichts zu sagen. Der Begleiter meldet sich, sobald etwas schiefgeht oder auffällt.")), w) + "\n")
	}
	selE, selS := c.chosen()
	for i := range c.feed {
		e := &c.feed[i]
		head := st.Muted.Render(e.at.Format("15:04") + "  ")
		switch e.kind {
		case compKindNote:
			b.WriteString(head + wrap(st.Muted.Render(e.text), w-7) + "\n")
			continue
		case compKindIntent:
			head += st.AI.Bold(true).Render(i18n.T("Absicht erkannt"))
		case agent.EventDanger:
			head += st.Warn.Render(i18n.T("vor ⏎ ")) + st.Accent.Render(trunc(e.cmd, w-14))
		case agent.EventFail:
			head += st.Danger.Render("× ") + st.Accent.Render(trunc(e.cmd, w-10))
		default:
			head += st.Accent.Render(trunc(e.cmd, w-8))
		}
		b.WriteString(head + "\n")
		b.WriteString(indent(wrap(e.text, w-3), "   ") + "\n")
		for j, sg := range e.cmds {
			style := st.Code
			mark := "   "
			if selE == e && selS == &e.cmds[j] && s.panelFocus {
				style, mark = st.Selected, " ▸ "
			}
			line := trunc(sg.Command, w-5)
			if len(sg.Danger) > 0 {
				line = st.Danger.Render("! ") + style.Render(line)
			} else {
				line = style.Render(line)
			}
			b.WriteString(mark + line + "\n")
		}
		if e.fact != "" {
			if e.factSaved {
				b.WriteString(indent(wrap(st.OK.Render("✓ ")+st.Muted.Render(i18n.T("gemerkt: ")+e.fact), w-3), "   ") + "\n")
			} else {
				b.WriteString(indent(wrap(st.Muted.Render("+ "+i18n.T("merken? ")+e.fact)+st.Key.Render(" l"), w-3), "   ") + "\n")
			}
		}
	}
	if c.intent != nil {
		in := c.intent
		b.WriteString("\n" + st.AI.Bold(true).Render("◆ "+in.Goal) + "\n")
		if in.Evidence != "" {
			b.WriteString(wrap(st.Muted.Render(i18n.T("erkannt aus: ")+in.Evidence), w) + "\n")
		}
		b.WriteString(a.hints(st.Muted, [2]string{"p", i18n.T("als Plan öffnen")}, [2]string{"g", i18n.T("Agent „Vorsichtig“ übernimmt")}) + "\n")
		b.WriteString(a.hints(st.Muted, [2]string{"n", i18n.T("selbst machen")}, [2]string{"x", i18n.T("nicht mehr fragen")}) + "\n")
	}
	if c.busy || c.intentBusy {
		b.WriteString("\n" + a.shimmer(i18n.T("liest mit …"), st.T.Muted, st.T.AI, time.Now()) + "\n")
	}
	cfg := a.compCfg()
	u := &a.compUse
	u.roll(time.Now())
	b.WriteString("\n" + st.Muted.Render(strings.Repeat("─", max(4, w))) + "\n")
	b.WriteString(st.Muted.Render(fmt.Sprintf(i18n.T("KI heute: %s · ~%s Tokens · %d %% Budget"),
		plural(u.requests, i18n.T("Anfrage"), i18n.T("Anfragen")), kTok(u.tokens), u.tokens*100/max(1, cfg.DayTokens()))) + "\n")
	priv := privacyLabel(a.aiLevel(s.conn))
	if cfg.OnlyLocal || cfg.Raw {
		priv = i18n.T("nur eigene KI")
	}
	if cfg.Raw {
		priv += i18n.T(" · Filter aus")
	}
	b.WriteString(st.Muted.Render(fmt.Sprintf(i18n.T("Stunde: %d/%d · %s"), len(u.hits), cfg.Hour(), priv)) + "\n")
	if level >= domain.CompanionCoach || len(c.runs) > 0 {
		b.WriteString(a.hints(st.Muted, [2]string{"r", i18n.T("Rückblick")}) + "\n")
	}
}
