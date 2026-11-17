package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
)

// „Wo finde ich …?“ (Vorschlag 7 vom 2026-11-17): findet die Palette nichts,
// fragt Tab die KI. Sie bekommt nur die Frage und die Liste der Einträge,
// keine Daten aus Sitzungen oder Verbindungen, und nennt einen Eintrag; die
// Palette zeigt ihn dann wie einen Suchtreffer.

type palFindMsg struct {
	q    string
	pick agent.MenuPick
	cand []palItem
	err  error
}

// palAskWhereItem: Eintrag „KI fragen: Wo finde ich …?“ (Tab).
func (a *App) palAskWhereItem(q string, cands []palItem) palItem {
	return palItem{group: pgMaybe, icon: ic("ai"), label: i18n.F("KI fragen: Wo finde ich „%s“?", trunc(oneLine(q), 40)),
		hint: "Tab", tabRun: true, keep: true, run: func() tea.Cmd { return a.palAskWhere(q, cands) }}
}

// palAskWhere schickt Frage und Einträge an die KI.
func (a *App) palAskWhere(q string, cands []palItem) tea.Cmd {
	var list []palItem
	var lines []string
	for _, it := range cands {
		if it.open != "" || it.hidden || it.run == nil {
			continue
		}
		e := it.label
		if w := palWhere(it); w != "" {
			e = w + " › " + e
		}
		list = append(list, it)
		lines = append(lines, e)
	}
	ag, err := a.agentClientFor()
	if err != nil {
		a.palNote(err.Error())
		return nil
	}
	a.palNote(i18n.T("KI sucht den passenden Eintrag …"))
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		p, err := ag.FindMenu(ctx, q, lines)
		return palFindMsg{q: q, pick: p, cand: list, err: err}
	}
}

func (a *App) palNote(s string) {
	if a.pal != nil {
		a.pal.note = s
	}
}

// palFindDone zeigt den Eintrag, den die KI gewählt hat, als Suchtreffer.
func (a *App) palFindDone(m palFindMsg) tea.Cmd {
	if m.err != nil {
		a.palNote(i18n.T("KI: ") + m.err.Error())
		if a.pal == nil {
			a.flashErr(i18n.T("KI: ") + m.err.Error())
		}
		return nil
	}
	if m.pick.Index < 0 || m.pick.Index >= len(m.cand) {
		why := m.pick.Why
		if why == "" {
			why = i18n.T("Dazu gibt es keinen Eintrag.")
		}
		a.palNote(i18n.T("KI: ") + why)
		if a.pal == nil {
			a.flash(i18n.T("KI: ") + why)
		}
		return nil
	}
	it := m.cand[m.pick.Index]
	var cmd tea.Cmd
	if a.pal == nil {
		cmd = a.openPalette()
	}
	p := a.pal
	a.palEnter("")
	p.in.SetValue(it.label)
	p.in.CursorEnd()
	p.sel = 0
	for i, x := range a.paletteItems() {
		if x.label == it.label {
			p.sel = i
			break
		}
	}
	why := m.pick.Why
	if why == "" {
		why = it.label
	}
	p.note = i18n.T("KI: ") + why
	return cmd
}
