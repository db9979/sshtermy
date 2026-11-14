package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/logscan"
	"github.com/db9979/sshtermy/internal/psremote"
)

// Ordner-Scan: Log-Blick für alle Hosts eines Ordners auf einmal, nur
// lesend und ohne Rückfragen (wie Zeitpläne). Ergebnis: eine Zeile pro Host
// und die Muster, die auf mehreren Hosts gleichzeitig auftauchen.

const folderScanParallel = 6

type folderHost struct {
	conn domain.Connection
	rep  *logscan.Report
	news int // neue Muster gegenüber der letzten Grundlinie
	err  string
	done bool
}

type folderScanState struct {
	seq    int
	folder string
	hosts  []folderHost
	cancel context.CancelFunc
	start  time.Time
	open   bool // Ergebnisfenster gehört dem Ordner-Scan
}

type folderHostMsg struct {
	seq int
	i   int
	rep *logscan.Report
	err error
}

// folderScanDial: in Tests austauschbar.
var folderScanDial = func(a *App) func(ctx context.Context, c domain.Connection, hours int) (*logscan.Report, error) {
	return a.folderScanHost
}

// openFolderScan: Ordner wählen (vorbelegt mit dem Ordner unter dem Cursor).
func (a *App) openFolderScan() tea.Cmd {
	count := map[string]int{}
	for _, c := range a.st.Connections() {
		if !folderScanable(c) {
			continue
		}
		count[""]++
		p := c.Group
		for p != "" {
			count[p]++
			i := strings.LastIndex(p, domain.PathSep)
			if i < 0 {
				break
			}
			p = p[:i]
		}
	}
	if count[""] == 0 {
		a.flash(i18n.T("Keine Verbindungen für den Ordner-Scan (SSH, PowerShell)"))
		return nil
	}
	var paths []string
	for p := range count {
		if p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	items := []pickItem{{value: "\x00all", label: i18n.T("Alle Verbindungen"), hint: fmt.Sprint(count[""])}}
	for _, p := range paths {
		items = append(items, pickItem{value: p, label: p, hint: fmt.Sprint(count[p]), indent: strings.Count(p, domain.PathSep)})
	}
	cur := ""
	if a.scr == scrManager {
		if r, ok := a.mgrCur(); ok {
			if r.isFolder() {
				cur = r.node.Path
			} else {
				cur = r.conn.Group
			}
		}
	}
	a.pick = newPicker(i18n.T("Ordner-Scan: Logs aller Hosts im Ordner"), i18n.T("Ordner suchen …"), items, cur, func(it pickItem) tea.Cmd {
		f := it.value
		if f == "\x00all" {
			f = ""
		}
		return a.runFolderScan(f)
	})
	return nil
}

// folderScanable: Verbindungen, die der Scan erreicht.
func folderScanable(c domain.Connection) bool {
	switch c.Protocol {
	case domain.ProtoSSH, domain.ProtoWinRM:
		return true
	}
	return false
}

func (a *App) runFolderScan(folder string) tea.Cmd {
	f := &a.lfold
	if f.cancel != nil {
		f.cancel()
	}
	var hosts []folderHost
	for _, c := range a.st.Connections() {
		if folderScanable(c) && domain.InFolder(c.Group, folder) {
			hosts = append(hosts, folderHost{conn: c})
		}
	}
	if len(hosts) == 0 {
		a.flash(i18n.T("Keine Verbindungen für den Ordner-Scan (SSH, PowerShell)"))
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	f.seq++
	*f = folderScanState{seq: f.seq, folder: folder, hosts: hosts, cancel: cancel, start: time.Now(), open: true}
	seq := f.seq
	hours := a.st.Settings().LogScan.LogHours()
	scan := folderScanDial(a)
	ev := a.events
	conns := make([]domain.Connection, len(hosts))
	for i, h := range hosts {
		conns[i] = h.conn
	}
	go func() {
		defer cancel()
		scanHosts(ctx, conns, hours, scan, func(i int, r *logscan.Report, err error) {
			ev <- folderHostMsg{seq: seq, i: i, rep: r, err: err}
		})
	}()
	a.showFolderScan()
	return nil
}

// scanHosts liest die Logs der Verbindungen, folderScanParallel gleichzeitig.
func scanHosts(ctx context.Context, conns []domain.Connection, hours int,
	scan func(context.Context, domain.Connection, int) (*logscan.Report, error), done func(int, *logscan.Report, error)) {
	sem := make(chan struct{}, folderScanParallel)
	var wg sync.WaitGroup
	for i, c := range conns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				done(i, nil, ctx.Err())
				return
			}
			defer func() { <-sem }()
			hctx, hcancel := context.WithTimeout(ctx, 3*time.Minute)
			r, err := scan(hctx, c, hours)
			hcancel()
			done(i, r, err)
		}()
	}
	wg.Wait()
}

// folderScanHost liest die Logs eines Hosts ohne Rückfragen.
func (a *App) folderScanHost(ctx context.Context, c domain.Connection, hours int) (*logscan.Report, error) {
	if c.Protocol == domain.ProtoWinRM {
		pw, err := winrmPassword(c, func(label string) (string, error) {
			return "", fmt.Errorf(i18n.T("braucht eine Eingabe (%s), der Ordner-Scan fragt nie"), label)
		}, func(string) {})
		if err != nil {
			return nil, err
		}
		out, err := psremote.Run(ctx, winrmConfig(c, pw), logscan.WindowsScript(hours))
		if err != nil && strings.TrimSpace(out) == "" {
			return nil, winrmError(err, c)
		}
		return logscan.ParseWindows(out, hours, time.Now()), nil
	}
	if c.Profile != nil && c.Profile.Family == domain.FamilyWindows {
		return nil, fmt.Errorf("%s", i18n.T("Windows über SSH: bitte als PowerShell-Verbindung (WinRM) anlegen, dann geht der Log-Blick"))
	}
	ex, err := a.quietDialer()(ctx, c)
	if err != nil {
		return nil, err
	}
	defer ex.Close()
	if ex.NetDevice() || c.Device == domain.DeviceNetwork || (c.Profile != nil && c.Profile.Family == domain.FamilyNetwork) {
		v := hostinfo.VendorUnknown
		if c.Profile != nil {
			v = hostVendor(*c.Profile)
		}
		logs, ports := logscan.NetCommands(v)
		var cmds []string
		if p := v.PagingOff(); p != "" {
			cmds = append(cmds, p)
		}
		cmds = append(cmds, logs...)
		if ports != "" {
			cmds = append(cmds, ports)
		}
		res, err := ex.RunInShell(ctx, cmds)
		if err != nil && len(res.Output) == 0 {
			return nil, err
		}
		return logscan.ParseNetwork(v, []string{string(res.Output)}, string(res.Output), hours, time.Now()), nil
	}
	res, err := ex.Exec(ctx, "sh -c "+shQuote(logscan.UnixScript(hours)))
	if err != nil && len(res.Output) == 0 {
		return nil, err
	}
	return logscan.ParseUnix(string(res.Output), hours, time.Now()), nil
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (a *App) folderScanMsg(msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(folderHostMsg)
	if !ok {
		return nil, false
	}
	f := &a.lfold
	if m.seq != f.seq || m.i >= len(f.hosts) {
		return a.listen(), true
	}
	a.finishFolderHost(&f.hosts[m.i], m.rep, m.err)
	if f.allDone() {
		a.flashOK(fmt.Sprintf(i18n.T("Ordner-Scan fertig: %d Hosts in %s"), len(f.hosts), durText(time.Since(f.start))))
	}
	if f.open && a.modal != nil && a.modal.kind == mText && a.modal.copy != "" && !a.modal.vuln {
		a.showFolderScan()
	}
	return a.listen(), true
}

// finishFolderHost: Ergebnis eines Hosts übernehmen (Rauschen, Vergleich mit
// der Grundlinie, neue Grundlinie speichern).
func (a *App) finishFolderHost(h *folderHost, rep *logscan.Report, err error) {
	h.done = true
	if err != nil {
		h.err = err.Error()
	} else if rep != nil {
		r := rep
		r.Host = h.conn.Alias
		logscan.ApplyNoise(r, a.noiseFor(h.conn))
		var prev *logscan.Base
		if raw := a.st.LogBase(h.conn.ID); len(raw) > 0 {
			var b logscan.Base
			if json.Unmarshal(raw, &b) == nil {
				prev = &b
			}
		}
		logscan.Compare(r, prev)
		if prev != nil {
			for _, x := range r.Findings {
				if x.Status == "new" && !x.Noise {
					h.news++
				}
			}
		}
		if raw, err := json.Marshal(r.Base()); err == nil {
			_ = a.st.SetLogBase(h.conn.ID, raw)
			delete(a.lwatch.base, h.conn.ID)
		}
		h.rep = r
	}
}

func (f *folderScanState) allDone() bool {
	for _, h := range f.hosts {
		if !h.done {
			return false
		}
	}
	return true
}

// folderCommon: Muster, die auf mindestens zwei Hosts vorkommen.
type folderCommonRow struct {
	key   string
	title string
	sev   logscan.Sev
	hosts []string
	count int
}

func (f *folderScanState) common() []folderCommonRow {
	m := map[string]*folderCommonRow{}
	for _, h := range f.hosts {
		if h.rep == nil {
			continue
		}
		for _, x := range h.rep.Findings {
			if x.Noise || x.Sev < logscan.SevWarn {
				continue
			}
			r := m[x.Key]
			if r == nil {
				r = &folderCommonRow{key: x.Key, title: x.Title, sev: x.Sev}
				m[x.Key] = r
			}
			r.hosts = append(r.hosts, h.conn.Alias)
			r.count += x.Count
			r.sev = max(r.sev, x.Sev)
		}
	}
	var out []folderCommonRow
	for _, r := range m {
		if len(r.hosts) >= 2 {
			out = append(out, *r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].hosts) != len(out[j].hosts) {
			return len(out[i].hosts) > len(out[j].hosts)
		}
		if out[i].sev != out[j].sev {
			return out[i].sev > out[j].sev
		}
		return out[i].key < out[j].key
	})
	return out
}

// folderLines: Inhalt des Ergebnisfensters (und Markdown zum Kopieren).
func (a *App) folderLines() (lines []string, md string) {
	f := &a.lfold
	var b strings.Builder
	where := f.folder
	if where == "" {
		where = i18n.T("alle Verbindungen")
	}
	done := 0
	for _, h := range f.hosts {
		if h.done {
			done++
		}
	}
	fmt.Fprintf(&b, i18n.T("## Ordner-Scan %s, letzte %s, %s\n\n"), where, hoursText(a.st.Settings().LogScan.LogHours()), time.Now().Format("02.01.2006 15:04"))
	if done < len(f.hosts) {
		lines = append(lines, fmt.Sprintf(i18n.T("läuft: %d von %d Hosts fertig …"), done, len(f.hosts)), "")
	}
	lines = append(lines, fmt.Sprintf("%-22s %5s %5s %5s %4s  %s", i18n.T("Host"), "KRIT", "FEHL", "WARN", i18n.T("neu"), i18n.T("Wichtigstes Muster")))
	b.WriteString("| Host | " + i18n.T("kritisch") + " | " + i18n.T("Fehler") + " | " + i18n.T("Warnungen") + " | " + i18n.T("neu") + " | " + i18n.T("Wichtigstes Muster") + " |\n|---|---|---|---|---|---|\n")
	hs := append([]folderHost(nil), f.hosts...)
	sort.SliceStable(hs, func(i, j int) bool { return folderRank(hs[i]) > folderRank(hs[j]) })
	for _, h := range hs {
		name := trunc(h.conn.Alias, 22)
		switch {
		case !h.done:
			lines = append(lines, fmt.Sprintf("%-22s %s", name, i18n.T("…")))
		case h.err != "":
			lines = append(lines, fmt.Sprintf("%-22s × %s", name, trunc(h.err, 90)))
			fmt.Fprintf(&b, "| %s | | | | | × %s |\n", h.conn.Alias, h.err)
		default:
			crit, errs, warn := h.rep.Counts()
			top := i18n.T("✓ ruhig")
			for _, x := range h.rep.Findings {
				if !x.Noise && x.Sev >= logscan.SevWarn {
					top = fmt.Sprintf("%s (%d×)", x.Title, x.Count)
					break
				}
			}
			news := "–"
			if h.news > 0 {
				news = fmt.Sprint(h.news)
			}
			lines = append(lines, fmt.Sprintf("%-22s %5d %5d %5d %4s  %s", name, crit, errs, warn, news, trunc(top, 80)))
			fmt.Fprintf(&b, "| %s | %d | %d | %d | %s | %s |\n", h.conn.Alias, crit, errs, warn, news, top)
		}
	}
	if cs := f.common(); len(cs) > 0 {
		lines = append(lines, "", i18n.T("Gemeinsam (auf mehreren Hosts):"))
		b.WriteString("\n### " + i18n.T("Gemeinsam (auf mehreren Hosts):") + "\n\n")
		for i, c := range cs {
			if i >= 15 {
				break
			}
			hosts := strings.Join(c.hosts, ", ")
			lines = append(lines, fmt.Sprintf("  %-4s %d Hosts · %s · %s", c.sev.Label(), len(c.hosts), trunc(c.title, 60), trunc(hosts, 60)))
			fmt.Fprintf(&b, "- **%s** %s (%d×): %s\n", c.sev.Label(), c.title, c.count, hosts)
		}
	}
	lines = append(lines, "", i18n.T("Einzelheiten: Tab des Hosts öffnen, Alt+E. Grundlinie für „Was ist neu?“ ist gespeichert."))
	return lines, b.String()
}

func folderRank(h folderHost) int {
	if !h.done {
		return -1
	}
	if h.err != "" {
		return 0
	}
	c, e, w := h.rep.Counts()
	return 1 + c*1000000 + e*1000 + w + h.news*100000
}

func (a *App) showFolderScan() {
	f := &a.lfold
	if len(f.hosts) == 0 {
		a.flash(i18n.T("Noch kein Ordner-Scan gelaufen"))
		return
	}
	lines, md := a.folderLines()
	where := f.folder
	if where == "" {
		where = i18n.T("alle Verbindungen")
	}
	scroll := 0
	if a.modal != nil && a.modal.kind == mText && a.modal.copy != "" {
		scroll = a.modal.scroll
	}
	a.modal = textModal(i18n.T("Ordner-Scan: ")+where, lines)
	a.modal.copy, a.modal.scroll = md, scroll
	f.open = true
}

// ---------------------------------------------------------------- Zeitplan

type schedLogsMsg struct {
	id     string
	folder string
	hosts  []folderHost
	reps   []*logscan.Report
	errs   []error
	start  time.Time
}

// startScheduleLogs: Ordner-Scan nach Zeitplan (ohne Rückfragen).
func (a *App) startScheduleLogs(sc domain.Schedule, conns []domain.Connection) tea.Cmd {
	scan := folderScanDial(a)
	hours := a.st.Settings().LogScan.LogHours()
	a.flash(i18n.T("Zeitplan startet: ") + sc.Name)
	id, folder := sc.ID, sc.Folder
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		m := schedLogsMsg{id: id, folder: folder, start: time.Now(), reps: make([]*logscan.Report, len(conns)), errs: make([]error, len(conns))}
		var mu sync.Mutex
		scanHosts(ctx, conns, hours, scan, func(i int, r *logscan.Report, err error) {
			mu.Lock()
			m.reps[i], m.errs[i] = r, err
			mu.Unlock()
		})
		for _, c := range conns {
			m.hosts = append(m.hosts, folderHost{conn: c})
		}
		return m
	}
}

func (a *App) schedLogsMsg(msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(schedLogsMsg)
	if !ok {
		return nil, false
	}
	delete(a.schedRun, m.id)
	if a.lfold.cancel != nil {
		a.lfold.cancel()
	}
	a.lfold = folderScanState{seq: a.lfold.seq + 1, folder: m.folder, hosts: m.hosts, start: m.start}
	okN, failN, news := 0, 0, 0
	for i := range a.lfold.hosts {
		a.finishFolderHost(&a.lfold.hosts[i], m.reps[i], m.errs[i])
		if a.lfold.hosts[i].err != "" {
			failN++
		} else {
			okN++
			news += a.lfold.hosts[i].news
		}
	}
	lines, _ := a.folderLines()
	text := strings.Join(lines, "\n")
	if len(text) > 6000 {
		text = text[:6000] + "\n…"
	}
	a.updateSchedule(m.id, func(x *domain.Schedule) { x.LastOK, x.LastFail, x.LastText = okN, failN, text })
	name := i18n.T("Ordner-Scan (Logs)")
	if sc, ok := a.scheduleByID(m.id); ok {
		name = sc.Name
	}
	msgText := fmt.Sprintf(i18n.T("Zeitplan „%s“: Ordner-Scan fertig, %d Hosts, %d neue Muster, %d Fehler"), name, okN+failN, news, failN)
	switch {
	case failN > 0:
		a.flashErr(msgText)
	case news > 0:
		a.pushToast(toastWarn, msgText)
	default:
		a.flashOK(msgText)
	}
	a.schedDesktop(name, fmt.Sprintf(i18n.T("%d neue Muster, %d Fehler · Palette: Ordner-Scan: letztes Ergebnis"), news, failN))
	return nil, true
}
