package ui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/netscan"
	"github.com/db9979/sshtermy/internal/store"
)

// Netzwerk-Scan (Manager: f): ein Subnetz nach SSH-Geräten durchsuchen und
// die Funde als Verbindungen anlegen. Am SSH-Banner wird erkannt, ob es ein
// Switch/Router ist; dann wird die Verbindung gleich als Netzwerkgerät
// gespeichert (keine Systemabfrage, kein EOF bei Aruba & Co.).

type scanPhase int

const (
	scSetup scanPhase = iota
	scRun
)

type scanState struct {
	phase   scanPhase
	f       *form
	hosts   []netscan.Host
	picked  map[string]bool
	known   map[string]string // ip:port → Alias einer vorhandenen Verbindung
	cur     int
	done    int
	total   int
	running bool
	cancel  context.CancelFunc
	seq     int
	started time.Time
	elapsed time.Duration
	spec    string
}

type scanFoundMsg struct {
	seq int
	h   netscan.Host
}
type scanProgMsg struct {
	seq, done, total int
}
type scanDoneMsg struct{ seq int }

func hostKey(ip string, port int) string { return fmt.Sprintf("%s:%d", ip, port) }

func (a *App) openScan() tea.Cmd {
	prev := a.scan
	sc := &scanState{picked: map[string]bool{}}
	if prev != nil {
		sc.seq = prev.seq
	}
	f := &form{title: i18n.T("Netzwerk-Scan: SSH-Geräte finden")}
	subnet := ""
	if l := netscan.LocalSubnets(); len(l) > 0 {
		subnet = l[0]
	}
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME")
	}
	auth, keypath, group := string(domain.AuthAgent), "", i18n.T("Netzwerk-Scan")
	if prev != nil && prev.f != nil {
		subnet, user, auth, keypath, group = prev.f.str("subnet"), prev.f.str("user"), prev.f.sel("auth"), prev.f.str("keypath"), prev.f.str("group")
	}
	f.text("subnet", i18n.T("Subnetz"), subnet, i18n.T("z. B. 192.168.1.0/24 · 10.0.0.10-50 · mehrere mit Komma (höchstens 4096 Adressen)"))
	port := 22
	if prev != nil && prev.f != nil && prev.f.num("port") > 0 {
		port = prev.f.num("port")
	}
	f.number("port", "Port", port, i18n.T("Standard 22"))
	f.text("user", i18n.T("Benutzer"), user, i18n.T("für die neuen Verbindungen"))
	f.choice("auth", i18n.T("Anmeldung"), []string{"agent", "keyfile", "password"}, auth, i18n.T("agent = ssh-agent (empfohlen) · Passwort fragt beim Verbinden"))
	f.text("keypath", "Keyfile", keypath, i18n.T("z. B. ~/.ssh/id_ed25519")).hidden = func(f *form) bool { return f.sel("auth") != "keyfile" }
	f.text("group", i18n.T("Ordner"), group, i18n.T("Ordner der neuen Verbindungen, „A / B“ verschachtelt (leer = oberste Ebene)"))
	sc.f = f
	a.scan = sc
	a.scr = scrScan
	return f.setFocus(0)
}

func (a *App) scanStart() tea.Cmd {
	sc := a.scan
	f := sc.f
	f.err = ""
	addrs, err := netscan.ParseTargets(f.str("subnet"))
	if err != nil {
		f.err = err.Error()
		return nil
	}
	if f.str("user") == "" {
		f.err = i18n.T("Benutzer fehlt (wird für die neuen Verbindungen gebraucht)")
		return nil
	}
	port := f.num("port")
	if port <= 0 || port > 65535 {
		port = 22
	}
	if sc.cancel != nil {
		sc.cancel()
	}
	sc.seq++
	seq := sc.seq
	ctx, cancel := context.WithCancel(context.Background())
	sc.cancel, sc.running, sc.started, sc.elapsed = cancel, true, time.Now(), 0
	sc.hosts, sc.picked, sc.cur, sc.done, sc.total = nil, map[string]bool{}, 0, 0, len(addrs)
	sc.spec = f.str("subnet")
	sc.known = map[string]string{}
	for _, c := range a.st.Connections() {
		p := c.Port
		if p == 0 {
			p = 22
		}
		sc.known[hostKey(c.Host, p)] = c.Alias
	}
	sc.phase = scRun
	ev := a.events
	go func() {
		last := time.Time{}
		netscan.Scan(ctx, addrs, netscan.Options{Port: port},
			func(d, n int) {
				// Fortschritt höchstens 10× pro Sekunde, das Ende immer.
				if d == n || time.Since(last) > 100*time.Millisecond {
					last = time.Now()
					select {
					case ev <- scanProgMsg{seq, d, n}:
					default:
					}
				}
			},
			func(h netscan.Host) { ev <- scanFoundMsg{seq, h} })
		ev <- scanDoneMsg{seq}
	}()
	return nil
}

func (a *App) scanMsg(msg tea.Msg) (tea.Cmd, bool) {
	sc := a.scan
	switch m := msg.(type) {
	case scanFoundMsg:
		if sc != nil && m.seq == sc.seq {
			sc.hosts = append(sc.hosts, m.h)
			sortHosts(sc.hosts)
			k := hostKey(m.h.IP, m.h.Port)
			_, known := sc.known[k]
			if !known && m.h.Name != "" {
				_, known = sc.known[hostKey(m.h.Name, m.h.Port)]
			}
			sc.picked[k] = !known
		}
		return a.listen(), true
	case scanProgMsg:
		if sc != nil && m.seq == sc.seq {
			sc.done, sc.total = max(sc.done, m.done), m.total
		}
		return a.listen(), true
	case scanDoneMsg:
		if sc != nil && m.seq == sc.seq {
			sc.running, sc.done = false, sc.total
			sc.elapsed = time.Since(sc.started)
			a.flash(fmt.Sprintf(i18n.T("Scan fertig: %s gefunden"), plural(len(sc.hosts), i18n.T("SSH-Gerät"), i18n.T("SSH-Geräte"))))
		}
		return a.listen(), true
	}
	return nil, false
}

func sortHosts(h []netscan.Host) {
	sort.SliceStable(h, func(i, j int) bool { return ipLess(h[i].IP, h[j].IP) })
}

func ipLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	if len(pa) != 4 || len(pb) != 4 {
		return a < b
	}
	for i := range pa {
		var x, y int
		fmt.Sscan(pa[i], &x)
		fmt.Sscan(pb[i], &y)
		if x != y {
			return x < y
		}
	}
	return false
}

// scanConnection: aus einem Fund wird eine Verbindung.
func scanConnection(h netscan.Host, user string, auth domain.AuthMethod, keypath, group string) domain.Connection {
	alias := h.IP
	if h.Name != "" {
		alias = h.Name
		if i := strings.Index(alias, "."); i > 0 && !looksLikeIP(alias) {
			alias = alias[:i]
		}
	}
	c := domain.Connection{Alias: alias, Host: h.IP, Port: h.Port, Username: user, Auth: auth, Group: group, Tags: []string{"scan"}}
	if auth == domain.AuthKeyFile {
		c.KeyPath = keypath
	}
	if h.Network {
		c.Device = domain.DeviceNetwork
	}
	return c
}

func looksLikeIP(s string) bool {
	return strings.Trim(s, "0123456789.") == ""
}

func (a *App) scanCreate() tea.Cmd {
	sc := a.scan
	f := sc.f
	auth := domain.AuthMethod(f.sel("auth"))
	n, nets := 0, 0
	var firstErr error
	for _, h := range sc.hosts {
		k := hostKey(h.IP, h.Port)
		if !sc.picked[k] {
			continue
		}
		c := scanConnection(h, f.str("user"), auth, f.str("keypath"), f.str("group"))
		if err := store.Validate(c); err != nil {
			firstErr = err
			continue
		}
		if _, err := a.st.UpsertConnection(c); err != nil {
			firstErr = err
			continue
		}
		sc.known[k] = c.Alias
		sc.picked[k] = false
		n++
		if h.Network {
			nets++
		}
	}
	switch {
	case firstErr != nil:
		a.flashErr(i18n.T("Nicht alle angelegt: ") + firstErr.Error())
	case n == 0:
		a.flash(i18n.T("Nichts gewählt (Leertaste wählt, a = alle)"))
		return nil
	default:
		msg := plural(n, i18n.T("Verbindung"), i18n.T("Verbindungen")) + i18n.T(" angelegt")
		if nets > 0 {
			msg += fmt.Sprintf(i18n.T(", davon %d als Netzwerkgerät"), nets)
		}
		a.flash(msg)
	}
	if n > 0 {
		a.scr = scrManager
		a.mgr.search.SetValue("")
		a.mgr.tag = ""
		if g := domain.NormPath(f.str("group")); g != "" {
			a.expandTo(g)
			a.mgr.want = "f:" + g
		}
	}
	return nil
}

func (a *App) updateScan(msg tea.Msg) tea.Cmd {
	sc := a.scan
	if sc == nil {
		a.scr = scrManager
		return nil
	}
	if sc.phase == scSetup {
		if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "esc" {
			a.scr = scrManager
			return nil
		}
		submit, cmd := sc.f.update(msg)
		if submit {
			return a.scanStart()
		}
		return cmd
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	n := len(sc.hosts)
	switch k.String() {
	case "esc":
		if sc.running {
			sc.cancel()
		}
		a.scr = scrManager
	case "up", "k":
		sc.cur = max(0, sc.cur-1)
	case "down", "j":
		sc.cur = min(n-1, sc.cur+1)
	case "space":
		if sc.cur < n {
			k := hostKey(sc.hosts[sc.cur].IP, sc.hosts[sc.cur].Port)
			sc.picked[k] = !sc.picked[k]
		}
	case "a":
		all := false
		for _, h := range sc.hosts {
			if !sc.picked[hostKey(h.IP, h.Port)] {
				all = true
			}
		}
		for _, h := range sc.hosts {
			sc.picked[hostKey(h.IP, h.Port)] = all
		}
	case "enter", "ctrl+s":
		return a.scanCreate()
	case "x":
		if sc.running {
			sc.cancel()
		}
	case "r", "e":
		if sc.running {
			sc.cancel()
		}
		sc.phase = scSetup
		return sc.f.setFocus(0)
	case "f1":
		a.modal = a.helpModal()
	}
	return nil
}

func (a *App) scanKeys() [][2]string {
	if a.scan == nil || a.scan.phase == scSetup {
		return [][2]string{{"Tab", i18n.T("weiter")}, {"←/→", i18n.T("wählen")}, {"Strg+S", i18n.T("Scan starten")}, {"Esc", i18n.T("abbrechen")}, {a.keys.label(actHelp), i18n.T("Hilfe")}}
	}
	k := [][2]string{{"Space", i18n.T("wählen")}, {"a", i18n.T("alle")}, {"⏎", i18n.T("anlegen")}}
	if a.scan.running {
		k = append(k, [2]string{"x", i18n.T("stoppen")})
	}
	return append(k, [2]string{"r", i18n.T("neu scannen")}, [2]string{"Esc", i18n.T("zum Manager")}, [2]string{a.keys.label(actHelp), i18n.T("Hilfe")})
}

func (a *App) viewScan() string {
	sc, s := a.scan, a.s
	if sc == nil {
		return ""
	}
	w := min(110, a.w-4)
	if sc.phase == scSetup {
		var o strings.Builder
		o.WriteString(sc.f.view(s, w-4, &a.z))
		o.WriteString("\n" + s.Muted.Render(i18n.T("Gesucht wird nur nach offenem SSH-Port und der SSH-Kennung (ohne Anmeldung).\n")+
			i18n.T("Switches und Router (Aruba, Cisco, MikroTik …) werden am Banner erkannt.")) + "\n")
		if l := netscan.LocalSubnets(); len(l) > 1 {
			o.WriteString(s.Muted.Render(i18n.T("Eigene Netze: ")+strings.Join(l, ", ")) + "\n")
		}
		o.WriteString("\n" + a.keyZone("Strg+S", s.Muted.Render(i18n.T("Strg+S oder ⏎ im letzten Feld startet den Scan"))))
		return lipgloss.Place(a.w, a.h-2, lipgloss.Center, lipgloss.Center, s.Panel.Width(w).Render(o.String()))
	}

	var o strings.Builder
	o.WriteString(s.Accent.Render(i18n.T("⌕ Netzwerk-Scan ")) + s.Text.Render(trunc(sc.spec, 40)) + s.Muted.Render(fmt.Sprintf(i18n.T("  Port %d"), sc.f.num("port"))) + "\n")
	// Fortschrittsbalken
	barW := max(10, w-40)
	frac := 0.0
	if sc.total > 0 {
		frac = float64(sc.done) / float64(sc.total)
	}
	fill := int(frac * float64(barW))
	bar := s.OK.Render(strings.Repeat("█", fill)) + s.Muted.Render(strings.Repeat("░", barW-fill))
	status := fmt.Sprintf(" %d/%d · %s", sc.done, sc.total, plural(len(sc.hosts), i18n.T("Fund"), i18n.T("Funde")))
	if !sc.running {
		status += " · " + bcDur(sc.elapsed)
	}
	o.WriteString(bar + s.Muted.Render(status) + "\n\n")

	picked := 0
	for _, h := range sc.hosts {
		if sc.picked[hostKey(h.IP, h.Port)] {
			picked++
		}
	}
	if len(sc.hosts) == 0 {
		if sc.running {
			o.WriteString(s.Muted.Render(i18n.T("  suche …")) + "\n")
		} else {
			o.WriteString(s.Muted.Render(i18n.T("  Keine SSH-Geräte gefunden. Subnetz oder Port prüfen (r).")) + "\n")
		}
	} else {
		o.WriteString(s.Muted.Render(fmt.Sprintf(i18n.T("  %d gewählt · Ordner „%s“ · Benutzer %s"), picked, orDefault(domain.NormPath(sc.f.str("group")), i18n.T("oberste Ebene")), sc.f.str("user"))) + "\n")
	}
	rows := max(3, a.h-2-12)
	start := 0
	if sc.cur >= rows {
		start = sc.cur - rows + 1
	}
	for i := start; i < len(sc.hosts) && i < start+rows; i++ {
		h := sc.hosts[i]
		k := hostKey(h.IP, h.Port)
		box := s.Muted.Render("[ ]")
		if sc.picked[k] {
			box = s.OK.Render("[x]")
		}
		kind := s.Text.Render("Server       ")
		if h.Network {
			kind = s.Warn.Render(i18n.T("Netzwerkgerät"))
		}
		extra := ""
		if alias, ok := sc.known[k]; ok {
			extra = s.Muted.Render(i18n.T(" · schon gespeichert: ") + alias)
		}
		line := fmt.Sprintf("  %s %-15s %-22s %s  %s%s", box, h.IP, trunc(h.Name, 22), kind, s.Muted.Render(trunc(h.Label(), 30)), extra)
		if i == sc.cur {
			line = s.Selected.Width(w - 4).Render(ansi.Truncate(ansi.Strip(line), w-4, "…"))
		} else {
			line = ansi.Truncate(line, w-4, "…")
		}
		i, k := i, k
		line = a.z.click(line, func(tea.Mouse, bool) tea.Cmd { sc.cur, sc.picked[k] = i, !sc.picked[k]; return nil })
		o.WriteString(line + "\n")
	}
	return lipgloss.Place(a.w, a.h-2, lipgloss.Center, lipgloss.Center, s.Panel.Width(w).Render(o.String()))
}
