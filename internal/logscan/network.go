package logscan

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/cmdset"
	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
)

// NetCommands: lesende Befehle für Switches und Firewalls aus dem
// Befehlssatz. logs liefern die Meldungen, ports (falls vorhanden) den
// Zustand der Ports für die Leiste. FortiOS: „execute log filter“ setzt nur
// den Anzeigefilter dieser CLI-Sitzung, nichts an der Konfiguration.
func NetCommands(v hostinfo.Vendor) (logs []string, ports string) {
	l := hostinfo.SetOf(v, "", "").Logs
	if len(l.Commands) == 0 {
		return []string{"show logging"}, l.Ports
	}
	return l.Commands, l.Ports
}

// ParseNetwork liest die Ausgaben der Log-Befehle (in der Reihenfolge von
// NetCommands) und optional die Port-Übersicht.
func ParseNetwork(v hostinfo.Vendor, outs []string, ports string, hours int, now time.Time) *Report {
	r := &Report{Kind: KindNetwork, Hours: hours, At: now}
	logs, pc := NetCommands(v)
	reader := hostinfo.SetOf(v, "", "").Logs.Reader
	for _, c := range logs {
		if !strings.HasPrefix(c, "execute log filter") {
			r.Read = append(r.Read, c)
		}
	}
	if pc != "" && ports != "" {
		r.Read = append(r.Read, pc)
	}
	since := r.Since()
	for _, out := range outs {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimRight(line, "\r ")
			if strings.TrimSpace(line) == "" {
				continue
			}
			e, ok := netLine(reader, line, now)
			if !ok {
				continue
			}
			r.Lines++
			if !e.Time.IsZero() && e.Time.Before(since) {
				continue
			}
			if isConfigChange(e.ID + " " + e.Msg) {
				r.Marks = append(r.Marks, Mark{Time: e.Time, Kind: MarkConfig, Text: i18n.T("Konfiguration geändert")})
			}
			r.add(e)
		}
	}
	r.Ports = parsePorts(ports)
	sortEntries(r.Entries)
	Group(r)
	return r
}

// CountLines: so viele Zeilen erkennt der Leser (für den Selbsttest).
func CountLines(reader, out string, now time.Time) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r ")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if _, ok := netLine(reader, line, now); ok {
			n++
		}
	}
	return n
}

var (
	reProCurve = regexp.MustCompile(`^\s*([IWMEDP])\s+(\d\d/\d\d/\d\d)\s+(\d\d:\d\d:\d\d)\s+\d+\s+([\w-]+):\s+(.*)$`)
	reCXLog    = regexp.MustCompile(`^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:?\d\d)?)\s+\S+\s+([^\s:\[]+)(?:\[\d+\])?:\s*(.*)$`)
	reCXLevel  = regexp.MustCompile(`LOG_(EMER|ALERT|CRIT|ERR|WARN|NOTICE|INFO|DEBUG)`)
	reCisco    = regexp.MustCompile(`^(?:\d+:\s*)?[*.]?([A-Z][a-z]{2}\s+\d{1,2}\s+(?:\d{4}\s+)?\d\d:\d\d:\d\d)(?:\.\d+)?(?:\s+\w{2,5})?:\s*%([\w-]+?)-(\d)-([\w]+):\s*(.*)$`)
	reCiscoAny = regexp.MustCompile(`%([\w-]+?)-(\d)-([\w]+):\s*(.*)$`)
	reROS      = regexp.MustCompile(`^\s*(?:(\d{4}-\d\d-\d\d)\s+|([a-z]{3}/\d\d(?:/\d{4})?)\s+)?(\d\d:\d\d:\d\d)\s+([\w,]+)\s+(.*)$`)
	reFortiKV  = regexp.MustCompile(`(\w+)=("(?:[^"\\]|\\.)*"|\S+)`)
	reBusybox  = regexp.MustCompile(`^([A-Z][a-z]{2}\s+\d{1,2}\s+\d\d:\d\d:\d\d)\s+(?:\S+\s+)?(\w+)\.(\w+)\s+([^\s:\[]+)(?:\[\d+\])?:\s*(.*)$`)
)

func netLine(reader, line string, now time.Time) (Entry, bool) {
	switch reader {
	case "procurve":
		if m := reProCurve.FindStringSubmatch(line); m != nil {
			e := Entry{Source: m[4], Msg: m[5]}
			switch m[1] {
			case "W":
				e.Sev = SevWarn
			case "M", "E":
				e.Sev = SevErr
			case "D":
				return e, false
			}
			if t, err := time.ParseInLocation("01/02/06 15:04:05", m[2]+" "+m[3], time.Local); err == nil && t.Year() >= 2010 {
				e.Time = t // ohne Zeitabgleich zählt der Switch ab 1990: dann unbekannt
			}
			return e, true
		}
		return Entry{}, false
	case "arubaos-cx":
		if m := reCXLog.FindStringSubmatch(line); m != nil {
			e := Entry{Time: parseISO(m[1]), Source: m[2], Msg: m[3]}
			e.Sev = sevOfWords(e.Msg)
			if l := reCXLevel.FindStringSubmatch(m[3]); l != nil {
				e.Sev = map[string]Sev{"EMER": SevCrit, "ALERT": SevCrit, "CRIT": SevCrit, "ERR": SevErr, "WARN": SevWarn}[l[1]]
			}
			if i := strings.LastIndex(e.Msg, "|"); i >= 0 && strings.Contains(e.Msg, "Event|") {
				e.Msg = strings.TrimSpace(e.Msg[i+1:])
			}
			return e, true
		}
		return Entry{}, false
	case "routeros":
		if m := reROS.FindStringSubmatch(line); m != nil && strings.Contains(m[4], ",") || m != nil && rosTopic(m[4]) {
			e := Entry{Source: m[4], Msg: m[5], Sev: SevInfo}
			switch {
			case strings.Contains(m[4], "critical"):
				e.Sev = SevCrit
			case strings.Contains(m[4], "error"):
				e.Sev = SevErr
			case strings.Contains(m[4], "warning"):
				e.Sev = SevWarn
			}
			e.Time = rosTime(m[1], m[2], m[3], now)
			return e, true
		}
		return Entry{}, false
	case "fortios":
		if !strings.Contains(line, "logid=") && !strings.Contains(line, "date=") {
			return Entry{}, false
		}
		kv := map[string]string{}
		for _, m := range reFortiKV.FindAllStringSubmatch(line, -1) {
			kv[m[1]] = strings.Trim(m[2], `"`)
		}
		e := Entry{Source: kv["subtype"], ID: kv["logid"]}
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", kv["date"]+" "+kv["time"], time.Local); err == nil {
			e.Time = t
		}
		switch kv["level"] {
		case "emergency", "alert", "critical":
			e.Sev = SevCrit
		case "error":
			e.Sev = SevErr
		case "warning":
			e.Sev = SevWarn
		}
		msg := kv["logdesc"]
		if x := kv["msg"]; x != "" && x != msg {
			msg = strings.TrimSpace(msg + ": " + x)
		}
		for _, k := range []string{"vpntunnel", "user", "srcip", "ui", "status"} {
			if kv[k] != "" {
				msg += " " + k + "=" + kv[k]
			}
		}
		e.Msg = strings.TrimSpace(msg)
		return e, e.Msg != ""
	case "vrp":
		if e, ok := vrpLine(line, now); ok {
			return e, true
		}
		return Entry{}, false
	case "busybox":
		if m := reBusybox.FindStringSubmatch(line); m != nil {
			e := Entry{Time: parseBSD(m[1], now), Source: m[4], Msg: m[5]}
			e.Sev = map[string]Sev{"emerg": SevCrit, "alert": SevCrit, "crit": SevCrit, "err": SevErr, "warn": SevWarn, "warning": SevWarn}[m[3]]
			return e, true
		}
		if e, ok := syslogLine(line, now); ok {
			return e, true
		}
		return Entry{}, false
	}
	// Cisco IOS und alles mit %FAC-SEV-MNEMONIC; sonst Syslog (Junos).
	if m := reCisco.FindStringSubmatch(line); m != nil {
		lv, _ := strconv.Atoi(m[3])
		e := Entry{Time: parseBSD(dropYear(m[1]), now), Sev: sevOfSyslog(lv), Source: m[2], ID: m[2] + "-" + m[4], Msg: m[5]}
		return e, true
	}
	if m := reCiscoAny.FindStringSubmatch(line); m != nil {
		lv, _ := strconv.Atoi(m[2])
		return Entry{Sev: sevOfSyslog(lv), Source: m[1], ID: m[1] + "-" + m[3], Msg: m[4]}, true
	}
	if e, ok := otherLine(line, now); ok {
		return e, true
	}
	if e, ok := syslogLine(line, now); ok {
		if reader == "junos" {
			e.Sev = max(e.Sev, junosSev(e.Msg))
		}
		return e, true
	}
	return Entry{}, false
}

func rosTopic(t string) bool {
	for _, w := range []string{"system", "interface", "info", "error", "warning", "critical", "firewall", "dhcp", "wireless", "account", "script", "ipsec", "ospf", "bgp"} {
		if strings.HasPrefix(t, w) {
			return true
		}
	}
	return false
}

func rosTime(date, short, clock string, now time.Time) time.Time {
	switch {
	case date != "":
		t, _ := time.ParseInLocation("2006-01-02 15:04:05", date+" "+clock, time.Local)
		return t
	case short != "":
		if strings.Count(short, "/") == 2 {
			t, _ := time.ParseInLocation("Jan/02/2006 15:04:05", strings.ToUpper(short[:1])+short[1:]+" "+clock, time.Local)
			return t
		}
		return parseBSD(strings.ToUpper(short[:1])+short[1:3]+" "+short[4:]+" "+clock, now)
	}
	t, err := time.ParseInLocation("15:04:05", clock, time.Local)
	if err != nil {
		return time.Time{}
	}
	t = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local)
	if t.After(now.Add(time.Minute)) {
		t = t.AddDate(0, 0, -1)
	}
	return t
}

var reYearIn = regexp.MustCompile(`\s\d{4}\s`)

func dropYear(s string) string { return reYearIn.ReplaceAllString(s, " ") }

func junosSev(msg string) Sev {
	switch {
	case strings.Contains(msg, "SNMP_TRAP_LINK_DOWN"), strings.Contains(msg, "LOGIN_FAILED"), strings.Contains(msg, "SSHD_LOGIN_FAILED"):
		return SevWarn
	case strings.Contains(msg, "CHASSISD") && strings.Contains(strings.ToLower(msg), "fail"):
		return SevErr
	}
	return SevInfo
}

var reConfigChange = regexp.MustCompile(`(?i)CONFIG_I\b|UI_COMMIT\b|configuration (?:changed|saved|committed)|config(?:uration)? change|write memory|running-config.*changed|logdesc="?Configuration changed`)

func isConfigChange(msg string) bool { return reConfigChange.MatchString(msg) }

// parsePorts: Ports aus „show interfaces brief“ (ProCurve/CX), „show
// interfaces status“ (Cisco) oder „show interfaces terse“ (Junos).
func parsePorts(out string) []Port {
	var ps []Port
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(strings.ReplaceAll(line, "|", " "))
		if len(f) < 3 || !portLike(f[0]) || seen[f[0]] {
			continue
		}
		low := " " + strings.ToLower(strings.Join(f[1:], " ")) + " "
		p := Port{Name: f[0]}
		switch {
		case strings.Contains(low, " disabled ") || strings.Contains(low, " err-disabled "):
			p.Off = true
		case strings.Contains(low, " up ") || strings.Contains(low, " connected "):
			p.Up = true
		case strings.Contains(low, " down ") || strings.Contains(low, " notconnect "):
		default:
			continue
		}
		if strings.Contains(f[0], ".") { // Junos: logische Einheit ge-0/0/1.0
			continue
		}
		seen[f[0]] = true
		ps = append(ps, p)
		if len(ps) >= 128 {
			break
		}
	}
	return ps
}

var rePortName = regexp.MustCompile(`^(?:[A-Za-z]{1,3}\d|\d)[\d/:.]*$|^(?:Gi|Te|Fa|Tw|Fo|Hu|Eth|ge-|xe-|et-)[\w/:-]+$`)

func portLike(s string) bool { return rePortName.MatchString(s) && len(s) <= 16 }

// SyslogAbort: Befehle, die eine abgebrochene Einrichtung verwerfen und den
// Konfigurationsmodus verlassen (aus dem Befehlssatz).
func SyslogAbort(v hostinfo.Vendor) []string {
	return hostinfo.SetOf(v, "", "").SyslogAbort()
}

// SyslogSetup: Befehle aus dem Befehlssatz, mit denen das Gerät Syslog an
// ip:port schickt. Leer = per CLI nicht einstellbar (dann SyslogNote).
// Gespeichert wird nichts außer bei Junos (commit); write memory macht der
// Benutzer selbst.
func SyslogSetup(v hostinfo.Vendor, ip string, port int) []string {
	return SyslogSetupWith(hostinfo.SetOf(v, "", ""), ip, port)
}

// SyslogSetupWith wie SyslogSetup mit einem (aufgelösten) Befehlssatz.
func SyslogSetupWith(set *cmdset.Set, ip string, port int) []string {
	out, err := cmdset.SyslogSteps(set.Syslog.Steps, ip, port)
	if err != nil {
		return nil
	}
	return out
}
