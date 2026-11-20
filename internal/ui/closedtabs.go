package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Ausgabe vom letzten Mal und „Zuletzt geschlossen“
//
// Öffnet sshTermy einen Tab wieder (nach dem Neustart oder aus der Liste
// der zuletzt geschlossenen Tabs), steht über der neuen Sitzung gedämpft,
// was zuletzt darin lief, mit einer Trennlinie. Die Zeilen kommen aus dem
// verschlüsselten Sitzungsprotokoll; ist es aus, bleibt der Tab leer. Die
// Liste der geschlossenen Tabs hält nur Verweise (wie restore-tabs.json).

const (
	closedName   = "closed-tabs.json"
	closedMax    = 10
	closedMaxAge = 14 * 24 * time.Hour
	oldOutLines  = 200 // so viele Zeilen vom letzten Mal
)

// oldOutput: die letzten Zeilen aus dem Protokoll id, fertig für den Emulator.
func (a *App) oldOutput(id string) string {
	if id == "" || !a.logsEnabled() {
		return ""
	}
	b, err := a.logBook()
	if err != nil {
		return ""
	}
	l, err := b.Load(id)
	if err != nil {
		return ""
	}
	lines := l.Tail(oldOutLines)
	if len(lines) == 0 {
		return ""
	}
	when := l.Ended
	if when.IsZero() && len(l.Entries) > 0 {
		when = l.Entries[len(l.Entries)-1].At
	}
	return oldOutText(lines, when)
}

// oldOutText: gedämpfte Zeilen, Steuerzeichen entfernt, dann die Trennlinie.
func oldOutText(lines []string, when time.Time) string {
	var b strings.Builder
	b.WriteString("\x1b[0m\x1b[2m")
	for _, ln := range lines {
		b.WriteString(strings.Map(func(r rune) rune {
			if r == '\t' {
				return ' '
			}
			if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
				return -1
			}
			return r
		}, ansi.Strip(ln)))
		b.WriteString("\r\n")
	}
	label := i18n.T("Ausgabe vom letzten Mal")
	if !when.IsZero() {
		label += " · " + when.Local().Format("02.01. 15:04")
	}
	fmt.Fprintf(&b, "\x1b[0m\x1b[2m── %s ──\x1b[0m\r\n", label)
	return b.String()
}

// seedOld: im neuen Emulator vor der ersten Ausgabe des Servers (läuft im
// Verbindungs-Läufer, gleich nach termemu.New).
func (s *session) seedOld() {
	if s.oldOut == "" || s.term == nil {
		return
	}
	_, _ = s.term.Write([]byte(s.oldOut))
	s.oldOut = ""
}

func (a *App) closedPath() string {
	if a.st == nil || a.st.Dir == "" {
		return ""
	}
	return filepath.Join(a.st.Dir, closedName)
}

func (a *App) loadClosed() {
	if a.closedLoaded {
		return
	}
	a.closedLoaded = true
	p := a.closedPath()
	if p == "" {
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var l []restoreTab
	if json.Unmarshal(b, &l) != nil {
		return
	}
	for _, t := range l {
		if time.Since(t.Closed) < closedMaxAge {
			a.closed = append(a.closed, t)
		}
	}
}

func (a *App) saveClosed() {
	p := a.closedPath()
	if p == "" {
		return
	}
	if len(a.closed) == 0 {
		_ = os.Remove(p)
		return
	}
	b, err := json.Marshal(a.closed)
	if err == nil && os.WriteFile(p+".tmp", b, 0o600) == nil {
		_ = os.Rename(p+".tmp", p)
	}
}

// rememberClosed: aus closeTab, bevor das Protokoll endet.
func (a *App) rememberClosed(s *session) {
	if s == nil || a.st == nil {
		return
	}
	a.loadClosed()
	t := a.tabRef(s)
	t.Closed = time.Now()
	a.closed = append([]restoreTab{t}, a.closed...)
	if len(a.closed) > closedMax {
		a.closed = a.closed[:closedMax]
	}
	a.saveClosed()
}

// openClosed: Liste der zuletzt geschlossenen Tabs, ⏎ öffnet wieder.
func (a *App) openClosed() tea.Cmd {
	a.loadClosed()
	if len(a.closed) == 0 {
		a.flash(i18n.T("Noch kein Tab geschlossen"))
		return nil
	}
	var items []menuItem
	for i, t := range a.closed {
		items = append(items, menuItem{label: a.restoreName(t), hint: ago(t.Closed), run: func() tea.Cmd {
			return a.reopenClosed(i)
		}})
	}
	return a.showMenu(i18n.T("Zuletzt geschlossen · ⏎ öffnet mit der Ausgabe von damals"), items)
}

// reopenClosed: Eintrag i wieder öffnen (und aus der Liste nehmen).
func (a *App) reopenClosed(i int) tea.Cmd {
	if i < 0 || i >= len(a.closed) {
		return nil
	}
	t := a.closed[i]
	s, cmd := a.openRef(t)
	if s == nil {
		a.flashErr(i18n.T("Verbindung gibt es nicht mehr"))
		return nil
	}
	a.closed = append(a.closed[:i:i], a.closed[i+1:]...)
	a.saveClosed()
	a.scr = scrTerminal
	return cmd
}
