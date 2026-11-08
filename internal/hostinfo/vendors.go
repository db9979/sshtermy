package hostinfo

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Herstellerprofile Juniper, MikroTik und Fortinet: eigene Befehle und
// Ausgabeformate für die Switch-/Router-Übersicht (ParseSwitch).

var (
	kvLine       = regexp.MustCompile(`(?m)^\s*([\w .-]+?)\s*:\s*(.*?)\s*$`)
	percentWord  = regexp.MustCompile(`(?i)(\d{1,3})\s*percent`)
	junosPort    = regexp.MustCompile(`^\s*((?:ge|xe|et|mge|fe|ae)-[\d/]+)\s+(up|down)\s+(up|down)\b`)
	junosVLAN    = regexp.MustCompile(`^\s*(?:(\S+)\s+)?([\w.-]+)\s+(\d{1,4})\b`)
	rosFlags     = regexp.MustCompile(`^\s*\d+\s+([A-Z ]*?)\s*name=`)
	rosField     = regexp.MustCompile(`\b([\w-]+)=("[^"]*"|\S+)`)
	fortiCPUIdle = regexp.MustCompile(`(?i)cpu states:.*?(\d{1,3})%\s+idle`)
	fortiMem     = regexp.MustCompile(`(?i)memory:.*?\((\d{1,3})(?:\.\d+)?%\)`)
	fortiVersion = regexp.MustCompile(`(?im)^version\s*:\s*(\S+)\s+(v[\d.]+)`)
	fortiStatus  = regexp.MustCompile(`(?im)^\s*status\s*:\s*(up|down)\b`)
	sizeRe       = regexp.MustCompile(`^([\d.]+)\s*([KMGT]?i?B)?$`)
)

// kv: „schlüssel: wert“-Zeilen, Schlüssel klein und ohne Leerzeichen am Rand.
func kv(out string) map[string]string {
	m := map[string]string{}
	for _, x := range kvLine.FindAllStringSubmatch(out, -1) {
		k := strings.ToLower(strings.TrimSpace(x[1]))
		if _, ok := m[k]; !ok {
			m[k] = x[2]
		}
	}
	return m
}

func parseJunos(s *Switch, version, system, ports, vlans string) {
	v := kv(version)
	s.Name, s.Model = v["hostname"], v["model"]
	s.Firmware = v["junos"]
	if s.Firmware == "" {
		if m := regexp.MustCompile(`(?i)junos[^\[]*\[([^\]]+)\]`).FindStringSubmatch(version); m != nil {
			s.Firmware = m[1]
		}
	}
	for _, l := range strings.Split(system, "\n") {
		t := strings.ToLower(strings.TrimSpace(l))
		m := percentWord.FindStringSubmatch(t)
		switch {
		case m == nil:
			if strings.HasPrefix(t, "uptime") && s.Uptime == "" {
				s.Uptime = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "Uptime"))
			}
		case strings.HasPrefix(t, "memory utilization") && s.MemPct < 0:
			s.MemPct = pct(m[1])
		case strings.HasPrefix(t, "idle") && s.CPUPct < 0:
			if p := pct(m[1]); p >= 0 {
				s.CPUPct = 100 - p
			}
		}
	}
	for _, l := range strings.Split(ports, "\n") {
		m := junosPort.FindStringSubmatch(l)
		if m == nil || strings.Contains(m[1], ".") {
			continue
		}
		if m[3] == "up" {
			s.PortsUp++
		} else {
			s.PortsDown++
		}
	}
	seen := map[int]bool{}
	head := false
	for _, l := range strings.Split(vlans, "\n") {
		if strings.Contains(strings.ToLower(l), "tag") && strings.Contains(strings.ToLower(l), "name") {
			head = true
			continue
		}
		m := junosVLAN.FindStringSubmatch(l)
		if !head || m == nil {
			continue
		}
		id, _ := strconv.Atoi(m[3])
		if id < 1 || id > 4094 || seen[id] {
			continue
		}
		seen[id] = true
		s.VLANs = append(s.VLANs, VLAN{ID: id, Name: m[2]})
	}
	sortVLANs(s.VLANs)
}

func parseRouterOS(s *Switch, resource, identity, ports, vlans string) {
	r := kv(resource)
	s.Uptime, s.Model = r["uptime"], r["board-name"]
	s.Firmware = strings.Fields(r["version"] + " ")[0]
	if p := strings.TrimSuffix(r["cpu-load"], "%"); p != "" {
		s.CPUPct = pct(p)
	}
	if tot, free := bytesOf(r["total-memory"]), bytesOf(r["free-memory"]); tot > 0 && free <= tot {
		s.MemPct = int((tot - free) * 100 / tot)
	}
	s.Name = kv(identity)["name"]
	for _, l := range strings.Split(ports, "\n") {
		m := rosFlags.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		switch {
		case strings.Contains(m[1], "R"):
			s.PortsUp++
		default:
			s.PortsDown++
		}
	}
	seen := map[int]bool{}
	for _, l := range strings.Split(vlans, "\n") {
		f := map[string]string{}
		for _, x := range rosField.FindAllStringSubmatch(l, -1) {
			f[x[1]] = strings.Trim(x[2], `"`)
		}
		id, err := strconv.Atoi(f["vlan-id"])
		if err != nil || id < 1 || id > 4094 || seen[id] {
			continue
		}
		seen[id] = true
		s.VLANs = append(s.VLANs, VLAN{ID: id, Name: f["name"]})
	}
	sortVLANs(s.VLANs)
}

func parseFortiOS(s *Switch, status, perf, ports string) {
	v := kv(status)
	s.Name = v["hostname"]
	if m := fortiVersion.FindStringSubmatch(status); m != nil {
		s.Model, s.Firmware = m[1], strings.TrimSuffix(m[2], ",")
	}
	if m := fortiCPUIdle.FindStringSubmatch(perf); m != nil {
		if p := pct(m[1]); p >= 0 {
			s.CPUPct = 100 - p
		}
	}
	if m := fortiMem.FindStringSubmatch(perf); m != nil {
		s.MemPct = pct(m[1])
	}
	if u := kv(perf)["uptime"]; u != "" {
		s.Uptime = strings.Join(strings.Fields(u), " ")
	}
	for _, m := range fortiStatus.FindAllStringSubmatch(ports, -1) {
		if strings.EqualFold(m[1], "up") {
			s.PortsUp++
		} else {
			s.PortsDown++
		}
	}
}

// bytesOf: „820.5MiB“ → Bytes.
func bytesOf(v string) uint64 {
	m := sizeRe.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return 0
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	mult := map[string]float64{"": 1, "B": 1, "KiB": 1 << 10, "MiB": 1 << 20, "GiB": 1 << 30, "TiB": 1 << 40, "KB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12}[m[2]]
	return uint64(f * mult)
}

func sortVLANs(vs []VLAN) { sort.Slice(vs, func(i, j int) bool { return vs[i].ID < vs[j].ID }) }
