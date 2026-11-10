package logscan

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/hostinfo"
)

// Logwache: einzelne Zeilen aus einem laufenden Strom (journalctl -f,
// Syslog-Empfang, Windows-Abfrage) einordnen. Gleiche Schlüssel wie der
// Log-Blick, damit Grundlinie und Rauschen gelten.

// Classify ordnet eine einzelne Zeile einem Muster zu (ok = false: unter
// Warnung und kein bekanntes Muster).
func Classify(k Kind, e Entry) (Finding, bool) {
	f := classify(k, &e)
	if f == nil {
		return Finding{}, false
	}
	f.Count, f.First, f.Last, f.Sample = 1, e.Time, e.Time, e.Line()
	if f.rule != nil && f.rule.after != nil {
		f.rule.after(f)
	}
	f.rule = nil
	return *f, true
}

// UnixWatchScript folgt dem Journal ab jetzt (JSON, damit jede Zeile ihre
// Schwere mitbringt), ohne Journal den Syslog-Dateien. warn: auch Warnungen.
func UnixWatchScript(warn bool) string {
	p := "0..3"
	if warn {
		p = "0..4"
	}
	return `S=""; if [ "$(id -u)" != 0 ] && sudo -n true 2>/dev/null; then S="sudo -n"; fi
if command -v journalctl >/dev/null 2>&1; then
  $S journalctl -f -n 0 -q -p ` + p + ` -o json --output-fields=PRIORITY,SYSLOG_IDENTIFIER,_COMM,MESSAGE 2>/dev/null ||
  $S journalctl -f -n 0 -q -p ` + p + ` -o json
else
  for f in /var/log/syslog /var/log/messages /var/log/system.log; do [ -r "$f" ] && exec tail -n 0 -F "$f"; done
  for f in /var/log/syslog /var/log/messages; do [ -e "$f" ] && exec $S tail -n 0 -F "$f"; done
fi
`
}

type journalJSON struct {
	Time  string          `json:"__REALTIME_TIMESTAMP"`
	Prio  string          `json:"PRIORITY"`
	Ident string          `json:"SYSLOG_IDENTIFIER"`
	Comm  string          `json:"_COMM"`
	Msg   json.RawMessage `json:"MESSAGE"`
}

// UnixWatchLine liest eine Zeile aus UnixWatchScript (JSON vom Journal oder
// eine Syslog-Zeile).
func UnixWatchLine(line string, now time.Time) (Entry, bool) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "{") {
		var j journalJSON
		if json.Unmarshal([]byte(line), &j) != nil {
			return Entry{}, false
		}
		var msg string
		if json.Unmarshal(j.Msg, &msg) != nil || msg == "" {
			return Entry{}, false // Binärnachricht
		}
		e := Entry{Time: now, Source: j.Ident, Msg: strings.TrimSpace(msg), Sev: SevWarn}
		if e.Source == "" {
			e.Source = j.Comm
		}
		if us, err := strconv.ParseInt(j.Time, 10, 64); err == nil && us > 0 {
			e.Time = time.UnixMicro(us)
		}
		if p, err := strconv.Atoi(j.Prio); err == nil {
			e.Sev = sevOfSyslog(p)
		}
		return e, true
	}
	return syslogLine(line, now)
}

var rePRI = regexp.MustCompile(`^<(\d{1,3})>(?:\d \S+ )?`)

// SyslogPacket liest eine empfangene Syslog-Nachricht (RFC 3164 oder 5424).
// Geräte ohne Zeit oder Hostnamen im Paket bekommen die Empfangszeit.
func SyslogPacket(pkt string, now time.Time) (Entry, bool) {
	pkt = strings.TrimRight(strings.TrimSpace(pkt), "\x00")
	if pkt == "" {
		return Entry{}, false
	}
	if e, ok := syslogLine(pkt, now); ok {
		if e.Time.IsZero() || e.Time.After(now.Add(time.Hour)) || e.Time.Before(now.Add(-48*time.Hour)) {
			e.Time = now
		}
		return e, true
	}
	e := Entry{Time: now}
	rest := pkt
	if m := rePRI.FindStringSubmatch(pkt); m != nil {
		p, _ := strconv.Atoi(m[1])
		e.Sev = sevOfSyslog(p % 8)
		rest = pkt[len(m[0]):]
	} else {
		e.Sev = sevOfWords(pkt)
	}
	// FortiOS: date=… time=… logid=… level=… msg="…"
	// Cisco: "123: *Mar  1 00:01:02: %LINK-3-UPDOWN: …"
	for _, v := range []hostinfo.Vendor{hostinfo.VendorFortiOS, hostinfo.VendorCisco} {
		if (v == hostinfo.VendorFortiOS) != strings.Contains(rest, "logid=") || (v == hostinfo.VendorCisco && !strings.Contains(rest, "%")) {
			continue
		}
		if x, ok := netLine(v, rest, now); ok {
			x.Time = now
			return x, true
		}
	}
	e.Source, e.Msg = splitIdent(rest)
	if e.Msg == "" {
		return Entry{}, false
	}
	return e, true
}
