package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/inventory"
)

// Inventar & Änderungen (F6 i, Palette): Pakete, Benutzer, Admins, Cron,
// lauschende Ports, Dienste und Kernel – und was sich seit dem letzten
// Stand geändert hat. Nur lesend; gespeichert in inventory.enc (nur dieser
// Rechner).

type invHost struct {
	conn    domain.Connection
	snap    *inventory.Snapshot
	changes []inventory.Change
	first   bool // erster Stand, noch nichts zum Vergleichen
	err     string
	done    bool
}

type invState struct {
	seq    int
	busy   bool
	title  string
	single bool
	hosts  []invHost
	cancel context.CancelFunc
}

type invMsg struct {
	seq    int
	i      int
	snap   *inventory.Snapshot
	err    error
	viaCmd bool
}

// invRunHost: Skript auf einem Host ohne Rückfragen (in Tests austauschbar).
var invRunHost = func(a *App) func(ctx context.Context, c domain.Connection, script string) (string, error) {
	return a.quietScript
}

func (a *App) openInventory(s *session) tea.Cmd {
	if s == nil || s.client == nil || s.isWinRM() || !a.hostServerOK(s) {
		if s != nil {
			s.host.err = i18n.T("Inventar geht nur bei Linux-Servern über SSH")
		}
		return nil
	}
	v := &a.inv
	if v.cancel != nil {
		v.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	*v = invState{seq: v.seq + 1, busy: true, title: s.conn.Alias, single: true, hosts: []invHost{{conn: s.conn}}, cancel: cancel}
	seq, cl := v.seq, s.client
	a.showInventory()
	return func() tea.Msg {
		defer cancel()
		out, err := cl.RunScript(ctx, inventory.Script)
		if err != nil && strings.TrimSpace(out) == "" {
			return invMsg{seq: seq, err: err, viaCmd: true}
		}
		snap := inventory.Parse(out, time.Now())
		return invMsg{seq: seq, snap: &snap, viaCmd: true}
	}
}

func (a *App) openInventoryFolder() tea.Cmd {
	return a.folderPicker(i18n.T("Inventar & Änderungen: Ordner wählen"), vulnScanable,
		i18n.T("Nur lesend, ohne Rückfragen. Der erste Lauf legt den Stand an, jeder weitere zeigt, was sich geändert hat."),
		func(folder string) tea.Cmd { return a.runInventoryFolder(folder) })
}

func (a *App) runInventoryFolder(folder string) tea.Cmd {
	conns := a.vulnFolderConns(folder)
	if len(conns) == 0 {
		a.flash(i18n.T("Keine Linux-Server (SSH) im Ordner"))
		return nil
	}
	v := &a.inv
	if v.cancel != nil {
		v.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	where := folder
	if where == "" {
		where = i18n.T("alle Verbindungen")
	}
	*v = invState{seq: v.seq + 1, busy: true, title: where, cancel: cancel}
	for _, c := range conns {
		v.hosts = append(v.hosts, invHost{conn: c})
	}
	seq, ev, run := v.seq, a.events, invRunHost(a)
	go func() {
		defer cancel()
		forHosts(ctx, conns, 4, func(ctx context.Context, i int, c domain.Connection) {
			if ctx.Err() != nil {
				ev <- invMsg{seq: seq, i: i, err: ctx.Err()}
				return
			}
			out, err := run(ctx, c, inventory.Script)
			if err != nil {
				ev <- invMsg{seq: seq, i: i, err: err}
				return
			}
			snap := inventory.Parse(out, time.Now())
			ev <- invMsg{seq: seq, i: i, snap: &snap}
		})
	}()
	a.showInventory()
	return nil
}

func (a *App) invRecord(connID string) inventory.Record {
	var r inventory.Record
	if raw := a.st.Inventory(connID); len(raw) > 0 {
		_ = json.Unmarshal(raw, &r)
	}
	return r
}

func (a *App) invMsgs(msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(invMsg)
	if !ok {
		return nil, false
	}
	v := &a.inv
	next := a.listen()
	if m.viaCmd {
		next = nil
	}
	if m.seq != v.seq || m.i >= len(v.hosts) {
		return next, true
	}
	x := &v.hosts[m.i]
	x.done = true
	if m.err != nil {
		x.err = m.err.Error()
	}
	if m.snap != nil {
		x.snap = m.snap
		r := a.invRecord(x.conn.ID)
		x.first = r.Last.At.IsZero()
		x.changes = r.Update(*m.snap)
		if raw, err := json.Marshal(r); err == nil {
			_ = a.st.SetInventory(x.conn.ID, raw)
		}
	}
	v.busy = false
	for _, y := range v.hosts {
		if !y.done {
			v.busy = true
		}
	}
	if a.modal != nil && a.modal.inv {
		a.showInventory()
	}
	return next, true
}

var invKinds = map[string]string{
	"pkg": "Paket", "user": "Benutzer", "admin": "Admin", "cron": "Cron",
	"port": "Port", "svc": "Dienst", "kernel": "Kernel", "os": "System",
}

func changeText(c inventory.Change) string {
	op := map[string]string{"+": "+", "-": "−", "~": "~"}[c.Op]
	s := op + " " + i18n.T(invKinds[c.Kind]) + " "
	if c.Kind != "kernel" && c.Kind != "os" {
		s += c.Item
	}
	switch {
	case c.Op == "~":
		if c.Kind != "kernel" && c.Kind != "os" {
			s += ": "
		}
		s += c.From + " → " + c.To
	case c.Op == "+" && c.To != "":
		s += " (" + c.To + ")"
	}
	return s
}

// changeSummary: „3 Änderungen (12 Paket-Updates)“.
func changeSummary(ch []inventory.Change) string {
	imp := inventory.Important(ch)
	up := len(ch) - len(imp)
	if len(ch) == 0 {
		return i18n.T("keine Änderungen")
	}
	var p []string
	if len(imp) > 0 {
		p = append(p, plural(len(imp), i18n.T("Änderung"), i18n.T("Änderungen")))
	}
	if up > 0 {
		p = append(p, plural(up, i18n.T("Paket-Update"), i18n.T("Paket-Updates")))
	}
	return strings.Join(p, " · ")
}

func snapSummary(s *inventory.Snapshot) string {
	p := []string{plural(len(s.Packages), i18n.T("Paket"), i18n.T("Pakete")),
		plural(len(s.Users), i18n.T("Benutzer"), i18n.T("Benutzer")),
		plural(len(s.Ports), i18n.T("Port"), i18n.T("Ports")),
		plural(len(s.Services), i18n.T("Dienst"), i18n.T("Dienste"))}
	return strings.Join(p, " · ")
}

func (a *App) showInventory() {
	v := &a.inv
	var lines []string
	var md strings.Builder
	title := i18n.T("Inventar: ") + v.title
	fmt.Fprintf(&md, "# %s\n\n", title)
	if v.busy {
		lines = append(lines, spinGlyph(time.Now())+" "+i18n.T("liest Pakete, Benutzer, Ports, Dienste (nur lesend) …"), "")
	}
	if v.single && len(v.hosts) == 1 {
		x := v.hosts[0]
		if x.err != "" {
			lines = append(lines, "× "+x.err)
		}
		if s := x.snap; s != nil {
			head := orDefault(s.OS, x.conn.Alias) + " · Kernel " + s.Kernel
			lines = append(lines, head, snapSummary(s), "")
			fmt.Fprintf(&md, "%s\n%s\n\n", head, snapSummary(s))
			r := a.invRecord(x.conn.ID)
			switch {
			case x.first:
				lines = append(lines, i18n.T("Erster Stand gespeichert. Ab dem nächsten Mal steht hier, was sich geändert hat."))
			case len(x.changes) == 0:
				lines = append(lines, "✓ "+i18n.T("Keine Änderungen seit ")+localTime(prevAt(r)))
				fmt.Fprintf(&md, "%s\n", i18n.T("Keine Änderungen."))
			default:
				h := fmt.Sprintf(i18n.T("Seit %s: %s"), localTime(r.History[0].Since), changeSummary(x.changes))
				lines = append(lines, h)
				fmt.Fprintf(&md, "## %s\n\n", h)
				for _, c := range inventory.Important(x.changes) {
					lines = append(lines, "  "+trunc(changeText(c), 150))
					fmt.Fprintf(&md, "- %s\n", changeText(c))
				}
			}
			if len(r.History) > 1 || len(r.History) == 1 && len(x.changes) == 0 {
				lines = append(lines, "", i18n.T("Frühere Änderungen:"))
				start := 0
				if len(x.changes) > 0 {
					start = 1
				}
				for i, cs := range r.History[start:] {
					if i >= 6 {
						break
					}
					lines = append(lines, "  "+localTime(cs.At)+"  "+changeSummary(cs.Changes))
					for j, c := range inventory.Important(cs.Changes) {
						if j >= 4 {
							lines = append(lines, "      …")
							break
						}
						lines = append(lines, "      "+trunc(changeText(c), 140))
					}
				}
			}
			sec := func(name string, xs []string) {
				if len(xs) == 0 {
					return
				}
				lines = append(lines, "", name)
				fmt.Fprintf(&md, "\n## %s\n\n", name)
				for i, x := range xs {
					if i >= 12 {
						lines = append(lines, fmt.Sprintf(i18n.T("  … %d weitere (e speichert alles als CSV)"), len(xs)-12))
						break
					}
					lines = append(lines, "  "+trunc(x, 150))
				}
				for _, x := range xs {
					fmt.Fprintf(&md, "- %s\n", x)
				}
			}
			sec(i18n.T("Admins (sudo/wheel)"), s.Admins)
			sec(i18n.T("Lauschende Ports"), s.Ports)
			sec("Cron", s.Cron)
		}
	} else {
		lines = append(lines, fmt.Sprintf("%-22s %-30s %s", "Host", i18n.T("Änderungen"), i18n.T("wichtigste")))
		md.WriteString(i18n.T("| Host | Änderungen | wichtigste |\n|---|---|---|\n"))
		rows := append([]invHost(nil), v.hosts...)
		sort.SliceStable(rows, func(i, j int) bool {
			return len(inventory.Important(rows[i].changes)) > len(inventory.Important(rows[j].changes))
		})
		for _, x := range rows {
			name := trunc(x.conn.Alias, 22)
			switch {
			case !x.done:
				lines = append(lines, fmt.Sprintf("%-22s %s", name, i18n.T("…")))
			case x.snap == nil:
				lines = append(lines, fmt.Sprintf("%-22s × %s", name, trunc(x.err, 60)))
				fmt.Fprintf(&md, "| %s | × %s | |\n", x.conn.Alias, x.err)
			case x.first:
				lines = append(lines, fmt.Sprintf("%-22s %s", name, i18n.T("erster Stand gespeichert")))
				fmt.Fprintf(&md, "| %s | %s | |\n", x.conn.Alias, i18n.T("erster Stand"))
			default:
				top := ""
				if imp := inventory.Important(x.changes); len(imp) > 0 {
					top = changeText(imp[0])
				}
				lines = append(lines, fmt.Sprintf("%-22s %-30s %s", name, trunc(changeSummary(x.changes), 30), trunc(top, 60)))
				fmt.Fprintf(&md, "| %s | %s | %s |\n", x.conn.Alias, changeSummary(x.changes), top)
			}
		}
		lines = append(lines, "", i18n.T("Einzelheiten: Tab des Hosts öffnen, F6 › i."))
	}
	lines = append(lines, "", i18n.T("c kopieren · e als CSV speichern · nur lesend"))
	scroll := 0
	if a.modal != nil && a.modal.kind == mText && a.modal.inv {
		scroll = a.modal.scroll
	}
	a.modal = textModal(title, lines)
	a.modal.copy, a.modal.scroll, a.modal.inv = md.String(), scroll, true
}

func prevAt(r inventory.Record) time.Time {
	if len(r.History) > 0 {
		return r.History[0].At
	}
	return r.Last.At
}

// invKey: e speichert das Inventar als CSV.
func (a *App) invKey(key string) (tea.Cmd, bool) {
	if key != "e" {
		return nil, false
	}
	v := &a.inv
	var b strings.Builder
	for _, x := range v.hosts {
		if x.snap == nil {
			continue
		}
		c := x.snap.CSV(x.conn.Alias)
		if b.Len() > 0 {
			_, c, _ = strings.Cut(c, "\n") // Kopfzeile nur einmal
		}
		b.WriteString(c)
	}
	if b.Len() == 0 {
		a.flash(i18n.T("Noch kein Inventar zum Speichern"))
		return nil, true
	}
	p := filepath.Join(exportDir(), "sshtermy-inventar-"+unsafeName.ReplaceAllString(v.title, "_")+"-"+time.Now().Format("2006-01-02-1504")+".csv")
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		a.flash(err.Error())
		return nil, true
	}
	a.flashOK(i18n.T("Gespeichert: ") + p)
	return nil, true
}

// viewInventoryRow: Zeile „Inventar“ in F6.
func (a *App) viewInventoryRow(b *strings.Builder, s *session, w int) {
	st := a.s
	label := st.Muted.Render(fmt.Sprintf("%-9s ", i18n.T("Inventar")))
	r := a.invRecord(s.conn.ID)
	if r.Last.At.IsZero() {
		b.WriteString(label + st.Muted.Render(trunc(i18n.T("nicht erfasst · i erfasst (nur lesend)"), w-10)) + "\n")
		return
	}
	txt := plural(len(r.Last.Packages), i18n.T("Paket"), i18n.T("Pakete"))
	style := st.Text
	if len(r.History) > 0 {
		cs := r.History[0]
		txt += " · " + changeSummary(cs.Changes) + " " + i18n.T("am") + " " + localTime(cs.At)
		if len(inventory.Important(cs.Changes)) > 0 && time.Since(cs.At) < 7*24*time.Hour {
			style = st.Warn
		}
	}
	b.WriteString(label + style.Render(trunc(txt, w-22)) + st.Muted.Render(" · "+ago(r.Last.At)) + "\n")
}
