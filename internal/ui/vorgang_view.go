package ui

// Leiste „Vorgang“ unten über die ganze Breite: Einrichten, Fragen,
// Fahrplan, Plan in Bahnen, Rückblick. Sie liegt unter allen Bereichen der
// geteilten Ansicht, weil sie zu keinem einzelnen Fenster gehört.

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/llm"
)

// vgRows: Höhe der Leiste. Je Modus fest, damit die Terminals nicht bei
// jeder neuen Zeile umbrechen müssen.
func (a *App) vgRows() int {
	g := a.vg
	if g == nil || !g.open {
		return 0
	}
	avail := a.h - topRows - bottomRows - a.companionRows()
	want := clamp(a.h/3, 7, 14)
	switch g.mode {
	case vgSetup:
		want = clamp(len(g.rows)+8, 9, 18)
	case vgLanes, vgRecap:
		want = clamp(a.h/2, 10, 24)
	}
	return max(0, min(want, avail-6))
}

// ---------------------------------------------------------------- Ansicht

func (a *App) viewVorgang() string {
	g := a.vg
	h := a.vgRows()
	if h == 0 {
		return ""
	}
	st := a.s
	w := a.w
	var lines []string
	add := func(l string) { lines = append(lines, l) }
	switch g.mode {
	case vgSetup:
		a.vgViewSetup(add, w)
	case vgAsk:
		a.vgViewAsk(add, w, h-1)
	case vgRun:
		a.vgViewRun(add, w)
	case vgLanes:
		a.vgViewLanes(add, w)
	case vgRecap:
		a.vgViewRecap(add, w)
	}
	body := lines
	if len(body) > h-1 {
		body = body[:h-1]
	}
	for len(body) < h-1 {
		body = append(body, "")
	}
	out := []string{a.vgHeader(w)}
	for _, l := range body {
		l = ansi.Truncate(" "+l, w, "…")
		out = append(out, l+strings.Repeat(" ", max(0, w-lipgloss.Width(l))))
	}
	return a.z.click(strings.Join(out, "\n"), func(tea.Mouse, bool) tea.Cmd {
		g.focus = true
		_ = st
		return a.vgFocusInput()
	})
}

// vgHeader: Kopf der Leiste mit Fenstern und Modus.
func (a *App) vgHeader(w int) string {
	g := a.vg
	st := a.s
	line := lipgloss.NewStyle().Foreground(st.T.AI)
	if g.focus {
		line = line.Bold(true)
	}
	name := orDefault(g.name, i18n.T("ohne Namen"))
	head := st.AI.Bold(true).Render(" ◆ "+i18n.T("Vorgang")+" ") + st.Text.Bold(true).Render("„"+trunc(name, 40)+"“ ")
	if !g.active {
		head = st.AI.Bold(true).Render(" ◆ " + i18n.T("Vorgang verbinden") + " ")
	}
	for _, s := range a.vgSessions() {
		head += a.winBadge(s, true) + " "
	}
	modes := map[vgMode]string{vgSetup: i18n.T("Fenster wählen"), vgAsk: i18n.T("KI fragen"), vgRun: i18n.T("Fahrplan"),
		vgLanes: i18n.T("Plan in Bahnen"), vgRecap: i18n.T("Rückblick")}
	right := st.Muted.Render(" " + modes[g.mode] + " ")
	if g.auto && (g.mode == vgRun || g.mode == vgLanes) {
		right = st.Warn.Render(" "+i18n.T("Agent: liest allein")+" ") + right
	}
	fill := w - lipgloss.Width(head) - lipgloss.Width(right)
	if fill < 2 {
		head = ansi.Truncate(head, max(0, w-lipgloss.Width(right)-2), "…")
		fill = max(0, w-lipgloss.Width(head)-lipgloss.Width(right))
	}
	return head + line.Render(strings.Repeat("━", max(0, fill))) + right
}

func (a *App) vgViewSetup(add func(string), w int) {
	g := a.vg
	st := a.s
	lbl := st.Muted.Render(i18n.T("Worum geht's? (optional, hilft der KI)  "))
	g.goalIn.SetWidth(max(20, w-lipgloss.Width(lbl)-4))
	add(lbl + g.goalIn.View())
	add("")
	add(st.Muted.Render(fmt.Sprintf("     %-24s %-34s %s", i18n.T("Fenster"), i18n.T("System"), i18n.T("KI sieht"))))
	for i, r := range g.rows {
		s := a.byID(r.sid)
		if s == nil {
			continue
		}
		box := st.Muted.Render("[ ]")
		if r.on {
			box = st.OK.Render("[x]")
		}
		cur := "  "
		if g.inList && i == g.sel {
			cur = st.Accent.Render("▶ ")
		}
		badge := a.winBadge(s, true)
		sys := trunc(orDefault(s.profileSummary(), s.conn.Host), 34)
		see := "‹ " + seeLabel(r.see) + " ›"
		if tagPrivacy(s.conn) == llm.PrivacyOff {
			see = i18n.T("– KI aus (Tag ki-aus) –")
		} else if !r.on {
			see = st.Muted.Render(i18n.T("– sieht nichts –"))
		}
		add(cur + box + " " + padRight(badge, 26) + " " + padRight(st.Text.Render(sys), 34) + " " + see)
	}
	add("")
	add(st.Muted.Render(i18n.T("Vorausgewählt: die Bereiche, die gerade zu sehen sind. Fenster außerhalb des Vorgangs sieht die KI nie.")))
	add(st.Muted.Render(i18n.T("Ausgabe geht durch den Datenschutzfilter (Passwörter, Schlüssel, Tokens raus). Es gilt die strengste KI-Regel aller Fenster.")))
}

func (a *App) vgViewAsk(add func(string), w, h int) {
	g := a.vg
	st := a.s
	a.vgHintLines(add)
	if g.ans == nil {
		switch {
		case g.busy:
			add(a.shimmer(i18n.T("KI liest alle Fenster des Vorgangs …"), st.T.Muted, st.T.AI, time.Now()) + "  " + st.Muted.Render(trunc(g.question, 60)))
		case g.err != "":
			add(st.Danger.Render("× " + trunc(g.err, w-6)))
			add("")
			fallthrough
		default:
			g.in.SetWidth(max(20, w-6))
			add(st.AI.Render(i18n.T("Frage an alle Fenster:")))
			for _, l := range strings.Split(g.in.View(), "\n") {
				add(st.AI.Render("› ") + l)
			}
			if len(g.steps) > 0 || len(g.events) > 0 {
				add(st.Muted.Render(i18n.T("Tab: zur letzten Antwort / Zeitleiste")))
			}
		}
		if !g.busy {
			a.vgTimeline(add, 4)
		}
		return
	}
	ans := g.ans
	var lines []string
	if ans.Summary != "" {
		for _, l := range wrapText(ans.Summary, w-4) {
			lines = append(lines, st.Text.Render(l))
		}
	}
	if len(ans.Findings) > 0 {
		lines = append(lines, st.Text.Bold(true).Render(i18n.T("Befund")))
		for _, f := range ans.Findings {
			lines = append(lines, a.winBadgeN(f.Win, false)+" "+st.Text.Render(trunc(f.Text, w-10)))
		}
	}
	first := len(lines)
	if len(g.steps) > 0 {
		lines = append(lines, st.Text.Bold(true).Render(i18n.T("Schritte – in dieser Reihenfolge")))
		first = len(lines)
		for i := range g.steps {
			lines = append(lines, a.vgStepLine(i, w, false))
		}
	}
	for _, m := range ans.Missing {
		lines = append(lines, st.Warn.Render("○ "+i18n.T("fehlt: "))+st.Text.Render(trunc(m.Text, w-30))+"  "+st.Key.Render("o")+st.Muted.Render(" "+i18n.T("öffnen")))
	}
	// Auswahl sichtbar halten
	off := 0
	if sel := first + g.stepSel; sel >= h-1 {
		off = sel - (h - 2)
	}
	for _, l := range lines[min(off, len(lines)):] {
		add(l)
	}
}

// vgStepLine: eine Zeile je Schritt (Antwort und Fahrplan).
func (a *App) vgStepLine(i, w int, run bool) string {
	g := a.vg
	st := a.s
	s := &g.steps[i]
	mark := st.Key.Render(itoa(i + 1))
	if i >= 9 {
		mark = st.Muted.Render(" " + itoa(i+1))
	}
	if run {
		switch s.state {
		case vsDone:
			mark = st.OK.Render(" ✓")
		case vsFailed:
			mark = st.Danger.Render(" ×")
		case vsSkipped:
			mark = st.Muted.Render(" –")
		case vsTyped, vsRunning, vsArmed:
			mark = st.Warn.Render(" ▶")
		default:
			mark = st.Muted.Render(" ·")
		}
	}
	sel := " "
	if (!run && g.stepSel == i) || (run && g.cur == i) {
		sel = st.Accent.Render("▸")
	}
	target := ""
	if s.Win > 0 {
		target = a.winBadgeN(s.Win, true)
	} else {
		target = st.Muted.Render(" ? " + trunc(s.System, 14) + " " + i18n.T("(nicht offen)"))
	}
	cmd := s.Command
	if g.vals != nil {
		cmd = agent.FillPlaceholders(cmd, g.vals)
	}
	kind := st.Warn.Render(i18n.T("ändert"))
	switch {
	case len(s.Danger) > 0:
		kind = st.Danger.Render(i18n.T("Gefahr ⏎⏎"))
	case s.ReadOnly:
		kind = st.OK.Render(i18n.T("liest"))
	}
	extra := ""
	if s.Save != "" {
		extra = st.Muted.Render(" → {{" + s.Save + "}}")
	}
	if s.off {
		kind = st.Muted.Render(i18n.T("abgewählt"))
	}
	note := ""
	switch {
	case s.note != "":
		note = "  " + st.Muted.Render(trunc(s.note, 40))
	case s.state == vsTyped:
		note = "  " + st.Warn.Render(i18n.F("wartet auf ⏎ in Fenster %d", s.Win))
	case s.state == vsArmed:
		note = "  " + st.Danger.Render(i18n.T("⏎ hier tippt vor"))
	}
	left := mark + sel + padRight(target, 24) + " "
	room := max(10, w-lipgloss.Width(left)-lipgloss.Width(kind)-lipgloss.Width(extra)-lipgloss.Width(note)-6)
	line := left + st.Accent.Render(trunc(cmd, room)) + "  " + kind + extra + note
	idx := i
	return a.z.click(line, func(_ tea.Mouse, double bool) tea.Cmd {
		g.focus = true
		if run {
			return nil
		}
		g.stepSel = idx
		if double {
			return a.vgInsertStep(idx)
		}
		return nil
	})
}

func (a *App) vgViewRun(add func(string), w int) {
	g := a.vg
	st := a.s
	a.vgHintLines(add)
	if g.why != "" {
		add(st.Warn.Render("■ " + g.why))
	} else if g.curStep() == nil {
		add(st.OK.Render("✓ " + i18n.T("Fahrplan fertig · r Rückblick · n neue Frage")))
	}
	for i := range g.steps {
		add(a.vgStepLine(i, w, true))
	}
	if len(g.vals) > 0 {
		var vs []string
		for k, v := range g.vals {
			vs = append(vs, "{{"+k+"}} = "+v)
		}
		add(st.Muted.Render(i18n.T("Gemerkte Werte: ") + strings.Join(sortedStrings(vs), " · ")))
	}
}

// vgViewLanes: Plan in Bahnen, eine Spalte je Fenster.
func (a *App) vgViewLanes(add func(string), w int) {
	g := a.vg
	st := a.s
	var cols []int
	seen := map[int]bool{}
	for _, s := range a.vgSessions() {
		n := a.winNo(s)
		cols, seen[n] = append(cols, n), true
	}
	missing := false
	for _, s := range g.steps {
		if s.Win == 0 {
			missing = true
		} else if !seen[s.Win] {
			cols, seen[s.Win] = append(cols, s.Win), true
		}
	}
	if missing {
		cols = append(cols, 0)
	}
	if len(cols) == 0 {
		add(st.Muted.Render(i18n.T("Noch kein Plan · p plant den Vorgang")))
		return
	}
	first := max(9, ansi.StringWidth(i18n.T("Schritt"))+2)
	cw := max(12, (w-first-2)/len(cols))
	cell := func(s string, width int) string {
		return padRight(ansi.Truncate(s, width-1, "…"), width)
	}
	head := padRight(st.Muted.Render(i18n.T("Schritt")), first)
	for _, c := range cols {
		if c == 0 {
			head += cell(st.Muted.Render(i18n.T("nicht offen")), cw)
		} else {
			head += cell(a.winBadgeN(c, true), cw)
		}
	}
	add(head)
	add(st.Muted.Render(strings.Repeat("─", min(w-2, first+cw*len(cols)))))
	saver := map[string]int{}
	for i, s := range g.steps {
		if s.Save != "" {
			saver[s.Save] = s.Win
		}
		box := st.OK.Render("[x]")
		if s.off {
			box = st.Muted.Render("[ ]")
		}
		sel := " "
		if g.stepSel == i {
			sel = st.Accent.Render("▸")
		}
		row := padRight(sel+box+" "+itoa(i+1), first)
		dot := st.Warn.Render("●")
		switch {
		case len(s.Danger) > 0:
			dot = st.Danger.Render("●")
		case s.ReadOnly:
			dot = st.OK.Render("●")
		}
		for _, c := range cols {
			if c != s.Win {
				row += strings.Repeat(" ", cw)
				continue
			}
			txt := s.Command
			if s.Win == 0 {
				txt = s.System + ": " + txt
			}
			pre := ""
			for _, p := range agent.Placeholders(s.Command) {
				if from, ok := saver[p]; ok && from != s.Win {
					pre = st.Muted.Render("→ ")
				}
			}
			label := dot + " " + pre + st.Text.Render(txt)
			if len(s.Danger) > 0 {
				label += st.Danger.Render(" ⏎⏎")
			}
			row += cell(label, cw)
		}
		add(row)
	}
	// Rückweg je Fenster
	undo := map[int][]string{}
	for _, s := range g.steps {
		if s.Undo != "" {
			undo[s.Win] = append([]string{s.Undo}, undo[s.Win]...)
		}
	}
	if len(undo) > 0 {
		add(st.Muted.Render(strings.Repeat("─", min(w-2, first+cw*len(cols)))))
		row := padRight(st.Muted.Render(i18n.T("Zurück")), first)
		for _, c := range cols {
			row += cell(st.Muted.Render(strings.Join(undo[c], " · ")), cw)
		}
		add(row)
	}
	add("")
	if g.auto {
		add(st.Warn.Render(i18n.T("Agent: Lesendes läuft in allen Fenstern allein (nach dem Schutz-Check). ")) +
			st.Muted.Render(i18n.T("Jede Änderung wird nur vorgetippt, ⏎ im Zielfenster. Gefahr: ⏎ hier, dann ⏎ dort.")))
	} else {
		add(st.Muted.Render(i18n.T("Schritt für Schritt: jeder Befehl wird im Zielfenster vorgetippt, ⏎ dort. Tab: Agent liest allein.")))
	}
	add(st.Muted.Render(i18n.T("● grün liest · gelb ändert · rot Gefahr · → braucht einen Wert aus einem anderen Fenster")))
}

func (a *App) vgViewRecap(add func(string), w int) {
	g := a.vg
	st := a.s
	end := time.Now()
	if n := len(g.events); n > 0 {
		end = g.events[n-1].At
	}
	add(st.Text.Bold(true).Render(fmt.Sprintf("%s · %s–%s · %s", orDefault(g.name, i18n.T("Vorgang")), g.started.Format("15:04"), end.Format("15:04"),
		plural(len(g.events), i18n.T("Befehl"), i18n.T("Befehle")))))
	switch {
	case g.recapBusy:
		add(a.shimmer(i18n.T("KI fasst den Vorgang zusammen …"), st.T.Muted, st.T.AI, time.Now()))
	case g.recapErr != "":
		add(st.Danger.Render("× " + trunc(g.recapErr, w-6)))
	}
	if r := g.recap; r != nil {
		for _, p := range r.Points {
			add(st.Text.Render("• " + trunc(p, w-6)))
		}
	}
	add("")
	a.vgTimeline(add, 8)
	if r := g.recap; r != nil {
		if len(r.Undo) > 0 {
			add("")
			add(st.Text.Bold(true).Render(i18n.T("Zurück, falls nötig")) + st.Muted.Render(i18n.T("  (1–9 tippt im Fenster vor)")))
			for i, u := range r.Undo {
				add(st.Key.Render(itoa(i+1)) + " " + padRight(a.winBadgeN(u.Win, true), 24) + " " + st.Accent.Render(trunc(u.Command, w-36)))
			}
		}
		if len(r.Open) > 0 {
			add("")
			add(st.Text.Bold(true).Render(i18n.T("Offen")))
			for _, o := range r.Open {
				add(st.Muted.Render("○ " + trunc(o, w-6)))
			}
		}
	}
}

// vgTimeline: letzte Befehle aller Fenster, neueste unten.
func (a *App) vgTimeline(add func(string), n int) {
	g := a.vg
	st := a.s
	if len(g.events) == 0 {
		return
	}
	add(st.Muted.Render(i18n.T("Zeitleiste aller Fenster")))
	evs := g.events
	if len(evs) > n {
		evs = evs[len(evs)-n:]
	}
	for _, e := range evs {
		mark := st.OK.Render("✓")
		if e.Failed {
			mark = st.Danger.Render("×")
		}
		l := st.Muted.Render(e.At.Format("15:04")) + " " + a.winBadgeN(e.Win, false) + " " + mark + " " + st.Text.Render(trunc(e.Command, 70))
		if e.Note != "" {
			l += st.Muted.Render("  " + trunc(e.Note, 40))
		}
		add(l)
	}
}

// vgHintLines: Begleiter-Hinweis über Fenster hinweg.
func (a *App) vgHintLines(add func(string)) {
	g := a.vg
	h := g.hint
	if h == nil {
		return
	}
	st := a.s
	add(st.Warn.Bold(true).Render(i18n.T("◆ Begleiter: Zusammenhang über Fenster")) + " " + a.winBadgeN(h.from, false) +
		st.Muted.Render(" → ") + a.winBadgeN(h.win, true) + "  " + st.Text.Render(trunc(h.text, 90)))
	if h.cmd != "" {
		c := st.Accent.Render(h.cmd)
		if h.danger {
			c = st.Danger.Render("▲ ") + c
		}
		add("   " + i18n.F("In Fenster %d eingeben: ", h.win) + c + "  " + st.Key.Render("v") + st.Muted.Render(" "+i18n.T("vortippen")) +
			"  " + st.Key.Render("x") + st.Muted.Render(" "+i18n.T("weg")))
	} else {
		add("   " + st.Key.Render("x") + st.Muted.Render(" "+i18n.T("weg")))
	}
}

func wrapText(s string, w int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			if line != "" && len([]rune(line))+1+len([]rune(word)) > w {
				out = append(out, line)
				line = ""
			}
			if line != "" {
				line += " "
			}
			line += word
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func sortedStrings(x []string) []string {
	out := append([]string(nil), x...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// ---------------------------------------------------------------- Tasten

// vgKeys: Tastenzeile, solange die Leiste die Tasten hat.
func (a *App) vgKeys() [][2]string {
	g := a.vg
	ex := [][2]string{{"Esc", i18n.T("zum Terminal")}, {"Strg+D", i18n.T("Vorgang beenden")}}
	switch g.mode {
	case vgSetup:
		if !g.inList {
			return [][2]string{{"⏎", i18n.T("Vorgang starten")}, {"Tab", i18n.T("zur Fensterliste")}, {"Esc", i18n.T("abbrechen")}}
		}
		return [][2]string{{"Leertaste", i18n.T("dabei")}, {"←/→", i18n.T("was die KI sieht")}, {"↑/↓", i18n.T("wählen")}, {"⏎", i18n.T("Vorgang starten")}, {"Tab", i18n.T("Ziel")}, {"Esc", i18n.T("abbrechen")}}
	case vgAsk:
		if g.busy {
			return append([][2]string{{"Esc", i18n.T("Antwort abbrechen")}}, ex[1])
		}
		if g.ans == nil {
			return append([][2]string{{"⏎", i18n.T("fragen")}, {"Tab", i18n.T("Befehle")}, {"Strg+E", i18n.T("Fenster ändern")}}, ex...)
		}
		k := [][2]string{{"1–9", i18n.T("im Fenster vortippen")}, {"z", i18n.T("Fahrplan")}, {"p", i18n.T("Plan in Bahnen")}, {"f", i18n.T("Folgefrage")}, {"n", i18n.T("neue Frage")}, {"r", i18n.T("Rückblick")}, {"c", i18n.T("kopieren")}, {"e", i18n.T("ändern")}}
		return append(k, ex...)
	case vgRun:
		return append([][2]string{{"⏎", i18n.T("weiter / vortippen")}, {"s", i18n.T("überspringen")}, {"w", i18n.T("nochmal")}, {"e", i18n.T("ändern")}, {"Tab", i18n.T("Agent liest allein")}, {"f", i18n.T("KI fragen")}, {"r", i18n.T("Rückblick")}}, ex...)
	case vgLanes:
		return append([][2]string{{"⏎", i18n.T("freigeben")}, {"Leertaste", i18n.T("Schritt an/aus")}, {"↑/↓", i18n.T("wählen")}, {"e", i18n.T("ändern")}, {"Tab", i18n.T("Agent liest allein")}, {"a", i18n.T("Antwort")}}, ex...)
	case vgRecap:
		return append([][2]string{{"1–9", i18n.T("Rückweg vortippen")}, {"m", i18n.T("Change-Protokoll")}, {"b", i18n.T("als Runbook")}, {"c", i18n.T("kopieren")}, {"a", i18n.T("zurück")}}, ex...)
	}
	return ex
}

// vgKey: Tasten, solange die Leiste die Tasten hat.
func (a *App) vgKey(k tea.KeyPressMsg) tea.Cmd {
	g := a.vg
	key := k.String()
	if key == "ctrl+d" && g.active {
		a.modal = confirmModal(i18n.T("Vorgang beenden?"), i18n.T("Die Fenster bleiben offen. Fahrplan, Antworten und Zeitleiste des Vorgangs sind danach weg."), func() tea.Cmd {
			a.vgEnd()
			return nil
		})
		return nil
	}
	if key == "ctrl+e" && g.active {
		return a.vgOpenSetup()
	}
	switch g.mode {
	case vgSetup:
		return a.vgSetupKey(k)
	case vgAsk:
		return a.vgAskKey(k)
	}
	if key == "esc" {
		g.focus = false
		return nil
	}
	if g.hint != nil {
		switch key {
		case "v":
			return a.vgHintInsert()
		case "x":
			g.hint = nil
			return nil
		}
	}
	switch g.mode {
	case vgRun:
		switch key {
		case "enter":
			return a.vgConfirm()
		case "s":
			return a.vgSkip()
		case "w":
			return a.vgRetry()
		case "e":
			return a.vgEditStep(g.cur)
		case "o":
			if st := g.curStep(); st != nil && st.Win == 0 {
				return a.vgOpenMissing(st.System)
			}
		case "tab":
			g.auto = !g.auto
			return a.vgAdvance()
		case "f":
			g.mode, g.ans = vgAsk, nil
			a.resizeAll()
			return g.in.Focus()
		case "r":
			return a.vgOpenRecap()
		case "n":
			g.mode, g.ans, g.steps = vgAsk, nil, nil
			g.in.SetValue("")
			a.resizeAll()
			return g.in.Focus()
		case "b":
			g.mode = vgLanes
			a.resizeAll()
		}
	case vgLanes:
		switch key {
		case "up", "k":
			g.stepSel = max(0, g.stepSel-1)
		case "down", "j":
			g.stepSel = min(len(g.steps)-1, g.stepSel+1)
		case "space", " ":
			if g.stepSel < len(g.steps) {
				g.steps[g.stepSel].off = !g.steps[g.stepSel].off
			}
		case "tab":
			g.auto = !g.auto
		case "e":
			return a.vgEditStep(g.stepSel)
		case "a":
			g.mode = vgAsk
			a.resizeAll()
		case "o":
			for _, st := range g.steps {
				if st.Win == 0 {
					return a.vgOpenMissing(st.System)
				}
			}
		case "enter":
			return a.vgStartRun(g.auto)
		}
	case vgRecap:
		switch key {
		case "m":
			a.vgSaveRecap()
		case "b":
			a.vgRecapRunbook()
		case "c":
			txt := a.vgRecapText()
			go func() { _ = clipboard.WriteAll(txt) }()
			a.flash(i18n.T("Rückblick kopiert (Markdown)"))
			return tea.SetClipboard(txt)
		case "a", "backspace":
			g.mode = vgAsk
			if len(g.steps) > 0 && g.curStep() != nil && g.cur > 0 {
				g.mode = vgRun
			}
			a.resizeAll()
		case "r":
			return a.vgOpenRecap()
		default:
			if r := g.recap; r != nil && len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
				i := int(key[0] - '1')
				if i < len(r.Undo) {
					u := r.Undo[i]
					if s := a.byWin(u.Win); s != nil {
						if len(u.Danger) > 0 && g.armedIns != 100+i {
							g.armedIns = 100 + i
							a.flash(i18n.F("▲ %s · nochmal %d zum Vortippen", u.Danger[0].Reason, i+1))
							return nil
						}
						g.armedIns = -1
						a.vgTypeInto(s, u.Command)
					}
				}
			}
		}
	}
	return nil
}

func (a *App) vgSetupKey(k tea.KeyPressMsg) tea.Cmd {
	g := a.vg
	key := k.String()
	switch key {
	case "esc":
		if g.active {
			g.mode = vgAsk
			g.goalIn.Blur()
			a.resizeAll()
			return g.in.Focus()
		}
		a.vg = nil
		a.resizeAll()
		return nil
	case "enter":
		return a.vgStart()
	case "tab", "shift+tab":
		g.inList = !g.inList
		if g.inList {
			g.goalIn.Blur()
			return nil
		}
		return g.goalIn.Focus()
	}
	if !g.inList {
		if key == "down" {
			g.inList = true
			g.goalIn.Blur()
			return nil
		}
		var cmd tea.Cmd
		g.goalIn, cmd = g.goalIn.Update(k)
		return cmd
	}
	if len(g.rows) == 0 {
		return nil
	}
	r := &g.rows[clamp(g.sel, 0, len(g.rows)-1)]
	switch key {
	case "up", "k":
		if g.sel == 0 {
			g.inList = false
			return g.goalIn.Focus()
		}
		g.sel--
	case "down", "j":
		g.sel = min(len(g.rows)-1, g.sel+1)
	case "space", " ", "x":
		r.on = !r.on
	case "left", "h":
		r.see = max(seeProfile, r.see-1)
	case "right", "l":
		r.see = min(seeOutput, r.see+1)
		if !a.st.LLM().SendScreen && r.see == seeOutput {
			r.see = seeCommands
			a.flash(i18n.T("Terminal-Inhalt an die KI ist in den KI-Einstellungen ausgeschaltet"))
		}
	}
	return nil
}

func (a *App) vgAskKey(k tea.KeyPressMsg) tea.Cmd {
	g := a.vg
	key := k.String()
	if g.busy {
		if key == "esc" || key == "ctrl+c" {
			g.cancel()
			g.busy = false
			g.seq++
			return g.in.Focus()
		}
		return nil
	}
	if g.in.Focused() {
		switch key {
		case "esc":
			g.in.Blur()
			g.focus = false
			return nil
		case "tab":
			g.in.Blur()
			if g.ans == nil && len(g.steps) > 0 {
				g.mode = vgRun
				a.resizeAll()
			}
			return nil
		case "enter":
			follow := g.ans != nil || len(g.history) > 0
			return a.vgAsk(g.in.Value(), false, follow && g.question != "" && g.followNext)
		}
		var cmd tea.Cmd
		g.in, cmd = g.in.Update(k)
		return cmd
	}
	if g.hint != nil {
		switch key {
		case "v":
			return a.vgHintInsert()
		case "x":
			g.hint = nil
			return nil
		}
	}
	switch key {
	case "esc":
		g.focus = false
	case "up", "k":
		g.stepSel = max(0, g.stepSel-1)
		g.armedIns = -1
	case "down", "j":
		g.stepSel = min(max(0, len(g.steps)-1), g.stepSel+1)
		g.armedIns = -1
	case "enter":
		return a.vgInsertStep(g.stepSel)
	case "z":
		return a.vgStartRun(false)
	case "p":
		q := orDefault(g.name, g.question)
		if q == "" {
			a.flash(i18n.T("Erst ein Ziel (Strg+E) oder eine Frage, dann kann die KI planen"))
			return nil
		}
		return a.vgAsk(q, true, false)
	case "b":
		if len(g.steps) > 0 {
			g.mode = vgLanes
			a.resizeAll()
		}
	case "f":
		g.followNext = true
		g.in.SetValue("")
		return g.in.Focus()
	case "n":
		g.followNext, g.history = false, nil
		g.ans, g.steps = nil, nil
		g.in.SetValue("")
		return g.in.Focus()
	case "r":
		return a.vgOpenRecap()
	case "e":
		return a.vgEditStep(g.stepSel)
	case "o":
		if g.ans != nil && len(g.ans.Missing) > 0 {
			return a.vgOpenMissing(g.ans.Missing[0].Text)
		}
		if g.stepSel < len(g.steps) && g.steps[g.stepSel].Win == 0 {
			return a.vgOpenMissing(g.steps[g.stepSel].System)
		}
	case "c":
		if g.ans == nil {
			return nil
		}
		txt := g.ans.Summary + "\n\n"
		for i, st := range g.steps {
			txt += fmt.Sprintf("%d. %s %d: %s\n", i+1, i18n.T("Fenster"), st.Win, st.Command)
		}
		go func() { _ = clipboard.WriteAll(txt) }()
		a.flash(i18n.T("Antwort kopiert"))
		return tea.SetClipboard(txt)
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			return a.vgInsertStep(int(key[0] - '1'))
		}
	}
	return nil
}

// vgPaste: Einfügen in die Felder der Leiste.
func (a *App) vgPaste(m tea.PasteMsg) tea.Cmd {
	g := a.vg
	var cmd tea.Cmd
	switch {
	case g.mode == vgSetup && !g.inList:
		g.goalIn, cmd = g.goalIn.Update(m)
	case g.mode == vgAsk && g.in.Focused():
		g.in, cmd = g.in.Update(m)
	}
	return cmd
}
