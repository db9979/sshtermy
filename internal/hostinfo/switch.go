package hostinfo

import (
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
	}
	return "unbekannt"
}

// DetectVendor erkennt die Familie an der Ausgabe von "show version".
func DetectVendor(showVersion string) Vendor {
	l := strings.ToLower(showVersion)
	switch {
	case strings.Contains(l, "arubaos-cx") || strings.Contains(l, "aos-cx"):
		return VendorArubaCX
	case strings.Contains(l, "cisco ios") || strings.Contains(l, "cisco internetwork"):
		return VendorCisco
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
	}
	return ""
}

// InfoCommands: show-Befehle für die Übersicht (nach "show version").
func (v Vendor) InfoCommands() (system, ports, vlans string) {
	switch v {
	case VendorProCurve:
		return "show system", "show interfaces brief", "show vlans"
	case VendorArubaCX:
		return "show system", "show interface brief", "show vlan"
	case VendorCisco:
		return "", "show interfaces status", "show vlan brief"
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
	At        time.Time
}

// Summary: eine Zeile für die Sitzungsleiste.
func (s Switch) Summary() string {
	parts := []string{}
	if s.Firmware != "" {
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
)

// ParseSwitch wertet die Ausgaben aus (fehlende Befehle sind leer).
func ParseSwitch(v Vendor, version, system, ports, vlans string) Switch {
	s := Switch{Vendor: v, At: time.Now()}
	all := version + "\n" + system
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
	s.PortsUp, s.PortsDown = countPorts(ports)
	s.VLANs = parseVLANs(vlans)
	return s
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
