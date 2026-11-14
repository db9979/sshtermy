package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/vuln"
)

// Sicherheitscheck für alle Server eines Ordners (Palette, o im
// Sicherheitscheck, Zeitplan „Sicherheitscheck“): nur lesend und ohne
// Rückfragen. Ergebnis: eine Zeile pro Host, gespeichert je Verbindung
// (F6 und Manager zeigen es).

const vulnParallel = 4

type vulnHost struct {
	conn   domain.Connection
	rep    *vuln.Report
	c      vuln.Counts
	news   int // neu betroffene Pakete seit der letzten Prüfung
	err    string
	done   bool
	system string
}

type vulnFolderState struct {
	seq    int
	folder string
	hosts  []vulnHost
	cancel context.CancelFunc
	start  time.Time
}

type vulnHostMsg struct {
	seq int
	i   int
	rep *vuln.Report
	err error
}

// vulnScanHost: in Tests austauschbar.
var vulnScanHost = func(a *App) func(ctx context.Context, c domain.Connection, src vuln.Source, h *vuln.HTTP, cfg domain.VulnConfig) (*vuln.Report, error) {
	return a.vulnScanHostReal
}

// vulnScanable: nur SSH-Verbindungen, die kein bekannter Switch und kein
// Windows sind.
func vulnScanable(c domain.Connection) bool {
	if c.Protocol != domain.ProtoSSH || isNetConn(c) || c.Device == domain.DeviceNetwork {
		return false
	}
	return c.Profile == nil || (c.Profile.Family != domain.FamilyWindows && c.Profile.Family != domain.FamilyNetwork)
}

func (a *App) openVulnFolder() tea.Cmd {
	if a.st.Settings().Vuln.Mode == "" {
		return a.askVulnMode(func() tea.Cmd { return a.openVulnFolder() })
	}
	count := map[string]int{}
	for _, c := range a.st.Connections() {
		if !vulnScanable(c) {
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
		a.flash(i18n.T("Keine Linux-Server (SSH) für den Sicherheitscheck"))
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
	} else if a.scr == scrVuln {
		cur = a.vscan.conn.Group
	}
	a.pick = newPicker(i18n.T("Sicherheitscheck: alle Server im Ordner"), i18n.T("Ordner suchen …"), items, cur, func(it pickItem) tea.Cmd {
		f := it.value
		if f == "\x00all" {
			f = ""
		}
		return a.runVulnFolder(f)
	})
	a.pick.help = i18n.T("Nur lesend, ohne Rückfragen: Hosts, die ein Passwort oder eine Eingabe bräuchten, werden übersprungen. Switches und Windows-Server zählen nicht mit.")
	return nil
}

func (a *App) vulnFolderConns(folder string) []domain.Connection {
	var out []domain.Connection
	for _, c := range a.st.Connections() {
		if vulnScanable(c) && domain.InFolder(c.Group, folder) {
			out = append(out, c)
		}
	}
	return out
}

func (a *App) runVulnFolder(folder string) tea.Cmd {
	f := &a.vfold
	if f.cancel != nil {
		f.cancel()
	}
	conns := a.vulnFolderConns(folder)
	if len(conns) == 0 {
		a.flash(i18n.T("Keine Linux-Server (SSH) für den Sicherheitscheck"))
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	hosts := make([]vulnHost, len(conns))
	for i, c := range conns {
		hosts[i] = vulnHost{conn: c}
	}
	*f = vulnFolderState{seq: f.seq + 1, folder: folder, hosts: hosts, cancel: cancel, start: time.Now()}
	seq, ev := f.seq, a.events
	cfg := a.st.Settings().Vuln
	src, h := vulnSources(a)
	scan := vulnScanHost(a)
	go func() {
		defer cancel()
		vulnHosts(ctx, conns, func(ctx context.Context, c domain.Connection) (*vuln.Report, error) {
			return scan(ctx, c, src, h, cfg)
		}, func(i int, r *vuln.Report, err error) {
			ev <- vulnHostMsg{seq: seq, i: i, rep: r, err: err}
		})
	}()
	a.showVulnFolder()
	return nil
}

// vulnHosts prüft die Verbindungen, vulnParallel gleichzeitig.
func vulnHosts(ctx context.Context, conns []domain.Connection, scan func(context.Context, domain.Connection) (*vuln.Report, error), done func(int, *vuln.Report, error)) {
	sem := make(chan struct{}, vulnParallel)
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
			hctx, hcancel := context.WithTimeout(ctx, 15*time.Minute)
			r, err := scan(hctx, c)
			hcancel()
			done(i, r, err)
		}()
	}
	wg.Wait()
}

// vulnScanHostReal: ohne Rückfragen verbinden, Pakete lesen, prüfen.
func (a *App) vulnScanHostReal(ctx context.Context, c domain.Connection, src vuln.Source, h *vuln.HTTP, cfg domain.VulnConfig) (*vuln.Report, error) {
	ex, err := a.quietDialer()(ctx, c)
	if err != nil {
		return nil, err
	}
	defer ex.Close()
	if ex.NetDevice() {
		return nil, fmt.Errorf("%s", i18n.T("Netzwerkgerät: OSV kennt keine Hersteller-Firmware"))
	}
	rctx, rcancel := context.WithTimeout(ctx, 3*time.Minute)
	res, err := ex.Exec(rctx, "sh -c "+shQuote(vuln.Script(!cfg.NoContainers)))
	rcancel()
	if err != nil && len(res.Output) == 0 {
		return nil, err
	}
	inv := vuln.ParseInventory(string(res.Output))
	return scanInventory(ctx, inv, src, h, cfg.Mode, func(string) {})
}

func (a *App) vulnFolderMsg(msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(vulnHostMsg)
	if !ok {
		return nil, false
	}
	f := &a.vfold
	if m.seq != f.seq || m.i >= len(f.hosts) {
		return a.listen(), true
	}
	a.finishVulnHost(&f.hosts[m.i], m.rep, m.err)
	if f.allDone() {
		a.flashOK(fmt.Sprintf(i18n.T("Sicherheitscheck fertig: %d Hosts in %s"), len(f.hosts), durText(time.Since(f.start))))
	}
	if a.modal != nil && a.modal.kind == mText && a.modal.vuln {
		a.showVulnFolder()
	}
	return a.listen(), true
}

// finishVulnHost: Ergebnis übernehmen, mit dem letzten Stand vergleichen,
// speichern.
func (a *App) finishVulnHost(h *vulnHost, rep *vuln.Report, err error) {
	h.done = true
	if err != nil {
		h.err = vulnErr(err).Error()
		return
	}
	prev := a.storedVuln(h.conn.ID)
	skip := a.vulnSkip(h.conn)
	for k := range vulnNew(prev, rep) {
		for _, f := range rep.Findings {
			if f.Key() == k && !skip(f) && !(f.Minor && !f.KEV) && (f.KEV || f.Level() >= vuln.LevelHigh) {
				h.news++
				break
			}
		}
	}
	if prev == nil {
		h.news = 0
	}
	h.rep, h.c, h.system = rep, rep.Count(skip), rep.System
	a.saveVuln(h.conn.ID, rep)
	if a.vscan.conn.ID == h.conn.ID && !a.vscan.busy {
		a.vscan.prev, a.vscan.rep = a.vscan.rep, rep
	}
}

func (f *vulnFolderState) allDone() bool {
	for _, h := range f.hosts {
		if !h.done {
			return false
		}
	}
	return true
}

// vulnFolderLines: Inhalt des Ergebnisfensters (und Markdown).
func (a *App) vulnFolderLines(f *vulnFolderState) (lines []string, md string) {
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
	fmt.Fprintf(&b, i18n.T("## Sicherheitscheck %s, %s\n\n"), where, time.Now().Format("02.01.2006 15:04"))
	if done < len(f.hosts) {
		lines = append(lines, fmt.Sprintf(i18n.T("läuft: %d von %d Hosts fertig …"), done, len(f.hosts)), "")
	}
	lines = append(lines, fmt.Sprintf("%-22s %-18s %4s %5s %5s %6s %4s  %s", i18n.T("Host"), i18n.T("System"), "KEV", "KRIT", "HOCH", "MITTEL", i18n.T("neu"), i18n.T("Wichtigstes Paket")))
	b.WriteString("| Host | System | KEV | " + i18n.T("kritisch") + " | " + i18n.T("hoch") + " | " + i18n.T("mittel") + " | " + i18n.T("neu") + " | " + i18n.T("Wichtigstes Paket") + " |\n|---|---|---|---|---|---|---|---|\n")
	hs := append([]vulnHost(nil), f.hosts...)
	sortVulnHosts(hs)
	for _, h := range hs {
		name := trunc(h.conn.Alias, 22)
		switch {
		case !h.done:
			lines = append(lines, fmt.Sprintf("%-22s %s", name, i18n.T("…")))
		case h.err != "":
			lines = append(lines, fmt.Sprintf("%-22s × %s", name, trunc(h.err, 90)))
			fmt.Fprintf(&b, "| %s | | | | | | | × %s |\n", h.conn.Alias, h.err)
		default:
			top := i18n.T("✓ keine bekannten Lücken")
			skip := a.vulnSkip(h.conn)
			for _, x := range h.rep.Findings {
				if !skip(x) && !(x.Minor && !x.KEV) {
					top = fmt.Sprintf("%s %s", vulnLevelLabel(x.Level()), x.Package)
					if x.KEV {
						top += " KEV"
					}
					if x.Container != "" {
						top += " (" + x.Container + ")"
					}
					break
				}
			}
			news := "–"
			if h.news > 0 {
				news = fmt.Sprint(h.news)
			}
			c := h.c
			lines = append(lines, fmt.Sprintf("%-22s %-18s %4d %5d %5d %6d %4s  %s", name, trunc(h.system, 18), c.KEV, c.Critical, c.High, c.Medium, news, trunc(top, 60)))
			fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %d | %s | %s |\n", h.conn.Alias, h.system, c.KEV, c.Critical, c.High, c.Medium, news, top)
		}
	}
	lines = append(lines, "", i18n.T("Einzelheiten: Tab des Hosts öffnen, F6 › s. Gezählt werden Pakete (nicht einzelne Lücken), ohne Ausgeblendetes."))
	return lines, b.String()
}

func (a *App) showVulnFolder() {
	f := &a.vfold
	if len(f.hosts) == 0 {
		a.flash(i18n.T("Noch kein Sicherheitscheck über einen Ordner gelaufen"))
		return
	}
	lines, md := a.vulnFolderLines(f)
	where := f.folder
	if where == "" {
		where = i18n.T("alle Verbindungen")
	}
	scroll := 0
	if a.modal != nil && a.modal.kind == mText && a.modal.vuln {
		scroll = a.modal.scroll
	}
	a.modal = textModal(i18n.T("Sicherheitscheck: ")+where, lines)
	a.modal.copy, a.modal.scroll, a.modal.vuln = md, scroll, true
}

// ---------------------------------------------------------------- Zeitplan

type schedVulnMsg struct {
	id     string
	folder string
	conns  []domain.Connection
	reps   []*vuln.Report
	errs   []error
	start  time.Time
}

// startScheduleVuln: Sicherheitscheck nach Zeitplan (ohne Rückfragen).
func (a *App) startScheduleVuln(sc domain.Schedule, conns []domain.Connection) tea.Cmd {
	cfg := a.st.Settings().Vuln
	if cfg.Mode == "" {
		cfg.Mode = domain.VulnOnline
	}
	src, h := vulnSources(a)
	scan := vulnScanHost(a)
	a.flash(i18n.T("Zeitplan startet: ") + sc.Name)
	id, folder := sc.ID, sc.Folder
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
		defer cancel()
		m := schedVulnMsg{id: id, folder: folder, conns: conns, start: time.Now(), reps: make([]*vuln.Report, len(conns)), errs: make([]error, len(conns))}
		var mu sync.Mutex
		vulnHosts(ctx, conns, func(ctx context.Context, c domain.Connection) (*vuln.Report, error) {
			return scan(ctx, c, src, h, cfg)
		}, func(i int, r *vuln.Report, err error) {
			mu.Lock()
			m.reps[i], m.errs[i] = r, err
			mu.Unlock()
		})
		return m
	}
}

func (a *App) schedVulnMsg(msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(schedVulnMsg)
	if !ok {
		return nil, false
	}
	delete(a.schedRun, m.id)
	if a.vfold.cancel != nil {
		a.vfold.cancel()
	}
	f := vulnFolderState{seq: a.vfold.seq + 1, folder: m.folder, start: m.start}
	okN, failN, news, kev := 0, 0, 0, 0
	for i, c := range m.conns {
		h := vulnHost{conn: c}
		a.finishVulnHost(&h, m.reps[i], m.errs[i])
		if h.err != "" {
			failN++
		} else {
			okN++
			news += h.news
			kev += h.c.KEV
		}
		f.hosts = append(f.hosts, h)
	}
	a.vfold = f
	lines, _ := a.vulnFolderLines(&a.vfold)
	text := strings.Join(lines, "\n")
	if len(text) > 6000 {
		text = text[:6000] + "\n…"
	}
	a.updateSchedule(m.id, func(x *domain.Schedule) { x.LastOK, x.LastFail, x.LastText = okN, failN, text })
	name := i18n.T("Sicherheitscheck (OSV)")
	if sc, ok := a.scheduleByID(m.id); ok {
		name = sc.Name
	}
	msgText := fmt.Sprintf(i18n.T("Zeitplan „%s“: Sicherheitscheck fertig, %d Hosts, %d neu betroffene Pakete, %d aktiv ausgenutzt, %d Fehler"), name, okN+failN, news, kev, failN)
	switch {
	case failN > 0:
		a.flashErr(msgText)
	case news > 0 || kev > 0:
		a.pushToast(toastWarn, msgText)
	default:
		a.flashOK(msgText)
	}
	// Desktop-Meldung nur, wenn es etwas Neues gibt (sonst jede Nacht dieselbe)
	if news > 0 || failN > 0 {
		a.schedDesktop(name, fmt.Sprintf(i18n.T("%d neu betroffene Pakete (hoch/kritisch), %d Fehler · Palette: Sicherheitscheck: letztes Ergebnis"), news, failN))
	}
	return nil, true
}
