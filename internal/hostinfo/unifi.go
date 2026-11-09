package hostinfo

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Ubiquiti-UniFi-Switch: Nach der Anmeldung steht man in einer BusyBox-Shell.
// „info“ nennt Modell, Firmware, Name und Laufzeit, „swctrl port show“ die
// Ports, „free“ und „top“ Speicher und CPU. Die Konfiguration verteilt der
// UniFi-Controller; /tmp/system.cfg ist die gerade gültige Fassung.

var (
	unifiInfo    = regexp.MustCompile(`(?im)^\s*model:\s+US[^Gg\s][\w-]*\s*$|^\s*status:\s+.*?/inform\b`)
	unifiUptime  = regexp.MustCompile(`(?i)^(\d+)\s*(?:seconds?|s)?$`)
	unifiTopIdle = regexp.MustCompile(`(?im)^\s*CPU:.*?(\d{1,3})(?:\.\d+)?%\s+idle`)
)

func parseUniFi(s *Switch, info, system, ports string) {
	v := kv(info)
	if m := v["model"]; m != "" {
		s.Model = m
	}
	if f := strings.Fields(v["version"] + " "); len(f) > 0 {
		s.Firmware = f[0]
	}
	if h := v["hostname"]; h != "" {
		s.Name = h
	}
	if u := v["uptime"]; u != "" {
		s.Uptime = u
		if m := unifiUptime.FindStringSubmatch(strings.TrimSpace(u)); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				s.Uptime = secondsText(n)
			}
		}
	}
	s.MemPct = freeMemPct(system)
	if m := unifiTopIdle.FindStringSubmatch(system); m != nil {
		if p := pct(m[1]); p >= 0 {
			s.CPUPct = 100 - p
		}
	}
	s.PortsUp, s.PortsDown = countPorts(ports)
}

// freeMemPct wertet „free“ (BusyBox alt und neu) aus: belegt ohne Puffer und
// Cache bzw. total − available. -1 = nicht lesbar.
func freeMemPct(out string) int {
	var head []string
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) == 0 {
			continue
		}
		if f[0] == "total" {
			head = f
			continue
		}
		if f[0] != "Mem:" || head == nil || len(f)-1 < len(head) {
			continue
		}
		col := map[string]uint64{}
		for i, h := range head {
			n, _ := strconv.ParseUint(f[i+1], 10, 64)
			col[h] = n
		}
		tot := col["total"]
		if tot == 0 {
			return -1
		}
		var used uint64
		if av, ok := col["available"]; ok && av <= tot {
			used = tot - av
		} else {
			rest := col["free"] + col["buffers"] + col["cached"] + col["buff/cache"]
			if rest > tot {
				return -1
			}
			used = tot - rest
		}
		return int(used * 100 / tot)
	}
	return -1
}

// secondsText: 266460 → „3d 2h 1m“.
func secondsText(n int) string {
	d, h, m := n/86400, n%86400/3600, n%3600/60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	case h > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
