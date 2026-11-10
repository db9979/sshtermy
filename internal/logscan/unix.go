package logscan

import (
	"bufio"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// UnixScript: nur lesendes sh-Skript für Linux und BSD (auch OPNsense/
// pfSense). journalctl getrennt nach Schwere, sonst die klassischen
// Syslog-Dateien; dazu fehlgeschlagene Dienste, Startzeit und die letzten
// Paket-Updates. sudo nur ohne Passwort (-n), sonst die eigenen Rechte.
func UnixScript(hours int) string {
	return strings.ReplaceAll(unixScript, "@H@", strconv.Itoa(hours))
}

const unixScript = `export LC_ALL=C
H=@H@
S=""
if [ "$(id -u)" != 0 ] && sudo -n true 2>/dev/null; then S="sudo -n"; fi
echo "##S meta"
echo "uid=$(id -u)"
echo "sudo=${S:+1}"
echo "groups=$(id -Gn 2>/dev/null | tr ' ' ',')"
echo "now=$(date +%s)"
if [ -r /proc/uptime ]; then echo "up=$(cut -d. -f1 /proc/uptime)"; else echo "boot=$(sysctl -n kern.boottime 2>/dev/null | sed -n 's/.*sec = \([0-9]*\).*/\1/p')"; fi
if command -v journalctl >/dev/null 2>&1; then
  for p in "0..2 crit" "3..3 err" "4..4 warn"; do
    set -- $p
    echo "##S journal $2"
    $S journalctl -q --no-pager -p "$1" --since="-${H}h" -o short-unix 2>/dev/null | tail -n 3000
  done
  echo "##S failed"
  systemctl --failed --no-legend --plain 2>/dev/null | head -n 50
else
  for f in /var/log/syslog /var/log/messages /var/log/system/latest.log /var/log/system.log; do
    if [ -f "$f" ]; then
      echo "##S file $f"
      $S tail -n 6000 "$f" 2>/dev/null
      break
    fi
  done
fi
for f in /var/log/dpkg.log /var/log/dnf.rpm.log /var/log/yum.log /var/log/pacman.log; do
  if [ -r "$f" ]; then echo "##S pkg $f"; tail -n 400 "$f"; fi
done
echo "##S end"
`

var (
	reShortUnix = regexp.MustCompile(`^(\d{9,11})(?:\.\d+)?\s+(\S+)\s+(.*)$`)
	reIdent     = regexp.MustCompile(`^([^\s:\[]+)(?:\[\d+\])?:\s?(.*)$`)
	reSyslogBSD = regexp.MustCompile(`^(?:<(\d{1,3})>)?([A-Z][a-z]{2}\s+\d{1,2}\s+\d\d:\d\d:\d\d)\s+(\S+)\s+(.*)$`)
	reSyslogISO = regexp.MustCompile(`^(?:<(\d{1,3})>\d?\s*)?(\d{4}-\d\d-\d\d[T ]\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:?\d\d)?)\s+(\S+)\s+(.*)$`)
	reRFC5424   = regexp.MustCompile(`^(\S+)\s+(\S+)\s+\S+\s+(?:-|\[.*?\])\s*(.*)$`)
)

// ParseUnix liest die Ausgabe von UnixScript.
func ParseUnix(out string, hours int, now time.Time) *Report {
	r := &Report{Kind: KindUnix, Hours: hours, At: now}
	var sec, arg string
	meta := map[string]string{}
	var pkgs []Mark
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.HasPrefix(line, "##S ") {
			f := strings.SplitN(line[4:], " ", 2)
			sec, arg = f[0], ""
			if len(f) > 1 {
				arg = f[1]
			}
			switch sec {
			case "journal":
				r.Read = appendOnce(r.Read, "journalctl -p warning --since -"+strconv.Itoa(hours)+"h")
			case "failed":
				r.Read = appendOnce(r.Read, "systemctl --failed")
			case "file":
				r.Read = appendOnce(r.Read, arg)
			case "pkg":
				r.Read = appendOnce(r.Read, arg)
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		switch sec {
		case "meta":
			if k, v, ok := strings.Cut(line, "="); ok {
				meta[k] = v
			}
		case "journal":
			r.Lines++
			if e, ok := journalLine(line, sevOfJournal(arg)); ok {
				r.add(e)
			}
		case "failed":
			// "nginx.service loaded failed failed A high performance …"
			f := strings.Fields(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "●")))
			if len(f) == 0 {
				continue
			}
			if name := strings.TrimSuffix(f[0], ".service"); name != "" {
				r.add(Entry{Time: now, Sev: SevErr, Source: "systemd", Msg: "FAILED-UNIT " + name})
			}
		case "file":
			r.Lines++
			if e, ok := syslogLine(line, now); ok && (e.Time.IsZero() || !e.Time.Before(r.Since())) {
				r.add(e)
			}
		case "pkg":
			if m, ok := pkgLine(arg, line); ok {
				pkgs = append(pkgs, m)
			}
		}
	}
	if n, err := strconv.ParseInt(meta["now"], 10, 64); err == nil && n > 0 {
		if up, err := strconv.ParseInt(meta["up"], 10, 64); err == nil {
			r.Boot = time.Unix(n-up, 0)
		}
	}
	if b, err := strconv.ParseInt(meta["boot"], 10, 64); err == nil && b > 0 {
		r.Boot = time.Unix(b, 0)
	}
	if !r.Boot.IsZero() && r.Boot.After(r.Since()) {
		r.Marks = append(r.Marks, Mark{Time: r.Boot, Kind: MarkBoot, Text: i18n.T("Neustart")})
		r.add(Entry{Time: r.Boot, Sev: SevInfo, Source: "system", Msg: i18n.T("Neustart")})
	}
	r.Marks = append(r.Marks, groupPkgs(pkgs, r.Since())...)
	if meta["uid"] != "0" && meta["sudo"] == "" && !strings.Contains(meta["groups"], "adm") && !strings.Contains(meta["groups"], "systemd-journal") && !strings.Contains(meta["groups"], "wheel") {
		r.Notes = append(r.Notes, i18n.T("Ohne Root/sudo und ohne Gruppe adm/systemd-journal: nur ein Teil der Meldungen ist lesbar"))
	}
	sortEntries(r.Entries)
	Group(r)
	return r
}

func appendOnce(xs []string, x string) []string {
	if contains(xs, x) {
		return xs
	}
	return append(xs, x)
}

func (r *Report) add(e Entry) {
	if len(r.Entries) >= MaxEntries {
		r.Entries = r.Entries[1:]
	}
	r.Entries = append(r.Entries, e)
}

func sortEntries(es []Entry) {
	sort.SliceStable(es, func(i, j int) bool { return es[i].Time.Before(es[j].Time) })
}

func sevOfJournal(arg string) Sev {
	switch arg {
	case "crit":
		return SevCrit
	case "err":
		return SevErr
	}
	return SevWarn
}

// journalLine: "1731203527.123456 web01 kernel: Out of memory …"
func journalLine(line string, sev Sev) (Entry, bool) {
	m := reShortUnix.FindStringSubmatch(line)
	if m == nil {
		return Entry{}, false
	}
	sec, _ := strconv.ParseInt(m[1], 10, 64)
	e := Entry{Time: time.Unix(sec, 0), Sev: sev}
	e.Source, e.Msg = splitIdent(m[3])
	return e, true
}

func splitIdent(s string) (src, msg string) {
	if m := reIdent.FindStringSubmatch(s); m != nil {
		return m[1], strings.TrimSpace(m[2])
	}
	return "", strings.TrimSpace(s)
}

// syslogLine: klassische Syslog-Datei (BSD-Format oder ISO-Zeit, auch mit
// <PRI> wie bei OPNsense). Ohne PRI zählt die Schwere nach Stichworten.
func syslogLine(line string, now time.Time) (Entry, bool) {
	var e Entry
	var pri, rest string
	if m := reSyslogISO.FindStringSubmatch(line); m != nil {
		pri, rest = m[1], m[4]
		e.Time = parseISO(m[2])
		if pri != "" {
			if x := reRFC5424.FindStringSubmatch(rest); x != nil { // "sshd 1234 - [meta …] msg"
				e.Source, e.Msg = x[1], x[3]
			}
		}
	} else if m := reSyslogBSD.FindStringSubmatch(line); m != nil {
		pri, rest = m[1], m[4]
		e.Time = parseBSD(m[2], now)
	} else {
		return e, false
	}
	if e.Msg == "" {
		e.Source, e.Msg = splitIdent(rest)
	}
	if pri != "" {
		p, _ := strconv.Atoi(pri)
		e.Sev = sevOfSyslog(p % 8)
	} else {
		e.Sev = sevOfWords(e.Msg)
	}
	return e, true
}

func sevOfSyslog(level int) Sev {
	switch {
	case level <= 2:
		return SevCrit
	case level == 3:
		return SevErr
	case level == 4:
		return SevWarn
	}
	return SevInfo
}

var (
	reCritWords = regexp.MustCompile(`(?i)\b(panic|emerg\w*|critical|fatal|crit)\b`)
	reErrWords  = regexp.MustCompile(`(?i)\b(error|errors|failed|failure|denied|refused|cannot|can't|unable|segfault)\b`)
	reWarnWords = regexp.MustCompile(`(?i)\b(warn\w*|timeout|timed out|retry\w*|invalid|unreachable|lost)\b`)
)

func sevOfWords(msg string) Sev {
	switch {
	case reCritWords.MatchString(msg):
		return SevCrit
	case reErrWords.MatchString(msg):
		return SevErr
	case reWarnWords.MatchString(msg):
		return SevWarn
	}
	return SevInfo
}

func parseISO(s string) time.Time {
	s = strings.Replace(s, " ", "T", 1)
	for _, f := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999Z0700", "2006-01-02T15:04:05Z0700", "2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

// parseBSD: "Nov 10 03:12:07" (ohne Jahr: liegt es in der Zukunft, war es
// letztes Jahr).
func parseBSD(s string, now time.Time) time.Time {
	t, err := time.ParseInLocation("Jan _2 15:04:05", strings.Join(strings.Fields(s), " "), time.Local)
	if err != nil {
		t, err = time.ParseInLocation("Jan 2 15:04:05", strings.Join(strings.Fields(s), " "), time.Local)
		if err != nil {
			return time.Time{}
		}
	}
	t = t.AddDate(now.Year(), 0, 0)
	if t.After(now.Add(48 * time.Hour)) {
		t = t.AddDate(-1, 0, 0)
	}
	return t
}

var (
	reDpkg   = regexp.MustCompile(`^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d) (upgrade|install|remove) (\S+?)(?::\S+)? `)
	reDnf    = regexp.MustCompile(`^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\S*) \S+ (Upgrade|Installed|Upgraded|Erase|Install): (\S+)`)
	reYum    = regexp.MustCompile(`^([A-Z][a-z]{2} +\d+ \d\d:\d\d:\d\d) (Updated|Installed|Erased): (\S+)`)
	rePacman = regexp.MustCompile(`^\[(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\S*)\] \[ALPM\] (upgraded|installed|removed) (\S+)`)
)

func pkgLine(file, line string) (Mark, bool) {
	var ts, name string
	var t time.Time
	switch {
	case strings.HasSuffix(file, "dpkg.log"):
		if m := reDpkg.FindStringSubmatch(line); m != nil {
			ts, name = m[1], m[3]
			t, _ = time.ParseInLocation("2006-01-02 15:04:05", ts, time.Local)
		}
	case strings.HasSuffix(file, "dnf.rpm.log"):
		if m := reDnf.FindStringSubmatch(line); m != nil {
			name, t = m[3], parseISO(m[1])
		}
	case strings.HasSuffix(file, "yum.log"):
		if m := reYum.FindStringSubmatch(line); m != nil {
			name, t = m[3], parseBSD(m[1], time.Now())
		}
	case strings.HasSuffix(file, "pacman.log"):
		if m := rePacman.FindStringSubmatch(line); m != nil {
			name, t = m[3], parseISO(m[1])
		}
	}
	if name == "" || t.IsZero() {
		return Mark{}, false
	}
	return Mark{Time: t, Kind: MarkPkg, Text: name}, true
}

// groupPkgs: Paket-Änderungen innerhalb von 10 Minuten zu einer Marke.
func groupPkgs(ms []Mark, since time.Time) []Mark {
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].Time.Before(ms[j].Time) })
	var out []Mark
	var names []string
	var start time.Time
	flush := func() {
		if len(names) == 0 {
			return
		}
		txt := strings.Join(names[:min(3, len(names))], ", ")
		if len(names) > 3 {
			txt += fmt.Sprintf(" +%d", len(names)-3)
		}
		out = append(out, Mark{Time: start, Kind: MarkPkg, Text: fmt.Sprintf(i18n.T("Pakete geändert: %s"), txt)})
		names = nil
	}
	for _, m := range ms {
		if m.Time.Before(since) {
			continue
		}
		if len(names) > 0 && m.Time.Sub(start) > 10*time.Minute {
			flush()
		}
		if len(names) == 0 {
			start = m.Time
		}
		if !contains(names, m.Text) {
			names = append(names, m.Text)
		}
	}
	flush()
	return out
}
