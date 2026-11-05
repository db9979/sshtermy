package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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

// quickBar: Knöpfe für die Vorschlagszeile.
func (a *App) quickBar(s *session) string {
	if s.state != stConnected {
		return ""
	}
	st := a.s
	cmds, _ := a.quickCmds(s)
	var b strings.Builder
	b.WriteString(st.Muted.Render(" ↯ "))
	used := 3
	for i, c := range cmds {
		if strings.TrimSpace(c) == "" {
			continue
		}
		style := st.Chip
		if len(safety.Check(c, safety.Kind(a.remoteText(s)))) > 0 {
			style = st.ChipDanger
		}
		chip := st.Key.Render(a.quickLabel(i)) + style.Render(trunc(c, 22))
		if used+lipgloss.Width(chip)+1 > a.w {
			continue // passt nicht mehr, kürzere dahinter evtl. schon
		}
		used += lipgloss.Width(chip) + 1
		i := i
		b.WriteString(a.z.click(chip, func(tea.Mouse, bool) tea.Cmd { s.panelFocus = false; return a.runQuick(s, i) }) + " ")
	}
	return b.String()
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
