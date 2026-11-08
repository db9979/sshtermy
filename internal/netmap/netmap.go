// Package netmap liest MAC-Tabellen, ARP-Tabellen und LLDP/CDP-Nachbarn von
// Switches (ArubaOS-Switch/ProCurve, ArubaOS-CX, Cisco IOS) und setzt daraus
// zusammen, wo ein Gerät hängt und wie die Switches verbunden sind.
//
// Die Befehle laufen über die interaktive Shell (Netzwerkgeräte vertragen
// keine Exec-Kanäle). Die Ausgabe aller Befehle kommt als ein Strom zurück;
// Sections teilt ihn am Echo der Befehlszeilen wieder auf.
package netmap

import (
	"regexp"
	"strconv"
	"strings"
)

// Vendor wie in hostinfo, hier als Text, damit das Paket ohne UI auskommt.
const (
	ProCurve = "procurve"
	ArubaCX  = "aruba-cx"
	Cisco    = "cisco"
)

// Commands: show-Befehle für Suche und Topologie. Unbekannter Hersteller
// bekommt alle Varianten; was das Gerät nicht kennt, lehnt es harmlos ab.
func Commands(vendor string, mac, arp, lldp bool) []string {
	var out []string
	add := func(c ...string) { out = append(out, c...) }
	switch vendor {
	case ProCurve:
		add("no page")
		if mac {
			add("show mac-address")
		}
		if arp {
			add("show arp")
		}
		if lldp {
			add("show lldp info remote-device")
		}
	case ArubaCX:
		add("no page")
		if mac {
			add("show mac-address-table")
		}
		if arp {
			add("show arp")
		}
		if lldp {
			add("show lldp neighbor-info")
		}
	case Cisco:
		add("terminal length 0")
		if mac {
			add("show mac address-table")
		}
		if arp {
			add("show ip arp")
		}
		if lldp {
			add("show lldp neighbors", "show cdp neighbors")
		}
	default:
		add("no page", "terminal length 0")
		if mac {
			add("show mac-address", "show mac-address-table", "show mac address-table")
		}
		if arp {
			add("show arp", "show ip arp")
		}
		if lldp {
			add("show lldp info remote-device", "show lldp neighbor-info", "show lldp neighbors", "show cdp neighbors")
		}
	}
	return out
}

// Sections teilt die Shell-Ausgabe nach Befehlen auf. name ist der Name aus
// dem Prompt („sw-core# show arp“ → sw-core).
func Sections(out string, cmds []string) (sec map[string]string, name string) {
	sec = map[string]string{}
	cur := ""
	var b strings.Builder
	flush := func() {
		if cur != "" {
			sec[cur] += b.String()
		}
		b.Reset()
	}
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		t := strings.TrimSpace(l)
		if c, pre, ok := echoOf(t, cmds); ok {
			flush()
			cur = c
			if name == "" {
				name = promptName(pre)
			}
			continue
		}
		if isPrompt(t) {
			if name == "" {
				name = promptName(t)
			}
			continue
		}
		b.WriteString(l + "\n")
	}
	flush()
	return sec, name
}

// echoOf: ist t das Echo eines der Befehle („sw# show arp“ oder nur „show arp“)?
// Der längste passende Befehl gewinnt („show lldp neighbors“ vor „show lldp“).
func echoOf(t string, cmds []string) (cmd, prompt string, ok bool) {
	for _, c := range cmds {
		if !strings.HasSuffix(t, c) || len(c) <= len(cmd) {
			continue
		}
		pre := strings.TrimSpace(t[:len(t)-len(c)])
		if pre == "" || isPrompt(pre) {
			cmd, prompt, ok = c, pre, true
		}
	}
	return
}

func isPrompt(l string) bool {
	if l == "" || len(l) > 80 {
		return false
	}
	end := l[len(l)-1]
	return (end == '#' || end == '>') && !strings.ContainsAny(l[:len(l)-1], " \t#>")
}

func promptName(p string) string {
	p = strings.TrimRight(strings.TrimSpace(p), "#>")
	if i := strings.Index(p, "("); i > 0 {
		p = p[:i]
	}
	return p
}

// ---------------------------------------------------------------- MAC

var (
	macColon = regexp.MustCompile(`(?i)\b[0-9a-f]{2}(?:[:-][0-9a-f]{2}){5}\b`)
	macDot   = regexp.MustCompile(`(?i)\b[0-9a-f]{4}\.[0-9a-f]{4}\.[0-9a-f]{4}\b`)
	macHP    = regexp.MustCompile(`(?i)\b[0-9a-f]{6}-[0-9a-f]{6}\b`)
	macBare  = regexp.MustCompile(`(?i)^[0-9a-f]{12}$`)
	ipv4     = regexp.MustCompile(`\b(?:25[0-5]|2[0-4]\d|1?\d?\d)(?:\.(?:25[0-5]|2[0-4]\d|1?\d?\d)){3}\b`)
)

// NormMAC bringt jede Schreibweise auf 12 Kleinbuchstaben ohne Trenner
// ("" = keine MAC).
func NormMAC(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if macColon.MatchString(s) || macDot.MatchString(s) || macHP.MatchString(s) {
		s = strings.NewReplacer(":", "", "-", "", ".", "").Replace(s)
	}
	if !macBare.MatchString(s) {
		return ""
	}
	return s
}

// FormatMAC: aa:bb:cc:dd:ee:ff.
func FormatMAC(norm string) string {
	if len(norm) != 12 {
		return norm
	}
	var b strings.Builder
	for i := 0; i < 12; i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(norm[i : i+2])
	}
	return b.String()
}

// IsIP: sieht s wie eine IPv4-Adresse aus?
func IsIP(s string) bool {
	s = strings.TrimSpace(s)
	return ipv4.FindString(s) == s && s != ""
}

func findMAC(l string) (string, int) {
	for _, rx := range []*regexp.Regexp{macColon, macDot, macHP} {
		if loc := rx.FindStringIndex(l); loc != nil {
			return NormMAC(l[loc[0]:loc[1]]), loc[0]
		}
	}
	return "", -1
}

// MACEntry: eine Zeile der MAC-Tabelle.
type MACEntry struct {
	MAC  string // normiert
	Port string
	VLAN int
}

var macType = map[string]bool{"dynamic": true, "static": true, "learned": true, "secure": true, "sticky": true, "permanent": true, "self": true, "port-access": true, "delete-on-timeout": true}

// ParseMACTable liest MAC-Tabellen aller drei Familien:
//
//	ProCurve: 001122-334455   24     1
//	CX:       00:11:22:33:44:55   10   dynamic   1/1/24
//	Cisco:      10    0011.2233.4455    DYNAMIC     Gi1/0/24
func ParseMACTable(text string) []MACEntry {
	var out []MACEntry
	for _, l := range strings.Split(text, "\n") {
		mac, _ := findMAC(l)
		if mac == "" {
			continue
		}
		f := strings.Fields(l)
		pos := -1
		for i, x := range f {
			if NormMAC(x) == mac {
				pos = i
				break
			}
		}
		if pos < 0 || len(f) < 3 {
			continue
		}
		typed := false
		for _, x := range f {
			if macType[strings.ToLower(x)] {
				typed = true
			}
		}
		var e MACEntry
		e.MAC = mac
		switch {
		case pos == 0 && typed: // CX
			e.VLAN, _ = strconv.Atoi(f[1])
			e.Port = f[len(f)-1]
		case pos == 0: // ProCurve
			e.Port = f[1]
			e.VLAN, _ = strconv.Atoi(f[2])
		case pos == 1: // Cisco
			v, err := strconv.Atoi(f[0])
			if err != nil {
				continue
			}
			e.VLAN = v
			e.Port = f[len(f)-1]
		default:
			continue
		}
		if e.Port == "" || macType[strings.ToLower(e.Port)] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// ParseARP: IP → MAC aus „show arp“ / „show ip arp“ (jede Zeile mit beidem).
func ParseARP(text string) map[string]string {
	out := map[string]string{}
	for _, l := range strings.Split(text, "\n") {
		ip := ipv4.FindString(l)
		mac, _ := findMAC(l)
		if ip != "" && mac != "" {
			out[ip] = mac
		}
	}
	return out
}

// ---------------------------------------------------------------- LLDP / CDP

// Neighbor: ein Nachbar an einem Port.
type Neighbor struct {
	LocalPort  string
	RemoteName string
	RemotePort string
}

// ParseNeighbors liest die Nachbar-Tabellen:
//
//	ProCurve „show lldp info remote-device“:  24 | 00 11 22 33 44 55  25  25  sw-core
//	CX „show lldp neighbor-info“:              1/1/1  08:97:34:aa:bb:cc  1/1/49  1/1/49  120  core1
//	Cisco „show lldp neighbors“:               sw2  Gi1/0/1  120  B,R  Gi1/0/2
//	Cisco „show cdp neighbors“:                sw2.lab  Gig 1/0/1  150  S I  WS-C3750  Gig 1/0/2
func ParseNeighbors(text string) []Neighbor {
	var out []Neighbor
	mode, pending := "", ""
	for _, l := range strings.Split(text, "\n") {
		t := strings.TrimSpace(l)
		low := strings.ToLower(t)
		switch {
		case strings.HasPrefix(low, "localport") && strings.Contains(t, "|"):
			mode = "procurve"
			continue
		case strings.HasPrefix(low, "local-port") || strings.HasPrefix(low, "local port") && strings.Contains(low, "chassis"):
			mode = "cx"
			continue
		case strings.HasPrefix(low, "device id") && strings.Contains(low, "local intrfce"):
			mode = "cdp"
			continue
		case strings.HasPrefix(low, "device id") && strings.Contains(low, "local intf"):
			mode = "lldp-cisco"
			continue
		}
		if t == "" || strings.HasPrefix(t, "---") || strings.Contains(t, "-----") || isPrompt(t) {
			continue
		}
		if strings.HasPrefix(low, "total ") {
			mode = ""
			continue
		}
		var n Neighbor
		switch mode {
		case "procurve":
			i := strings.Index(t, "|")
			if i < 0 {
				continue
			}
			n.LocalPort = strings.TrimSpace(t[:i])
			f := strings.Fields(t[i+1:])
			if len(f) < 2 {
				continue
			}
			n.RemoteName = f[len(f)-1]
			if len(f) >= 4 {
				n.RemotePort = f[len(f)-3]
			}
		case "cx":
			f := strings.Fields(t)
			if len(f) < 4 {
				continue
			}
			n.LocalPort, n.RemotePort = f[0], f[2]
			if len(f) >= 6 {
				n.RemoteName = f[len(f)-1]
			}
		case "cdp", "lldp-cisco":
			f := joinIfNames(strings.Fields(t))
			if len(f) == 1 { // Name zu lang: Rest steht in der nächsten Zeile
				pending = f[0]
				continue
			}
			if pending != "" {
				f = append([]string{pending}, f...)
				pending = ""
			}
			if len(f) < 3 {
				continue
			}
			n.RemoteName, n.LocalPort, n.RemotePort = f[0], f[1], f[len(f)-1]
		default:
			continue
		}
		n.RemoteName = strings.Trim(n.RemoteName, `"`)
		if n.LocalPort == "" {
			continue
		}
		out = append(out, n)
	}
	return out
}

var ifPrefix = regexp.MustCompile(`(?i)^(gig|gi|ten|te|fas|fa|eth|et|twe|two|fiv|hun|for|port)$`)

// joinIfNames: „Gig 1/0/1“ → „Gig1/0/1“ (CDP trennt Typ und Nummer).
func joinIfNames(f []string) []string {
	var out []string
	for i := 0; i < len(f); i++ {
		if ifPrefix.MatchString(f[i]) && i+1 < len(f) && len(f[i+1]) > 0 && f[i+1][0] >= '0' && f[i+1][0] <= '9' {
			out = append(out, f[i]+f[i+1])
			i++
			continue
		}
		out = append(out, f[i])
	}
	return out
}

// ShortName: „sw2.lab.example.com“ → „sw2“, zum Vergleichen.
func ShortName(s string) string {
	s = strings.ToLower(strings.Trim(strings.TrimSpace(s), `"`))
	if IsIP(s) {
		return s
	}
	if i := strings.IndexByte(s, '.'); i > 0 {
		s = s[:i]
	}
	return s
}

// PagingOff: Befehle, die das Blättern abschalten (unbekannt = beide).
func PagingOff(vendor string) []string {
	switch vendor {
	case ProCurve, ArubaCX:
		return []string{"no page"}
	case Cisco:
		return []string{"terminal length 0"}
	}
	return []string{"no page", "terminal length 0"}
}
