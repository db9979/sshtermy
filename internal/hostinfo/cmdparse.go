package hostinfo

import (
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/db9979/sshtermy/internal/cmdset"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Muster aus Befehlssätzen: einmal übersetzt, dann wiederverwendet.
var (
	rxMu    sync.Mutex
	rxCache = map[string]*regexp.Regexp{}
)

func rxOf(p string) *regexp.Regexp {
	if p == "" {
		return nil
	}
	rxMu.Lock()
	defer rxMu.Unlock()
	if rx, ok := rxCache[p]; ok {
		return rx
	}
	rx, err := regexp.Compile(p)
	if err != nil {
		rx = nil
	}
	if len(rxCache) > 2000 {
		rxCache = map[string]*regexp.Regexp{}
	}
	rxCache[p] = rx
	return rx
}

// firstGroup: erste nicht leere Klammer des ersten Treffers (ohne Klammer:
// der ganze Treffer).
func firstGroup(p, text string) (string, bool) {
	rx := rxOf(p)
	if rx == nil {
		return "", false
	}
	m := rx.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	if len(m) == 1 {
		return strings.TrimSpace(m[0]), true
	}
	for _, g := range m[1:] {
		if g = strings.TrimSpace(g); g != "" {
			return g, true
		}
	}
	return "", false
}

// named: benannte Klammern eines Treffers.
func named(rx *regexp.Regexp, text string) map[string]string {
	m := rx.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	out := map[string]string{}
	for i, n := range rx.SubexpNames() {
		if i == 0 {
			continue
		}
		if n == "" {
			n = "#" + strconv.Itoa(i)
		}
		if _, ok := out[n]; !ok || out[n] == "" {
			out[n] = strings.TrimSpace(m[i])
		}
	}
	return out
}

func fnum(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
	return f, err == nil
}

func pctOf(f float64) int {
	if f < 0 || f > 100 {
		return -1
	}
	return int(f)
}

// cpuFrom: cpu-Muster; (?P<idle>…) = frei, sonst erste Klammer = belegt.
func cpuFrom(p, text string) int {
	rx := rxOf(p)
	if rx == nil {
		return -1
	}
	g := named(rx, text)
	if g == nil {
		return -1
	}
	if v, ok := fnum(g["idle"]); ok && g["idle"] != "" {
		return pctOf(100 - v)
	}
	if v, ok := fnum(firstNonEmpty(g)); ok {
		return pctOf(v)
	}
	return -1
}

// memFrom: mem-Muster; used/free mit total, sonst erste Klammer = belegt %.
func memFrom(p, text string) int {
	rx := rxOf(p)
	if rx == nil {
		return -1
	}
	g := named(rx, text)
	if g == nil {
		return -1
	}
	tot, okT := fnum(g["total"])
	if okT && tot > 0 {
		if u, ok := fnum(g["used"]); ok && g["used"] != "" && u <= tot {
			return pctOf(u * 100 / tot)
		}
		if f, ok := fnum(g["free"]); ok && g["free"] != "" && f <= tot {
			return pctOf((tot - f) * 100 / tot)
		}
	}
	if v, ok := fnum(firstNonEmpty(g)); ok {
		return pctOf(v)
	}
	return -1
}

func firstNonEmpty(g map[string]string) string {
	for i := 1; i < 20; i++ {
		if v := g["#"+strconv.Itoa(i)]; v != "" {
			return v
		}
	}
	for k, v := range g {
		if k != "idle" && k != "total" && k != "used" && k != "free" && v != "" {
			return v
		}
	}
	return ""
}

// applyParse legt die Muster des Befehlssatzes über die gelesenen Werte.
func applyParse(s *Switch, p cmdset.Parse, all, ports, vlans string) {
	for _, f := range []struct {
		rx  string
		dst *string
	}{{p.Model, &s.Model}, {p.Firmware, &s.Firmware}, {p.Name, &s.Name}, {p.Uptime, &s.Uptime}} {
		if v, ok := firstGroup(f.rx, all); ok {
			*f.dst = cleanField(v)
		}
	}
	if p.CPU != "" {
		if v := cpuFrom(p.CPU, all); v >= 0 {
			s.CPUPct = v
		}
	}
	if p.Mem != "" {
		if v := memFrom(p.Mem, all); v >= 0 {
			s.MemPct = v
		}
	}
	if up, down := rxOf(p.PortUp), rxOf(p.PortDown); up != nil || down != nil {
		s.PortsUp, s.PortsDown = 0, 0
		for _, l := range strings.Split(strings.ReplaceAll(ports, "\r", ""), "\n") {
			switch {
			case up != nil && up.MatchString(l):
				s.PortsUp++
			case down != nil && down.MatchString(l):
				s.PortsDown++
			}
		}
	}
	if rx := rxOf(p.VLAN); rx != nil {
		s.VLANs = nil
		seen := map[int]bool{}
		for _, l := range strings.Split(strings.ReplaceAll(vlans, "\r", ""), "\n") {
			g := named(rx, l)
			if g == nil {
				continue
			}
			idS, name := g["id"], g["name"]
			if idS == "" {
				idS, name = g["#1"], g["#2"]
			}
			id, err := strconv.Atoi(idS)
			if err != nil || id < 1 || id > 4094 || seen[id] {
				continue
			}
			seen[id] = true
			s.VLANs = append(s.VLANs, VLAN{ID: id, Name: name})
		}
		sortVLANs(s.VLANs)
	}
}

// ExtraValue: Wert einer Zusatzzeile aus der Ausgabe ihres Befehls.
func ExtraValue(e cmdset.Extra, out string) string {
	out = strings.ReplaceAll(out, "\r", "")
	switch {
	case e.Count != "":
		rx := rxOf(e.Count)
		if rx == nil {
			return ""
		}
		return strconv.Itoa(len(rx.FindAllStringIndex(out, -1)))
	case e.Pick != "":
		v, _ := firstGroup(e.Pick, out)
		return v
	}
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			if len([]rune(l)) > 60 {
				l = string([]rune(l)[:60]) + "…"
			}
			return l
		}
	}
	return ""
}

// ExtraLabel: Beschriftung in der aktiven Sprache.
func ExtraLabel(e cmdset.Extra) string {
	if i18n.English() && e.LabelEN != "" {
		return e.LabelEN
	}
	return e.Label
}
