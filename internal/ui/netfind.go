package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/broadcast"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/netmap"
)

// Gerät finden und Topologie (Palette)
//
// „Gerät finden“ nimmt eine MAC oder IP und fragt alle Switches in
// sshTermy nach MAC-Tabelle, ARP-Tabelle und LLDP/CDP-Nachbarn. Der Port, an
// dem kein weiterer Switch hängt und der die wenigsten MACs kennt, ist der
// Zugangsport. „Netzwerk-Topologie“ setzt aus den LLDP/CDP-Nachbarn einen
// Baum zusammen und legt ihn als Mermaid-Diagramm in Downloads ab.
//
// Alles läuft wie der Broadcast über eine eigene Shell je Switch, nur mit
// show-Befehlen. Die Antworten bleiben drei Minuten im Speicher, damit die
// nächste Suche sofort fertig ist.

const netCacheFor = 3 * time.Minute

type netCache struct {
	at    time.Time
	lldp  bool
	mac   bool
	devs  []netmap.Device
	fails []string
}

type netDataMsg struct {
	query string // "" = Topologie
	data  netCache
}

// netDialer: in Tests austauschbar.
var netDialer = func(a *App) broadcast.Dialer { return a.bcDialer() }

// netDevices: SSH-Verbindungen, die als Netzwerkgerät bekannt sind.
func (a *App) netDevices() []domain.Connection {
	var out []domain.Connection
	for _, c := range a.st.Connections() {
		if c.Protocol.Raw() {
			continue
		}
		if isNetConn(c) {
			out = append(out, c)
		}
	}
	return out
}

func isNetConn(c domain.Connection) bool {
	if c.Device == domain.DeviceNetwork {
		return true
	}
	if c.Device == domain.DeviceServer {
		return false
	}
	return c.Profile != nil && (c.Profile.Family == domain.FamilyNetwork || domain.FamilyOf(c.Profile.Dialect) == domain.FamilyNetwork)
}

// connVendor: Befehlssatz aus dem Systemprofil.
func connVendor(c domain.Connection) string {
	if c.Profile == nil {
		return ""
	}
	switch c.Profile.Dialect {
	case "procurve":
		return netmap.ProCurve
	case "arubaos-cx":
		return netmap.ArubaCX
	case "cisco-ios":
		return netmap.Cisco
	}
	return ""
}

func (a *App) askNetFind() tea.Cmd {
	m := inputModal(i18n.T("Gerät finden: MAC/IP → Switch-Port"),
		i18n.T("MAC (jede Schreibweise) oder IP-Adresse. sshTermy fragt alle Switches nach MAC-, ARP- und LLDP-Tabellen (nur show-Befehle)."), "",
		func(v string) tea.Cmd { return a.netFind(v) })
	m.in.Placeholder = "00:11:22:aa:bb:cc · 10.0.20.15"
	a.modal = m
	return m.in.Focus()
}

// looksLikeAddr: Palette bietet „Gerät finden“ direkt an.
func looksLikeAddr(q string) bool {
	q = strings.TrimSpace(q)
	return netmap.NormMAC(q) != "" || netmap.IsIP(q)
}

func (a *App) netFind(q string) tea.Cmd {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	if !looksLikeAddr(q) {
		a.flashErr(fmt.Sprintf(i18n.T("„%s“ ist weder MAC noch IP-Adresse"), q))
		return nil
	}
	if c := a.netData; c != nil && c.mac && time.Since(c.at) < netCacheFor {
		a.showNetFind(q, *c)
		return nil
	}
	return a.netCollect(q, true)
}

func (a *App) netTopology() tea.Cmd {
	if c := a.netData; c != nil && c.lldp && time.Since(c.at) < netCacheFor {
		a.showTopology(*c)
		return nil
	}
	return a.netCollect("", false)
}

// netCollect fragt alle Switches ab; mac=false nur die Nachbarn.
func (a *App) netCollect(query string, mac bool) tea.Cmd {
	if a.netBusy {
		a.flash(i18n.T("Abfrage läuft schon …"))
		return nil
	}
	conns := a.netDevices()
	if len(conns) == 0 {
		a.flashErr(i18n.T("Keine Switches in sshTermy: Verbindung auf „Netzwerkgerät“ stellen oder einmal verbinden"))
		return nil
	}
	job := broadcast.Job{Conns: conns, Parallel: 8, Timeout: 3 * time.Minute}
	cmds := make([][]string, len(conns))
	for i, c := range conns {
		cmds[i] = netmap.Commands(connVendor(c), mac, mac, true)
		job.PerHost = append(job.PerHost, cmds[i])
	}
	job.Steps = cmds[0]
	a.netBusy = true
	a.flash(fmt.Sprintf(i18n.T("Frage %d Switches ab (show-Befehle) …"), len(conns)))
	dial := netDialer(a)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		res := broadcast.Run(ctx, job, dial, func(int, broadcast.Result) {})
		data := netCache{at: time.Now(), lldp: true, mac: mac}
		for i, r := range res {
			if r.Err != "" && r.Output == "" {
				data.fails = append(data.fails, "× "+r.Alias+": "+r.Err)
				continue
			}
			data.devs = append(data.devs, netmap.ParseDevice(conns[i].Alias, conns[i].Host, r.Output, cmds[i]))
		}
		return netDataMsg{query: query, data: data}
	}
}

func (a *App) netMsg(msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(netDataMsg)
	if !ok {
		return nil, false
	}
	a.netBusy = false
	a.netData = &m.data
	if m.query == "" {
		a.showTopology(m.data)
	} else {
		a.showNetFind(m.query, m.data)
	}
	return nil, true
}

func (a *App) showNetFind(q string, d netCache) {
	r, err := netmap.Locate(d.devs, q)
	var lines []string
	title := i18n.T("Gerät finden: ") + q
	if err != nil {
		lines = append(lines, "× "+err.Error())
	} else {
		head := netmap.FormatMAC(r.MAC)
		if r.IP != "" {
			head += "  ·  " + r.IP
		}
		lines = append(lines, head, "")
		for i, s := range r.Found {
			mark := "  "
			if i == 0 {
				mark = "→ "
			}
			l := fmt.Sprintf("%s%s  %s %s  VLAN %d", mark, s.Device, i18n.T("Port"), s.Port, s.VLAN)
			switch {
			case s.Uplink != "":
				l += "  · " + i18n.T("Uplink zu ") + s.Uplink
			case s.Behind != "":
				l += "  · " + i18n.T("hinter ") + s.Behind
			}
			if s.PortMACs > 1 {
				l += fmt.Sprintf(i18n.T("  (%d MACs am Port)"), s.PortMACs)
			}
			lines = append(lines, l)
		}
		lines = append(lines, "")
		if r.Edge {
			lines = append(lines, fmt.Sprintf(i18n.T("Das Gerät hängt an %s, Port %s."), r.Found[0].Device, r.Found[0].Port))
		} else {
			lines = append(lines, i18n.T("Nur auf Uplinks gesehen: der Zugangs-Switch ist nicht in sshTermy oder antwortet nicht."))
		}
	}
	lines = append(lines, a.netFooter(d)...)
	a.modal = textModal(title, lines)
}

func (a *App) showTopology(d netCache) {
	links := netmap.Topology(d.devs)
	var lines []string
	if len(links) == 0 {
		lines = append(lines, i18n.T("Keine LLDP/CDP-Nachbarn gefunden (LLDP auf den Switches aus?)"))
	} else {
		lines = append(lines, netmap.TreeLines(d.devs, links)...)
		lines = append(lines, "", i18n.T("· = Nachbar, der nicht in sshTermy steht (AP, Telefon, fremder Switch)"))
		if p, err := writeTopology(d.devs, links); err == nil {
			lines = append(lines, i18n.T("Diagramm (Mermaid): ")+p)
		} else {
			lines = append(lines, i18n.T("Diagramm nicht gespeichert: ")+err.Error())
		}
	}
	lines = append(lines, a.netFooter(d)...)
	a.modal = textModal(fmt.Sprintf(i18n.T("Netzwerk-Topologie · %d Switches"), len(d.devs)), lines)
}

func (a *App) netFooter(d netCache) []string {
	out := []string{""}
	if len(d.fails) > 0 {
		sort.Strings(d.fails)
		out = append(out, d.fails...)
	}
	out = append(out, fmt.Sprintf(i18n.T("Stand %s · %d Switches abgefragt · neue Abfrage nach 3 Minuten"), d.at.Format("15:04:05"), len(d.devs)))
	return out
}

func writeTopology(devs []netmap.Device, links []netmap.Link) (string, error) {
	if len(links) == 0 {
		return "", errors.New(i18n.T("keine Verbindungen"))
	}
	var b strings.Builder
	b.WriteString("# " + i18n.T("Netzwerk-Topologie") + " " + time.Now().Format("2006-01-02 15:04") + "\n\n")
	b.WriteString(netmap.Mermaid(links))
	b.WriteString("\n```\n" + strings.Join(netmap.TreeLines(devs, links), "\n") + "\n```\n")
	p := filepath.Join(exportDir(), "sshtermy-topologie-"+time.Now().Format("20060102-1504")+".md")
	return p, os.WriteFile(p, []byte(b.String()), 0o600)
}
