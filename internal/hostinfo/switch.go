package hostinfo

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/cmdset"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Vendor: Befehlssatz des Netzwerkgeräts (Kennung aus internal/cmdset),
// erkannt an "show version" oder aus dem Systemprofil.
type Vendor string

// Die Hersteller, die der Code selbst kennt (eigene Leser). Alle anderen
// kommen nur aus den Befehlssatz-Dateien.
const (
	VendorUnknown  Vendor = ""
	VendorProCurve Vendor = "procurve"   // ArubaOS-Switch / HP ProCurve
	VendorArubaCX  Vendor = "arubaos-cx" // ArubaOS-CX
	VendorCisco    Vendor = "cisco-ios"  // IOS / IOS-XE
	VendorJunos    Vendor = "junos"      // Juniper EX/SRX/MX
	VendorMikroTik Vendor = "routeros"   // MikroTik RouterOS
	VendorFortiOS  Vendor = "fortios"    // Fortinet FortiGate
	VendorUniFi    Vendor = "unifi"      // Ubiquiti UniFi-Switch (BusyBox-Shell)
)

// SetOf: wirksamer Befehlssatz für ein Gerät (mit Variante für Modell und
// Firmware). Unbekannt ⇒ der allgemeine Satz „network“.
func SetOf(v Vendor, model, firmware string) *cmdset.Set {
	if s := cmdset.Resolve(string(v), model, firmware); s != nil {
		return s
	}
	if s := cmdset.Resolve("network", "", ""); s != nil {
		return s
	}
	return &cmdset.Set{ID: "network", Commands: cmdset.Commands{Version: "show version", Config: "show running-config"}}
}

// Name für die Anzeige.
func (v Vendor) Name() string {
	if s, ok := cmdset.Lookup(string(v)); ok && v != "" && s.ID != "network" {
		return s.DisplayName()
	}
	return i18n.T("unbekannt")
}

// DetectVendor erkennt den Befehlssatz an der Ausgabe von "show version".
func DetectVendor(showVersion string) Vendor {
	if s := cmdset.DetectVersion(showVersion); s != nil && s.EffFamily() == cmdset.FamilyNetwork && s.ID != "network" {
		return Vendor(s.ID)
	}
	return VendorUnknown
}

// PagingOff: Befehl, der das Blättern (-- MORE --) für diese Sitzung
// abschaltet. Wirkt nur bis zum Abmelden, ändert keine Konfiguration.
// Ohne Befehl beantwortet shellRun „--More--“ selbst.
func (v Vendor) PagingOff() string { return SetOf(v, "", "").Commands.PagingOff }

// VersionCommand: Befehl, der Modell und Firmware nennt.
func (v Vendor) VersionCommand() string {
	if c := SetOf(v, "", "").Commands.Version; c != "" {
		return c
	}
	return "show version"
}

// ConfigCommand: Befehl, der die laufende Konfiguration ausgibt (für die
// Konfigurationssicherung). Ändert nichts am Gerät. "" = unbekannt.
func (v Vendor) ConfigCommand() string { return SetOf(v, "", "").Commands.Config }

// InfoCommands: Befehle für die Übersicht (nach "show version").
func (v Vendor) InfoCommands() (system, ports, vlans string) {
	c := SetOf(v, "", "").Commands
	return c.System, c.Ports, c.VLANs
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
	Extras    []Extra
	At        time.Time
}

// Extra: Zusatzzeile aus dem Befehlssatz (z. B. „APs: 12“).
type Extra struct {
	Label, Value string
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
	return ParseWith(SetOf(v, "", ""), version, system, ports, vlans)
}

// ParseWith liest die Ausgaben mit einem Befehlssatz: erst der allgemeine
// Leser, dann ein eingebauter Herstellerleser, dann die Muster des Satzes.
func ParseWith(set *cmdset.Set, version, system, ports, vlans string) Switch {
	v := Vendor(set.ID)
	if set.ID == "network" {
		v = VendorUnknown
	}
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
	switch set.Parse.Reader {
	case "junos":
		parseJunos(&s, version, system, ports, vlans)
	case "routeros":
		parseRouterOS(&s, version, system, ports, vlans)
	case "fortios":
		parseFortiOS(&s, version, system, ports)
	case "unifi":
		parseUniFi(&s, version, system, ports)
	default:
		s.PortsUp, s.PortsDown = countPorts(ports)
		s.VLANs = parseVLANs(vlans)
	}
	applyParse(&s, set.Parse, all, ports, vlans)
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
