package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/vuln"
)

// Sicherheitscheck (F6 s, Palette „Sicherheitscheck“)
//
// sshTermy liest die installierten Pakete des Servers über eine eigene,
// nur lesende Exec-Sitzung (dpkg, rpm, apk; dazu laufende Programme und
// die Pakete laufender Container) und gleicht sie mit der offenen
// Schwachstellen-Datenbank OSV ab. Online gehen dabei nur Paketnamen und
// Versionen an osv.dev, offline gar nichts (die Datenbank wird vorher
// geladen). Aktiv ausgenutzte Lücken (CISA-Liste) stehen oben. Die KI
// erklärt auf Wunsch; Updates werden nur eingefügt, nie ausgeführt.

type vulnState struct {
	sid      int
	conn     domain.Connection
	back     screen
	busy     bool
	step     string
	err      string
	seq      int
	cancel   context.CancelFunc
	rep      *vuln.Report
	prev     *vuln.Report // Stand vor dieser Prüfung (neu seit …)
	sel, top int
	hidden   bool // ausgeblendete zeigen
	onlyFix  bool // nur Pakete mit Update
}

type vulnMsg struct {
	seq int
	rep *vuln.Report
	err error
}

type vulnStepMsg struct {
	seq  int
	step string
}

// vulnCacheDir: OSV-Einträge, Offline-Datenbank und KEV-Liste (öffentliche
// Daten, keine Ergebnisse). In Tests austauschbar.
var vulnCacheDir = func() string {
	d, err := os.UserCacheDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "sshtermy", "osv")
}

// vulnSources: in Tests austauschbar.
var vulnSources = func(a *App) (vuln.Source, *vuln.HTTP) { return a.vulnSourceReal() }

var vulnSrcMu sync.Mutex

func (a *App) vulnSourceReal() (vuln.Source, *vuln.HTTP) {
	cfg := a.st.Settings().Vuln
	h := vuln.HTTP{Cache: vulnCacheDir(), Bucket: strings.TrimSpace(cfg.Mirror)}
	if cfg.Mode != domain.VulnOffline {
		return &vuln.Online{HTTP: h}, &h
	}
	vulnSrcMu.Lock()
	defer vulnSrcMu.Unlock()
	// Offline-Index bleibt im Speicher, solange sich der Spiegel nicht ändert.
	if a.voff == nil || a.voff.Bucket != h.Bucket {
		a.voff = &vuln.Offline{HTTP: h}
	}
	a.voff.Stale = nil
	return a.voff, &a.voff.HTTP
}

// vulnOK: kann der Tab geprüft werden? Sonst ein Grund.
func (a *App) vulnOK(s *session) error {
	switch {
	case s == nil || s.state != stConnected:
		return errors.New(i18n.T("Sicherheitscheck braucht eine verbundene Sitzung"))
	case s.netDevice():
		return errors.New(i18n.T("Switches und Firewalls: OSV kennt keine Hersteller-Firmware. Firmware-Stand steht in F6."))
	case s.isWinRM() || s.prof.Family == domain.FamilyWindows:
		return errors.New(i18n.T("Windows: OSV kennt keine Windows-Updates. Bitte über WSUS, Intune oder Windows Update prüfen."))
	case s.client == nil || !a.hostServerOK(s):
		return errors.New(i18n.T("Sicherheitscheck gibt es für Linux-Server über SSH"))
	}
	return nil
}

// openVuln öffnet den Sicherheitscheck des Tabs und prüft, wenn noch kein
// Ergebnis da ist.
func (a *App) openVuln(s *session) tea.Cmd {
	if err := a.vulnOK(s); err != nil {
		a.flash(err.Error())
		return nil
	}
	if a.st.Settings().Vuln.Mode == "" {
		return a.askVulnMode(func() tea.Cmd { return a.openVuln(s) })
	}
	back := a.scr
	if back == scrVuln {
		back = a.vscan.back
	}
	if a.vscan.sid != s.id || a.vscan.rep == nil {
		a.vscan.stop()
		a.vscan = vulnState{sid: s.id, conn: s.conn, back: back, prev: a.storedVuln(s.conn.ID)}
	}
	a.vscan.back = back
	a.scr = scrVuln
	if a.vscan.rep == nil && !a.vscan.busy {
		return a.runVuln(s)
	}
	return nil
}

func (v *vulnState) stop() {
	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
}

// askVulnMode: beim ersten Mal fragen, woher die Daten kommen.
func (a *App) askVulnMode(then func() tea.Cmd) tea.Cmd {
	items := []pickItem{
		{value: domain.VulnOnline, label: i18n.T("Online über osv.dev (empfohlen)"),
			detail: i18n.T("schnell, sendet nur Paketnamen und Versionen, keine Hostnamen")},
		{value: domain.VulnOffline, label: i18n.T("Offline-Datenbank laden"),
			detail: i18n.T("beim Prüfen geht nichts nach außen; Debian ~80 MB, Ubuntu ~800 MB je Tag")},
	}
	p := newPicker(i18n.T("Sicherheitscheck: woher kommen die Daten?"), "", items, domain.VulnOnline, func(it pickItem) tea.Cmd {
		_ = a.st.UpdateSettings(func(x *domain.Settings) { x.Vuln.Mode = it.value })
		return then()
	})
	p.help = i18n.T("OSV (osv.dev) ist die offene Schwachstellen-Datenbank von Google und der Open-Source-Gemeinschaft. Später änderbar: Einstellungen › Sicherheitscheck (OSV).")
	return a.openPicker(p)
}

// runVuln liest die Pakete und prüft sie im Hintergrund.
func (a *App) runVuln(s *session) tea.Cmd {
	v := &a.vscan
	if err := a.vulnOK(s); err != nil {
		v.err = err.Error()
		return nil
	}
	v.stop()
	v.seq++
	v.busy, v.err, v.step = true, "", i18n.T("liest die installierten Pakete …")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	v.cancel = cancel
	seq, cl, ev := v.seq, s.client, a.events
	cfg := a.st.Settings().Vuln
	src, h := vulnSources(a)
	step := func(t string) {
		select {
		case ev <- vulnStepMsg{seq, t}:
		default:
		}
	}
	return func() tea.Msg {
		defer cancel()
		rctx, rcancel := context.WithTimeout(ctx, 3*time.Minute)
		out, err := cl.RunScript(rctx, vuln.Script(!cfg.NoContainers))
		rcancel()
		if err != nil && strings.TrimSpace(out) == "" {
			return vulnMsg{seq: seq, err: err}
		}
		inv := vuln.ParseInventory(out)
		rep, err := scanInventory(ctx, inv, src, h, cfg.Mode, step)
		return vulnMsg{seq: seq, rep: rep, err: err}
	}
}

// scanInventory: KEV laden, prüfen, Hinweise ergänzen.
func scanInventory(ctx context.Context, inv *vuln.Inventory, src vuln.Source, h *vuln.HTTP, mode string, step func(string)) (*vuln.Report, error) {
	step(i18n.T("lädt die Liste aktiv ausgenutzter Lücken (CISA) …"))
	kev, kerr := vuln.LoadKEV(ctx, h)
	rep, err := vuln.Scan(ctx, inv, src, kev, vuln.Options{Step: step, Enrich: mode != domain.VulnOffline, Mode: orDefault(mode, domain.VulnOnline)})
	if err != nil {
		return nil, vulnErr(err)
	}
	if kerr != nil && len(kev) == 0 {
		rep.Notes = append(rep.Notes, i18n.T("CISA-Liste nicht erreichbar: „aktiv ausgenutzt“ fehlt diesmal"))
	}
	if o, ok := src.(*vuln.Offline); ok && len(o.Stale) > 0 {
		rep.Notes = append(rep.Notes, i18n.T("Offline-Datenbank ließ sich nicht auffrischen, älterer Stand: ")+strings.Join(o.Stale, ", "))
	}
	if !inv.Root && len(inv.Packages) > 0 {
		rep.Notes = append(rep.Notes, i18n.T("ohne sudo: „läuft“ kennt nur Programme dieses Benutzers"))
	}
	return rep, nil
}

// vulnErr: Netzfehler verständlich machen.
func vulnErr(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "api.osv.dev") || strings.Contains(msg, "osv.dev:"):
		return fmt.Errorf(i18n.T("osv.dev nicht erreichbar (%v). Ohne Internet: Einstellungen › Sicherheitscheck (OSV) › Quelle offline + eigener Spiegel"), err)
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New(i18n.T("Zeit abgelaufen. Offline-Datenbank wird beim ersten Mal geladen, das kann dauern; einfach r drücken."))
	}
	return err
}

// vulnMsgs: Ergebnis und Fortschritt.
func (a *App) vulnMsgs(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case vulnStepMsg:
		if m.seq == a.vscan.seq && a.vscan.busy {
			a.vscan.step = m.step
		}
		return a.listen(), true
	case vulnMsg:
		v := &a.vscan
		if m.seq != v.seq {
			return nil, true
		}
		v.busy, v.cancel = false, nil
		if m.err != nil {
			v.err = m.err.Error()
			return nil, true
		}
		v.rep = m.rep
		v.sel, v.top = 0, 0
		a.saveVuln(v.conn.ID, m.rep)
		c := m.rep.Count(a.vulnSkip(v.conn))
		if c.KEV > 0 {
			a.pushToast(toastWarn, fmt.Sprintf(i18n.T("%s: %d Pakete mit aktiv ausgenutzten Lücken"), v.conn.Alias, c.KEV))
		}
		return nil, true
	}
	return nil, false
}

// ---------------------------------------------------------------- gespeicherter Stand

// vulnSum: Kurzfassung für F6 und Manager.
type vulnSum struct {
	at     time.Time
	c      vuln.Counts
	fixes  int // Pakete mit Update
	ok     bool
	system string
}

func (a *App) storedVuln(connID string) *vuln.Report {
	raw := a.st.VulnReport(connID)
	if len(raw) == 0 {
		return nil
	}
	var r vuln.Report
	if json.Unmarshal(raw, &r) != nil {
		return nil
	}
	return &r
}

// saveVuln: Bericht gekürzt speichern (Beschreibungen nur kurz).
func (a *App) saveVuln(connID string, r *vuln.Report) {
	c := *r
	c.Findings = make([]vuln.Finding, len(r.Findings))
	for i, f := range r.Findings {
		f.CVEs = append([]vuln.CVE(nil), f.CVEs...)
		for j := range f.CVEs {
			f.CVEs[j].Text = trunc(f.CVEs[j].Text, 200)
		}
		c.Findings[i] = f
	}
	if raw, err := json.Marshal(&c); err == nil {
		_ = a.st.SetVulnReport(connID, raw)
	}
	delete(a.vsum, connID)
}

// vulnSummary: letzte Prüfung der Verbindung (zwischengespeichert).
func (a *App) vulnSummary(c domain.Connection) vulnSum {
	if a.vsum == nil {
		a.vsum = map[string]vulnSum{}
	}
	if s, ok := a.vsum[c.ID]; ok {
		return s
	}
	var s vulnSum
	if r := a.storedVuln(c.ID); r != nil {
		s = vulnSum{at: r.At, c: r.Count(a.vulnSkip(c)), ok: true, system: r.System}
		for _, f := range r.Findings {
			if f.Fixed != "" && f.Container == "" && !(f.Minor && !f.KEV) {
				s.fixes++
			}
		}
	}
	a.vsum[c.ID] = s
	return s
}

// vulnSumText: „2 kritisch · 5 hoch · 1 aktiv ausgenutzt“.
func vulnSumText(c vuln.Counts) string {
	var parts []string
	if c.KEV > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T("%d aktiv ausgenutzt"), c.KEV))
	}
	if c.Critical > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T("%d kritisch"), c.Critical))
	}
	if c.High > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T("%d hoch"), c.High))
	}
	if c.Medium > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T("%d mittel"), c.Medium))
	}
	if n := c.Low + c.Unknown; n > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T("%d weitere"), n))
	}
	if len(parts) == 0 {
		return i18n.T("✓ keine bekannten Lücken")
	}
	return strings.Join(parts, " · ")
}

// ---------------------------------------------------------------- ausblenden

// vulnSkip: was auf dieser Verbindung als „betrifft uns nicht“ gilt.
func (a *App) vulnSkip(c domain.Connection) func(vuln.Finding) bool {
	list := a.st.Settings().Vuln.Ignore
	return func(f vuln.Finding) bool {
		ids := cveIDs(f)
		for _, n := range list {
			if n.Hides(f.Key(), ids, c) {
				return true
			}
		}
		return false
	}
}

func cveIDs(f vuln.Finding) []string {
	out := make([]string, len(f.CVEs))
	for i, c := range f.CVEs {
		out[i] = c.ID
	}
	return out
}

// vulnIgnore: Paket mit seinen jetzigen Lücken ausblenden (Host oder Ordner).
func (a *App) vulnIgnore(f vuln.Finding, folder bool) {
	c := a.vscan.conn
	n := domain.VulnIgnore{Pkg: f.Key(), CVEs: cveIDs(f), Conn: c.ID, At: time.Now()}
	where := c.Alias
	if folder {
		n.Conn, n.Folder = "", c.Group
		where = i18n.T("überall")
		if n.Folder != "" {
			where = i18n.T("Ordner ") + n.Folder
		}
	}
	_ = a.st.UpdateSettings(func(s *domain.Settings) {
		out := s.Vuln.Ignore[:0]
		for _, x := range s.Vuln.Ignore {
			if !(x.Pkg == n.Pkg && x.Conn == n.Conn && x.Folder == n.Folder) {
				out = append(out, x)
			}
		}
		s.Vuln.Ignore = append(out, n)
		if len(s.Vuln.Ignore) > 500 {
			s.Vuln.Ignore = s.Vuln.Ignore[len(s.Vuln.Ignore)-500:]
		}
	})
	a.vsum = nil
	a.flash(fmt.Sprintf(i18n.T("Ausgeblendet (%s): %s · kommt eine neue Lücke dazu, ist es wieder da · n zeigt Ausgeblendetes"), where, f.Package))
}

// vulnUnignore: Ausblendung für das Paket auf dieser Verbindung aufheben.
func (a *App) vulnUnignore(f vuln.Finding) {
	c := a.vscan.conn
	ids := cveIDs(f)
	_ = a.st.UpdateSettings(func(s *domain.Settings) {
		out := s.Vuln.Ignore[:0]
		for _, x := range s.Vuln.Ignore {
			if !x.Hides(f.Key(), ids, c) {
				out = append(out, x)
			}
		}
		s.Vuln.Ignore = out
	})
	a.vsum = nil
	a.flash(i18n.T("Wird wieder angezeigt"))
}

// vulnVisible: Zeilen der Liste.
func (a *App) vulnVisible() []vuln.Finding {
	v := &a.vscan
	if v.rep == nil {
		return nil
	}
	skip := a.vulnSkip(v.conn)
	var out []vuln.Finding
	for _, f := range v.rep.Findings {
		if skip(f) != v.hidden {
			continue
		}
		if v.onlyFix && f.Fixed == "" {
			continue
		}
		out = append(out, f)
	}
	return out
}

func (a *App) vulnCurrent() (vuln.Finding, bool) {
	vis := a.vulnVisible()
	if len(vis) == 0 {
		return vuln.Finding{}, false
	}
	return vis[clamp(a.vscan.sel, 0, len(vis)-1)], true
}

// vulnNew: Pakete, die beim letzten Mal noch nicht betroffen waren.
func vulnNew(prev, cur *vuln.Report) map[string]bool {
	if prev == nil || cur == nil {
		return nil
	}
	old := map[string]map[string]bool{}
	for _, f := range prev.Findings {
		m := map[string]bool{}
		for _, c := range f.CVEs {
			m[c.ID] = true
		}
		old[f.Key()] = m
	}
	out := map[string]bool{}
	for _, f := range cur.Findings {
		m, ok := old[f.Key()]
		if !ok {
			out[f.Key()] = true
			continue
		}
		for _, c := range f.CVEs {
			if !m[c.ID] {
				out[f.Key()] = true
				break
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- Tasten

func (a *App) updateVuln(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		if mw, ok := msg.(tea.MouseWheelMsg); ok {
			if mw.Button == tea.MouseWheelUp {
				a.vulnMove(-3)
			} else if mw.Button == tea.MouseWheelDown {
				a.vulnMove(3)
			}
		}
		return nil
	}
	v := &a.vscan
	s := a.byID(v.sid)
	key := k.String()
	switch key {
	case "esc", "q":
		if v.hidden {
			v.hidden, v.sel = false, 0
			return nil
		}
		a.scr = v.back
		if a.scr == scrVuln || a.scr == scrUnlock {
			a.scr = scrTerminal
		}
		return nil
	case "r":
		if s == nil {
			v.err = i18n.T("Der Tab ist geschlossen")
			return nil
		}
		if !v.busy {
			if v.rep != nil {
				v.prev = v.rep
			}
			return a.runVuln(s)
		}
	case "x":
		if v.busy {
			v.stop()
			v.busy, v.err = false, i18n.T("Abgebrochen.")
			return nil
		}
		if f, ok := a.vulnCurrent(); ok {
			if v.hidden {
				a.vulnUnignore(f)
			} else {
				a.vulnIgnore(f, false)
			}
			v.sel = clamp(v.sel, 0, max(0, len(a.vulnVisible())-1))
		}
	case "X":
		if f, ok := a.vulnCurrent(); ok && !v.hidden {
			a.vulnIgnore(f, true)
			v.sel = clamp(v.sel, 0, max(0, len(a.vulnVisible())-1))
		}
	case "n":
		v.hidden, v.sel, v.top = !v.hidden, 0, 0
	case "f":
		v.onlyFix, v.sel, v.top = !v.onlyFix, 0, 0
	case "k":
		if f, ok := a.vulnCurrent(); ok && s != nil {
			return a.explainVuln(s, f)
		}
	case "up":
		a.vulnMove(-1)
	case "down":
		a.vulnMove(1)
	case "pgup":
		a.vulnMove(-10)
	case "pgdown":
		a.vulnMove(10)
	case "home":
		v.sel = 0
	case "end":
		v.sel = max(0, len(a.vulnVisible())-1)
	case "enter":
		if f, ok := a.vulnCurrent(); ok {
			a.showVulnDetail(f)
		}
	case "u":
		if f, ok := a.vulnCurrent(); ok {
			return a.vulnInsertUpdate(s, f)
		}
	case "o":
		return a.openVulnFolder()
	case "c":
		if v.rep != nil {
			a.flash(i18n.T("Sicherheitscheck als Text kopiert"))
			return tea.SetClipboard(a.vulnMarkdown())
		}
	}
	return nil
}

func (a *App) vulnMove(d int) {
	v := &a.vscan
	v.sel = clamp(v.sel+d, 0, max(0, len(a.vulnVisible())-1))
}

// vulnInsertUpdate fügt den Update-Befehl ein (⏎ im Terminal führt aus,
// auf geschützten Hosts mit der üblichen Rückfrage).
func (a *App) vulnInsertUpdate(s *session, f vuln.Finding) tea.Cmd {
	if f.Container != "" {
		a.flash(fmt.Sprintf(i18n.T("Container %s: Image aktualisieren (neu bauen bzw. ziehen) und Container neu starten. Im Container selbst zu aktualisieren hält nur bis zum nächsten Start."), f.Container))
		return nil
	}
	if f.Fixed == "" {
		a.flash(i18n.T("Für dieses Paket gibt es noch kein Update der Distribution"))
		return nil
	}
	cmd := vuln.UpdateCommand(f)
	if cmd == "" {
		return nil
	}
	if s == nil || s.state != stConnected {
		a.vscan.err = i18n.T("Der Tab ist geschlossen")
		return nil
	}
	a.scr = scrTerminal
	return a.insertCommand(s, cmd)
}

// showVulnDetail: alle Lücken eines Pakets.
func (a *App) showVulnDetail(f vuln.Finding) {
	st := a.s
	var lines []string
	lines = append(lines, st.Muted.Render(i18n.T("installiert ")+f.Installed+i18n.T(" · behoben in ")+orDefault(f.Fixed, i18n.T("– (kein Fix)"))))
	if len(f.Binaries) > 0 {
		lines = append(lines, st.Muted.Render(i18n.T("Pakete: ")+strings.Join(f.Binaries, ", ")))
	}
	if len(f.Running) > 0 {
		lines = append(lines, st.Warn.Render(i18n.T("läuft: ")+strings.Join(f.Running, ", ")))
	}
	if f.Container != "" {
		lines = append(lines, st.Muted.Render(i18n.T("Container: ")+f.Container))
	}
	if cmd := vuln.UpdateCommand(f); cmd != "" && f.Fixed != "" {
		lines = append(lines, "", st.Muted.Render(i18n.T("Update (u fügt ein): ")), "  "+st.AI.Render(cmd))
	}
	lines = append(lines, "")
	for _, c := range f.CVEs {
		head := fmt.Sprintf("%-5s %-16s", vulnLevelLabel(vuln.LevelOf(c.Score)), c.ID)
		if c.Score > 0 {
			head += fmt.Sprintf(" %4.1f", c.Score)
		}
		style := st.Text
		switch {
		case c.KEV:
			style = st.Danger
			head += i18n.T(" · AKTIV AUSGENUTZT")
			if c.Ransomware {
				head += i18n.T(" (Ransomware)")
			}
		case c.Minor:
			style = st.Muted
			head += i18n.T(" · laut Distribution unwichtig")
		}
		if c.Fixed == "" {
			head += i18n.T(" · kein Fix")
		}
		lines = append(lines, style.Render(head))
		if c.Text != "" {
			lines = append(lines, st.Muted.Render("      "+trunc(c.Text, 220)))
		}
		if len(c.Advisories) > 0 && !(len(c.Advisories) == 1 && strings.HasSuffix(c.Advisories[0], c.ID)) {
			lines = append(lines, st.Muted.Render("      "+strings.Join(c.Advisories, ", ")))
		}
	}
	title := fmt.Sprintf(i18n.T("%s · %d Lücken"), f.Package, len(f.CVEs))
	a.modal = textModal(title, lines)
}

// explainVuln: die KI erklärt das Paket (ohne Hostnamen und Terminal).
func (a *App) explainVuln(s *session, f vuln.Finding) tea.Cmd {
	var b strings.Builder
	fmt.Fprintf(&b, "Paket: %s (installiert %s, behoben in %s)\n", f.Package, f.Installed, orDefault(f.Fixed, "kein Fix"))
	if len(f.Binaries) > 0 {
		fmt.Fprintf(&b, "Installierte Pakete daraus: %s\n", strings.Join(f.Binaries, ", "))
	}
	if f.Container != "" {
		fmt.Fprintf(&b, "Im Container: %s\n", f.Container)
	}
	if len(f.Running) > 0 {
		fmt.Fprintf(&b, "Läuft gerade: %s\n", strings.Join(f.Running, ", "))
	} else {
		b.WriteString("Läuft gerade: kein Programm aus dem Paket erkannt\n")
	}
	if cmd := vuln.UpdateCommand(f); cmd != "" {
		fmt.Fprintf(&b, "Update-Befehl: %s\n", cmd)
	}
	b.WriteString("Lücken:\n")
	for i, c := range f.CVEs {
		if i >= 15 {
			fmt.Fprintf(&b, "… %d weitere\n", len(f.CVEs)-15)
			break
		}
		fmt.Fprintf(&b, "- %s CVSS %.1f", c.ID, c.Score)
		if c.KEV {
			b.WriteString(" AKTIV AUSGENUTZT (CISA)")
		}
		if c.Minor {
			b.WriteString(" (Distribution: unwichtig)")
		}
		fmt.Fprintf(&b, ", behoben in %s: %s\n", orDefault(c.Fixed, "kein Fix"), trunc(c.Text, 400))
	}
	a.scr = scrTerminal
	a.panelOpened(withPanel(s, panelAsk))
	a.resize(s)
	q := &s.ask
	ag, err := a.agentClientFor(s.conn)
	if err != nil {
		q.err = err.Error()
		return nil
	}
	ctx, cancel := q.begin(fmt.Sprintf(i18n.T("KI erklärt %s (%d Lücken, ohne Hostname)"), f.Package, len(f.CVEs)))
	q.history = nil
	q.input.SetValue("")
	sid, seq, ev := s.id, q.seq, a.events
	osName := s.prof.OS
	if a.vscan.rep != nil && a.vscan.rep.System != "" {
		osName = a.vscan.rep.System
	}
	text := b.String()
	return func() tea.Msg {
		defer cancel()
		err := ag.VulnExplain(ctx, osName, text, func(d string) {
			if d != "" {
				ev <- askDeltaMsg{sid, seq, d}
			}
		})
		return askDoneMsg{sid, seq, err}
	}
}

func vulnLevelLabel(l vuln.Level) string {
	switch l {
	case vuln.LevelCritical:
		return i18n.T("KRIT")
	case vuln.LevelHigh:
		return i18n.T("HOCH")
	case vuln.LevelMedium:
		return i18n.T("MITT")
	case vuln.LevelLow:
		return i18n.T("NIED")
	}
	return "?"
}

// vulnMarkdown: Ergebnis als Text (Ticket, Audit).
func (a *App) vulnMarkdown() string {
	v := &a.vscan
	r := v.rep
	var b strings.Builder
	c := r.Count(a.vulnSkip(v.conn))
	fmt.Fprintf(&b, i18n.T("## Sicherheitscheck %s (%s), %s\n\n"), v.conn.Alias, r.System, r.At.Local().Format("02.01.2006 15:04"))
	fmt.Fprintf(&b, i18n.T("%s · %d Pakete geprüft, Quelle OSV (%s)\n\n"), vulnSumText(c), r.Packages, r.Mode)
	b.WriteString("| " + i18n.T("Stufe") + " | " + i18n.T("Paket") + " | " + i18n.T("installiert") + " | " + i18n.T("behoben in") + " | CVE | " + i18n.T("läuft") + " |\n|---|---|---|---|---|---|\n")
	skip := a.vulnSkip(v.conn)
	for _, f := range r.Findings {
		if skip(f) {
			continue
		}
		lvl := vulnLevelLabel(f.Level())
		if f.KEV {
			lvl += " KEV"
		}
		pkg := f.Package
		if f.Container != "" {
			pkg += " (" + f.Container + ")"
		}
		ids := cveIDs(f)
		if len(ids) > 4 {
			ids = append(ids[:4], fmt.Sprintf("+%d", len(f.CVEs)-4))
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", lvl, pkg, f.Installed, orDefault(f.Fixed, "–"), strings.Join(ids, ", "), strings.Join(f.Running, ", "))
	}
	for _, n := range r.Notes {
		b.WriteString("\n> " + n + "\n")
	}
	return b.String()
}

// sortVulnHosts: für die Ordner-Ansicht (schlimmste zuerst).
func sortVulnHosts(hs []vulnHost) {
	rank := func(h vulnHost) int {
		if !h.done {
			return -2
		}
		if h.err != "" {
			return -1
		}
		return h.c.KEV*1000000 + h.c.Critical*10000 + h.c.High*100 + h.c.Medium
	}
	sort.SliceStable(hs, func(i, j int) bool { return rank(hs[i]) > rank(hs[j]) })
}

// ---------------------------------------------------------------- Einstellungen

func vulnModeLabel(m string) string {
	switch m {
	case domain.VulnOffline:
		return i18n.T("offline")
	case domain.VulnOnline:
		return i18n.T("online (osv.dev)")
	}
	return i18n.T("fragt beim ersten Mal")
}

func (a *App) cycleVulnMode() tea.Cmd {
	_ = a.st.UpdateSettings(func(s *domain.Settings) {
		if s.Vuln.Mode == domain.VulnOffline {
			s.Vuln.Mode = domain.VulnOnline
		} else {
			s.Vuln.Mode = domain.VulnOffline
		}
	})
	if a.st.Settings().Vuln.Mode == domain.VulnOffline {
		a.flash(i18n.T("Sicherheitscheck offline: die Datenbank wird beim nächsten Prüfen geladen und einmal am Tag aufgefrischt"))
	} else {
		a.flash(i18n.T("Sicherheitscheck online: an osv.dev gehen nur Paketnamen und Versionen"))
	}
	return nil
}

func (a *App) toggleVulnContainers() tea.Cmd {
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Vuln.NoContainers = !s.Vuln.NoContainers })
	return nil
}

func (a *App) askVulnMirror() tea.Cmd {
	m := inputModal(i18n.T("Sicherheitscheck: eigener Spiegel"),
		i18n.T("Adresse eines Webservers mit dem Inhalt von osv-vulnerabilities.storage.googleapis.com (je Ökosystem all.zip), z. B. https://osv.firma.lan. Leer = osv.dev. Gilt für die Offline-Datenbank."),
		a.st.Settings().Vuln.Mirror,
		func(v string) tea.Cmd {
			v = strings.TrimRight(strings.TrimSpace(v), "/")
			if v != "" && !strings.HasPrefix(v, "https://") && !strings.HasPrefix(v, "http://") {
				a.flashErr(i18n.T("Bitte mit https:// oder http:// angeben"))
				return nil
			}
			_ = a.st.UpdateSettings(func(s *domain.Settings) {
				s.Vuln.Mirror = v
				if v != "" {
					s.Vuln.Mode = domain.VulnOffline
				}
			})
			a.flash(i18n.T("Spiegel: ") + orDefault(v, "osv.dev"))
			return nil
		})
	m.in.Placeholder = "https://osv.firma.lan"
	a.modal = m
	return m.in.Focus()
}
