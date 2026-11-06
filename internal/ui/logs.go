package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sessionlog"
	"github.com/db9979/sshtermy/internal/syncx"
)

// Sitzungsprotokolle (Manager: l, Palette): Liste aller mitgeschriebenen
// Sitzungen, Volltextsuche über Host, Befehle und Ausgabe, KI-Zusammenfassung
// einer Sitzung (z) oder aller Treffer (Z, z. B. „Was habe ich auf Host X
// gemacht?“ = nach X suchen, dann Z) und Markdown-Export (x).

type logsState struct {
	all        []*sessionlog.Log
	unreadable int
	err        string
	search     textinput.Model
	searching  bool
	sel        int
	scroll     int
	busy       string // läuft: Zusammenfassung von …
	group      string // Zusammenfassung über alle Treffer
	groupQ     string
	back       screen
}

type logSummaryMsg struct {
	id   string // "" = alle Treffer
	text string
	err  error
}

func (a *App) openLogs() tea.Cmd {
	in := newInput("", false)
	in.Placeholder = i18n.T("Suchen (Host, Befehl, Ausgabe, Datum)")
	back := a.scr
	if back == scrLogs {
		back = a.logs.back
	}
	a.logs = logsState{search: in, back: back}
	a.reloadLogs()
	a.scr = scrLogs
	return nil
}

func (a *App) reloadLogs() {
	b, err := a.logBook()
	if err != nil {
		a.logs.err = err.Error()
		return
	}
	a.logs.all, a.logs.unreadable, err = b.List()
	if err != nil {
		a.logs.err = err.Error()
	}
	a.logs.sel = min(a.logs.sel, max(0, len(a.logs.all)-1))
}

func (a *App) filteredLogs() []*sessionlog.Log {
	q := a.logs.search.Value()
	var out []*sessionlog.Log
	for _, l := range a.logs.all {
		if l.Matches(q) {
			out = append(out, l)
		}
	}
	return out
}

func (a *App) activeLog(id string) *session {
	for _, s := range a.sessions {
		if s.log != nil && s.log.ID == id {
			return s
		}
	}
	return nil
}

func (a *App) updateLogs(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	m := &a.logs
	list := a.filteredLogs()
	if m.searching {
		switch k.String() {
		case "esc":
			m.searching = false
			m.search.SetValue("")
			m.search.Blur()
		case "enter":
			m.searching = false
			m.search.Blur()
		default:
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(k)
			m.sel, m.scroll = 0, 0
			return cmd
		}
		return nil
	}
	var cur *sessionlog.Log
	if m.sel < len(list) {
		cur = list[m.sel]
	}
	switch k.String() {
	case "up", "k":
		m.sel, m.scroll, m.group = max(0, m.sel-1), 0, ""
	case "down", "j":
		m.sel, m.scroll, m.group = max(0, min(len(list)-1, m.sel+1)), 0, ""
	case "pgdown", "space":
		m.scroll += max(1, a.h/2)
	case "pgup":
		m.scroll = max(0, m.scroll-max(1, a.h/2))
	case "home":
		m.scroll = 0
	case "/":
		m.searching = true
		return m.search.Focus()
	case "z":
		if cur != nil && m.busy == "" {
			return a.summarizeLogs([]*sessionlog.Log{cur}, cur.ID)
		}
	case "Z":
		if len(list) > 0 && m.busy == "" {
			return a.summarizeLogs(list, "")
		}
	case "x":
		if cur != nil {
			a.modal = a.exportMenu(cur)
		}
	case "d", "delete":
		if cur != nil {
			if a.activeLog(cur.ID) != nil {
				a.flash(i18n.T("Die Sitzung läuft noch; erst Tab schließen"))
				return nil
			}
			l := cur
			a.modal = confirmModal(i18n.T("Protokoll löschen"), i18n.T("Protokoll „")+l.Host+i18n.T("“ vom ")+l.Started.Format("02.01.2006 15:04")+i18n.T(" wirklich löschen?"), func() tea.Cmd {
				if b, err := a.logBook(); err == nil {
					if err := b.Delete(l.ID); err != nil {
						a.flashErr(err.Error())
					} else if a.st.SyncConfig().Archive {
						_ = a.st.ForgetArchived(syncx.ArchiveTombstone(syncx.KindLog, l.ID))
					}
				}
				a.reloadLogs()
				return nil
			})
		}
	case "a":
		return a.toggleSessionLog()
	case "r":
		a.reloadLogs()
	case "esc", "q", "f10":
		if m.group != "" {
			m.group = ""
			return nil
		}
		a.scr = m.back
		if a.scr == scrTerminal && a.current() == nil {
			a.scr = scrManager
		}
	case "f1":
		a.modal = a.helpModal()
	}
	return nil
}

func (a *App) toggleSessionLog() tea.Cmd {
	on := !a.logsEnabled()
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.NoSessionLog = !on }); err != nil {
		a.flashErr(err.Error())
		return nil
	}
	if on {
		for _, s := range a.sessions {
			if s.state == stConnected {
				a.startLog(s)
			}
		}
		a.flash(i18n.T("Sitzungsprotokoll an (neue Befehle werden verschlüsselt mitgeschrieben)"))
	} else {
		for _, s := range a.sessions {
			a.endLog(s)
		}
		a.flash(i18n.T("Sitzungsprotokoll aus"))
	}
	return nil
}

func (a *App) summarizeLogs(list []*sessionlog.Log, id string) tea.Cmd {
	ag, err := a.agentClient()
	if err != nil {
		a.flashErr("KI: " + err.Error())
		return nil
	}
	var b strings.Builder
	budget := 24000
	for i, l := range list {
		if i > 0 {
			b.WriteString("\n---\n")
		}
		b.WriteString(l.Transcript(max(2000, budget/len(list))))
		if b.Len() > budget {
			break
		}
	}
	if id == "" {
		a.logs.busy = fmt.Sprintf(i18n.T("%d Sitzungen"), len(list))
		a.logs.groupQ = a.logs.search.Value()
	} else {
		a.logs.busy = list[0].Host
	}
	tr := b.String()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		text, err := ag.Summarize(ctx, tr)
		return logSummaryMsg{id, text, err}
	}
}

func (a *App) logsMsg(msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(logSummaryMsg)
	if !ok {
		return nil, false
	}
	a.logs.busy = ""
	if m.err != nil {
		a.flashErr(i18n.T("Zusammenfassung: ") + m.err.Error())
		return nil, true
	}
	if m.id == "" {
		a.logs.group, a.logs.scroll = m.text, 0
		return nil, true
	}
	if s := a.activeLog(m.id); s != nil {
		s.log.Summary = m.text
		a.saveLog(s)
	} else if b, err := a.logBook(); err == nil {
		if l, err := b.Load(m.id); err == nil {
			l.Summary = m.text
			if err := b.Save(l); err != nil {
				a.flashErr(err.Error())
			}
		}
	}
	a.reloadLogs()
	return nil, true
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// exportDir: Downloads, sonst Home-Ordner.
func exportDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	for _, d := range []string{"Downloads", "Download"} {
		if st, err := os.Stat(filepath.Join(home, d)); err == nil && st.IsDir() {
			return filepath.Join(home, d)
		}
	}
	return home
}

// ---------------------------------------------------------------- Ansicht

func (a *App) logsKeys() [][2]string {
	if a.logs.searching {
		return [][2]string{{"⏎", i18n.T("fertig")}, {"Esc", i18n.T("Suche leeren")}}
	}
	onOff := i18n.T("aus")
	if !a.logsEnabled() {
		onOff = i18n.T("an")
	}
	return [][2]string{{"↑↓", i18n.T("wählen")}, {"/", i18n.T("suchen")}, {"z", i18n.T("KI-Zusammenfassung")}, {"Z", i18n.T("alle Treffer zusammenfassen")},
		{"x", i18n.T("exportieren")}, {"d", i18n.T("löschen")}, {"PgDn", i18n.T("blättern")}, {"a", i18n.T("Mitschreiben ") + onOff}, {"Esc", i18n.T("zurück")}}
}

func (a *App) viewLogs() string {
	s := a.s
	m := &a.logs
	list := a.filteredLogs()
	h := a.h - 2
	leftW := min(50, max(32, a.w/3))
	rightW := a.w - leftW - 1

	var lb strings.Builder
	if m.searching || m.search.Value() != "" {
		lb.WriteString(" " + s.Accent.Render("/") + " " + m.search.View() + "\n\n")
	} else {
		lb.WriteString(s.PanelTitle.Foreground(s.T.Accent).Render(i18n.T(" Sitzungsprotokolle")) + s.Muted.Render(fmt.Sprintf("  %d", len(list))) +
			"  " + a.keyZone("/", s.Muted.Render(i18n.T("/ suchen"))) + "\n\n")
	}
	if !a.logsEnabled() {
		lb.WriteString(" " + s.Warn.Render(i18n.T("Mitschreiben ist aus")) + s.Muted.Render(" · a = an") + "\n\n")
	}
	room := max(1, h-4)
	start := 0
	if m.sel >= room {
		start = m.sel - room + 1
	}
	for i := start; i < len(list) && i < start+room; i++ {
		l := list[i]
		fails := ""
		if n := l.Failures(); n > 0 {
			fails = s.Danger.Render(fmt.Sprintf(" ✗%d", n))
		}
		live := " "
		if a.activeLog(l.ID) != nil {
			live = s.OK.Render("●")
		}
		date := l.Started.Format("02.01. 15:04")
		cnt := fmt.Sprintf("%3d", len(l.Entries))
		name := trunc(l.Host, max(6, leftW-26))
		line := fmt.Sprintf(" %s %s  %-*s %s", live, s.Muted.Render(date), max(6, leftW-26), name, s.Muted.Render(cnt)) + fails
		if i == m.sel {
			line = s.Selected.Width(leftW - 1).Render(fmt.Sprintf(" ▶ %s  %-*s %s", date, max(6, leftW-26), name, cnt))
		}
		i := i
		lb.WriteString(a.z.click(line, func(tea.Mouse, bool) tea.Cmd { a.logs.sel, a.logs.scroll, a.logs.group = i, 0, ""; return nil }) + "\n")
	}
	switch {
	case m.err != "":
		lb.WriteString(s.Danger.Render(" " + m.err))
	case len(a.logs.all) == 0:
		lb.WriteString(s.Muted.Render(i18n.T("  Noch keine Protokolle.\n  Jede SSH-Sitzung wird ab dem\n  ersten Befehl mitgeschrieben.")))
	case len(list) == 0:
		lb.WriteString(s.Muted.Render(i18n.T("  Keine Treffer.\n  Andere Wörter probieren oder\n  Esc leert die Suche.")))
	}
	if m.unreadable > 0 {
		lb.WriteString("\n" + s.Muted.Render(fmt.Sprintf(i18n.T("  %d Datei(en) nicht lesbar"), m.unreadable)))
	}
	left := lipgloss.NewStyle().Width(leftW).Height(h).MaxHeight(h).Render(lb.String())
	sep := lipgloss.NewStyle().Foreground(s.T.Border).Render(strings.Repeat("│\n", max(0, h-1)) + "│")

	var rb strings.Builder
	w := rightW - 2
	if m.busy != "" {
		rb.WriteString(" " + s.AI.Render(i18n.T("✦ KI fasst zusammen: ")+m.busy+" …") + "\n\n")
	}
	switch {
	case m.group != "":
		title := fmt.Sprintf(i18n.T("✦ Zusammenfassung über %s"), plural(len(list), i18n.T("Sitzung"), i18n.T("Sitzungen")))
		if m.groupQ != "" {
			title += i18n.T(" zu „") + m.groupQ + "“"
		}
		rb.WriteString(" " + s.PanelTitle.Foreground(s.T.AI).Render(title) + s.Muted.Render("  ") + a.hints(s.Muted, [2]string{"Esc", i18n.T("schließen")}) + "\n\n")
		rb.WriteString(indent(wrap(s.Text.Render(m.group), w), " "))
	case m.sel < len(list):
		l := list[m.sel]
		rb.WriteString(" " + s.Accent.Render(l.Host) + "  " + s.Muted.Render(l.Addr) + "   " +
			a.keyZone("z", s.Key.Render("z")+s.KeyDesc.Render(i18n.T("zusammenfassen"))) + " " +
			a.keyZone("x", s.Key.Render("x")+s.KeyDesc.Render(i18n.T("exportieren"))) + "\n")
		end := i18n.T("läuft")
		if a.activeLog(l.ID) == nil {
			end = i18n.T("abgebrochen")
		}
		if !l.Ended.IsZero() {
			end = l.Ended.Format("15:04") + " (" + l.Ended.Sub(l.Started).Round(time.Minute).String() + ")"
		}
		info := l.Started.Format("02.01.2006 15:04") + " – " + end + " · " + plural(len(l.Entries), i18n.T("Befehl"), i18n.T("Befehle"))
		if n := l.Failures(); n > 0 {
			info += fmt.Sprintf(i18n.T(" · %d fehlgeschlagen"), n)
		}
		if l.System != "" {
			info += " · " + l.System
		}
		if l.Device != "" {
			info += i18n.T(" · von ") + l.Device
		}
		rb.WriteString(" " + s.Muted.Render(trunc(info, w)) + "\n\n")
		if l.Summary != "" {
			rb.WriteString(" " + s.PanelTitle.Foreground(s.T.AI).Render(i18n.T("✦ Zusammenfassung")) + "\n")
			rb.WriteString(indent(wrap(s.Text.Render(l.Summary), w), " ") + "\n\n")
		}
		idx := make([]int, len(l.Entries))
		for i := range idx {
			idx[i] = i
		}
		if q := m.search.Value(); strings.TrimSpace(q) != "" {
			if hits := l.MatchingEntries(q); len(hits) > 0 {
				idx = hits
				rb.WriteString(" " + s.Muted.Render(fmt.Sprintf(i18n.T("%d von %d Befehlen passen zur Suche"), len(hits), len(l.Entries))) + "\n\n")
			}
		}
		for _, i := range idx {
			e := l.Entries[i]
			mark := s.Accent.Render("$")
			if e.Failed {
				mark = s.Danger.Render("✗")
			}
			rb.WriteString(" " + s.Muted.Render(e.At.Format("15:04:05")) + " " + mark + " " + s.Text.Bold(true).Render(trunc(e.Command, w-12)) + "\n")
			if e.Failed && e.Reason != "" {
				rb.WriteString("   " + s.Danger.Render(trunc(e.Reason, w-4)) + "\n")
			}
			out := strings.Split(strings.TrimSpace(e.Output), "\n")
			if len(out) > 4 {
				out = append(out[:3:3], fmt.Sprintf(i18n.T("… %d weitere Zeilen"), len(out)-3))
			}
			for _, o := range out {
				if strings.TrimSpace(o) != "" {
					rb.WriteString("   " + s.Muted.Render(trunc(o, w-4)) + "\n")
				}
			}
		}
	}
	lines := strings.Split(rb.String(), "\n")
	m.scroll = min(m.scroll, max(0, len(lines)-h))
	lines = lines[m.scroll:]
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, rightW, "")
	}
	right := lipgloss.NewStyle().Width(rightW).Height(h).Render(strings.Join(lines, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, sep, right)
}

func indent(s, pre string) string {
	l := strings.Split(s, "\n")
	for i := range l {
		l[i] = pre + l[i]
	}
	return strings.Join(l, "\n")
}
