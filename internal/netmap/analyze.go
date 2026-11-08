package netmap

import (
	"fmt"
	"sort"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Device: was ein Switch geliefert hat.
type Device struct {
	Alias string // Name der Verbindung in sshTermy
	Host  string
	Name  string // Name aus dem Prompt
	MACs  []MACEntry
	ARP   map[string]string
	Neigh []Neighbor
	Err   string
}

// ParseDevice zerlegt die Shell-Ausgabe eines Switches.
func ParseDevice(alias, host, out string, cmds []string) Device {
	sec, name := Sections(out, cmds)
	d := Device{Alias: alias, Host: host, Name: name, ARP: map[string]string{}}
	for c, text := range sec {
		switch {
		case strings.Contains(c, "mac"):
			d.MACs = append(d.MACs, ParseMACTable(text)...)
		case strings.Contains(c, "arp"):
			for ip, mac := range ParseARP(text) {
				d.ARP[ip] = mac
			}
		case strings.Contains(c, "lldp") || strings.Contains(c, "cdp"):
			d.Neigh = append(d.Neigh, ParseNeighbors(text)...)
		}
	}
	d.Neigh = dedupNeighbors(d.Neigh)
	return d
}

func dedupNeighbors(ns []Neighbor) []Neighbor {
	seen := map[string]bool{}
	var out []Neighbor
	for _, n := range ns {
		k := normPort(n.LocalPort) + "|" + ShortName(n.RemoteName)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, n)
	}
	return out
}

// normPort: „GigabitEthernet1/0/1“, „Gig1/0/1“ und „Gi1/0/1“ sind derselbe Port.
func normPort(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	i := strings.IndexAny(p, "0123456789")
	if i <= 0 {
		return p
	}
	pre := p[:i]
	if len(pre) > 2 {
		pre = pre[:2]
	}
	return pre + p[i:]
}

// names: Namen, unter denen ein Switch bei seinen Nachbarn auftauchen kann.
func (d Device) names() []string {
	var out []string
	for _, s := range []string{d.Alias, d.Name, d.Host} {
		if s = ShortName(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// index: Kurzname → Gerät.
func index(devs []Device) map[string]int {
	m := map[string]int{}
	for i, d := range devs {
		for _, n := range d.names() {
			if _, ok := m[n]; !ok {
				m[n] = i
			}
		}
	}
	return m
}

// ---------------------------------------------------------------- Suche

// Sighting: ein Switch hat die MAC an einem Port gelernt.
type Sighting struct {
	Device   string // Alias
	Port     string
	VLAN     int
	PortMACs int    // so viele MACs kennt dieser Port insgesamt
	Uplink   string // Port führt zu diesem Switch ("" = kein Switch dahinter)
	Behind   string // Nachbar am Port, der kein Switch in sshTermy ist (AP, Telefon …)
}

// Located: Ergebnis der Suche.
type Located struct {
	MAC   string // normiert
	IP    string
	Found []Sighting // Zugangsport zuerst
	Edge  bool       // Found[0] ist sicher der Zugangsport (kein Uplink)
}

// Locate sucht eine MAC oder IP in den Tabellen aller Switches.
func Locate(devs []Device, query string) (Located, error) {
	q := strings.TrimSpace(query)
	var r Located
	switch {
	case NormMAC(q) != "":
		r.MAC = NormMAC(q)
	case IsIP(q):
		r.IP = q
		for _, d := range devs {
			if m := d.ARP[q]; m != "" {
				r.MAC = m
				break
			}
		}
		if r.MAC == "" {
			return r, fmt.Errorf(i18n.T("%s steht in keiner ARP-Tabelle; den Router oder Core-Switch mit in die Suche nehmen"), q)
		}
	default:
		return r, fmt.Errorf(i18n.T("„%s“ ist weder MAC noch IP-Adresse"), q)
	}
	if r.IP == "" {
		for _, d := range devs {
			for ip, m := range d.ARP {
				if m == r.MAC {
					r.IP = ip
				}
			}
		}
	}
	idx := index(devs)
	for _, d := range devs {
		count := map[string]int{}
		for _, e := range d.MACs {
			count[normPort(e.Port)]++
		}
		for _, e := range d.MACs {
			if e.MAC != r.MAC {
				continue
			}
			s := Sighting{Device: d.Alias, Port: e.Port, VLAN: e.VLAN, PortMACs: count[normPort(e.Port)]}
			for _, n := range d.Neigh {
				if normPort(n.LocalPort) != normPort(e.Port) {
					continue
				}
				if j, ok := idx[ShortName(n.RemoteName)]; ok {
					s.Uplink = devs[j].Alias
				} else if n.RemoteName != "" {
					s.Behind = n.RemoteName
				}
			}
			r.Found = append(r.Found, s)
		}
	}
	if len(r.Found) == 0 {
		return r, fmt.Errorf(i18n.T("%s steht in keiner MAC-Tabelle (Gerät aus oder länger still?)"), FormatMAC(r.MAC))
	}
	sort.SliceStable(r.Found, func(i, j int) bool {
		a, b := r.Found[i], r.Found[j]
		if (a.Uplink == "") != (b.Uplink == "") {
			return a.Uplink == ""
		}
		return a.PortMACs < b.PortMACs
	})
	r.Edge = r.Found[0].Uplink == ""
	return r, nil
}

// ---------------------------------------------------------------- Topologie

// Link zwischen zwei Geräten.
type Link struct {
	A, APort string
	B, BPort string
	Known    bool // B ist ein Switch in sshTermy
}

// Topology: Verbindungen zwischen den Switches (doppelte Sicht von beiden
// Seiten zusammengefasst) und Nachbarn, die nicht in sshTermy stehen.
func Topology(devs []Device) []Link {
	idx := index(devs)
	seen := map[string]bool{}
	var out []Link
	for _, d := range devs {
		for _, n := range d.Neigh {
			name := n.RemoteName
			if name == "" {
				continue
			}
			l := Link{A: d.Alias, APort: n.LocalPort, B: name, BPort: n.RemotePort}
			if j, ok := idx[ShortName(name)]; ok {
				l.B, l.Known = devs[j].Alias, true
				if l.B == l.A {
					continue
				}
			}
			k1 := l.A + "|" + normPort(l.APort) + "|" + l.B
			k2 := l.B + "|" + normPort(l.BPort) + "|" + l.A
			if seen[k1] || (l.Known && seen[k2]) {
				continue
			}
			seen[k1] = true
			out = append(out, l)
		}
	}
	return out
}

// TreeLines: Topologie als eingerückter Baum, beginnend beim Switch mit den
// meisten Verbindungen.
func TreeLines(devs []Device, links []Link) []string {
	adj := map[string][]Link{}
	deg := map[string]int{}
	for _, l := range links {
		adj[l.A] = append(adj[l.A], l)
		if l.Known {
			adj[l.B] = append(adj[l.B], Link{A: l.B, APort: l.BPort, B: l.A, BPort: l.APort, Known: true})
			deg[l.A]++
			deg[l.B]++
		}
	}
	order := make([]string, 0, len(devs))
	for _, d := range devs {
		order = append(order, d.Alias)
	}
	sort.SliceStable(order, func(i, j int) bool { return deg[order[i]] > deg[order[j]] })
	var out []string
	done := map[string]bool{}
	var walk func(name, indent string)
	walk = func(name, indent string) {
		done[name] = true
		ls := adj[name]
		sort.SliceStable(ls, func(i, j int) bool { return ls[i].Known && !ls[j].Known })
		for _, l := range ls {
			port := l.APort
			if l.BPort != "" {
				port += " ⇄ " + l.BPort
			}
			switch {
			case !l.Known:
				out = append(out, fmt.Sprintf("%s  %s  · %s", indent+"└", l.B, port))
			case done[l.B]:
				out = append(out, fmt.Sprintf("%s  %s  %s (%s)", indent+"└", l.B, port, i18n.T("schon oben")))
			default:
				out = append(out, fmt.Sprintf("%s  %s  %s", indent+"└", l.B, port))
				walk(l.B, indent+"   ")
			}
		}
	}
	for _, name := range order {
		if done[name] {
			continue
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, name)
		walk(name, " ")
	}
	return out
}

// Mermaid: Topologie als Mermaid-Diagramm (rendert in GitHub, GitLab,
// Obsidian, Confluence …).
func Mermaid(links []Link) string {
	var b strings.Builder
	b.WriteString("```mermaid\ngraph LR\n")
	ids := map[string]string{}
	id := func(name string) string {
		if v, ok := ids[name]; ok {
			return v
		}
		v := fmt.Sprintf("n%d", len(ids)+1)
		ids[name] = v
		return v
	}
	for _, l := range links {
		a, c := id(l.A), id(l.B)
		label := strings.ReplaceAll(l.APort+" – "+l.BPort, `"`, "'")
		shape := `["` + esc(l.B) + `"]`
		if !l.Known {
			shape = `("` + esc(l.B) + `")`
		}
		fmt.Fprintf(&b, "  %s[\"%s\"] ---|\"%s\"| %s%s\n", a, esc(l.A), strings.TrimSuffix(label, " – "), c, shape)
	}
	b.WriteString("```\n")
	return b.String()
}

func esc(s string) string { return strings.ReplaceAll(s, `"`, "'") }
