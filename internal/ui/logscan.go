package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/logscan"
)

// Log-Blick (Alt+E, F6 l, Palette „logs“)
//
// sshTermy liest die Logs des Systems, auf dem der Tab gerade ist, selbst
// und nur lesend: Linux/BSD über eine eigene Exec-Sitzung (journalctl bzw.
// Syslog-Dateien), Windows über eine zweite WinRS-Shell (Get-WinEvent),
// Switches und Firewalls sichtbar am Prompt (show logging …), weil Exec dort
// tabu ist. Gleiche Meldungen werden lokal zu Mustern zusammengefasst und
// nach Schwere geordnet. Die KI sieht erst etwas, wenn man k oder b drückt –
// und dann nur die verdichteten Muster.

type lsView int

const (
	lsList lsView = iota // Auffälligkeiten
	lsNews               // Was ist neu? + ausgeblendetes Rauschen
	lsTime               // Zeitleiste (logtime.go)
)

// logScanState: der offene Log-Blick.
type logScanState struct {
	sid     int
	conn    domain.Connection
	back    screen
	busy    bool
	step    string
	err     string
	seq     int
	cancel  context.CancelFunc
	rep     *logscan.Report
	gone    []logscan.Gone
	prev    *logscan.Base // Grundlinie beim Öffnen (bleibt beim Neuladen)
	prevOK  bool          // Grundlinie schon gelesen
	sel     int
	top     int
	view    lsView
	hours   int
	newsSel int
	focus   string     // Muster, das nach dem Lesen gewählt wird (Logwache)
	tl      timeCursor // Zeitleiste
}

type logScanMsg struct {
	seq  int
	rep  *logscan.Report
	err  error
	page bool // Netzwerkgerät: Blättern ist jetzt aus
	vend hostinfo.Vendor
}

var lsHours = []int{1, 6, 24, 72, 168}

// logScanOK: kann der Tab überhaupt Logs liefern?
func (a *App) logScanOK(s *session) error {
	switch {
	case s == nil || s.state != stConnected:
		return errors.New(i18n.T("Log-Blick braucht eine verbundene Sitzung"))
	case s.isWinRM():
		return nil
	case s.netDevice():
		if s.shell == nil || s.term == nil {
			return errors.New(i18n.T("Keine Shell zum Gerät"))
		}
		return nil
	case s.client != nil && a.hostServerOK(s):
		return nil
	case s.client != nil && s.prof.Family == domain.FamilyWindows:
		return errors.New(i18n.T("Windows über SSH: bitte als PowerShell-Verbindung (WinRM) anlegen, dann geht der Log-Blick"))
	}
	return errors.New(i18n.T("Log-Blick gibt es für Linux/BSD-Server (SSH), Windows (PowerShell/WinRM), Switches und Firewalls"))
}

// openLogScan öffnet den Log-Blick für die Sitzung und liest sofort.
func (a *App) openLogScan(s *session) tea.Cmd {
	if err := a.logScanOK(s); err != nil {
		a.flash(err.Error())
		return nil
	}
	back := a.scr
	if back == scrLogScan {
		back = a.lscan.back
	}
	if a.lscan.sid != s.id || a.lscan.rep == nil {
		a.lscan.stop()
		a.lscan = logScanState{sid: s.id, conn: s.conn, back: back, hours: a.st.Settings().LogScan.LogHours()}
	}
	a.lscan.back = back
	a.scr = scrLogScan
	a.lsApplyFocus()
	if a.lscan.rep == nil && !a.lscan.busy {
		return a.runLogScan(s)
	}
	return nil
}

func (l *logScanState) stop() {
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
}

// runLogScan liest die Logs im Hintergrund.
func (a *App) runLogScan(s *session) tea.Cmd {
	l := &a.lscan
	if err := a.logScanOK(s); err != nil {
		l.err = err.Error()
		return nil
	}
	if s.netDevice() && (s.host.busy || s.term.AltScreen() || !hostinfo.IsPrompt(strings.TrimSpace(s.term.CursorLine()))) {
		l.err = i18n.T("Abfrage nur an einem leeren Prompt (z. B. switch#). Bitte erst Eingabe beenden oder ⏎ drücken.")
		return nil
	}
	if !l.prevOK {
		l.prevOK = true
		if raw := a.st.LogBase(s.conn.ID); len(raw) > 0 {
			var b logscan.Base
			if json.Unmarshal(raw, &b) == nil {
				l.prev = &b
			}
		}
	}
	l.stop()
	l.seq++
	l.busy, l.err = true, ""
	hours := l.hours
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	l.cancel = cancel
	seq := l.seq
	start := time.Now()
	switch {
	case s.isWinRM():
		l.step = i18n.T("liest Ereignisprotokolle über WinRM …")
		return func() tea.Msg {
			defer cancel()
			out, err := a.winrmRun(ctx, s, logscan.WindowsScript(hours))
			if err != nil && strings.TrimSpace(out) == "" {
				return logScanMsg{seq: seq, err: err}
			}
			r := logscan.ParseWindows(out, hours, time.Now())
			r.Took = time.Since(start)
			return logScanMsg{seq: seq, rep: r}
		}
	case s.netDevice():
		l.step = i18n.T("tippt die show-Befehle sichtbar im Terminal …")
		vendor, pagingOff := s.host.vendor, s.host.pagingOff
		if vendor == hostinfo.VendorUnknown {
			vendor = hostVendor(s.prof)
		}
		prompt := strings.TrimSpace(s.term.CursorLine())
		return func() tea.Msg {
			defer cancel()
			run := func(cmd string, limit time.Duration) (string, error) {
				lines, err := shellRun(ctx, s, prompt, cmd, limit)
				return strings.Join(lines, "\n"), err
			}
			if p := vendor.PagingOff(); p != "" && !pagingOff {
				if _, err := run(p, 10*time.Second); err != nil {
					return logScanMsg{seq: seq, err: err}
				}
				pagingOff = true
			}
			logs, portCmd := logscan.NetCommands(vendor)
			var outs []string
			for _, c := range logs {
				out, err := run(c, 2*time.Minute)
				if err != nil && out == "" {
					return logScanMsg{seq: seq, err: err, page: pagingOff, vend: vendor}
				}
				outs = append(outs, out)
			}
			var ports string
			if portCmd != "" {
				ports, _ = run(portCmd, 30*time.Second)
			}
			r := logscan.ParseNetwork(vendor, outs, ports, hours, time.Now())
			r.Took = time.Since(start)
			return logScanMsg{seq: seq, rep: r, page: pagingOff, vend: vendor}
		}
	}
	l.step = i18n.T("liest journalctl bzw. Syslog …")
	cl := s.client
	return func() tea.Msg {
		defer cancel()
		out, err := cl.RunScript(ctx, logscan.UnixScript(hours))
		if err != nil && strings.TrimSpace(out) == "" {
			return logScanMsg{seq: seq, err: err}
		}
		r := logscan.ParseUnix(out, hours, time.Now())
		r.Took = time.Since(start)
		return logScanMsg{seq: seq, rep: r}
	}
}

// logScanMsg: Ergebnis übernehmen, mit der Grundlinie vergleichen und die
// neue Grundlinie speichern.
func (a *App) logScanResult(msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(logScanMsg)
	if !ok {
		return nil, false
	}
	l := &a.lscan
	if m.seq != l.seq {
		return nil, true
	}
	l.busy, l.cancel = false, nil
	s := a.byID(l.sid)
	if s != nil && s.netDevice() {
		if m.page {
			s.host.pagingOff = true
		}
		if m.vend != hostinfo.VendorUnknown && s.host.vendor == hostinfo.VendorUnknown {
			s.host.vendor = m.vend
		}
	}
	if m.err != nil {
		l.err = m.err.Error()
		return nil, true
	}
	r := m.rep
	r.Host = l.conn.Alias
	if s != nil {
		r.System = s.prof.OS
		if r.System == "" {
			r.System = s.prof.Model
		}
		a.addOwnCommands(s, r)
	}
	if es := a.lwatch.syslog[l.conn.ID]; r.Kind == logscan.KindNetwork && len(es) > 0 {
		if n := r.MergeSyslog(es); n > 0 {
			r.Read = append(r.Read, fmt.Sprintf(i18n.T("Syslog-Empfang: %d Zeilen"), n))
		}
	}
	logscan.ApplyNoise(r, a.noiseFor(l.conn))
	l.gone = logscan.Compare(r, l.prev)
	l.rep = r
	l.sel, l.top = 0, 0
	a.lsApplyFocus()
	l.tl = timeCursor{}
	if raw, err := json.Marshal(r.Base()); err == nil {
		_ = a.st.SetLogBase(l.conn.ID, raw)
	}
	delete(a.lwatch.base, l.conn.ID) // Logwache liest die neue Grundlinie
	return nil, true
}

// noiseFor: welche Muster auf dieser Verbindung ausgeblendet sind.
func (a *App) noiseFor(c domain.Connection) func(string) bool {
	list := a.st.Settings().LogScan.Noise
	now := time.Now()
	return func(key string) bool {
		for _, n := range list {
			if n.Hides(key, c, now) {
				return true
			}
		}
		return false
	}
}

// addNoise: Muster als Rauschen ausblenden (Host oder Ordner).
func (a *App) addNoise(f logscan.Finding, folder bool) { a.addNoiseFor(a.lscan.conn, f, folder) }

// lsApplyFocus wählt das vorgemerkte Muster (aus der Logwache) in der Liste.
func (a *App) lsApplyFocus() {
	l := &a.lscan
	if l.focus == "" || l.rep == nil {
		return
	}
	for i, f := range a.lsVisible() {
		if f.Key == l.focus {
			l.view, l.sel = lsList, i
			l.top = max(0, i-3)
			break
		}
	}
	l.focus = ""
}

func (a *App) addNoiseFor(c domain.Connection, f logscan.Finding, folder bool) {
	n := domain.LogNoise{Key: f.Key, Title: f.Title, Conn: c.ID}
	where := c.Alias
	if folder {
		n.Conn = ""
		n.Folder = c.Group
		where = i18n.T("überall")
		if n.Folder != "" {
			where = i18n.T("Ordner ") + n.Folder
		}
	}
	_ = a.st.UpdateSettings(func(s *domain.Settings) {
		out := s.LogScan.Noise[:0]
		for _, x := range s.LogScan.Noise {
			if !(x.Key == n.Key && x.Conn == n.Conn && x.Folder == n.Folder) {
				out = append(out, x)
			}
		}
		s.LogScan.Noise = append(out, n)
		if len(s.LogScan.Noise) > 300 {
			s.LogScan.Noise = s.LogScan.Noise[len(s.LogScan.Noise)-300:]
		}
	})
	a.flash(fmt.Sprintf(i18n.T("Als Rauschen ausgeblendet (%s): %s · n zeigt es, u holt es zurück"), where, trunc(f.Title, 50)))
	a.reapplyNoise()
}

// dropNoise: Ausblendung für ein Muster auf dieser Verbindung aufheben.
func (a *App) dropNoise(key string) {
	l := &a.lscan
	_ = a.st.UpdateSettings(func(s *domain.Settings) {
		out := s.LogScan.Noise[:0]
		for _, x := range s.LogScan.Noise {
			if !(x.Key == key && x.Hides(key, l.conn, time.Now())) {
				out = append(out, x)
			}
		}
		s.LogScan.Noise = out
	})
	a.flash(i18n.T("Wird wieder angezeigt"))
	a.reapplyNoise()
}

func (a *App) reapplyNoise() {
	l := &a.lscan
	if l.rep == nil {
		return
	}
	logscan.ApplyNoise(l.rep, a.noiseFor(l.conn))
	l.sel = clamp(l.sel, 0, max(0, len(a.lsVisible())-1))
}

// lsVisible: Muster der Liste (ohne Rauschen).
func (a *App) lsVisible() []logscan.Finding {
	if a.lscan.rep == nil {
		return nil
	}
	var out []logscan.Finding
	for _, f := range a.lscan.rep.Findings {
		if !f.Noise {
			out = append(out, f)
		}
	}
	return out
}

// lsNoise: ausgeblendete Muster.
func (a *App) lsNoise() []logscan.Finding {
	if a.lscan.rep == nil {
		return nil
	}
	var out []logscan.Finding
	for _, f := range a.lscan.rep.Findings {
		if f.Noise {
			out = append(out, f)
		}
	}
	return out
}

func (a *App) lsCurrent() (logscan.Finding, bool) {
	vis := a.lsVisible()
	if len(vis) == 0 {
		return logscan.Finding{}, false
	}
	return vis[clamp(a.lscan.sel, 0, len(vis)-1)], true
}

// ---------------------------------------------------------------- Tasten

func (a *App) updateLogScan(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		if mw, ok := msg.(tea.MouseWheelMsg); ok {
			if mw.Button == tea.MouseWheelUp {
				a.lsMove(-3)
			} else if mw.Button == tea.MouseWheelDown {
				a.lsMove(3)
			}
		}
		return nil
	}
	l := &a.lscan
	s := a.byID(l.sid)
	key := k.String()
	if key == "esc" || key == "q" {
		if l.view != lsList {
			l.view = lsList
			return nil
		}
		l.stop()
		l.busy = false
		a.scr = l.back
		if a.scr == scrLogScan || a.scr == scrUnlock {
			a.scr = scrTerminal
		}
		return nil
	}
	if a.keys.is(actLogScan, key) {
		a.scr = l.back
		return nil
	}
	if l.view == lsTime {
		if cmd, done := a.timeKey(s, key); done {
			return cmd
		}
	}
	switch key {
	case "r":
		if s == nil {
			l.err = i18n.T("Der Tab ist geschlossen")
			return nil
		}
		if !l.busy {
			return a.runLogScan(s)
		}
	case "x":
		if l.busy {
			l.stop()
			l.busy, l.err = false, i18n.T("Abgebrochen.")
			return nil
		}
		if l.view == lsNews {
			return nil
		}
		if f, ok := a.lsCurrent(); ok {
			a.addNoise(f, false)
		}
	case "X":
		if f, ok := a.lsCurrent(); ok && l.view == lsList {
			a.addNoise(f, true)
		}
	case "u":
		if l.view == lsNews {
			if ns := a.lsNoise(); len(ns) > 0 {
				a.dropNoise(ns[clamp(l.newsSel, 0, len(ns)-1)].Key)
			}
		}
	case "z":
		if s == nil || l.busy {
			return nil
		}
		i := 0
		for j, h := range lsHours {
			if h == l.hours {
				i = j
			}
		}
		l.hours = lsHours[(i+1)%len(lsHours)]
		h := l.hours
		_ = a.st.UpdateSettings(func(x *domain.Settings) { x.LogScan.Hours = h })
		return a.runLogScan(s)
	case "n":
		if l.view == lsNews {
			l.view = lsList
		} else {
			l.view, l.newsSel = lsNews, 0
		}
	case "t":
		if l.rep != nil {
			if l.view == lsTime {
				l.view = lsList
			} else {
				l.view = lsTime
				a.timeOpen()
			}
		}
	case "up":
		if l.view == lsNews {
			l.newsSel = max(0, l.newsSel-1)
		} else {
			a.lsMove(-1)
		}
	case "down":
		if l.view == lsNews {
			l.newsSel = min(max(0, len(a.lsNoise())-1), l.newsSel+1)
		} else {
			a.lsMove(1)
		}
	case "pgup":
		a.lsMove(-10)
	case "pgdown":
		a.lsMove(10)
	case "home":
		l.sel = 0
	case "end":
		l.sel = max(0, len(a.lsVisible())-1)
	case "enter":
		if f, ok := a.lsCurrent(); ok && l.view == lsList {
			a.showFindingLines(f)
		}
	case "1", "2", "3", "4":
		if f, ok := a.lsCurrent(); ok && l.view == lsList {
			i := int(key[0] - '1')
			if i < len(f.Checks) && s != nil {
				a.scr = scrTerminal
				return a.insertCommand(s, f.Checks[i])
			}
		}
	case "k":
		if f, ok := a.lsCurrent(); ok && s != nil {
			return a.explainFinding(s, f)
		}
	case "b", "K":
		if s != nil && l.rep != nil {
			return a.logBrief(s)
		}
	case "w":
		return a.openWatchList()
	case "s":
		if s != nil && s.netDevice() {
			return a.askSyslogSetup(s)
		}
	case "c":
		if l.rep != nil {
			txt := a.lsMarkdown()
			a.flash(i18n.T("Log-Blick als Text kopiert"))
			return tea.SetClipboard(txt)
		}
	}
	return nil
}

func (a *App) lsMove(d int) {
	l := &a.lscan
	l.sel = clamp(l.sel+d, 0, max(0, len(a.lsVisible())-1))
}

// showFindingLines: alle Zeilen eines Musters (neueste zuerst).
func (a *App) showFindingLines(f logscan.Finding) {
	es := a.lscan.rep.LinesOf(f.Key)
	lines := make([]string, 0, len(es)+2)
	for i := len(es) - 1; i >= 0; i-- {
		lines = append(lines, es[i].Line())
	}
	if len(lines) == 0 {
		lines = append(lines, f.Sample)
	}
	a.modal = textModal(fmt.Sprintf("%s · %d×", trunc(f.Title, 60), f.Count), lines)
}

// explainFinding: die KI erklärt ein Muster (seine Zeilen, höchstens 80).
func (a *App) explainFinding(s *session, f logscan.Finding) tea.Cmd {
	if !a.st.LLM().SendScreen {
		a.flash(i18n.T("Terminal-Inhalt an die KI ist in den KI-Einstellungen ausgeschaltet"))
		return nil
	}
	es := a.lscan.rep.LinesOf(f.Key)
	if len(es) > 80 {
		es = es[len(es)-80:]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%d×, %s)\n", f.Title, f.Count, f.Sev.Label())
	for _, e := range es {
		b.WriteString(e.Line() + "\n")
	}
	if len(es) == 0 {
		b.WriteString(f.Sample + "\n")
	}
	a.scr = scrTerminal
	return a.diagnoseWith(s, b.String(), i18n.T("Muster aus dem Log-Blick: ")+trunc(f.Title, 50))
}

// logBrief: KI-Lagebericht aus den verdichteten Mustern.
func (a *App) logBrief(s *session) tea.Cmd {
	if !a.st.LLM().SendScreen {
		a.flash(i18n.T("Terminal-Inhalt an die KI ist in den KI-Einstellungen ausgeschaltet"))
		return nil
	}
	brief := a.lscan.rep.Brief(25)
	a.scr = scrTerminal
	a.panelOpened(withPanel(s, panelAsk))
	a.resize(s)
	q := &s.ask
	ag, err := a.agentClientFor(s.conn)
	if err != nil {
		q.err = err.Error()
		return nil
	}
	ctx, cancel := q.begin(fmt.Sprintf(i18n.T("KI-Lagebericht aus dem Log-Blick (%d Muster, %s)"), min(25, len(a.lsVisible())), sizeText(len(brief))))
	q.history = nil
	q.input.SetValue("")
	sid, seq, ev := s.id, q.seq, a.events
	c := a.agentContext(s, false)
	return func() tea.Msg {
		defer cancel()
		err := ag.LogBrief(ctx, brief, c, func(d string) {
			if d != "" {
				ev <- askDeltaMsg{sid, seq, d}
			}
		})
		return askDoneMsg{sid, seq, err}
	}
}

func sizeText(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f kB", float64(n)/1024)
}

// lsMarkdown: Ergebnis als Text (Ticket, Doku).
func (a *App) lsMarkdown() string {
	r := a.lscan.rep
	var b strings.Builder
	crit, errs, warn := r.Counts()
	fmt.Fprintf(&b, i18n.T("## Log-Blick %s (%s), letzte %d h, %s\n\n"), r.Host, r.System, r.Hours, r.At.Local().Format("02.01.2006 15:04"))
	fmt.Fprintf(&b, i18n.T("%d kritisch, %d Fehler, %d Warnungen · %d Zeilen gelesen\n\n"), crit, errs, warn, r.Lines)
	for _, f := range a.lsVisible() {
		fmt.Fprintf(&b, "- **%s** %d× %s", f.Sev.Label(), f.Count, f.Title)
		if !f.Last.IsZero() {
			fmt.Fprintf(&b, i18n.T(" (zuletzt %s)"), f.Last.Local().Format("02.01. 15:04"))
		}
		b.WriteString("\n")
	}
	return b.String()
}
