package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

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
	group      string // Zusammenfassung über mehrere Sitzungen
	groupQ     string
	back       screen
	byHost     bool            // Tab: nach Host statt nach Tagen
	sortN      bool            // Hosts nach Anzahl sortieren
	filter     logFilter       // Alle / Heute / 7 Tage / Mit Fehlern
	open       map[string]bool // auf-/zugeklappt (überschreibt die Vorgabe)
	hostSum    map[string]string
	report     string // Änderungsbericht (report.go), "" = keiner offen
	reportFile string
}

type logSummaryMsg struct {
	id   string // "" = mehrere Sitzungen
	key  string // Host-Zeile, für die die Zusammenfassung gemerkt wird
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
	old := a.logs
	a.logs = logsState{search: in, back: back, byHost: old.byHost, sortN: old.sortN, open: old.open, hostSum: old.hostSum}
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
}

func (a *App) filteredLogs() []*sessionlog.Log {
	q := a.logs.search.Value()
	now := time.Now()
	var out []*sessionlog.Log
	for _, l := range a.logs.all {
		if a.logs.filter.ok(l, now) && l.Matches(q) {
			out = append(out, l)
		}
	}
	return out
}

// logRows: sichtbare Zeilen der Liste (nach Tagen oder nach Host).
func (a *App) logRows() []logRow {
	m := &a.logs
	folders := map[string]string{}
	for _, c := range a.st.Connections() {
		folders[c.ID] = c.Group
	}
	return logTree{
		logs:     a.filteredLogs(),
		byHost:   m.byHost,
		sortN:    m.sortN,
		now:      time.Now(),
		open:     m.open,
		openDays: strings.TrimSpace(m.search.Value()) != "" || m.filter != lfAll,
		folder:   func(l *sessionlog.Log) string { return folders[l.ConnID] },
	}.rows()
}

func (a *App) activeLog(id string) *session {
	for _, s := range a.sessions {
		if s.log != nil && s.log.ID == id {
			return s
		}
	}
	return nil
}

func (a *App) anyActive(logs []*sessionlog.Log) bool {
	for _, l := range logs {
		if a.activeLog(l.ID) != nil {
			return true
		}
	}
	return false
}

func (a *App) setLogOpen(r logRow, open bool) {
	if !r.expandable() || r.open == open {
		return
	}
	if a.logs.open == nil {
		a.logs.open = map[string]bool{}
	}
	a.logs.open[r.key] = open
}

func (a *App) selectLog(i int) {
	a.logs.sel, a.logs.scroll, a.logs.group, a.logs.report = i, 0, "", ""
}

func (a *App) updateLogs(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	m := &a.logs
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
	rows := a.logRows()
	m.sel = max(0, min(m.sel, len(rows)-1))
	var cur *logRow
	if m.sel < len(rows) {
		cur = &rows[m.sel]
	}
	switch k.String() {
	case "up", "k":
		a.selectLog(max(0, m.sel-1))
	case "down", "j":
		a.selectLog(max(0, min(len(rows)-1, m.sel+1)))
	case "enter":
		if cur != nil {
			a.setLogOpen(*cur, !cur.open)
		}
	case "right":
		if cur != nil {
			a.setLogOpen(*cur, true)
		}
	case "left":
		if cur == nil {
			break
		}
		if cur.open {
			a.setLogOpen(*cur, false)
			break
		}
		// zur übergeordneten Zeile springen
		for i := m.sel - 1; i >= 0; i-- {
			if rows[i].kind == lrDay && cur.kind != lrDay || rows[i].depth < cur.depth {
				a.selectLog(i)
				break
			}
		}
	case "tab":
		m.byHost = !m.byHost
		a.selectLog(0)
	case "f":
		m.filter = (m.filter + 1) % lfCount
		a.selectLog(0)
	case "s":
		if m.byHost {
			m.sortN = !m.sortN
			a.selectLog(0)
		}
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
		if cur == nil || m.busy != "" {
			break
		}
		if len(cur.logs) == 1 && cur.kind != lrDay {
			return a.summarizeLogs(cur.logs, cur.logs[0].ID, "", "")
		}
		key := ""
		if cur.kind == lrHost {
			key = cur.key
		}
		return a.summarizeLogs(cur.logs, "", a.rowTitle(*cur), key)
	case "b":
		if cur != nil && m.busy == "" {
			return a.reportLogs(cur.logs, a.rowTitle(*cur))
		}
	case "Z":
		if list := a.filteredLogs(); len(list) > 0 && m.busy == "" {
			return a.summarizeLogs(list, "", m.search.Value(), "")
		}
	case "x":
		if cur == nil {
			break
		}
		if len(cur.logs) == 1 && cur.kind != lrDay {
			a.modal = a.exportMenu(cur.logs[0])
		} else {
			a.flash(i18n.T("Exportieren geht pro Sitzung: ⏎ klappt die Zeile auf"))
		}
	case "d", "delete":
		if cur != nil {
			a.confirmDeleteLogs(cur.logs)
		}
	case "a":
		return a.toggleSessionLog()
	case "r":
		a.reloadLogs()
	case "esc", "q", "f10":
		if m.group != "" || m.report != "" {
			m.group, m.report = "", ""
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

func (a *App) rowTitle(r logRow) string {
	switch r.kind {
	case lrDay:
		return dayTitle(r.day, time.Now())
	case lrGroup:
		return r.label
	}
	return r.logs[0].Host
}

func (a *App) confirmDeleteLogs(logs []*sessionlog.Log) {
	if a.anyActive(logs) {
		a.flash(i18n.T("Die Sitzung läuft noch; erst Tab schließen"))
		return
	}
	var body string
	if len(logs) == 1 {
		l := logs[0]
		body = i18n.T("Protokoll „") + l.Host + i18n.T("“ vom ") + l.Started.Format("02.01.2006 15:04") + i18n.T(" wirklich löschen?")
	} else {
		body = fmt.Sprintf(i18n.T("%d Protokolle wirklich löschen?"), len(logs))
	}
	a.modal = confirmModal(i18n.T("Protokoll löschen"), body, func() tea.Cmd {
		if b, err := a.logBook(); err == nil {
			for _, l := range logs {
				if err := b.Delete(l.ID); err != nil {
					a.flashErr(err.Error())
					break
				} else if a.st.SyncConfig().Archive {
					_ = a.st.ForgetArchived(syncx.ArchiveTombstone(syncx.KindLog, l.ID))
				}
			}
		}
		a.reloadLogs()
		return nil
	})
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

func (a *App) summarizeLogs(list []*sessionlog.Log, id, title, key string) tea.Cmd {
	ag, err := a.agentClientFor(a.logConns(list)...)
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
		a.logs.groupQ = title
	} else {
		a.logs.busy = list[0].Host
	}
	tr := b.String()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		text, err := ag.Summarize(ctx, tr)
		return logSummaryMsg{id, key, text, err}
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
		if m.key != "" {
			if a.logs.hostSum == nil {
				a.logs.hostSum = map[string]string{}
			}
			a.logs.hostSum[m.key] = m.text
		}
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
	view := i18n.T("nach Host")
	if a.logs.byHost {
		view = i18n.T("nach Zeit")
	}
	return [][2]string{{"↑↓", i18n.T("wählen")}, {"⏎", i18n.T("auf/zu")}, {"Tab", view}, {"f", i18n.T("Filter")}, {"/", i18n.T("suchen")},
		{"z", i18n.T("KI-Zusammenfassung")}, {"Z", i18n.T("alle Treffer zusammenfassen")}, {"b", i18n.T("Änderungsbericht")},
		{"x", i18n.T("exportieren")}, {"d", i18n.T("löschen")}, {"PgDn", i18n.T("blättern")}, {"a", i18n.T("Mitschreiben ") + onOff}, {"Esc", i18n.T("zurück")}}
}

// logsLeftW: Breite der Liste.
func (a *App) logsLeftW() int {
	if a.w < 90 {
		return max(30, a.w*3/5)
	}
	return min(64, a.w/2)
}

func (a *App) viewLogs() string {
	s := a.s
	m := &a.logs
	rows := a.logRows()
	m.sel = max(0, min(m.sel, len(rows)-1))
	nLogs := len(a.filteredLogs())
	h := a.h - 2
	leftW := a.logsLeftW()
	rightW := a.w - leftW - 1
	now := time.Now()

	var lb strings.Builder
	view := i18n.T("Tab nach Host")
	if m.byHost {
		view = i18n.T("Tab nach Zeit")
	}
	if m.searching || m.search.Value() != "" {
		lb.WriteString(" " + s.Accent.Render("/") + " " + m.search.View() + "\n")
	} else {
		count := fmt.Sprintf("  %d", nLogs)
		if m.byHost {
			count = "  " + hostsText(len(rows)-countChildren(rows))
		}
		title := s.PanelTitle.Foreground(s.T.Accent).Render(i18n.T(" Sitzungsprotokolle")) + s.Muted.Render(count) +
			"  " + a.keyZone("/", s.Muted.Render(i18n.T("/ suchen")))
		if lipgloss.Width(title)+lipgloss.Width(view)+3 <= leftW {
			title += strings.Repeat(" ", leftW-lipgloss.Width(title)-lipgloss.Width(view)-2) + a.keyZone("Tab", s.Muted.Render(view))
			view = ""
		}
		lb.WriteString(ansi.Truncate(title, leftW, "") + "\n")
	}
	// Filter als anklickbare Chips, daneben die Ansicht (Tab)
	chips := " "
	for f := lfAll; f < lfCount; f++ {
		label := f.label()
		if f == lfFailed {
			if n := countFailed(a.logs.all); n > 0 {
				label += fmt.Sprintf(" (%d)", n)
			}
		}
		st := s.Muted.Padding(0, 1)
		if f == m.filter {
			st = lipgloss.NewStyle().Background(s.T.Accent).Foreground(s.T.Bg).Bold(true).Padding(0, 1)
		}
		f := f
		chips += a.z.click(st.Render(label), func(tea.Mouse, bool) tea.Cmd { a.logs.filter = f; a.selectLog(0); return nil })
	}
	if view != "" && lipgloss.Width(chips)+lipgloss.Width(view)+3 <= leftW {
		chips += strings.Repeat(" ", leftW-lipgloss.Width(chips)-lipgloss.Width(view)-2) + a.keyZone("Tab", s.Muted.Render(view))
	}
	lb.WriteString(ansi.Truncate(chips, leftW, "") + "\n")
	if !a.logsEnabled() {
		lb.WriteString(" " + s.Warn.Render(i18n.T("Mitschreiben ist aus")) + s.Muted.Render(" · a = an") + "\n")
	}

	// Spalten: Zeit, Name, Sitzungen, Befehle, Fehler
	timeW := 5
	for _, r := range rows {
		timeW = max(timeW, len([]rune(a.rowTime(r, now))))
	}
	sessW := 5 // Spalte „Sitz.“, bei schmalem Fenster weg
	if leftW-timeW-21 < 12 {
		sessW = 0
	}
	nameW := max(6, leftW-timeW-16-sessW)
	sessH := ""
	if sessW > 0 {
		sessH = i18n.T("Sitz.")
	}
	head := "   " + fmt.Sprintf("%-*s  %-*s%*s %4s", timeW, i18n.T("Zeit"), nameW, "  "+i18n.T("Host"), sessW, sessH, i18n.T("Bef."))
	if m.byHost {
		head = "   " + fmt.Sprintf("%-*s  %-*s%*s %4s", nameW, "  "+i18n.T("Host"), timeW, i18n.T("zuletzt"), sessW, sessH, i18n.T("Bef."))
	}
	if len(rows) > 0 {
		lb.WriteString(s.Muted.Render(ansi.Truncate(head, leftW-1, "")) + "\n")
	}
	used := strings.Count(lb.String(), "\n")
	room := max(1, h-used-1)
	start := 0
	if m.sel >= room {
		start = m.sel - room + 1
	}
	for i := start; i < len(rows) && i < start+room; i++ {
		i := i
		line := a.logRowLine(rows[i], i == m.sel, timeW, nameW, sessW, leftW, now)
		lb.WriteString(a.z.click(line, func(_ tea.Mouse, _ bool) tea.Cmd {
			if a.logs.sel == i {
				if rs := a.logRows(); i < len(rs) {
					a.setLogOpen(rs[i], !rs[i].open)
				}
				return nil
			}
			a.selectLog(i)
			return nil
		}) + "\n")
	}
	switch {
	case m.err != "":
		lb.WriteString(s.Danger.Render(" " + m.err))
	case len(a.logs.all) == 0:
		lb.WriteString("\n" + s.Muted.Render(i18n.T("  Noch keine Protokolle.\n  Jede SSH-Sitzung wird ab dem\n  ersten Befehl mitgeschrieben.")))
	case len(rows) == 0:
		lb.WriteString("\n" + s.Muted.Render(i18n.T("  Keine Treffer.\n  Andere Wörter probieren oder\n  Esc leert die Suche.")))
	}
	if m.unreadable > 0 {
		lb.WriteString("\n" + s.Muted.Render(fmt.Sprintf(i18n.T("  %d Datei(en) nicht lesbar"), m.unreadable)))
	}
	left := lipgloss.NewStyle().Width(leftW).Height(h).MaxHeight(h).Render(lb.String())
	sep := lipgloss.NewStyle().Foreground(s.T.Border).Render(strings.Repeat("│\n", max(0, h-1)) + "│")

	var rb strings.Builder
	w := rightW - 2
	if m.busy != "" {
		rb.WriteString(" " + s.AI.Render(i18n.T("◆ KI fasst zusammen: ")+m.busy+" …") + "\n\n")
	}
	switch {
	case m.report != "":
		rb.WriteString(" " + s.PanelTitle.Foreground(s.T.AI).Render(i18n.T("◆ Änderungsbericht")) + s.Muted.Render("  ") + a.hints(s.Muted, [2]string{"Esc", i18n.T("schließen")}) + "\n")
		rb.WriteString(" " + s.Muted.Render(trunc(i18n.T("kopiert und gespeichert: ")+m.reportFile, w)) + "\n\n")
		rb.WriteString(indent(wrap(s.Text.Render(m.report), w), " "))
	case m.group != "":
		title := fmt.Sprintf(i18n.T("◆ Zusammenfassung über %s"), sessionsText(nLogsOf(rows, m.sel, nLogs, m.groupQ == m.search.Value())))
		if m.groupQ != "" {
			title += i18n.T(" zu „") + m.groupQ + i18n.T("“")
		}
		rb.WriteString(" " + s.PanelTitle.Foreground(s.T.AI).Render(title) + s.Muted.Render("  ") + a.hints(s.Muted, [2]string{"Esc", i18n.T("schließen")}) + "\n\n")
		rb.WriteString(indent(wrap(s.Text.Render(m.group), w), " "))
	case m.sel < len(rows):
		a.logDetail(&rb, rows[m.sel], w, now)
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

// nLogsOf: Anzahl Sitzungen, über die die angezeigte Zusammenfassung geht.
func nLogsOf(rows []logRow, sel, all int, isAll bool) int {
	if isAll || sel >= len(rows) {
		return all
	}
	return len(rows[sel].logs)
}

func countChildren(rows []logRow) int {
	n := 0
	for _, r := range rows {
		if r.depth > 0 {
			n++
		}
	}
	return n
}

func countFailed(logs []*sessionlog.Log) int {
	n := 0
	for _, l := range logs {
		if l.Failures() > 0 {
			n++
		}
	}
	return n
}

// rowTime: Zeitspalte (nach Tagen: Uhrzeit, nach Host: zuletzt bzw. Datum).
func (a *App) rowTime(r logRow, now time.Time) string {
	switch {
	case r.kind == lrDay:
		return ""
	case r.kind == lrHost:
		return whenShort(r.logs[0].Started, now)
	case a.logs.byHost:
		return r.logs[0].Started.Format("02.01. ") + timeRange(r.logs)
	}
	return timeRange(r.logs)
}

func (a *App) logRowLine(r logRow, sel bool, timeW, nameW, sessW, leftW int, now time.Time) string {
	s := a.s
	nCmd, nFail := r.commands()
	fails := ""
	if nFail > 0 {
		fails = fmt.Sprintf(" ×%d", nFail)
	}
	arrow := "▸"
	if r.open {
		arrow = "▾"
	}
	if r.kind == lrDay {
		title := dayTitle(r.day, now)
		right := sessionsText(len(r.logs))
		room := leftW - 1 - 3 - len([]rune(title)) - 4 // 4 = Platz für „ ×N“
		if len([]rune(right))+1 > room {
			right = fmt.Sprintf("%d", len(r.logs))
		}
		title = trunc(title, max(4, leftW-1-3-4-len([]rune(right))-1))
		gap := max(1, leftW-1-3-4-len([]rune(title))-len([]rune(right)))
		if sel {
			return s.Selected.Width(leftW - 1).Render(" " + arrow + " " + title + strings.Repeat(" ", gap) + right + fails)
		}
		return " " + s.Accent.Render(arrow) + " " + s.Accent.Bold(true).Render(title) + strings.Repeat(" ", gap) + s.Muted.Render(right) + s.Danger.Render(fails)
	}
	name := r.label
	if r.kind == lrGroup {
		name = r.label + " · " + hostsText(r.hosts)
		if len([]rune(name)) > nameW-2-2*r.depth {
			name = fmt.Sprintf("%s (%d)", r.label, r.hosts)
		}
	}
	// Pfeil zum Aufklappen bzw. Baumlinie für Kinder
	mark := "  "
	switch {
	case r.expandable():
		mark = arrow + " "
	case r.depth > 0 && r.last:
		mark = "└ "
	case r.depth > 0:
		mark = "├ "
	}
	tree := strings.Repeat("  ", max(0, r.depth-1)) + mark
	nw := max(4, nameW-len([]rune(tree)))
	name = trunc(name, nw)
	sess := ""
	if len(r.logs) > 1 && r.kind != lrGroup {
		sess = fmt.Sprintf("%d×", len(r.logs))
	}
	sessF := fmt.Sprintf(" %4s", sess)
	if sessW == 0 {
		sessF = ""
	}
	tm := a.rowTime(r, now)
	live := " "
	if a.anyActive(r.logs) {
		live = s.OK.Render("●")
	}
	if sel {
		var line string
		if r.kind == lrHost {
			line = fmt.Sprintf(" ▶ %s%-*s  %-*s%s %4d%s", tree, nw, name, timeW, tm, sessF, nCmd, fails)
		} else {
			line = fmt.Sprintf(" ▶ %-*s  %s%-*s%s %4d%s", timeW, tm, tree, nw, name, sessF, nCmd, fails)
		}
		return s.Selected.Width(leftW - 1).Render(ansi.Truncate(line, leftW-1, ""))
	}
	pad := func(x string, n int) string { return x + strings.Repeat(" ", max(0, n-len([]rune(x)))) }
	cols := s.AI.Render(sessF) + " " + s.Muted.Render(fmt.Sprintf("%4d", nCmd)) + s.Danger.Render(fails)
	if r.kind == lrHost {
		return " " + live + " " + s.Accent.Render(tree) + pad(name, nw) + "  " + s.Muted.Render(pad(tm, timeW)) + cols
	}
	treeSt := s.Muted
	if r.expandable() {
		treeSt = s.Accent
	}
	return " " + live + " " + s.Muted.Render(pad(tm, timeW)) + "  " + treeSt.Render(tree) + pad(name, nw) + cols
}

// logDetail: rechte Seite für die gewählte Zeile.
func (a *App) logDetail(rb *strings.Builder, r logRow, w int, now time.Time) {
	s := a.s
	nCmd, nFail := r.commands()
	stats := func() string {
		info := plural(nCmd, i18n.T("Befehl"), i18n.T("Befehle"))
		if nFail > 0 {
			info += fmt.Sprintf(i18n.T(" · %d fehlgeschlagen"), nFail)
		}
		return info
	}
	keys := func() string {
		return a.keyZone("z", s.Key.Render("z")+s.KeyDesc.Render(i18n.T("zusammenfassen"))) + "  " +
			a.keyZone("b", s.Key.Render("b")+s.KeyDesc.Render(i18n.T("Änderungsbericht")))
	}
	switch {
	case r.kind == lrItem && len(r.logs) == 1:
		a.logSingle(rb, r.logs[0], w)
		return
	case r.kind == lrDay:
		rb.WriteString(" " + s.Accent.Render(dayTitle(r.day, now)) + "   " + keys() + "\n")
		rb.WriteString(" " + s.Muted.Render(trunc(sessionsText(len(r.logs))+" · "+hostsText(r.hosts)+" · "+stats(), w)) + "\n\n")
		a.activityBlock(rb, a.logs.all, w, r.day, now) // activity.go
		rb.WriteString(" " + s.PanelTitle.Foreground(s.T.Accent).Render(i18n.T("Hosts")) + "\n")
		a.logHostList(rb, r.logs, w)
		return
	case r.kind == lrGroup:
		from, to := r.span()
		rb.WriteString(" " + s.Accent.Render(r.label) + "  " + s.Muted.Render(hostsText(r.hosts)) + "   " + keys() + "\n")
		rb.WriteString(" " + s.Muted.Render(trunc(from.Format("02.01.2006 15:04")+" – "+to.Format("15:04")+" · "+sessionsText(len(r.logs))+" · "+stats(), w)) + "\n\n")
		if cmds := sameCommands(r.logs); cmds != nil {
			a.logSameCommands(rb, r.logs, cmds, w)
			return
		}
		a.logSessions(rb, r.logs, w, true)
		return
	case r.kind == lrHost:
		l := r.logs[0]
		head := " " + s.Accent.Render(l.Host) + "  " + s.Muted.Render(l.Addr)
		if l.System != "" {
			head += s.Muted.Render(" · " + l.System)
		}
		rb.WriteString(head + "   " + keys() + "\n")
		first := r.logs[len(r.logs)-1].Started
		rb.WriteString(" " + s.Muted.Render(trunc(sessionsText(len(r.logs))+i18n.T(" seit ")+first.Format("02.01.")+" · "+stats(), w)) + "\n\n")
		a.activityBlock(rb, r.logs, w, time.Time{}, now)
		if sum := a.logs.hostSum[r.key]; sum != "" {
			rb.WriteString(" " + s.PanelTitle.Foreground(s.T.AI).Render(i18n.T("◆ KI-Zusammenfassung")) + "\n")
			rb.WriteString(indent(wrap(s.Text.Render(sum), w), " ") + "\n\n")
		} else if len(r.logs) == 1 && l.Summary != "" {
			rb.WriteString(" " + s.PanelTitle.Foreground(s.T.AI).Render(i18n.T("◆ Zusammenfassung")) + "\n")
			rb.WriteString(indent(wrap(s.Text.Render(l.Summary), w), " ") + "\n\n")
		}
		if top := topCommands(r.logs, 5); len(top) > 0 {
			rb.WriteString(" " + s.PanelTitle.Foreground(s.T.Accent).Render(i18n.T("Häufigste Befehle")) + "\n")
			for _, c := range top {
				f := ""
				if c.failed > 0 {
					f = s.Danger.Render(fmt.Sprintf("  ×%d", c.failed))
				}
				rb.WriteString("  " + s.AI.Render(fmt.Sprintf("%3d×", c.n)) + " " + trunc(c.cmd, w-12) + f + "\n")
			}
			rb.WriteString("\n")
		}
		rb.WriteString(" " + s.PanelTitle.Foreground(s.T.Accent).Render(upperFirst(i18n.T("Sitzungen"))) + "\n")
		lastDay := ""
		for _, run := range logRuns(r.logs) {
			d := weekday(run[0].Started) + " " + run[0].Started.Format("02.01.")
			show := d
			if d == lastDay {
				show = ""
			}
			lastDay = d
			n, f := logRow{logs: run}.commands()
			line := "  " + s.Muted.Render(fmt.Sprintf("%-9s", show)) + "  " + fmt.Sprintf("%-11s", timeRange(run))
			sess := ""
			if len(run) > 1 {
				sess = fmt.Sprintf("%d×", len(run))
			}
			line += " " + s.AI.Render(fmt.Sprintf("%3s", sess)) + " " + s.Muted.Render(plural(n, i18n.T("Befehl"), i18n.T("Befehle")))
			if f > 0 {
				line += s.Danger.Render(fmt.Sprintf("  ×%d", f))
			}
			rb.WriteString(line + "\n")
		}
		if len(r.logs) > 1 {
			rb.WriteString("\n " + s.Muted.Render(i18n.T("⏎ klappt die Sitzungen auf, dann einzeln ansehen oder exportieren.")) + "\n")
		}
		return
	}
	// Folgesitzungen an einem Host
	l := r.logs[0]
	from, to := r.span()
	rb.WriteString(" " + s.Accent.Render(l.Host) + "  " + s.Muted.Render(l.Addr) + "   " + keys() + "\n")
	info := from.Format("02.01.2006 15:04") + " – " + to.Format("15:04") + " · " + sessionsText(len(r.logs)) + " · " + stats()
	if l.System != "" {
		info += " · " + l.System
	}
	rb.WriteString(" " + s.Muted.Render(trunc(info, w)) + "\n\n")
	a.logSessions(rb, r.logs, w, false)
}

// logHostList: Hosts eines Tages mit Anzahl.
func (a *App) logHostList(rb *strings.Builder, logs []*sessionlog.Log, w int) {
	s := a.s
	type hc struct {
		host       string
		n, cmd, ko int
	}
	idx := map[string]int{}
	var hs []hc
	for _, l := range logs {
		k := logHostKey(l)
		i, ok := idx[k]
		if !ok {
			i = len(hs)
			idx[k] = i
			hs = append(hs, hc{host: l.Host})
		}
		hs[i].n++
		hs[i].cmd += len(l.Entries)
		hs[i].ko += l.Failures()
	}
	sort.SliceStable(hs, func(i, j int) bool { return hs[i].n > hs[j].n })
	for _, h := range hs {
		line := "  " + s.AI.Render(fmt.Sprintf("%3d×", h.n)) + " " + trunc(h.host, max(8, w-26)) + "  " + s.Muted.Render(plural(h.cmd, i18n.T("Befehl"), i18n.T("Befehle")))
		if h.ko > 0 {
			line += s.Danger.Render(fmt.Sprintf("  ×%d", h.ko))
		}
		rb.WriteString(line + "\n")
	}
}

// logSameCommands: gleicher Befehl auf mehreren Hosts – Ergebnisse als
// Tabelle, Abweichungen zuerst.
func (a *App) logSameCommands(rb *strings.Builder, logs []*sessionlog.Log, cmds []string, w int) {
	s := a.s
	rb.WriteString(" " + s.Muted.Render(i18n.T("Gleicher Befehl auf allen Hosts:")) + "\n")
	for _, c := range cmds {
		rb.WriteString("   " + s.Text.Bold(true).Render(trunc(c, w-4)) + "\n")
	}
	rb.WriteString("\n")
	type res struct {
		l      *sessionlog.Log
		text   string
		failed bool
	}
	var rs []res
	count := map[string]int{}
	for _, l := range logs {
		t, f := lastResult(l)
		rs = append(rs, res{l, t, f})
		count[t]++
	}
	common, best := "", 0
	for t, n := range count {
		if n > best || n == best && t < common {
			common, best = t, n
		}
	}
	sort.SliceStable(rs, func(i, j int) bool {
		di, dj := rs[i].text != common || rs[i].failed, rs[j].text != common || rs[j].failed
		return di && !dj
	})
	hostW := 6
	for _, r := range rs {
		hostW = max(hostW, len([]rune(r.l.Host)))
	}
	hostW = min(hostW, max(8, w/3))
	rb.WriteString(" " + s.Muted.Render(fmt.Sprintf("%-*s  %-8s  %s", hostW, i18n.T("Host"), i18n.T("Zeit"), i18n.T("Ergebnis"))) + "\n")
	for _, r := range rs {
		mark := s.OK.Render("✓")
		txt := s.Text.Render(trunc(r.text, max(8, w-hostW-16)))
		switch {
		case r.failed:
			mark = s.Danger.Render("×")
			txt = s.Danger.Render(trunc(r.text, max(8, w-hostW-16)))
		case r.text != common:
			mark = s.Warn.Render("≠")
			txt = s.Warn.Render(trunc(r.text, max(8, w-hostW-16)))
		}
		rb.WriteString(" " + fmt.Sprintf("%-*s", hostW, trunc(r.l.Host, hostW)) + "  " + s.Muted.Render(r.l.Started.Format("15:04:05")) + "  " + mark + " " + txt + "\n")
	}
}

// logSessions: mehrere Sitzungen nacheinander (älteste zuerst) mit Trennern.
func (a *App) logSessions(rb *strings.Builder, logs []*sessionlog.Log, w int, withHost bool) {
	s := a.s
	for i := len(logs) - 1; i >= 0; i-- {
		l := logs[i]
		label := fmt.Sprintf(i18n.T("Sitzung %d"), len(logs)-i) + " · " + l.Started.Format("15:04") + "–" + logEnd(l).Format("15:04")
		if withHost {
			label = l.Host + " · " + l.Started.Format("15:04")
		}
		bar := "── " + label + " "
		bar += strings.Repeat("─", max(2, min(w, 60)-len([]rune(bar))))
		rb.WriteString(" " + s.Muted.Render(bar) + "\n")
		if len(l.Entries) == 0 {
			rb.WriteString("   " + s.Muted.Render(i18n.T("keine Befehle")) + "\n")
		}
		a.logEntries(rb, l, w, false)
	}
}

// logSingle: eine Sitzung mit allen Befehlen.
func (a *App) logSingle(rb *strings.Builder, l *sessionlog.Log, w int) {
	s := a.s
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
		rb.WriteString(" " + s.PanelTitle.Foreground(s.T.AI).Render(i18n.T("◆ Zusammenfassung")) + "\n")
		rb.WriteString(indent(wrap(s.Text.Render(l.Summary), w), " ") + "\n\n")
	}
	a.logEntries(rb, l, w, true)
}

// logEntries: Befehle einer Sitzung (bei Suche nur die Treffer).
func (a *App) logEntries(rb *strings.Builder, l *sessionlog.Log, w int, note bool) {
	s := a.s
	idx := make([]int, len(l.Entries))
	for i := range idx {
		idx[i] = i
	}
	if q := a.logs.search.Value(); strings.TrimSpace(q) != "" {
		if hits := l.MatchingEntries(q); len(hits) > 0 {
			idx = hits
			if note {
				rb.WriteString(" " + s.Muted.Render(fmt.Sprintf(i18n.T("%d von %d Befehlen passen zur Suche"), len(hits), len(l.Entries))) + "\n\n")
			}
		}
	}
	for _, i := range idx {
		e := l.Entries[i]
		mark := s.Accent.Render("$")
		if e.Failed {
			mark = s.Danger.Render("×")
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

func upperFirst(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

func indent(s, pre string) string {
	l := strings.Split(s, "\n")
	for i := range l {
		l[i] = pre + l[i]
	}
	return strings.Join(l, "\n")
}
