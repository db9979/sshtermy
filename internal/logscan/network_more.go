package logscan

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Weitere Log-Formate für Befehlssätze ohne eigenen Leser.

var (
	// Comware „%Nov 10 10:21:33:518 2026 H3C SHELL/5/SHELL_LOGIN: …“ und
	// Huawei „Nov 10 2026 10:21:33 HUAWEI %%01SHELL/5/CMDRECORD(s)[3]:…“.
	reVRP = regexp.MustCompile(`^%?([A-Z][a-z]{2})\s+(\d{1,2})\s+(?:\d{4}\s+)?(\d\d:\d\d:\d\d)(?::\d+)?(?:\s+\d{4})?(?:[+-]\d\d:\d\d)?\s+\S+\s+%*(?:\d\d)?([\w-]+)/(\d)/([\w-]+)(?:\([a-z]+\))?(?:\[\d+\])?:\s*(.*)$`)
	// Cisco Small Business: „%LINK-W-Down:  gi1/0/2“ (Buchstabe statt Zahl).
	reSBLog = regexp.MustCompile(`%([\w-]+?)-([EWNIACF])-([\w]+):\s*(.*)$`)
	// AireOS: „*emWeb: Nov 10 10:00:00.123: #CLI-3-LOGIN_FAILED: …“
	reAireOS = regexp.MustCompile(`#([\w-]+?)-(\d)-([\w]+):\s*(.*)$`)
	// ExtremeXOS: „11/10/2026 10:00:00.12 <Warn:AAA.authFail> Login failed …“
	reEXOS = regexp.MustCompile(`^(\d\d/\d\d/\d{4}\s+\d\d:\d\d:\d\d)(?:\.\d+)?\s+<(Crit|Erro|Warn|Noti|Info|Debu|Summ|Verb)[^:>]*:([\w.]+)>\s*(.*)$`)
	// PAN-OS: „2026/11/10 10:00:00 high general general 0 …“
	rePANLog = regexp.MustCompile(`^(\d{4}/\d\d/\d\d\s+\d\d:\d\d:\d\d)\s+(critical|high|medium|low|informational|info)\s+(\S+)\s+(\S+)\s+(.*)$`)
)

func vrpLine(line string, now time.Time) (Entry, bool) {
	m := reVRP.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return Entry{}, false
	}
	lv, _ := strconv.Atoi(m[5])
	e := Entry{Time: parseBSD(m[1]+" "+m[2]+" "+m[3], now), Sev: sevOfSyslog(lv), Source: m[4], ID: m[4] + "/" + m[6], Msg: m[7]}
	return e, true
}

// otherLine: Formate weiterer Hersteller im allgemeinen Leser.
func otherLine(line string, now time.Time) (Entry, bool) {
	if m := reSBLog.FindStringSubmatch(line); m != nil {
		sev := map[string]Sev{"E": SevErr, "W": SevWarn, "A": SevCrit, "C": SevCrit, "F": SevCrit}[m[2]]
		return Entry{Sev: sev, Source: m[1], ID: m[1] + "-" + m[3], Msg: m[4]}, true
	}
	if m := reAireOS.FindStringSubmatch(line); m != nil {
		lv, _ := strconv.Atoi(m[2])
		return Entry{Sev: sevOfSyslog(lv), Source: m[1], ID: m[1] + "-" + m[3], Msg: m[4]}, true
	}
	if m := reEXOS.FindStringSubmatch(line); m != nil {
		e := Entry{Source: m[3], Msg: m[4]}
		e.Sev = map[string]Sev{"Crit": SevCrit, "Erro": SevErr, "Warn": SevWarn}[m[2]]
		if t, err := time.ParseInLocation("01/02/2006 15:04:05", m[1], time.Local); err == nil {
			e.Time = t
		}
		return e, true
	}
	if m := rePANLog.FindStringSubmatch(line); m != nil {
		e := Entry{Source: m[3], Msg: m[5]}
		e.Sev = map[string]Sev{"critical": SevCrit, "high": SevErr, "medium": SevWarn}[m[2]]
		if t, err := time.ParseInLocation("2006/01/02 15:04:05", m[1], time.Local); err == nil {
			e.Time = t
		}
		return e, true
	}
	return Entry{}, false
}
