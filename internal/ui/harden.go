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
	"github.com/db9979/sshtermy/internal/harden"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Härtungs-Check (F6 h, Palette „Härtungs-Check“): nur lesend, Bewertung
// lokal (internal/harden). Vorschläge stehen als Text dabei, geändert wird
// nichts.

type hardenHost struct {
	conn domain.Connection
	rep  *harden.Report
	err  string
	done bool
}

type hardenState struct {
	seq    int
	busy   bool
	title  string
	single bool
	hosts  []hardenHost
	cancel context.CancelFunc
}

type hardenMsg struct {
	seq    int
	i      int
	rep    *harden.Report
	err    error
	viaCmd bool // kam als Rückgabe eines tea.Cmd, nicht über a.events
}

// hardenRunHost: Skript auf einem Host ohne Rückfragen (in Tests austauschbar).
var hardenRunHost = func(a *App) func(ctx context.Context, c domain.Connection, script string) (string, error) {
	return a.quietScript
}

// quietScript: ohne Rückfragen verbinden und ein sh-Skript ausführen.
func (a *App) quietScript(ctx context.Context, c domain.Connection, script string) (string, error) {
	ex, err := a.quietDialer()(ctx, c)
	if err != nil {
		return "", err
	}
	defer ex.Close()
	rctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	res, err := ex.Exec(rctx, "sh -c "+shQuote(script))
	if err != nil && len(res.Output) == 0 {
		return "", err
	}
	return string(res.Output), nil
}

// forHosts: fn für alle Verbindungen, par gleichzeitig.
func forHosts(ctx context.Context, conns []domain.Connection, par int, fn func(context.Context, int, domain.Connection)) {
	sem := make(chan struct{}, par)
	var wg sync.WaitGroup
	for i, c := range conns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				fn(ctx, i, c)
				return
			}
			defer func() { <-sem }()
			fn(ctx, i, c)
		}()
	}
	wg.Wait()
}

// openHarden: F6 h am Server.
func (a *App) openHarden(s *session) tea.Cmd {
	if s == nil || s.client == nil || s.netDevice() || s.isWinRM() || s.prof.Family == domain.FamilyWindows || !a.hostServerOK(s) {
		s.host.err = i18n.T("Härtungs-Check geht nur bei Linux-Servern über SSH")
		return nil
	}
	h := &a.hard
	if h.cancel != nil {
		h.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	*h = hardenState{seq: h.seq + 1, busy: true, title: s.conn.Alias, single: true, hosts: []hardenHost{{conn: s.conn}}, cancel: cancel}
	seq, cl := h.seq, s.client
	a.showHarden()
	return func() tea.Msg {
		defer cancel()
		out, err := cl.RunScript(ctx, harden.Script)
		if err != nil && strings.TrimSpace(out) == "" {
			return hardenMsg{seq: seq, err: err, viaCmd: true}
		}
		return hardenMsg{seq: seq, rep: harden.Check(harden.Parse(out), time.Now()), viaCmd: true}
	}
}

func (a *App) openHardenFolder() tea.Cmd {
	return a.folderPicker(i18n.T("Härtungs-Check: alle Server im Ordner"), vulnScanable,
		i18n.T("Nur lesend, ohne Rückfragen: Hosts, die ein Passwort oder eine Eingabe bräuchten, werden übersprungen. Mit sudo ohne Passwort sieht der Check mehr."),
		func(folder string) tea.Cmd { return a.runHardenFolder(folder) })
}

// folderPicker: Ordner mit passenden Verbindungen wählen.
func (a *App) folderPicker(title string, ok func(domain.Connection) bool, help string, run func(string) tea.Cmd) tea.Cmd {
	count := map[string]int{}
	for _, c := range a.st.Connections() {
		if !ok(c) {
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
		a.flash(i18n.T("Keine passenden Verbindungen"))
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
	} else if s := a.current(); s != nil {
		cur = s.conn.Group
	}
	a.pick = newPicker(title, i18n.T("Ordner suchen …"), items, cur, func(it pickItem) tea.Cmd {
		f := it.value
		if f == "\x00all" {
			f = ""
		}
		return run(f)
	})
	a.pick.help = help
	return nil
}

func (a *App) runHardenFolder(folder string) tea.Cmd {
	conns := a.vulnFolderConns(folder)
	if len(conns) == 0 {
		a.flash(i18n.T("Keine Linux-Server (SSH) im Ordner"))
		return nil
	}
	h := &a.hard
	if h.cancel != nil {
		h.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	where := folder
	if where == "" {
		where = i18n.T("alle Verbindungen")
	}
	*h = hardenState{seq: h.seq + 1, busy: true, title: where, cancel: cancel}
	for _, c := range conns {
		h.hosts = append(h.hosts, hardenHost{conn: c})
	}
	seq, ev, run := h.seq, a.events, hardenRunHost(a)
	go func() {
		defer cancel()
		forHosts(ctx, conns, 4, func(ctx context.Context, i int, c domain.Connection) {
			if ctx.Err() != nil {
				ev <- hardenMsg{seq: seq, i: i, err: ctx.Err()}
				return
			}
			out, err := run(ctx, c, harden.Script)
			if err != nil {
				ev <- hardenMsg{seq: seq, i: i, err: err}
				return
			}
			ev <- hardenMsg{seq: seq, i: i, rep: harden.Check(harden.Parse(out), time.Now())}
		})
	}()
	a.showHarden()
	return nil
}

func (a *App) hardenMsgs(msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(hardenMsg)
	if !ok {
		return nil, false
	}
	h := &a.hard
	next := a.listen()
	if m.viaCmd {
		next = nil
	}
	if m.seq != h.seq || m.i >= len(h.hosts) {
		return next, true
	}
	x := &h.hosts[m.i]
	x.done, x.rep = true, m.rep
	if m.err != nil {
		x.err = m.err.Error()
	}
	if m.rep != nil {
		if raw, err := json.Marshal(m.rep); err == nil {
			_ = a.st.SetExtraReport("hd", x.conn.ID, raw)
		}
	}
	h.busy = false
	for _, y := range h.hosts {
		if !y.done {
			h.busy = true
		}
	}
	if a.modal != nil && a.modal.harden {
		a.showHarden()
	}
	return next, true
}

func (a *App) storedHarden(connID string) *harden.Report {
	raw := a.st.ExtraReport("hd", connID)
	if len(raw) == 0 {
		return nil
	}
	var r harden.Report
	if json.Unmarshal(raw, &r) != nil {
		return nil
	}
	return &r
}

func hardenIcon(l harden.Level) string {
	switch l {
	case harden.Bad:
		return "×"
	case harden.Warn:
		return "▲"
	case harden.Info:
		return "i"
	}
	return "✓"
}

func hardenCountText(r *harden.Report) string {
	b, w, i := r.Counts()
	if b+w == 0 {
		if i > 0 {
			return fmt.Sprintf(i18n.T("in Ordnung · %d Hinweise"), i)
		}
		return i18n.T("in Ordnung")
	}
	var p []string
	if b > 0 {
		p = append(p, fmt.Sprintf(i18n.T("%d rot"), b))
	}
	if w > 0 {
		p = append(p, fmt.Sprintf(i18n.T("%d gelb"), w))
	}
	return strings.Join(p, " · ")
}

func (a *App) showHarden() {
	h := &a.hard
	var lines []string
	var md strings.Builder
	title := i18n.T("Härtungs-Check: ") + h.title
	fmt.Fprintf(&md, "# %s\n\n", title)
	if h.busy {
		lines = append(lines, spinGlyph(time.Now())+" "+i18n.T("liest Einstellungen (nur lesend) …"), "")
	}
	if h.single && len(h.hosts) == 1 {
		x := h.hosts[0]
		if x.err != "" {
			lines = append(lines, "× "+x.err)
		}
		if r := x.rep; r != nil {
			head := orDefault(r.System, x.conn.Alias) + " · " + hardenCountText(r)
			lines = append(lines, head, "")
			fmt.Fprintf(&md, "%s\n\n", head)
			for _, f := range r.Findings {
				lines = append(lines, hardenIcon(f.Level)+" "+f.Title)
				fmt.Fprintf(&md, "- %s **%s**", hardenIcon(f.Level), f.Title)
				if f.Detail != "" {
					lines = append(lines, "    "+trunc(f.Detail, 160))
					fmt.Fprintf(&md, " (%s)", f.Detail)
				}
				if f.Fix != "" {
					lines = append(lines, "    → "+f.Fix)
					fmt.Fprintf(&md, ": %s", f.Fix)
				}
				md.WriteString("\n")
			}
			if !r.Root {
				lines = append(lines, "", i18n.T("Ohne sudo gelesen: sshd, sudo-Regeln und Firewall sind nur teilweise sichtbar."))
			}
		}
	} else {
		lines = append(lines, fmt.Sprintf("%-22s %4s %5s  %s", "Host", i18n.T("rot"), i18n.T("gelb"), i18n.T("wichtigster Punkt")))
		md.WriteString(i18n.T("| Host | rot | gelb | wichtigster Punkt |\n|---|---|---|---|\n"))
		rows := append([]hardenHost(nil), h.hosts...)
		sort.SliceStable(rows, func(i, j int) bool { return hardenRank(rows[i]) > hardenRank(rows[j]) })
		for _, x := range rows {
			name := trunc(x.conn.Alias, 22)
			switch {
			case !x.done:
				lines = append(lines, fmt.Sprintf("%-22s %s", name, i18n.T("…")))
			case x.rep == nil:
				lines = append(lines, fmt.Sprintf("%-22s %4s %5s  × %s", name, "", "", trunc(x.err, 60)))
				fmt.Fprintf(&md, "| %s | | | × %s |\n", x.conn.Alias, x.err)
			default:
				b, w, _ := x.rep.Counts()
				top := ""
				if len(x.rep.Findings) > 0 && x.rep.Findings[0].Level >= harden.Warn {
					top = hardenIcon(x.rep.Findings[0].Level) + " " + x.rep.Findings[0].Title
				}
				lines = append(lines, fmt.Sprintf("%-22s %4d %5d  %s", name, b, w, trunc(top, 70)))
				fmt.Fprintf(&md, "| %s | %d | %d | %s |\n", x.conn.Alias, b, w, top)
			}
		}
		lines = append(lines, "", i18n.T("Einzelheiten mit Vorschlägen: Tab des Hosts öffnen, F6 › h."))
	}
	lines = append(lines, "", i18n.T("Nur lesend. sshTermy ändert nichts; die Vorschläge setzt du selbst um."))
	scroll := 0
	if a.modal != nil && a.modal.kind == mText && a.modal.harden {
		scroll = a.modal.scroll
	}
	a.modal = textModal(title, lines)
	a.modal.copy, a.modal.scroll, a.modal.harden = md.String(), scroll, true
}

func hardenRank(x hardenHost) int {
	if x.rep == nil {
		return -1
	}
	b, w, _ := x.rep.Counts()
	return b*100 + w
}

// viewHardenRow: Zeile „Härtung“ in F6.
func (a *App) viewHardenRow(b *strings.Builder, s *session, w int) {
	st := a.s
	label := st.Muted.Render(fmt.Sprintf("%-9s ", i18n.T("Härtung")))
	r := a.storedHarden(s.conn.ID)
	if r == nil {
		b.WriteString(label + st.Muted.Render(trunc(i18n.T("nicht geprüft · h prüft (nur lesend)"), w-10)) + "\n")
		return
	}
	bad, warn, _ := r.Counts()
	style := st.OK
	switch {
	case bad > 0:
		style = st.Danger
	case warn > 0:
		style = st.Warn
	}
	b.WriteString(label + style.Render(trunc(hardenCountText(r), w-22)) + st.Muted.Render(" · "+ago(r.At)) + "\n")
}
