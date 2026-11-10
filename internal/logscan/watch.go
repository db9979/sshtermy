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
func Classify(k Kind, e Entry) (Finding, bool) { return ClassifyBurst(k, e, 1) }

// ClassifyBurst wie Classify, aber als n-te gleiche Zeile in kurzer Zeit
// (Fehlanmeldungen zählen erst ab einer Menge).
func ClassifyBurst(k Kind, e Entry, n int) (Finding, bool) {
	f := classify(k, &e)
	if f == nil {
		return Finding{}, false
	}
	f.Count, f.First, f.Last, f.Sample = max(1, n), e.Time, e.Time, e.Line()
	if f.rule != nil && f.rule.after != nil {
		f.rule.after(f)
	}
	if f.rule != nil && f.rule.Burst && n >= BurstErr && f.Sev < SevErr {
		f.Sev = SevErr
	}
	f.rule = nil
	return *f, true
}

// BurstErr: so viele gleiche Fehlanmeldungen in BurstWindow sind ein Fehler.
const (
	BurstErr    = 20
	BurstWindow = 10 * time.Minute
)

// UnixWatchScript folgt dem Journal ab jetzt (JSON, damit jede Zeile ihre
// Schwere mitbringt), ohne Journal den Syslog-Dateien. warn: auch Warnungen.
func UnixWatchScript(warn bool) string {
	p := "0..3"
	if warn {
		p = "0..4"
	}
	return `S=""; if [ "$(id -u)" != 0 ] && sudo -n true 2>/dev/null; then S="sudo -n"; fi
F="--output-fields=PRIORITY,SYSLOG_IDENTIFIER,_COMM,MESSAGE"
if command -v journalctl >/dev/null 2>&1; then
  $S journalctl -n 0 -q $F >/dev/null 2>&1 || F=""
  # Fehlanmeldungen meldet sshd als Info: eigener Strom ohne Schwere-Filter
  $S journalctl -f -n 0 -q -t sshd -t sshd-session -o json $F &
  $S journalctl -f -n 0 -q -p ` + p + ` -o json $F
  wait
else
  L=""
  for f in /var/log/syslog /var/log/messages /var/log/system.log /var/log/auth.log /var/log/secure; do [ -e "$f" ] && L="$L $f"; done
  [ -n "$L" ] && exec $S tail -q -n 0 -F $L
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

// MergeSyslog nimmt empfangene Syslog-Zeilen eines Geräts in den Bericht
// auf (der Puffer im Switch ist oft klein). Zeilen, die der Switch selbst
// schon gezeigt hat (gleicher Text, höchstens 90 s Abstand), zählen nicht
// doppelt. Danach werden die Muster neu gebildet.
func (r *Report) MergeSyslog(es []Entry) int {
	since := r.Since()
	seen := map[string][]time.Time{}
	for _, e := range r.Entries {
		seen[e.Msg] = append(seen[e.Msg], e.Time)
	}
	added := 0
	for _, e := range es {
		if e.Time.Before(since) || e.Time.After(r.At.Add(time.Minute)) {
			continue
		}
		dup := false
		for _, t := range seen[e.Msg] {
			if d := e.Time.Sub(t); d < 90*time.Second && d > -90*time.Second {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		e.key = ""
		r.add(e)
		r.Lines++
		added++
	}
	if added > 0 {
		sortEntries(r.Entries)
		Group(r)
	}
	return added
}
