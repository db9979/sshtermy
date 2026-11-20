package ui

// Zwischenablage-Verlauf: was sshTermy in die Zwischenablage legt
// (Markieren mit der Maus oder Alt+C, Panel-Text, Befehle, Pfade, OSC 52
// vom Server), merkt es sich der Reihe nach, nur im Speicher und nur, bis
// sshTermy endet. Alt+Y zeigt die Liste; ⏎ oder die Ziffer fügt den
// Eintrag am Prompt ein (mit dem üblichen Einfügeschutz). Zeilen, die nach
// Passwort, Token oder Schlüssel aussehen, landen nie im Verlauf.

import (
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
)

const clipHistMax = 9 // so viele Einträge (Ziffern 1–9 im Menü)

type clipEntry struct {
	text string
	at   time.Time
	from string // woher (z. B. Server-Alias bei OSC 52)
}

var clipHist struct {
	mu   sync.Mutex
	list []clipEntry // neueste zuerst
}

// clipWrite legt text in die Zwischenablage des Systems und merkt ihn sich.
func clipWrite(text string) error {
	clipRemember(text, "")
	return clipboard.WriteAll(text)
}

// clipSecret: sieht nach Geheimnis aus (wird nicht gemerkt).
func clipSecret(t string) bool {
	if strings.Contains(t, "PRIVATE KEY") || agent.Redact(t) != t {
		return true
	}
	return !strings.Contains(t, "\n") && secretish.MatchString(t)
}

func clipRemember(text, from string) {
	if strings.TrimSpace(text) == "" || len(text) > 64<<10 || clipSecret(text) {
		return
	}
	clipHist.mu.Lock()
	defer clipHist.mu.Unlock()
	out := []clipEntry{{text: text, at: time.Now(), from: from}}
	for _, e := range clipHist.list {
		if e.text != text && len(out) < clipHistMax {
			out = append(out, e)
		}
	}
	clipHist.list = out
}

func clipEntries() []clipEntry {
	clipHist.mu.Lock()
	defer clipHist.mu.Unlock()
	return append([]clipEntry(nil), clipHist.list...)
}

// clipLabel: eine Zeile für das Menü.
func clipLabel(t string) string {
	n := countLines(t)
	first := strings.TrimSpace(strings.SplitN(strings.TrimSpace(t), "\n", 2)[0])
	first = strings.Join(strings.Fields(first), " ")
	if n > 1 {
		return trunc(first, 44) + " (+" + plural(n-1, i18n.T("Zeile"), i18n.T("Zeilen")) + ")"
	}
	return trunc(first, 56)
}

// openClipHist zeigt den Verlauf als Menü über dem aktiven Tab.
func (a *App) openClipHist() tea.Cmd {
	list := clipEntries()
	if len(list) == 0 {
		a.flash(i18n.T("Noch nichts kopiert, seit sshTermy läuft"))
		return nil
	}
	s := a.current()
	var items []menuItem
	for _, e := range list {
		e := e
		hint := ago(e.at)
		if e.from != "" {
			hint = trunc(e.from, 12) + " · " + hint
		}
		items = append(items, menuItem{label: clipLabel(e.text), hint: hint, run: func() tea.Cmd {
			if s != nil && s.term != nil && s.state == stConnected {
				return a.guardedPaste(s, e.text)
			}
			_ = clipWrite(e.text)
			a.flash(i18n.T("Wieder in der Zwischenablage: ") + clipLabel(e.text))
			return tea.SetClipboard(e.text)
		}})
	}
	return a.showMenu(i18n.T("Zwischenablage-Verlauf · ⏎ fügt ein"), items)
}

// showMenu: Kontextmenü mittig (ohne Mausposition).
func (a *App) showMenu(title string, items []menuItem) tea.Cmd {
	a.modal = &modal{kind: mMenu, title: title, items: items}
	return nil
}
