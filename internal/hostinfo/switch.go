package hostinfo

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Vendor: Familie des Netzwerkgeräts, erkannt an "show version".
type Vendor string

const (
	VendorUnknown  Vendor = ""
	VendorProCurve Vendor = "procurve" // ArubaOS-Switch / HP ProCurve
	VendorArubaCX  Vendor = "aruba-cx" // ArubaOS-CX
	VendorCisco    Vendor = "cisco"    // IOS / IOS-XE
	VendorJunos    Vendor = "junos"    // Juniper EX/SRX/MX
	VendorMikroTik Vendor = "routeros" // MikroTik RouterOS
	VendorFortiOS  Vendor = "fortios"  // Fortinet FortiGate
	VendorUniFi    Vendor = "unifi"    // Ubiquiti UniFi-Switch (BusyBox-Shell)
)

// Name für die Anzeige.
func (v Vendor) Name() string {
	switch v {
	case VendorProCurve:
		return "ArubaOS-Switch (ProCurve)"
	case VendorArubaCX:
		return "ArubaOS-CX"
	case VendorCisco:
		return "Cisco IOS"
	case VendorJunos:
		return "Juniper Junos"
	case VendorMikroTik:
		return "MikroTik RouterOS"
	case VendorFortiOS:
		return "FortiOS"
	case VendorUniFi:
		return "UniFi-Switch (Ubiquiti)"
	}
	return "unbekannt"
}

// DetectVendor erkennt die Familie an der Ausgabe von "show version".
func DetectVendor(showVersion string) Vendor {
	l := strings.ToLower(showVersion)
	switch {
	case unifiInfo.MatchString(showVersion):
		return VendorUniFi
	case strings.Contains(l, "arubaos-cx") || strings.Contains(l, "aos-cx"):
		return VendorArubaCX
	case strings.Contains(l, "cisco ios") || strings.Contains(l, "cisco internetwork"):
		return VendorCisco
	case strings.Contains(l, "junos"):
		return VendorJunos
	case strings.Contains(l, "routeros") || strings.Contains(l, "mikrotik") || strings.Contains(l, "board-name:"):
		return VendorMikroTik
	case strings.Contains(l, "fortigate") || strings.Contains(l, "fortios") || strings.Contains(l, "fortiswitch"):
		return VendorFortiOS
	case strings.Contains(l, "image stamp") || strings.Contains(l, "boot image") || procurveRev.MatchString(showVersion):
		return VendorProCurve
	}
	return VendorUnknown
}

// PagingOff: Befehl, der das Blättern (-- MORE --) für diese Sitzung
// abschaltet. Wirkt nur bis zum Abmelden, ändert keine Konfiguration.
func (v Vendor) PagingOff() string {
	switch v {
	case VendorProCurve, VendorArubaCX:
		return "no page"
	case VendorCisco:
		return "terminal length 0"
	case VendorJunos:
		return "set cli screen-length 0"
	}
	// RouterOS blättert bei „print terse“ nicht; FortiOS ginge nur über
	// eine Konfigurationsänderung (config system console) – dort beantwortet
	// shellRun „--More--“ selbst.
	return ""
}

// VersionCommand: Befehl, der Modell und Firmware nennt.
func (v Vendor) VersionCommand() string {
	switch v {
	case VendorMikroTik:
		return "/system resource print"
	case VendorFortiOS:
		return "get system status"
	case VendorUniFi:
		return "info"
	}
	return "show version"
}

// ConfigCommand: Befehl, der die laufende Konfiguration ausgibt (für die
// Konfigurationssicherung). Ändert nichts am Gerät.
func (v Vendor) ConfigCommand() string {
	switch v {
	case VendorJunos:
		return "show configuration | display set"
	case VendorMikroTik:
		return "/export"
	case VendorFortiOS:
		return "show full-configuration"
	case VendorUniFi:
		// vom Controller verteilte, gerade gültige Konfiguration
		return "cat /tmp/system.cfg"
	}
	return "show running-config"
}

// InfoCommands: show-Befehle für die Übersicht (nach "show version").
func (v Vendor) InfoCommands() (system, ports, vlans string) {
	switch v {
	case VendorProCurve:
		return "show system", "show interfaces brief", "show vlans"
	case VendorArubaCX:
		return "show system", "show interface brief", "show vlan"
	case VendorCisco:
		return "show processes cpu | include CPU utilization", "show interfaces status", "show vlan brief"
	case VendorJunos:
		return "show chassis routing-engine", "show interfaces terse", "show vlans brief"
	case VendorMikroTik:
		return "/system identity print", "/interface ethernet print terse", "/interface vlan print terse"
	case VendorFortiOS:
		return "get system performance status", "get system interface physical", ""
	case VendorUniFi:
		return "free; top -bn1 | head -n 3", "swctrl port show", ""
	}
	return "", "", ""
}

// VLAN: ID und Name.
type VLAN struct {
	ID   int
	Name string
}

// Switch: Kennzahlen eines Netzwerkgeräts.
type Switch struct {
	Vendor    Vendor
	Model     string
	Firmware  string
	Uptime    string
	Name      string
	PortsUp   int
	PortsDown int
	VLANs     []VLAN
	CPUPct    int // -1 = unbekannt
	MemPct    int // belegter Speicher in %, -1 = unbekannt
	At        time.Time
}

// Summary: eine Zeile für die Sitzungsleiste.
func (s Switch) Summary() string {
	parts := []string{}
	if s.CPUPct >= 0 {
		parts = append(parts, fmt.Sprintf("CPU %d%%", s.CPUPct))
	}
	if s.MemPct >= 0 {
		parts = append(parts, fmt.Sprintf(i18n.T("RAM %d%%"), s.MemPct))
	}
	if s.Firmware != "" && len(parts) == 0 {
		parts = append(parts, s.Firmware)
	}
	if s.PortsUp+s.PortsDown > 0 {
		parts = append(parts, strconv.Itoa(s.PortsUp)+"/"+strconv.Itoa(s.PortsUp+s.PortsDown)+i18n.T(" Ports aktiv"))
	}
	if len(s.VLANs) > 0 {
		parts = append(parts, strconv.Itoa(len(s.VLANs))+i18n.T(" VLANs"))
	}
	return strings.Join(parts, " · ")
}

var (
	procurveRev = regexp.MustCompile(`\b[A-Z]{1,3}\.\d{2}\.\d{2}\.\d{4}\b`) // YA.16.10.0009
	versionKV   = regexp.MustCompile(`(?im)^\s*(?:software revision|version)\s*:\s*(\S+)`)
	ciscoVer    = regexp.MustCompile(`(?i)cisco ios.*?version\s+([^\s,]+)`)
	uptimeRe    = regexp.MustCompile(`(?im)(?:up ?time\s*(?:is|:)\s*|uptime is\s+)(.+)$`)
	modelRe     = regexp.MustCompile(`(?im)^\s*(?:product name|model number|platform)\s*:\s*(.+)$`)
	ciscoModel  = regexp.MustCompile(`(?im)^cisco\s+(\S+)\s+\(.*\)\s+processor`)
	ciscoName   = regexp.MustCompile(`(?im)^(\S+)\s+uptime is\s+`)
	nameRe      = regexp.MustCompile(`(?im)^\s*(?:system name|hostname)\s*:\s*(.+)$`)
	procurveMdl = regexp.MustCompile(`(?m)^;\s*(\S+)\s+Configuration Editor`)
	portRow     = regexp.MustCompile(`^\s*((?:\d+/)*\d+|[A-Za-z]{1,2}\d+(?:/\d+)*|(?:Gi|Fa|Te|Tw|Fo|Hu|Et|Eth|Po)[A-Za-z]*\d+(?:/\d+)*|Trk\d+|lag\d+)\b`)
	upWord      = regexp.MustCompile(`(?i)\b(up|connected)\b`)
	downWord    = regexp.MustCompile(`(?i)\b(down|notconnect|disabled|err-disabled|inactive)\b`)
	vlanRow     = regexp.MustCompile(`^\s*(\d{1,4})\s+(\S+)`)
	dashLine    = regexp.MustCompile(`^[\s\-+|=]{5,}$`)
	cpuUtil     = regexp.MustCompile(`(?im)cpu\s+util(?:ization)?\s*(?:\(%\)|in %)?\s*:\s*(\d{1,3})\b`)
	ciscoCPU    = regexp.MustCompile(`(?i)cpu utilization for five seconds:\s*(\d{1,3})%`)
	memTotal    = regexp.MustCompile(`(?i)memory\s*-\s*total\s*:\s*([\d,.]+)`)
	memFree     = regexp.MustCompile(`(?i)\bfree\s*:\s*([\d,.]+)`)
	memUsage    = regexp.MustCompile(`(?im)memory\s+usage\s*(?:\(%\)|in %)?\s*:\s*(\d{1,3})\b`)
)

// ParseSwitch wertet die Ausgaben aus (fehlende Befehle sind leer).
func ParseSwitch(v Vendor, version, system, ports, vlans string) Switch {
	s := Switch{Vendor: v, At: time.Now(), CPUPct: -1, MemPct: -1}
	all := version + "\n" + system
	if m := cpuUtil.FindStringSubmatch(system); m != nil {
		s.CPUPct = pct(m[1])
	} else if m := ciscoCPU.FindStringSubmatch(system); m != nil {
		s.CPUPct = pct(m[1])
	}
	if m := memUsage.FindStringSubmatch(system); m != nil {
		s.MemPct = pct(m[1])
	} else if t, f := memTotal.FindStringSubmatch(system), memFree.FindStringSubmatch(system); t != nil && f != nil {
		tot, free := digits(t[1]), digits(f[1])
		if tot > 0 && free <= tot {
			s.MemPct = int((tot - free) * 100 / tot)
		}
	}
	if m := versionKV.FindStringSubmatch(all); m != nil {
		s.Firmware = m[1]
	} else if m := ciscoVer.FindStringSubmatch(version); m != nil {
		s.Firmware = m[1]
	} else if m := procurveRev.FindString(version); m != "" {
		s.Firmware = m
	}
	if m := uptimeRe.FindStringSubmatch(all); m != nil {
		s.Uptime = cleanField(m[1])
	}
	if m := modelRe.FindStringSubmatch(all); m != nil {
		s.Model = cleanField(m[1])
	} else if m := ciscoModel.FindStringSubmatch(version); m != nil {
		s.Model = m[1]
	}
	if m := nameRe.FindStringSubmatch(all); m != nil {
		s.Name = cleanField(m[1])
	} else if m := ciscoName.FindStringSubmatch(version); m != nil {
		s.Name = m[1]
	}
	switch v {
	case VendorJunos:
		parseJunos(&s, version, system, ports, vlans)
		return s
	case VendorMikroTik:
		parseRouterOS(&s, version, system, ports, vlans)
		return s
	case VendorFortiOS:
		parseFortiOS(&s, version, system, ports)
		return s
	case VendorUniFi:
		parseUniFi(&s, version, system, ports)
		return s
	}
	s.PortsUp, s.PortsDown = countPorts(ports)
	s.VLANs = parseVLANs(vlans)
	return s
}

func pct(v string) int {
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > 100 {
		return -1
	}
	return n
}

func digits(v string) uint64 {
	n, _ := strconv.ParseUint(strings.NewReplacer(",", "", ".", "").Replace(v), 10, 64)
	return n
}

// ModelFromConfig: ProCurve schreibt das Modell in den Kopf der Konfiguration
// ("; J9773A Configuration Editor; …").
func ModelFromConfig(cfg string) string {
	if m := procurveMdl.FindStringSubmatch(cfg); m != nil {
		return m[1]
	}
	return ""
}

// cleanField schneidet Folgespalten ab (ProCurve "show system" hat zwei
// Spalten je Zeile: "Up Time : 32 days      Memory - Total : …").
func cleanField(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.Index(v, "   "); i > 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

// countPorts zählt Tabellenzeilen, die mit einer Port-Bezeichnung beginnen.
func countPorts(out string) (up, down int) {
	for _, l := range strings.Split(out, "\n") {
		if !portRow.MatchString(l) {
			continue
		}
		rest := portRow.ReplaceAllString(l, "")
		// "Down" vor "Up" prüfen geht nicht pauschal (CX: "yes down"); maßgeblich
		// ist die erste Status-Angabe in der Zeile.
		ui, di := upWord.FindStringIndex(rest), downWord.FindStringIndex(rest)
		switch {
		case ui != nil && (di == nil || ui[0] < di[0]):
			up++
		case di != nil:
			down++
		}
	}
	return
}

// parseVLANs liest Zeilen "ID Name …" unterhalb einer Trennlinie.
func parseVLANs(out string) []VLAN {
	var res []VLAN
	seen := map[int]bool{}
	started := false
	for _, l := range strings.Split(out, "\n") {
		if dashLine.MatchString(l) {
			started = true
			continue
		}
		if !started {
			continue
		}
		m := vlanRow.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		id, _ := strconv.Atoi(m[1])
		if id < 1 || id > 4094 || seen[id] {
			continue
		}
		seen[id] = true
		res = append(res, VLAN{ID: id, Name: strings.Trim(m[2], "|")})
	}
	sort.Slice(res, func(i, j int) bool { return res[i].ID < res[j].ID })
	return res
}
