package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/safety"
)

// Schnellbefehle: häufige Befehle pro Verbindung auf ⇧F1 … ⇧F8 (und als
// Knöpfe in der Vorschlagszeile, wenn die KI gerade nichts vorschlägt).
// Ohne eigene Liste gilt eine Vorbelegung je Gerätetyp, z. B. „show vlans“
// für Aruba-Switches. Anders als KI-Vorschläge werden Schnellbefehle sofort
// ausgeführt, weil der Nutzer sie selbst festgelegt hat; rote Befehle
// (feste Gefahrenregeln) brauchen trotzdem einen zweiten Druck.

const maxQuick = 8

type quickPreset struct {
	name string
	cmds []string
}

var (
	presetAruba = quickPreset{"Aruba-Switch", []string{
		"show interfaces brief", "show vlans", "show lldp info remote-device", "show mac-address",
		"show logging -r", "show system", "show running-config", "show version",
	}}
	presetCisco = quickPreset{"Cisco", []string{
		"show ip interface brief", "show vlan brief", "show cdp neighbors", "show mac address-table",
		"show logging", "show interfaces status", "show running-config", "show version",
	}}
	presetMikroTik = quickPreset{"MikroTik", []string{
		"/interface print", "/ip address print", "/interface vlan print", "/ip route print",
		"/log print", "/system resource print", "/export", "/system routerboard print",
	}}
	presetComware = quickPreset{"HPE Comware / Huawei", []string{
		"display interface brief", "display vlan", "display lldp neighbor-information list", "display mac-address",
		"display logbuffer", "display device", "display current-configuration", "display version",
	}}
	presetLinux = quickPreset{"Linux-Server", []string{
		"df -h", "free -h", "uptime", "ip -br a",
		"systemctl --failed", "journalctl -p err -b -n 50 --no-pager", "docker ps", "top -bn1 | head -20",
	}}
	presetWindows = quickPreset{"Windows", []string{
		"ipconfig", "systeminfo", "tasklist", "netstat -ano",
		"whoami", "hostname", "driverquery", "net user",
	}}
)

// quickPresetFor wählt die Vorbelegung aus dem Gerätetyp und dem, was über
// die Gegenstelle bekannt ist (SSH-Kennung bzw. Systemabfrage).
func quickPresetFor(c domain.Connection, remote string) quickPreset {
	r := strings.ToLower(remote)
	has := func(xs ...string) bool {
		for _, x := range xs {
			if strings.Contains(r, x) {
				return true
			}
		}
		return false
	}
	switch {
	case has("cisco", "ssh-2.0--"):
		return presetCisco
	case has("mikrotik", "rosssh"):
		return presetMikroTik
	case has("comware", "h3c", "huawei"):
		return presetComware
	case c.Device == domain.DeviceNetwork || has("netzwerkgerät", "aruba", "procurve"):
		return presetAruba // Aruba ist das häufigste Gerät; Liste ist anpassbar
	case has("windows"):
		return presetWindows
	}
	return presetLinux
}

// quickCmds liefert die Schnellbefehle der Sitzung und ob es die Vorbelegung ist.
func (a *App) quickCmds(s *session) (cmds []string, preset string) {
	c := s.conn
	if fresh, ok := a.st.Connection(c.ID); ok {
		c = fresh // nach dem Bearbeiten sofort die neue Liste
	}
	if len(c.QuickCmds) > 0 {
		return c.QuickCmds, ""
	}
	p := quickPresetFor(c, a.remoteText(s))
	return p.cmds, p.name
}

func (a *App) remoteText(s *session) string {
	if s.client != nil {
		return s.client.Remote.String()
	}
	return ""
}

// quickIndex: liegt auf der Taste ein Schnellbefehl (Standard ⇧F1 … ⇧F8)?
func (a *App) quickIndex(key string) (int, bool) {
	for i, id := range actQuick {
		if a.keys.is(id, key) {
			return i, true
		}
	}
	return 0, false
}

// quickLabel: angezeigte Taste von Schnellbefehl i, z. B. „⇧F1“.
func (a *App) quickLabel(i int) string {
	return strings.Replace(a.keys.label(actQuick[i]), "Shift+", "⇧", 1)
}

// runQuick führt Schnellbefehl i aus (mit ⏎). Rote Befehle erst beim
// zweiten Druck.
func (a *App) runQuick(s *session, i int) tea.Cmd {
	if s.state != stConnected || s.term == nil {
		return nil
	}
	cmds, _ := a.quickCmds(s)
	if i < 0 || i >= len(cmds) || strings.TrimSpace(cmds[i]) == "" {
		a.flash(a.quickLabel(i) + " ist nicht belegt · Palette „Schnellbefehle bearbeiten“")
		return nil
	}
	cmd := cmds[i]
	if f := safety.Check(cmd, safety.Kind(a.remoteText(s))); len(f) > 0 && s.quickArmed != i {
		s.quickArmed = i
		a.flash("Vorsicht: " + f[0].Reason + ". Nochmal " + a.quickLabel(i) + " zum Ausführen")
		return nil
	}
	s.quickArmed = -1
	a.toast = ""
	s.sel, s.scroll, s.ghost, s.chips = nil, 0, "", nil
	s.term.SendText(cmd + "\r")
	s.history = append(s.history, cmd)
	return nil
}

// ---------------------------------------------------------------- Liste (F9)

// quickModal: alle Schnellbefehle der Sitzung in F-Tasten-Reihenfolge.
// ↑↓ wählen, ⏎ oder 1…8 ausführen, e bearbeiten, Esc schließt.
func (a *App) quickModal(s *session) *modal {
	return &modal{kind: mQuick, title: "Schnellbefehle", quickSID: s.id, armed: -1}
}

func (a *App) quickModalKey(key string) tea.Cmd {
	m := a.modal
	s := a.byID(m.quickSID)
	if s == nil {
		a.modal = nil
		return nil
	}
	run := func(i int) tea.Cmd {
		cmds, _ := a.quickCmds(s)
		if i >= len(cmds) || strings.TrimSpace(cmds[i]) == "" {
			return nil
		}
		m.scroll = i
		if len(safety.Check(cmds[i], safety.Kind(a.remoteText(s)))) > 0 && m.armed != i {
			m.armed = i // erster Druck: nur warnen, Liste bleibt offen
			s.quickArmed = -1
			return a.runQuick(s, i)
		}
		s.quickArmed = i // zweiter Druck bzw. ungefährlich: ausführen
		a.modal = nil
		s.panelFocus = false
		return a.runQuick(s, i)
	}
	switch key {
	case "up", "k":
		m.scroll = (m.scroll + maxQuick - 1) % maxQuick
		m.armed = -1
	case "down", "j", "tab":
		m.scroll = (m.scroll + 1) % maxQuick
		m.armed = -1
	case "enter":
		return run(m.scroll)
	case "e":
		a.modal = nil
		return a.editQuickFor(s)
	case "esc", "q":
		a.modal = nil
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '8' {
			return run(int(key[0] - '1'))
		}
		if i, ok := a.quickIndex(key); ok {
			return run(i)
		}
		if a.keys.is(actQuickMenu, key) {
			a.modal = nil
		}
	}
	return nil
}

func (a *App) quickModalView(m *modal) string {
	st := a.s
	s := a.byID(m.quickSID)
	if s == nil {
		return ""
	}
	cmds, preset := a.quickCmds(s)
	kind := safety.Kind(a.remoteText(s))
	cw := min(64, a.w-12)
	head := st.Accent.Render("↯ Schnellbefehle") + st.Muted.Render("  "+s.conn.Alias)
	if preset != "" {
		head += st.Muted.Render(" · Vorbelegung „" + preset + "“")
	}
	lines := []string{head, ""}
	for i := 0; i < maxQuick; i++ {
		c := ""
		if i < len(cmds) {
			c = strings.TrimSpace(cmds[i])
		}
		key := a.quickLabel(i)
		key += strings.Repeat(" ", max(0, 5-lipgloss.Width(key)))
		txt := st.Muted.Render("— frei —")
		if c != "" {
			txt = st.Text.Render(trunc(c, cw-12))
			if f := safety.Check(c, kind); len(f) > 0 {
				txt = st.Danger.Render(trunc(c, cw-14) + " ⚠")
			}
		}
		row := " " + st.Accent.Render(key) + "  " + txt
		row += strings.Repeat(" ", max(0, cw-lipgloss.Width(row)))
		if i == m.scroll {
			row = st.Selected.Render(ansi.Strip(row))
			if i == m.armed {
				row = st.ChipDanger.Render(ansi.Strip(row))
			}
		}
		i := i
		lines = append(lines, a.z.click(row, func(_ tea.Mouse, double bool) tea.Cmd {
			if m.scroll == i || double {
				return a.quickModalKey("enter")
			}
			m.scroll, m.armed = i, -1
			return nil
		}))
	}
	hint := "⏎ oder 1…8 ausführen · ↑↓ wählen · e bearbeiten · Esc schließt"
	if m.armed >= 0 {
		hint = "⚠ gefährlich: nochmal ⏎ zum Ausführen"
	}
	lines = append(lines, "", st.Muted.Render(hint))
	return st.Modal.Render(strings.Join(lines, "\n"))
}

func (a *App) quickPaletteItems(s *session, match func(string) bool) []palItem {
	cmds, _ := a.quickCmds(s)
	var items []palItem
	for i, c := range cmds {
		if strings.TrimSpace(c) == "" || !match(c+" schnellbefehl") {
			continue
		}
		i, c := i, c
		it := palItem{group: pgQuick, icon: "↯", label: c, hint: a.quickLabel(i),
			run: func() tea.Cmd { s.panelFocus = false; return a.runQuick(s, i) }}
		if f := safety.Check(c, safety.Kind(a.remoteText(s))); len(f) > 0 {
			it.danger = f[0].Reason
		}
		items = append(items, it)
	}
	return items
}

// ---------------------------------------------------------------- Bearbeiten

// openQuickForm: die acht Schnellbefehle einer Verbindung bearbeiten.
// remote = was über die Gegenstelle bekannt ist (für die Vorbelegung).
func (a *App) openQuickForm(c domain.Connection, remote string) tea.Cmd {
	p := quickPresetFor(c, remote)
	cur := c.QuickCmds
	help := "eigene Liste"
	if len(cur) == 0 {
		cur = p.cmds
		help = "Vorbelegung „" + p.name + "“"
	}
	f := &form{title: "Schnellbefehle: " + c.Alias + "  (" + help + ")"}
	for i := 0; i < maxQuick; i++ {
		v := ""
		if i < len(cur) {
			v = cur[i]
		}
		h := "wird mit ⏎ ausgeführt; rote Befehle brauchen zwei Drücke · leer = frei"
		if i == 0 {
			h = "Strg+R = Vorbelegung „" + p.name + "“ wiederherstellen · Strg+S speichern"
		}
		f.text(fmt.Sprintf("q%d", i), a.quickLabel(i), v, h)
	}
	a.form, a.formID, a.scr = f, c.ID, scrQuickForm
	a.formBase = c
	a.formQuickPreset = p.cmds
	return f.setFocus(0)
}

func (a *App) saveQuick() error {
	c, ok := a.st.Connection(a.formID)
	if !ok {
		return fmt.Errorf("Verbindung nicht gefunden")
	}
	var cmds []string
	for i := 0; i < maxQuick; i++ {
		cmds = append(cmds, a.form.str(fmt.Sprintf("q%d", i)))
	}
	for len(cmds) > 0 && cmds[len(cmds)-1] == "" {
		cmds = cmds[:len(cmds)-1]
	}
	if len(cmds) == 0 || sameStrings(cmds, a.formQuickPreset) {
		cmds = nil // Vorbelegung behalten (folgt späteren Verbesserungen)
	}
	c.QuickCmds = cmds
	_, err := a.st.UpsertConnection(c)
	return err
}

// quickFormKey: Strg+R setzt die Vorbelegung ein.
func (a *App) quickFormKey(k tea.KeyPressMsg) bool {
	if k.String() != "ctrl+r" {
		return false
	}
	for i := 0; i < maxQuick; i++ {
		v := ""
		if i < len(a.formQuickPreset) {
			v = a.formQuickPreset[i]
		}
		a.form.get(fmt.Sprintf("q%d", i)).in.SetValue(v)
	}
	return true
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// editQuickFor: aus dem Terminal die Liste der aktuellen Verbindung bearbeiten.
func (a *App) editQuickFor(s *session) tea.Cmd {
	c := s.conn
	if fresh, ok := a.st.Connection(c.ID); ok {
		c = fresh
	}
	a.returnToTerminal = true
	return a.openQuickForm(c, a.remoteText(s))
}

// sortFKeys: F-Tasten der Fußzeile nach Nummer ordnen (F1, F2 … F12, dann ⇧F).
func sortFKeys(keys [][2]string) [][2]string {
	num := func(label string) int {
		var n int
		l := strings.TrimPrefix(label, "Shift+")
		if _, err := fmt.Sscanf(l, "F%d", &n); err == nil && strings.HasPrefix(l, "F") {
			if l != label {
				n += 100 // ⇧F-Tasten nach den normalen
			}
			return n
		}
		return 1000
	}
	// Nur die F-Tasten untereinander tauschen; andere (Palette, Strg+Q)
	// bleiben an ihrem Platz.
	var idx []int
	var fk [][2]string
	for i, k := range keys {
		if num(k[0]) < 1000 {
			idx, fk = append(idx, i), append(fk, k)
		}
	}
	sort.SliceStable(fk, func(i, j int) bool { return num(fk[i][0]) < num(fk[j][0]) })
	out := append([][2]string(nil), keys...)
	for n, i := range idx {
		out[i] = fk[n]
	}
	return out
}
