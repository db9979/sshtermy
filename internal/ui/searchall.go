package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Suche über alle offenen Tabs (Alt+S)
//
// Fragt einen Suchbegriff ab und zeigt die Treffer aus dem Verlauf aller
// Tabs in einer Liste (je Tab die neuesten zuerst). ⏎ wechselt in den Tab
// und öffnet dort die normale Suche (Strg+F) an genau dieser Stelle.

const (
	searchAllPerTab = 40  // höchstens so viele Trefferzeilen je Tab
	searchAllTotal  = 400 // und insgesamt
)

func (a *App) openSearchAll() tea.Cmd {
	if len(a.sessions) == 0 {
		a.flash(i18n.T("Kein Tab offen"))
		return nil
	}
	m := inputModal(i18n.T("In allen Tabs suchen"),
		fmt.Sprintf(i18n.T("Durchsucht den Verlauf aller offenen Tabs (%d). Großbuchstaben = genau."), len(a.sessions)),
		a.searchAllLast, func(q string) tea.Cmd { return a.searchAll(q) })
	m.in.Placeholder = i18n.T("Suchbegriff")
	a.modal = m
	return nil
}

// searchAllItems: Trefferzeilen aller Tabs, aktueller Tab zuerst.
func (a *App) searchAllItems(q string) []pickItem {
	var items []pickItem
	order := make([]*session, 0, len(a.sessions))
	if c := a.current(); c != nil {
		order = append(order, c)
	}
	for _, s := range a.sessions {
		if len(order) == 0 || s != order[0] {
			order = append(order, s)
		}
	}
	for _, s := range order {
		if s.term == nil {
			continue
		}
		ms := s.term.Search(q, false)
		seen, n := -1, 0
		for i := len(ms) - 1; i >= 0 && n < searchAllPerTab && len(items) < searchAllTotal; i-- {
			m := ms[i]
			if m.Line == seen {
				continue // mehrere Treffer in einer Zeile: ein Eintrag
			}
			seen = m.Line
			n++
			text := strings.TrimSpace(s.term.PlainLine(m.Line))
			items = append(items, pickItem{
				value: strconv.Itoa(s.id) + ":" + strconv.Itoa(m.Line),
				label: trunc(text, max(20, a.w-34)),
				hint:  trunc(s.tabName(), 22),
			})
		}
	}
	return items
}

func (a *App) searchAll(q string) tea.Cmd {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	a.searchAllLast = q
	items := a.searchAllItems(q)
	if len(items) == 0 {
		a.flash(i18n.T("„") + q + i18n.T("“ steht in keinem offenen Tab"))
		return nil
	}
	title := fmt.Sprintf(i18n.T("„%s“ in allen Tabs · %s"), trunc(q, 24), plural(len(items), i18n.T("Zeile"), i18n.T("Zeilen")))
	p := newPicker(title, i18n.T("Treffer eingrenzen (Text oder Tab-Name)"), items, "\x00", func(it pickItem) tea.Cmd {
		return a.searchAllJump(q, it.value)
	})
	p.keys = [][2]string{{"⏎", i18n.T("hinspringen")}, {"↑↓", i18n.T("wählen")}, {"Esc", i18n.T("zurück")}}
	return a.openPicker(p)
}

// searchAllJump wechselt in den Tab und zeigt die Trefferzeile in der Suche.
func (a *App) searchAllJump(q, value string) tea.Cmd {
	id, line, ok := strings.Cut(value, ":")
	if !ok {
		return nil
	}
	sid, _ := strconv.Atoi(id)
	ln, _ := strconv.Atoi(line)
	s := a.byID(sid)
	if s == nil || s.term == nil {
		return nil
	}
	a.focusSession(a.indexOf(s))
	a.scr = scrTerminal
	s.search.open = false
	cmd := a.openSearch(s)
	s.search.in.SetValue(q)
	s.search.in.CursorEnd()
	a.refreshSearch(s, true)
	for i, m := range s.search.matches {
		if m.Line == ln {
			s.search.idx = i
			break
		}
	}
	a.revealMatch(s)
	return cmd
}
