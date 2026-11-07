package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
)

// viewModeChoice: unter dem Zielfeld Schritt für Schritt oder Agent (3 Stufen).
func (a *App) viewModeChoice(b *strings.Builder, s *session, w int) {
	st := a.s
	g := &s.goal
	b.WriteString(st.Muted.Render(i18n.T("MODUS")+"  ") + a.hintsIn(s, st.Muted, [2]string{"Tab", i18n.T("wechseln")}) + "\n")
	type opt struct {
		auto  bool
		level int
		name  string
		hint  string
		plan  bool
	}
	opts := []opt{{false, 0, i18n.T("Schritt für Schritt"), i18n.T("du bestätigst jeden Befehl"), false}}
	for _, l := range []int{lvlCareful, lvlNormal, lvlFull} {
		opts = append(opts, opt{true, l, i18n.T("Agent · ") + levelName(l), levelHint(l), false})
	}
	opts = append(opts, opt{true, lvlNormal, i18n.T("Erst planen"), i18n.T("erkundet nur lesend, du gibst den Plan frei"), true})
	hint := ""
	for _, o := range opts {
		sel := o.plan == g.planFirst && (o.plan || (o.auto == g.auto && (!o.auto || o.level == g.level)))
		mark, style := st.Muted.Render("○ "), st.Text
		if sel {
			mark, style, hint = st.AI.Render("● "), st.AI.Bold(true), o.hint
		}
		line := mark + style.Render(trunc(o.name, max(4, w-2)))
		o := o
		b.WriteString(a.z.click(line, func(tea.Mouse, bool) tea.Cmd {
			s.panelFocus = true
			g.auto, g.level, g.planFirst = o.auto, o.level, o.plan
			return g.input.Focus()
		}) + "\n")
	}
	b.WriteString(lipglossIndent(wrap(st.Muted.Render(hint), max(4, w-2)), 2) + "\n\n")
	if g.planFirst {
		b.WriteString(wrap(st.Muted.Render(i18n.T("Die KI sieht sich mit lesenden Befehlen um und schreibt einen vollständigen Plan: ")+
			i18n.T("Schritte, Skripte, Risiken und Prüfung. Du streichst oder änderst Schritte, lässt neu planen und gibst ihn frei.")), w))
		return
	}
	if !g.auto {
		b.WriteString(wrap(st.Muted.Render(i18n.T("Die KI zerlegt das Ziel in einzelne Befehle, erklärt jeden kurz und wartet auf ")+
			i18n.T("deine Bestätigung. Sie tippt nie selbst.")), w))
		return
	}
	b.WriteString(st.Muted.Render(i18n.T("GRENZEN")) + "\n")
	b.WriteString(st.Muted.Render(fmt.Sprintf(i18n.T("  %d Schritte · %d min · %d Fehler in Folge"), agentSteps, int(agentTime.Minutes()), agentFailsInRow)) + "\n")
	if a.agentFilesOK(s) {
		b.WriteString(st.Muted.Render(i18n.T("  Skripte in ")) + st.Text.Render("~/"+agentDirBase+"/…") + "\n")
	} else {
		b.WriteString(st.Muted.Render(i18n.T("  hier nur Befehle, keine Skripte")) + "\n")
	}
	if isProduction(s.conn) {
		b.WriteString(st.Warn.Render(i18n.T("▲ Produktiv-Host: jede Änderung fragt nach")) + "\n")
	} else if s.netDevice() {
		b.WriteString(st.Warn.Render(i18n.T("▲ Netzwerkgerät: Konfiguration fragt immer nach")) + "\n")
	}
	b.WriteString("\n" + wrap(st.Muted.Render(i18n.T("Erst zeigt der Agent seinen Plan, ⏎ startet ihn. Gefahrenbefehle fragen immer. ")+
		i18n.T("Esc oder Tippen im Terminal hält ihn sofort an.")), w))
}

// viewAgentStatus: Kopfzeile eines laufenden Agenten.
func (a *App) viewAgentStatus(b *strings.Builder, s *session, w int) {
	st := a.s
	g := &s.goal
	state := st.OK.Bold(true).Render(i18n.T("läuft"))
	switch {
	case g.done:
		state = st.OK.Bold(true).Render(i18n.T("✓ fertig"))
	case g.cur != nil && g.cur.Question != "":
		state = st.Warn.Bold(true).Render(i18n.T("wartet auf deine Antwort"))
	case g.paused && !g.planOK:
		state = st.Warn.Bold(true).Render(i18n.T("bereit"))
	case g.paused:
		state = st.Warn.Bold(true).Render(i18n.T("angehalten"))
	}
	dur := time.Since(g.started).Round(time.Second)
	meta := fmt.Sprintf(i18n.T("Schritt %d/%d · %s · %s"), len(g.steps), g.maxSteps, agentDur(dur), levelName(g.level))
	b.WriteString(st.AI.Bold(true).Render("◆ Agent ") + state + "\n" + st.Muted.Render(trunc(meta, w)) + "\n")
	if g.paused && g.pauseWhy != "" && !g.done {
		b.WriteString(wrap(st.Warn.Render("▲ "+g.pauseWhy), w) + "\n")
	}
	if g.paused && g.cur == nil && !g.done {
		b.WriteString(a.hintsIn(s, st.Muted, [2]string{"⏎", i18n.T("weiter")}, [2]string{"?", i18n.T("anweisen")}, [2]string{"x", i18n.T("beenden")}) + "\n")
	}
	b.WriteString("\n")
}

func agentDur(d time.Duration) string {
	m := int(d.Minutes())
	return fmt.Sprintf("%d:%02d min", m, int(d.Seconds())-m*60)
}

// viewAgentFile: vorgeschlagene Datei mit Vorschau.
func (a *App) viewAgentFile(b *strings.Builder, s *session, w int) {
	st := a.s
	g := &s.goal
	f := g.cur.File
	head := st.AI.Render(i18n.T("▪ Datei anlegen"))
	box := st.Panel
	if len(g.cur.Danger) > 0 {
		head = st.Danger.Render(i18n.T("▲ Vorsicht im Skript: ") + g.cur.Danger[0].Reason)
		box = st.DangerBox
	}
	b.WriteString(head + "  " + st.Muted.Render(g.agentDir()+"/") + st.Text.Bold(true).Render(f.Name) + "\n")
	lines := strings.Split(strings.TrimRight(f.Content, "\n"), "\n")
	var pv []string
	for i, l := range lines {
		if i == agentPreviewRows {
			pv = append(pv, st.Muted.Render(fmt.Sprintf(i18n.T("   … %d weitere Zeilen (v)"), len(lines)-agentPreviewRows)))
			break
		}
		pv = append(pv, st.Muted.Render(fmt.Sprintf("%2d ", i+1))+st.Text.Render(trunc(l, max(4, w-8))))
	}
	b.WriteString(box.Width(w).Render(strings.Join(pv, "\n")) + "\n")
	if g.cur.Reason != "" {
		b.WriteString(wrap(st.Muted.Render(g.cur.Reason), w) + "\n")
	}
	if g.cur.Elevated {
		b.WriteString(st.Warn.Render(i18n.T("↑ Skript nutzt sudo")) + "\n")
	}
	if g.armed {
		b.WriteString("\n" + st.Danger.Render(i18n.T("Nochmal ⏎ zum Anlegen. s lässt es weg.")) + "\n")
	}
	if g.paused {
		a.viewAgentPauseKeys(b, s, w)
	}
}

func (a *App) viewAgentPauseKeys(b *strings.Builder, s *session, w int) {
	st := a.s
	g := &s.goal
	run, enter := i18n.T("ausführen"), "⏎"
	switch {
	case !g.planOK:
		run = i18n.T("Agent starten")
	case g.cur == nil:
		run = i18n.T("weiter")
	case len(g.cur.Danger) > 0 && !g.armed:
		enter = "⏎ ⏎"
	}
	b.WriteString("\n" + a.hintsIn(s, st.Muted, [2]string{enter, run}, [2]string{"s", i18n.T("weglassen")}, [2]string{"?", i18n.T("anweisen")}, [2]string{"x", i18n.T("beenden")}) + "\n")
	if g.cur != nil && len(g.cur.Danger) > 0 {
		b.WriteString(wrap(st.Muted.Render(i18n.T("Gefahrenbefehle laufen nie automatisch, auch nicht in Stufe Voll.")), w) + "\n")
	}
}

// viewAgentDone: Zusammenfassung, angelegte Dateien, Runbook/Aufräumen.
func (a *App) viewAgentDone(b *strings.Builder, s *session, w int) {
	st := a.s
	g := &s.goal
	b.WriteString(st.OK.Render(i18n.T("✓ Ziel erreicht")) + "\n" + wrap(st.Text.Render(g.doneReason), w) + "\n\n")
	if len(g.files) > 0 {
		b.WriteString(st.Muted.Render(i18n.T("DATEIEN")) + "\n")
		for _, f := range g.files {
			mark, name := st.AI.Render("  ▪ "), st.Text.Render(trunc(g.agentDir()+"/"+f.Name, max(4, w-4)))
			if g.cleaned {
				mark, name = st.Muted.Render("  − "), st.Muted.Strikethrough(true).Render(trunc(g.agentDir()+"/"+f.Name, max(4, w-4)))
			}
			b.WriteString(mark + name + "\n")
		}
		b.WriteString("\n")
	}
	keys := [][2]string{{"s", i18n.T("als Runbook speichern")}}
	if len(g.files) > 0 && !g.cleaned {
		keys = append(keys, [2]string{"r", i18n.T("Skripte auf dem Host löschen")}, [2]string{"v", i18n.T("Skript ansehen")})
	}
	keys = append(keys, [2]string{"⏎", i18n.T("neues Ziel")})
	b.WriteString(a.hintsIn(s, st.Muted, keys...))
}

// agentPanelKeys: Fußzeile im Agent-Modus (nil = Ziel-Modus-Tasten).
func (a *App) agentPanelKeys(s *session) [][2]string {
	g := &s.goal
	switch {
	case !g.auto:
		return nil
	case !g.active && g.done:
		k := [][2]string{{"s", "Runbook"}}
		if len(g.files) > 0 && !g.cleaned {
			k = append(k, [2]string{"r", i18n.T("aufräumen")}, [2]string{"v", i18n.T("Skript ansehen")})
		}
		return append(k, [2]string{"⏎", i18n.T("neues Ziel")}, [2]string{"Esc", "Terminal"})
	case !g.active:
		return [][2]string{{"⏎", i18n.T("Plan zeigen")}, {"Tab", i18n.T("Modus")}, {"Esc", "Terminal"}}
	case g.editing, g.cur != nil && g.cur.Question != "":
		return nil
	case !g.paused:
		return [][2]string{{"Esc", i18n.T("anhalten")}, {"?", i18n.T("anweisen")}, {"v", i18n.T("Skript ansehen")}, {"x", i18n.T("beenden")}}
	}
	k := [][2]string{{"⏎", i18n.T("weiter")}}
	if g.cur != nil {
		k = append(k, [2]string{"s", i18n.T("weglassen")})
		if g.cur.File == nil {
			k = append(k, [2]string{"e", i18n.T("ändern")})
		} else {
			k = append(k, [2]string{"v", i18n.T("Skript ansehen")})
		}
	}
	return append(k, [2]string{"?", i18n.T("anweisen")}, [2]string{"x", i18n.T("beenden")}, [2]string{"Esc", "Terminal"})
}

// savePlanRunbook: den Plan (vor der Ausführung) als Runbook speichern.
func (a *App) savePlanRunbook(s *session) {
	g := &s.goal
	rb := g.planRunbook()
	for _, x := range a.st.Runbooks("") {
		if x.Goal == rb.Goal {
			rb.ID, rb.Name, rb.CreatedAt, rb.Uses, rb.LastUsed, rb.ConnectionID = x.ID, x.Name, x.CreatedAt, x.Uses, x.LastUsed, x.ConnectionID
			break
		}
	}
	if s.client != nil {
		rb.OS = s.client.Remote.OS
	}
	saved, err := a.st.UpsertRunbook(rb)
	if err != nil {
		a.flashErr(i18n.T("Runbook: ") + err.Error())
		return
	}
	a.flash(i18n.T("Runbook gespeichert: ") + trunc(saved.Name, 40) + " (" + plural(len(saved.Steps), i18n.T("Schritt"), i18n.T("Schritte")) + ")")
}
