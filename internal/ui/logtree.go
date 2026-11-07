package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sessionlog"
)

// Sitzungsprotokolle gruppiert: nach Tagen (Folgesitzungen am selben Host
// werden eine Zeile, mehrere Hosts kurz nacheinander eine Gruppe) oder per
// Tab nach Host. Die Liste wird bei jedem Bild aus den Protokollen gebaut;
// gemerkt wird nur, was der Nutzer auf- oder zugeklappt hat.

const (
	logRunGap   = 30 * time.Minute // Folgesitzung: Ende der älteren → Beginn der neueren
	logBurstGap = 2 * time.Minute  // Gruppe: Abstand zweier Starts
	logBurstMin = 3                // Gruppe ab so vielen verschiedenen Hosts
)

type logRowKind int

const (
	lrDay   logRowKind = iota
	lrItem             // eine Sitzung oder Folgesitzungen am selben Host
	lrGroup            // mehrere Hosts kurz nacheinander
	lrHost             // Ansicht nach Host
)

type logRow struct {
	kind  logRowKind
	key   string
	logs  []*sessionlog.Log // neueste zuerst
	label string
	depth int
	last  bool // letztes Kind einer aufgeklappten Zeile
	open  bool
	hosts int
	day   time.Time
}

// expandable: Zeile hat etwas zum Aufklappen.
func (r logRow) expandable() bool {
	return r.kind == lrDay || r.kind == lrGroup || len(r.logs) > 1
}

func (r logRow) commands() (n, failed int) {
	for _, l := range r.logs {
		n += len(l.Entries)
		failed += l.Failures()
	}
	return
}

// span: Beginn der ältesten und Ende der neuesten Sitzung.
func (r logRow) span() (from, to time.Time) {
	if len(r.logs) == 0 {
		return
	}
	from = r.logs[len(r.logs)-1].Started
	to = logEnd(r.logs[0])
	return
}

func logEnd(l *sessionlog.Log) time.Time {
	if l.Ended.IsZero() {
		return l.Started
	}
	return l.Ended
}

type logFilter int

const (
	lfAll logFilter = iota
	lfToday
	lfWeek
	lfFailed
	lfCount
)

func (f logFilter) label() string {
	switch f {
	case lfToday:
		return i18n.T("Heute")
	case lfWeek:
		return i18n.T("7 Tage")
	case lfFailed:
		return i18n.T("Mit Fehlern")
	}
	return i18n.T("Alle")
}

func (f logFilter) ok(l *sessionlog.Log, now time.Time) bool {
	switch f {
	case lfToday:
		return sameDay(l.Started, now)
	case lfWeek:
		return !l.Started.Before(dayStart(now).AddDate(0, 0, -6))
	case lfFailed:
		return l.Failures() > 0
	}
	return true
}

func dayStart(t time.Time) time.Time {
	t = t.Local()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func sameDay(a, b time.Time) bool { return dayStart(a).Equal(dayStart(b)) }

// logHostKey: dieselbe Verbindung (oder ohne ID derselbe Name).
func logHostKey(l *sessionlog.Log) string {
	if l.ConnID != "" {
		return "c:" + l.ConnID
	}
	return "h:" + l.Host
}

// logRuns fasst direkt aufeinanderfolgende Sitzungen am selben Host zusammen
// (logs neueste zuerst).
func logRuns(logs []*sessionlog.Log) [][]*sessionlog.Log {
	var runs [][]*sessionlog.Log
	for _, l := range logs {
		if n := len(runs); n > 0 {
			r := runs[n-1]
			older := r[len(r)-1]
			if logHostKey(r[0]) == logHostKey(l) && older.Started.Sub(logEnd(l)) < logRunGap {
				runs[n-1] = append(r, l)
				continue
			}
		}
		runs = append(runs, []*sessionlog.Log{l})
	}
	return runs
}

// logBursts teilt Läufe in Gruppen: Starts höchstens logBurstGap auseinander
// und – wenn die Verbindungen in Ordnern liegen – derselbe Ordner. Gruppen mit
// weniger als logBurstMin Hosts werden wieder einzelne Läufe.
//
// Ohne gemeinsamen Ordner müssen die Namen gleich anfangen (CVM1, CVM2 …),
// sonst würden zufällig gleichzeitige Sitzungen zur Gruppe.
func logBursts(runs [][]*sessionlog.Log, folder func(*sessionlog.Log) string) [][][]*sessionlog.Log {
	var out [][][]*sessionlog.Log
	var cur [][]*sessionlog.Log
	flush := func() {
		if len(cur) == 0 {
			return
		}
		hosts := map[string]bool{}
		for _, r := range cur {
			hosts[logHostKey(r[0])] = true
		}
		if len(hosts) >= logBurstMin && (folder(cur[0][0]) != "" || commonPrefix(flatten(cur)) != "") {
			out = append(out, cur)
		} else {
			for _, r := range cur {
				out = append(out, [][]*sessionlog.Log{r})
			}
		}
		cur = nil
	}
	for _, r := range runs {
		if n := len(cur); n > 0 {
			prev := cur[n-1]
			gap := prev[len(prev)-1].Started.Sub(r[0].Started)
			if gap > logBurstGap || folder(prev[0]) != folder(r[0]) {
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return out
}

// groupLabel: gemeinsamer Ordner, sonst gemeinsamer Namensanfang, sonst
// erster Host.
func groupLabel(logs []*sessionlog.Log, folder func(*sessionlog.Log) string) string {
	f := folder(logs[0])
	for _, l := range logs[1:] {
		if folder(l) != f {
			f = ""
			break
		}
	}
	if f != "" {
		parts := strings.Split(f, " / ")
		return parts[len(parts)-1]
	}
	if pre := commonPrefix(logs); pre != "" {
		return pre
	}
	return logs[len(logs)-1].Host
}

// commonPrefix: gemeinsamer Namensanfang (ohne Ziffern/Trenner am Ende),
// mindestens 3 Zeichen, sonst "".
func commonPrefix(logs []*sessionlog.Log) string {
	p := []rune(logs[0].Host)
	for _, l := range logs[1:] {
		q := []rune(l.Host)
		i := 0
		for i < len(p) && i < len(q) && unicode.ToLower(p[i]) == unicode.ToLower(q[i]) {
			i++
		}
		p = p[:i]
	}
	pre := strings.TrimRightFunc(string(p), func(r rune) bool {
		return unicode.IsDigit(r) || unicode.IsSpace(r) || strings.ContainsRune("-_.:/", r)
	})
	if len([]rune(pre)) >= 3 {
		return pre
	}
	return ""
}

func flatten(runs [][]*sessionlog.Log) []*sessionlog.Log {
	var out []*sessionlog.Log
	for _, r := range runs {
		out = append(out, r...)
	}
	return out
}

func countHosts(logs []*sessionlog.Log) int {
	m := map[string]bool{}
	for _, l := range logs {
		m[logHostKey(l)] = true
	}
	return len(m)
}

// logTree baut die sichtbaren Zeilen.
type logTree struct {
	logs     []*sessionlog.Log // gefiltert, neueste zuerst
	byHost   bool
	sortN    bool // Hosts nach Anzahl statt „zuletzt“
	now      time.Time
	open     map[string]bool
	openDays bool // Suche/Filter aktiv: alle Tage offen
	folder   func(*sessionlog.Log) string
}

func (t logTree) isOpen(key string, def bool) bool {
	if v, ok := t.open[key]; ok {
		return v
	}
	return def
}

func (t logTree) rows() []logRow {
	if t.byHost {
		return t.hostRows()
	}
	var out []logRow
	yesterday := dayStart(t.now).AddDate(0, 0, -1)
	for i := 0; i < len(t.logs); {
		d := dayStart(t.logs[i].Started)
		j := i
		for j < len(t.logs) && dayStart(t.logs[j].Started).Equal(d) {
			j++
		}
		day := t.logs[i:j]
		key := "d:" + d.Format("2006-01-02")
		open := t.isOpen(key, t.openDays || !d.Before(yesterday))
		out = append(out, logRow{kind: lrDay, key: key, logs: day, day: d, open: open, hosts: countHosts(day)})
		if open {
			for _, b := range logBursts(logRuns(day), t.folder) {
				if len(b) == 1 {
					out = t.item(out, b[0], 0, false, false)
					continue
				}
				all := flatten(b)
				gkey := "g:" + all[0].ID
				gopen := t.isOpen(gkey, false)
				out = append(out, logRow{kind: lrGroup, key: gkey, logs: all, label: groupLabel(all, t.folder), open: gopen, hosts: countHosts(all)})
				if gopen {
					for k, r := range b {
						out = t.item(out, r, 1, k == len(b)-1, false)
					}
				}
			}
		}
		i = j
	}
	return out
}

// item: Lauf (eine oder mehrere Sitzungen an einem Host), aufgeklappt mit
// den einzelnen Sitzungen darunter.
func (t logTree) item(out []logRow, run []*sessionlog.Log, depth int, last, withDay bool) []logRow {
	key := "i:" + run[0].ID
	open := len(run) > 1 && t.isOpen(key, false)
	out = append(out, logRow{kind: lrItem, key: key, logs: run, label: run[0].Host, depth: depth, last: last, open: open, day: dayStart(run[0].Started)})
	if open {
		for k, l := range run {
			out = append(out, logRow{kind: lrItem, key: "s:" + l.ID, logs: []*sessionlog.Log{l}, label: l.Host, depth: depth + 1, last: k == len(run)-1, day: dayStart(l.Started)})
		}
	}
	return out
}

func (t logTree) hostRows() []logRow {
	var order []string
	by := map[string][]*sessionlog.Log{}
	for _, l := range t.logs {
		k := logHostKey(l)
		if _, ok := by[k]; !ok {
			order = append(order, k)
		}
		by[k] = append(by[k], l)
	}
	if t.sortN {
		sort.SliceStable(order, func(i, j int) bool { return len(by[order[i]]) > len(by[order[j]]) })
	}
	var out []logRow
	for _, k := range order {
		logs := by[k]
		key := "h:" + k
		open := len(logs) > 1 && t.isOpen(key, false)
		out = append(out, logRow{kind: lrHost, key: key, logs: logs, label: logs[0].Host, open: open, hosts: 1})
		if open {
			runs := logRuns(logs)
			for i, r := range runs {
				out = t.item(out, r, 1, i == len(runs)-1, true)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- Texte

var weekdays = [...]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}

func weekday(t time.Time) string { return i18n.T(weekdays[t.Weekday()]) }

// dayTitle: „Heute · Sa 07.11.“, „Gestern · Fr 06.11.“, „Do 05.11.“.
func dayTitle(d, now time.Time) string {
	s := weekday(d) + " " + d.Format("02.01.")
	if d.Year() != now.Year() {
		s = weekday(d) + " " + d.Format("02.01.2006")
	}
	switch {
	case sameDay(d, now):
		return i18n.T("Heute") + " · " + s
	case sameDay(d, dayStart(now).AddDate(0, 0, -1)):
		return i18n.T("Gestern") + " · " + s
	}
	return s
}

// whenShort: „heute 09:48“, „Fr 14:10“ (diese Woche), sonst „05.11. 14:10“.
func whenShort(t, now time.Time) string {
	switch {
	case sameDay(t, now):
		return i18n.T("heute") + " " + t.Format("15:04")
	case !t.Before(dayStart(now).AddDate(0, 0, -6)):
		return weekday(t) + " " + t.Format("15:04")
	}
	return t.Format("02.01. 15:04")
}

// timeRange: „09:37–09:48“ (Beginn der ältesten bis Beginn der neuesten
// Sitzung) oder nur „09:48“.
func timeRange(logs []*sessionlog.Log) string {
	a, b := logs[len(logs)-1].Started.Format("15:04"), logs[0].Started.Format("15:04")
	if a == b {
		return b
	}
	return a + "–" + b
}

// topCommands: häufigste Befehle (Anzahl, davon fehlgeschlagen).
type cmdCount struct {
	cmd       string
	n, failed int
}

func topCommands(logs []*sessionlog.Log, max int) []cmdCount {
	idx := map[string]int{}
	var out []cmdCount
	for i := len(logs) - 1; i >= 0; i-- {
		for _, e := range logs[i].Entries {
			c := strings.TrimSpace(e.Command)
			if c == "" {
				continue
			}
			k, ok := idx[c]
			if !ok {
				k = len(out)
				idx[c] = k
				out = append(out, cmdCount{cmd: c})
			}
			out[k].n++
			if e.Failed {
				out[k].failed++
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].n > out[j].n })
	if len(out) > max {
		out = out[:max]
	}
	return out
}

// sameCommands: alle Sitzungen haben dieselbe (nicht leere) Befehlsfolge.
func sameCommands(logs []*sessionlog.Log) []string {
	var first []string
	for i, l := range logs {
		var cmds []string
		for _, e := range l.Entries {
			cmds = append(cmds, strings.TrimSpace(e.Command))
		}
		if len(cmds) == 0 {
			return nil
		}
		if i == 0 {
			first = cmds
			continue
		}
		if strings.Join(cmds, "\x00") != strings.Join(first, "\x00") {
			return nil
		}
	}
	return first
}

// lastResult: letzte nicht leere Ausgabezeile der Sitzung.
func lastResult(l *sessionlog.Log) (string, bool) {
	failed := false
	res := ""
	for _, e := range l.Entries {
		failed = failed || e.Failed
		if e.Failed && e.Reason != "" {
			res = e.Reason
		}
		lines := strings.Split(strings.TrimSpace(e.Output), "\n")
		if s := strings.TrimSpace(lines[len(lines)-1]); s != "" {
			res = s
		}
	}
	return res, failed
}

func sessionsText(n int) string {
	return plural(n, i18n.T("Sitzung"), i18n.T("Sitzungen"))
}

func hostsText(n int) string {
	return fmt.Sprintf(i18n.T("%d Hosts"), n)
}
