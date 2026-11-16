package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/audit"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/logscan"
	"github.com/db9979/sshtermy/internal/patchrun"
	"github.com/db9979/sshtermy/internal/sshx"
	"github.com/db9979/sshtermy/internal/vuln"
)

// Patch-Runde (Palette „Patch-Runde“): Updates gestaffelt über einen
// Ordner. Erst ein Host (Kanarienvogel), dann nach Freigabe (w) der Rest,
// einer nach dem anderen. Vor und nach dem Update: laufende Dienste,
// lauschende Ports, fehlgeschlagene Units, neue Fehler im Log. Neustart nur,
// wenn gewählt und nötig, mit Warten auf Rückkehr. Bei Auffälligkeiten hält
// die Runde an. Starten erst nach doppelter Bestätigung (Gefahr-Stufe).

type patchConn interface {
	ExecInput(ctx context.Context, cmd string, in []byte) (sshx.RunResult, error)
	Close() error
}

// patchDial: ohne Rückfragen verbinden (in Tests austauschbar).
var patchDial = func(a *App) func(ctx context.Context, c domain.Connection) (patchConn, error) {
	hk, st := a.hostKeys, a.st
	relayDial := a.relayDialer()
	return func(ctx context.Context, c domain.Connection) (patchConn, error) {
		opts := sshx.Options{
			HostKeys: hk,
			Resolve:  st.Connection,
			Relay:    relayDial,
			Prompt:   func(string, string, string) bool { return false },
			AskSecret: func(label string) (string, error) {
				return "", fmt.Errorf(i18n.T("braucht eine Eingabe (%s), die Patch-Runde meldet sich ohne Rückfrage an"), label)
			},
		}
		cl, err := sshx.Dial(ctx, c, opts)
		if err != nil {
			return nil, err
		}
		return cl, nil
	}
}

// Wartezeiten beim Neustart (in Tests kürzer).
var (
	patchRebootWait = 20 * time.Second // erst nach dieser Zeit wieder anklopfen
	patchPoll       = 10 * time.Second
	patchBackMax    = 15 * time.Minute
	patchSettle     = 30 * time.Second // nach dem Update/Neustart: Dienste hochkommen lassen
)

type patchJob struct {
	conn   domain.Connection
	scope  patchrun.Scope
	pkgs   []string
	reboot bool
	noise  func(string) bool
	pws    []string // Kandidaten fürs sudo-Passwort (gespeichert)
}

type patchHost struct {
	conn     domain.Connection
	phase    string
	done     bool
	err      string
	skipped  bool
	before   patchrun.State
	after    patchrun.State
	diff     patchrun.Diff
	upgraded int
	tail     string
	newLogs  []string
	rebooted bool
	reboot   bool // nach der Runde noch Neustart nötig
	start    time.Time
	end      time.Time
}

func (h patchHost) problems() int {
	if h.err != "" {
		return 1
	}
	return h.diff.Problems() + len(h.newLogs)
}

type patchState struct {
	seq     int
	folder  string
	scope   patchrun.Scope
	reboot  bool
	hosts   []patchHost
	jobs    []patchJob
	next    int  // nächster Host
	running bool // ein Host läuft gerade
	paused  bool // wartet auf w
	why     string
	done    bool
	ctx     context.Context
	cancel  context.CancelFunc
	start   time.Time
}

type patchHostMsg struct {
	seq   int
	i     int
	h     patchHost
	final bool
}

// ---------------------------------------------------------------- Start

func (a *App) openPatch() tea.Cmd {
	if a.patch.running {
		a.showPatch()
		return nil
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
		a.flash(i18n.T("Keine Linux-Server (SSH) für eine Patch-Runde"))
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
	a.pick = newPicker(i18n.T("Patch-Runde: Ordner wählen"), i18n.T("Ordner suchen …"), items, cur, func(it pickItem) tea.Cmd {
		f := it.value
		if f == "\x00all" {
			f = ""
		}
		return a.patchAskScope(f)
	})
	a.pick.help = i18n.T("Updates gestaffelt: erst ein Host, dann nach deiner Freigabe der Rest, einer nach dem anderen. Vorher und nachher prüft sshTermy Dienste, Ports und Logs. Braucht root oder sudo.")
	return nil
}

func (a *App) patchAskScope(folder string) tea.Cmd {
	conns := a.vulnFolderConns(folder)
	if len(conns) == 0 {
		a.flash(i18n.T("Keine Linux-Server (SSH) für eine Patch-Runde"))
		return nil
	}
	withCheck := 0
	for _, c := range conns {
		if len(a.patchPkgs(c.ID)) > 0 {
			withCheck++
		}
	}
	items := []pickItem{{value: "all", label: i18n.T("Alle verfügbaren Updates"), hint: fmt.Sprint(len(conns))}}
	if withCheck > 0 {
		items = append(items, pickItem{value: "sec", label: i18n.T("Nur Pakete mit Lücken aus dem Sicherheitscheck"), hint: fmt.Sprint(withCheck)})
	}
	a.pick = newPicker(i18n.T("Patch-Runde: was einspielen?"), "", items, "", func(it pickItem) tea.Cmd {
		scope := patchrun.ScopeAll
		if it.value == "sec" {
			scope = patchrun.ScopeSecurity
		}
		return a.patchAskReboot(folder, scope)
	})
	a.pick.help = i18n.T("„Nur Pakete mit Lücken“ nimmt je Host die Pakete aus dem letzten Sicherheitscheck, für die es ein Update gibt. Hosts ohne Sicherheitscheck werden dann übersprungen.")
	return nil
}

func (a *App) patchAskReboot(folder string, scope patchrun.Scope) tea.Cmd {
	items := []pickItem{
		{value: "yes", label: i18n.T("Neu starten, wenn nötig (einzeln, mit Warten auf Rückkehr)")},
		{value: "no", label: i18n.T("Nicht neu starten, nur melden")},
	}
	a.pick = newPicker(i18n.T("Patch-Runde: Neustart?"), "", items, "", func(it pickItem) tea.Cmd {
		return a.patchAskCanary(folder, scope, it.value == "yes")
	})
	return nil
}

func (a *App) patchAskCanary(folder string, scope patchrun.Scope, reboot bool) tea.Cmd {
	conns := a.vulnFolderConns(folder)
	sort.SliceStable(conns, func(i, j int) bool { return patchCanaryRank(conns[i]) < patchCanaryRank(conns[j]) })
	var items []pickItem
	for _, c := range conns {
		hint := c.Group
		if domain.IsProduction(c) {
			hint = strings.TrimSpace(hint + " · " + i18n.T("geschützt"))
		}
		items = append(items, pickItem{value: c.ID, label: c.Alias, hint: hint})
	}
	a.pick = newPicker(i18n.T("Patch-Runde: welcher Host zuerst?"), i18n.T("Host suchen …"), items, "", func(it pickItem) tea.Cmd {
		return a.patchConfirm(folder, scope, reboot, it.value)
	})
	a.pick.help = i18n.T("Der erste Host (Kanarienvogel) bekommt die Updates allein. Danach hält die Runde an und du entscheidest mit w, ob der Rest folgt. Am besten ein Test- oder unwichtiger Host.")
	return nil
}

// patchCanaryRank: Test-Hosts zuerst vorschlagen.
func patchCanaryRank(c domain.Connection) int {
	s := strings.ToLower(c.Alias + " " + strings.Join(c.Tags, " "))
	for _, w := range []string{"canary", "kanari", "test", "dev", "staging", "stage"} {
		if strings.Contains(s, w) {
			return 0
		}
	}
	if !domain.IsProduction(c) {
		return 1
	}
	return 2
}

func (a *App) patchConfirm(folder string, scope patchrun.Scope, reboot bool, canary string) tea.Cmd {
	conns := a.vulnFolderConns(folder)
	sort.SliceStable(conns, func(i, j int) bool {
		if (conns[i].ID == canary) != (conns[j].ID == canary) {
			return conns[i].ID == canary
		}
		return strings.ToLower(conns[i].Alias) < strings.ToLower(conns[j].Alias)
	})
	prot := 0
	var names []string
	for _, c := range conns {
		if domain.IsProduction(c) {
			prot++
		}
		names = append(names, c.Alias)
	}
	what := i18n.T("alle verfügbaren Updates")
	if scope == patchrun.ScopeSecurity {
		what = i18n.T("nur Pakete mit Lücken aus dem Sicherheitscheck")
	}
	rb := i18n.T("kein Neustart")
	if reboot {
		rb = i18n.T("Neustart, wenn nötig")
	}
	where := folder
	if where == "" {
		where = i18n.T("alle Verbindungen")
	}
	body := fmt.Sprintf(i18n.T("Ordner: %s\nHosts (%d): %s\nErster Host: %s\nEinspielen: %s · %s"), where, len(conns), trunc(strings.Join(names, ", "), 300), conns[0].Alias, what, rb)
	if prot > 0 {
		body += "\n" + fmt.Sprintf(i18n.T("▲ %d davon sind als produktiv geschützt."), prot)
	}
	body += "\n\n" + i18n.T("sshTermy führt die Updates mit root-Rechten aus. Nach dem ersten Host hält die Runde an.") + "\n" + i18n.T("j = starten · n = abbrechen")
	m := confirmModal(i18n.T("Patch-Runde starten?"), body, func() tea.Cmd { return a.startPatch(folder, conns, scope, reboot) })
	m.twice = true
	a.modal = m
	return nil
}

// patchPkgs: Binärpakete mit Update aus dem letzten Sicherheitscheck.
func (a *App) patchPkgs(connID string) []string {
	raw := a.st.VulnReport(connID)
	if len(raw) == 0 {
		return nil
	}
	var r vuln.Report
	if json.Unmarshal(raw, &r) != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, f := range r.Findings {
		if f.Container != "" || f.Fixed == "" {
			continue
		}
		for _, b := range f.Binaries {
			if !seen[b] {
				seen[b] = true
				out = append(out, b)
			}
		}
	}
	sort.Strings(out)
	return out
}

func (a *App) startPatch(folder string, conns []domain.Connection, scope patchrun.Scope, reboot bool) tea.Cmd {
	p := &a.patch
	if p.cancel != nil {
		p.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Hour)
	*p = patchState{seq: p.seq + 1, folder: folder, scope: scope, reboot: reboot, ctx: ctx, cancel: cancel, start: time.Now()}
	for _, c := range conns {
		j := patchJob{conn: c, scope: scope, reboot: reboot, noise: a.noiseFor(c)}
		if scope == patchrun.ScopeSecurity {
			j.pkgs = a.patchPkgs(c.ID)
		}
		if c.Password.IsSet() {
			j.pws = append(j.pws, c.Password.Reveal())
		}
		if c.Keychain {
			j.pws = append(j.pws, sshx.KeychainSecret(c.ID, sshx.SecretSudo), sshx.KeychainSecret(c.ID, sshx.SecretPassword))
		}
		p.jobs = append(p.jobs, j)
		p.hosts = append(p.hosts, patchHost{conn: c, phase: i18n.T("wartet")})
	}
	a.audit.Add(audit.EvBroadcast, orDefault(folder, "*"), fmt.Sprintf("Patch-Runde: %d Hosts", len(conns)))
	cmd := a.patchNext()
	a.showPatch()
	return cmd
}

// patchNext startet den nächsten Host im Hintergrund.
func (a *App) patchNext() tea.Cmd {
	p := &a.patch
	if p.next >= len(p.jobs) {
		p.done, p.running = true, false
		a.patchFinished()
		return nil
	}
	i := p.next
	p.next++
	p.running, p.paused, p.why = true, false, ""
	job, seq, ev, ctx := p.jobs[i], p.seq, a.events, p.ctx
	run := patchHostRun(a)
	go func() {
		h := run(ctx, job, func(h patchHost) { ev <- patchHostMsg{seq: seq, i: i, h: h} })
		ev <- patchHostMsg{seq: seq, i: i, h: h, final: true}
	}()
	return nil
}

func (a *App) patchMsgs(msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(patchHostMsg)
	if !ok {
		return nil, false
	}
	p := &a.patch
	if m.seq != p.seq || m.i >= len(p.hosts) {
		return a.listen(), true
	}
	p.hosts[m.i] = m.h
	var cmd tea.Cmd
	if m.final {
		p.running = false
		h := m.h
		switch {
		case p.ctx.Err() != nil:
			p.done = true
		case m.i == 0 && len(p.jobs) > 1:
			p.paused = true
			if h.problems() > 0 {
				p.why = i18n.T("Erster Host mit Auffälligkeiten. Bitte ansehen, bevor es weitergeht.")
				a.flashErr(i18n.T("Patch-Runde: erster Host mit Auffälligkeiten"))
			} else {
				p.why = i18n.T("Erster Host ist durch, ohne Auffälligkeiten.")
				a.flashOK(i18n.T("Patch-Runde: erster Host fertig, w setzt fort"))
			}
			a.schedDesktop(i18n.T("Patch-Runde"), p.why)
		case h.problems() > 0 && p.next < len(p.jobs):
			p.paused = true
			p.why = fmt.Sprintf(i18n.T("%s mit Auffälligkeiten. Runde angehalten."), h.conn.Alias)
			a.flashErr(i18n.T("Patch-Runde angehalten: ") + h.conn.Alias)
			a.schedDesktop(i18n.T("Patch-Runde"), p.why)
		default:
			cmd = a.patchNext()
		}
	}
	if a.modal != nil && a.modal.patch {
		a.showPatch()
	}
	if cmd != nil {
		return tea.Batch(cmd, a.listen()), true
	}
	return a.listen(), true
}

func (a *App) patchFinished() {
	p := &a.patch
	okN, probN := 0, 0
	for _, h := range p.hosts {
		if h.skipped {
			continue
		}
		if h.problems() > 0 {
			probN++
		} else if h.done {
			okN++
		}
	}
	txt := fmt.Sprintf(i18n.T("Patch-Runde fertig: %d ohne Auffälligkeiten, %d mit"), okN, probN)
	if probN > 0 {
		a.flashErr(txt)
	} else {
		a.flashOK(txt)
	}
	a.schedDesktop(i18n.T("Patch-Runde"), txt)
}

// patchKey: Tasten im Fenster der Runde. true = verarbeitet.
func (a *App) patchKey(key string) (tea.Cmd, bool) {
	p := &a.patch
	switch key {
	case "w":
		if p.paused && !p.done {
			cmd := a.patchNext()
			a.showPatch()
			return cmd, true
		}
	case "x":
		// Nie mitten im Update abbrechen: der laufende Host wird fertig.
		if !p.done {
			for i := p.next; i < len(p.hosts); i++ {
				p.hosts[i].skipped, p.hosts[i].phase = true, i18n.T("nicht gestartet")
			}
			p.next = len(p.jobs)
			if !p.running {
				p.paused, p.done = false, true
				p.why = i18n.T("Runde beendet.")
			}
			a.showPatch()
			return nil, true
		}
	}
	return nil, false
}

// ---------------------------------------------------------------- ein Host

// patchHostRun: einen Host aktualisieren (in Tests austauschbar).
var patchHostRun = func(a *App) func(ctx context.Context, j patchJob, upd func(patchHost)) patchHost {
	dial := patchDial(a)
	ask := a.events
	return func(ctx context.Context, j patchJob, upd func(patchHost)) patchHost {
		return runPatchHost(ctx, j, dial, func(label string) string {
			r := make(chan string, 1)
			select {
			case ask <- secretAskMsg{label, r}:
			case <-ctx.Done():
				return ""
			}
			select {
			case pw := <-r:
				if pw == "\x00cancel" {
					return ""
				}
				return pw
			case <-ctx.Done():
				return ""
			}
		}, upd)
	}
}

func runPatchHost(ctx context.Context, j patchJob, dial func(context.Context, domain.Connection) (patchConn, error),
	askPw func(string) string, upd func(patchHost)) (h patchHost) {
	h = patchHost{conn: j.conn, start: time.Now()}
	phase := func(s string) {
		h.phase = s
		upd(h)
	}
	fail := func(err error) patchHost {
		h.err, h.done, h.end = strings.TrimSpace(err.Error()), true, time.Now()
		h.phase = i18n.T("Fehler")
		return h
	}
	if j.scope == patchrun.ScopeSecurity && len(j.pkgs) == 0 {
		h.skipped, h.done, h.end = true, true, time.Now()
		h.phase = i18n.T("übersprungen: kein Sicherheitscheck mit Updates")
		return h
	}
	phase(i18n.T("verbindet …"))
	cctx, ccancel := context.WithTimeout(ctx, 45*time.Second)
	cl, err := dial(cctx, j.conn)
	ccancel()
	if err != nil {
		return fail(err)
	}
	defer func() {
		if cl != nil {
			cl.Close()
		}
	}()
	run := func(cl patchConn, script string, d time.Duration) (string, int, error) {
		rctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		res, err := cl.ExecInput(rctx, "sh -c "+sshx.Quote(script), nil)
		return string(res.Output), res.Exit, err
	}

	phase(i18n.T("liest Zustand …"))
	out, _, err := run(cl, patchrun.StateScript, 2*time.Minute)
	if err != nil && out == "" {
		return fail(err)
	}
	h.before = patchrun.ParseState(out)
	if h.before.Manager == patchrun.MgrNone {
		return fail(errors.New(i18n.T("keine bekannte Paketverwaltung (apt, dnf, yum, zypper, apk)")))
	}
	before := map[string]bool{}
	if out, _, err := run(cl, logscan.UnixScript(24), 2*time.Minute); err == nil || out != "" {
		for _, f := range logscan.ParseUnix(out, 24, time.Now()).Findings {
			before[f.Key] = true
		}
	}

	// root: direkt, sonst sudo (ohne Passwort, gespeichertes, sonst fragen)
	asRoot := func(cl patchConn, script string, d time.Duration) (string, int, error) {
		return run(cl, script, d)
	}
	if !h.before.Root {
		res, err := cl.ExecInput(ctx, "LC_ALL=C sudo -n true", nil)
		if err != nil {
			return fail(err)
		}
		var pw string
		if res.Exit != 0 {
			ok := false
			tried := map[string]bool{}
			try := func(p string) bool {
				if p == "" || tried[p] {
					return false
				}
				tried[p] = true
				r, err := cl.ExecInput(ctx, "LC_ALL=C sudo -S -p '' true", []byte(p+"\n"))
				return err == nil && r.Exit == 0
			}
			for _, p := range j.pws {
				if try(p) {
					pw, ok = p, true
					break
				}
			}
			if !ok && askPw != nil {
				if p := askPw(fmt.Sprintf(i18n.T("sudo-Passwort für %s@%s (Patch-Runde)"), j.conn.Username, j.conn.Host)); try(p) {
					pw, ok = p, true
				}
			}
			if !ok {
				return fail(errors.New(i18n.T("sudo braucht ein Passwort, das nicht stimmt oder fehlt")))
			}
		}
		asRoot = func(cl patchConn, script string, d time.Duration) (string, int, error) {
			rctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()
			cmd, in := "LC_ALL=C sudo -n sh -c "+sshx.Quote(script), []byte(nil)
			if pw != "" {
				cmd, in = "LC_ALL=C sudo -S -p '' sh -c "+sshx.Quote(script), []byte(pw+"\n")
			}
			res, err := cl.ExecInput(rctx, cmd, in)
			return string(res.Output), res.Exit, err
		}
	}

	phase(i18n.T("spielt Updates ein …"))
	script := patchrun.UpdateScript(h.before.Manager, j.scope, j.pkgs)
	if script == "" {
		return fail(errors.New(i18n.T("kein Update-Befehl für diese Paketverwaltung")))
	}
	out, exit, err := asRoot(cl, script, 45*time.Minute)
	h.tail = patchrun.Tail(out, 15)
	h.upgraded = patchrun.Upgraded(h.before.Manager, out)
	if err != nil && out == "" {
		return fail(err)
	}
	if exit != 0 {
		h.err = fmt.Sprintf(i18n.T("Update-Befehl endete mit %d"), exit)
	}

	sleep := func(d time.Duration) bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(d):
			return true
		}
	}

	phase(i18n.T("prüft …"))
	out, _, _ = run(cl, patchrun.StateScript, 2*time.Minute)
	mid := patchrun.ParseState(out)
	if mid.Reboot && j.reboot && h.err == "" {
		phase(i18n.T("startet neu …"))
		_, _, _ = asRoot(cl, patchrun.RebootScript, 30*time.Second)
		cl.Close()
		cl = nil
		if !sleep(patchRebootWait) {
			return fail(ctx.Err())
		}
		phase(i18n.T("wartet auf Rückkehr …"))
		deadline := time.Now().Add(patchBackMax)
		for {
			dctx, dcancel := context.WithTimeout(ctx, 20*time.Second)
			c2, err := dial(dctx, j.conn)
			dcancel()
			if err == nil {
				o, _, _ := run(c2, patchrun.StateScript, time.Minute)
				st := patchrun.ParseState(o)
				if st.Boot != "" && st.Boot != h.before.Boot {
					cl = c2
					break
				}
				c2.Close()
			}
			if time.Now().After(deadline) {
				return fail(fmt.Errorf(i18n.T("nach %s nicht zurückgekommen"), durText(patchBackMax)))
			}
			if !sleep(patchPoll) {
				return fail(ctx.Err())
			}
		}
	}
	phase(i18n.T("lässt Dienste hochkommen …"))
	if !sleep(patchSettle) {
		return fail(ctx.Err())
	}
	phase(i18n.T("prüft …"))
	out, _, err = run(cl, patchrun.StateScript, 2*time.Minute)
	if err != nil && out == "" {
		return fail(err)
	}
	h.after = patchrun.ParseState(out)
	h.diff = patchrun.Compare(h.before, h.after)
	h.rebooted, h.reboot = h.diff.Rebooted, h.after.Reboot
	if out, _, err := run(cl, logscan.UnixScript(1), 2*time.Minute); err == nil || out != "" {
		r := logscan.ParseUnix(out, 1, time.Now())
		if j.noise != nil {
			logscan.ApplyNoise(r, j.noise)
		}
		for _, f := range r.Findings {
			if f.Sev >= logscan.SevErr && !f.Noise && !before[f.Key] {
				h.newLogs = append(h.newLogs, f.Title)
			}
		}
	}
	h.done, h.end = true, time.Now()
	h.phase = i18n.T("fertig")
	return h
}

// ---------------------------------------------------------------- Anzeige

func (a *App) showPatch() {
	p := &a.patch
	if len(p.hosts) == 0 {
		a.flash(i18n.T("Noch keine Patch-Runde gelaufen"))
		return
	}
	lines, md := a.patchLines()
	where := p.folder
	if where == "" {
		where = i18n.T("alle Verbindungen")
	}
	scroll := 0
	if a.modal != nil && a.modal.kind == mText && a.modal.patch {
		scroll = a.modal.scroll
	}
	a.modal = textModal(i18n.T("Patch-Runde: ")+where, lines)
	a.modal.copy, a.modal.scroll, a.modal.patch = md, scroll, true
}

func (a *App) patchLines() (lines []string, md string) {
	p := &a.patch
	var b strings.Builder
	what := i18n.T("alle Updates")
	if p.scope == patchrun.ScopeSecurity {
		what = i18n.T("nur Pakete aus dem Sicherheitscheck")
	}
	rb := i18n.T("ohne Neustart")
	if p.reboot {
		rb = i18n.T("Neustart, wenn nötig")
	}
	head := fmt.Sprintf("%s · %s · %s %s", what, rb, i18n.T("seit"), p.start.Format("15:04"))
	fmt.Fprintf(&b, "# %s %s\n\n%s\n\n", i18n.T("Patch-Runde"), orDefault(p.folder, i18n.T("alle Verbindungen")), head)
	lines = append(lines, head)
	switch {
	case p.done:
		lines = append(lines, i18n.T("Runde beendet."))
	case p.paused:
		rest := len(p.jobs) - p.next
		lines = append(lines, "▶ "+p.why, fmt.Sprintf(i18n.T("w = weiter mit %d Hosts · x = Runde beenden · Esc = Fenster zu (Runde wartet)"), rest))
	case p.running:
		lines = append(lines, spinGlyph(time.Now())+" "+i18n.T("läuft · x = nach diesem Host beenden · Esc = Fenster zu (läuft weiter)"))
	}
	lines = append(lines, "", fmt.Sprintf("%-22s %-26s %7s %-9s %s", "Host", i18n.T("Stand"), i18n.T("Pakete"), i18n.T("Neustart"), i18n.T("Auffälligkeiten")))
	b.WriteString(i18n.T("| Host | Stand | Pakete | Neustart | Auffälligkeiten |\n|---|---|---|---|---|\n"))
	for i, h := range p.hosts {
		name := h.conn.Alias
		if i == 0 && len(p.hosts) > 1 {
			name += " (1)"
		}
		pk, rbt, prob := "", "", ""
		if h.done && !h.skipped {
			pk = fmt.Sprint(h.upgraded)
			switch {
			case h.rebooted:
				rbt = i18n.T("erfolgt")
			case h.reboot:
				rbt = i18n.T("nötig")
			default:
				rbt = "–"
			}
			if n := h.problems(); n > 0 {
				prob = fmt.Sprintf("▲ %d", n)
				if h.err != "" {
					prob = "× " + trunc(h.err, 40)
				}
			} else {
				prob = "✓"
			}
		}
		lines = append(lines, fmt.Sprintf("%-22s %-26s %7s %-9s %s", trunc(name, 22), trunc(h.phase, 26), pk, rbt, prob))
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", h.conn.Alias, h.phase, pk, rbt, prob)
	}
	// Einzelheiten zu Hosts mit Auffälligkeiten
	for _, h := range p.hosts {
		if h.problems() == 0 || !h.done {
			continue
		}
		lines = append(lines, "", "▲ "+h.conn.Alias)
		fmt.Fprintf(&b, "\n## %s\n\n", h.conn.Alias)
		add := func(label string, xs []string) {
			if len(xs) == 0 {
				return
			}
			l := label + ": " + strings.Join(xs, ", ")
			lines = append(lines, "  "+trunc(l, 200))
			fmt.Fprintf(&b, "- %s\n", l)
		}
		if h.err != "" {
			add(i18n.T("Fehler"), []string{h.err})
		}
		add(i18n.T("Dienste gestoppt"), h.diff.StoppedServices)
		add(i18n.T("Units fehlgeschlagen"), h.diff.NewFailed)
		add(i18n.T("Ports nicht mehr offen"), h.diff.ClosedPorts)
		add(i18n.T("Neue Fehler im Log"), h.newLogs)
		if h.tail != "" && h.err != "" {
			lines = append(lines, i18n.T("  Letzte Ausgabe:"))
			for _, l := range strings.Split(h.tail, "\n") {
				lines = append(lines, "    "+trunc(l, 160))
			}
			fmt.Fprintf(&b, "\n```\n%s\n```\n", h.tail)
		}
	}
	lines = append(lines, "", i18n.T("(1) = erster Host. Verglichen werden laufende Dienste, offene Ports, fehlgeschlagene Units und neue Fehler im Log (letzte Stunde gegen die 24 h davor). c kopiert den Bericht."))
	return lines, b.String()
}
