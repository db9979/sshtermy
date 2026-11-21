package cmdcheck

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
)

func tr(s string) string { return i18n.T(s) }

// Arten von Einwänden.
const (
	KindMissing   = "missing"   // Programm nicht installiert
	KindOption    = "option"    // Option nicht in der Doku
	KindSubcmd    = "subcmd"    // Unterbefehl nicht in der Doku
	KindNotLinux  = "notlinux"  // macOS-/BSD-Form auf Linux
	KindForbidden = "forbidden" // ausgeschlossenes Programm benutzt
	KindDetail    = "detail"    // Angabe aus der Frage fehlt
	KindSlip      = "slip"      // bekannter Ausrutscher
	KindSyntax    = "syntax"    // Zeile ist kein gültiges Shell
)

// Issue: ein Einwand gegen einen Befehl.
type Issue struct {
	Kind string
	Text string
}

// ---------------------------------------------------------------- macOS/BSD

var notLinux = []struct {
	re   *regexp.Regexp
	text string
}{
	{regexp.MustCompile(`\bsed\s+(?:-\w+\s+)*-i\s+(?:''|"")(?:\s|$)`), "sed -i '' ist die macOS-Form; unter Linux nur sed -i"},
	{regexp.MustCompile(`\bstat\s+(?:-\w+\s+)*-f\s+['"]?%`), "stat -f '%…' ist die macOS-Form; unter Linux stat -c"},
	{regexp.MustCompile(`\bdate\s+(?:-\w+\s+)*-v\s*[-+]?\d`), "date -v ist die macOS-Form; unter Linux date -d"},
	{regexp.MustCompile(`\btop\s+(?:-\S+\s+)*-l\s*\d`), "top -l gibt es nur auf macOS; unter Linux top -b -n 1"},
	{regexp.MustCompile(`\bifconfig\s+en\d`), "en0 ist ein macOS-Name; unter Linux heißen Netzwerkkarten eth0, ens…, enp…"},
}

var macOnly = set("pbcopy", "pbpaste", "launchctl", "diskutil", "sw_vers", "softwareupdate", "scutil", "networksetup", "defaults", "osascript", "mdfind", "brew", "dscl", "pmset", "caffeinate", "ditto", "hdiutil", "airport")

// NotLinux: Formen, die es nur auf macOS oder BSD gibt.
func NotLinux(line string) []Issue {
	var out []Issue
	for _, n := range notLinux {
		if n.re.MatchString(line) {
			out = append(out, Issue{KindNotLinux, tr(n.text)})
		}
	}
	for _, p := range Programs(line) {
		if macOnly[p] {
			out = append(out, Issue{KindNotLinux, fmt.Sprintf(tr("%s gibt es nur auf macOS"), p)})
		}
	}
	return out
}

// ---------------------------------------------------------------- ausgeschlossene Programme

// Ein Programmname: in Backticks/Anführungszeichen oder ein einzelnes Wort.
const ruledName = "(?:`([^`\\s]{1,40})`|'([^'\\s]{1,40})'|\"([^\"\\s]{1,40})\"|„([^“\\s]{1,40})“|([A-Za-z][\\w.+-]{0,38}[\\w+]|[A-Za-z]))"

// ruledNameLower: wie ruledName, ohne Anführung aber nur kleingeschrieben.
const ruledNameLower = "(?:`([^`\\s]{1,40})`|'([^'\\s]{1,40})'|\"([^\"\\s]{1,40})\"|„([^“\\s]{1,40})“|([a-z][a-z0-9_.+-]{0,38}[a-z0-9+]|[a-z]))"

var (
	// englisch: without X, don't use X, instead of X, avoid X, no X
	ruledEN = regexp.MustCompile(`(?i)\b(?:without(?:\s+using)?|(?:don'?t|do\s+not|never|cannot|can'?t)\s+(?:use|using|run|call)|not\s+(?:with|via|using)|instead\s+of(?:\s+using)?|rather\s+than(?:\s+using)?|avoid(?:ing)?(?:\s+using)?|other\s+than|no)\s+(?:the\s+)?` + ruledName + `(?:\s*(?:$|[,;.:!?)]|\s(?:or|and|nor|for|to|on|in|here|please|but|since|because|as|installed|available|command|tool|program|binary|is|isn|are|at\s+all)\b))`)
	// deutsch: ohne X, nicht mit X, kein(e) X, statt/anstatt/anstelle von X, X nicht verwenden/benutzen
	// deutsch: ohne X, nicht mit X, kein(e) X, statt/anstatt/anstelle von X. Das Verb steht im
	// Deutschen am Ende („ohne sudo neu starten“), darum zählt jedes kleingeschriebene Wort als
	// Programm: Hauptwörter („ohne Kopfzeile“) sind großgeschrieben.
	ruledDE     = regexp.MustCompile(`(?i:\bohne|\bnicht\s+(?:mit|über|per)|\bkein(?:e|en|em)?|\b(?:an)?statt(?:\s+von)?|\banstelle\s+von|\bvermeide)\s+(?:(?i:das|den|die)\s+)?` + ruledNameLower + `(?:[^\w.+-]|$)`)
	ruledDEverb = regexp.MustCompile(`(?i)` + ruledName + `\s+(?:bitte\s+)?(?:nicht|nie)\s+(?:verwenden|benutzen|nutzen|einsetzen|aufrufen)`)
	notInstEN   = regexp.MustCompile(`(?i)` + ruledName + `\s+(?:is\s*n[o']t|isn't|isn’t|is\s+not)\s+(?:installed|available)`)
	notInstDE   = regexp.MustCompile(`(?i)` + ruledName + `\s+(?:ist\s+nicht|gibt\s+es\s+(?:hier\s+)?nicht|fehlt)(?:\s+(?:installiert|vorhanden|verfügbar|da))?\b`)
	listMore    = regexp.MustCompile(`(?i)^\s*(?:,\s*|or\s+|and\s+|nor\s+|oder\s+|und\s+|noch\s+)(?:the\s+)?` + ruledName)
)

// Wörter, die nach „ohne/kein/no“ stehen, aber kein Programm sind.
var notPrograms = set("a", "an", "the", "any", "root", "rechte", "header", "headers", "kopfzeile", "überschrift", "output", "ausgabe",
	"fehler", "errors", "error", "passwort", "password", "neustart", "reboot", "pause", "warten", "rückfrage", "confirmation",
	"bestätigung", "prompt", "farbe", "farben", "color", "colors", "colour", "sudo-passwort", "internet", "netz", "network",
	"löschen", "deleting", "delete", "removing", "ändern", "changes", "änderung", "änderungen", "it", "them", "that", "this",
	"das", "die", "der", "den", "es", "sie", "ihn", "mehr", "more", "longer", "weiter", "problem", "probleme", "problems",
	"angst", "ende", "end", "limit", "grenze", "zeit", "time", "frage", "fragen", "questions", "question", "wait", "waiting",
	"one", "eine", "einen", "einer", "ein", "zu", "so", "ahnung", "idea", "versteckte", "hidden", "leerzeilen", "duplicates",
	"duplikate", "doppelte", "wildcards", "spaces", "leerzeichen", "subdirectories", "unterordner", "recursion", "rekursion",
	"neu", "weitere", "großen", "grossen", "lange", "langen", "wirklich", "ganz", "alle", "alles", "nichts", "viel",
	"bitte", "einfach", "extra", "vorher", "nachher", "schnell", "dauerhaft", "sofort", "gleich")

func ruledOut(m []string) string {
	for _, g := range m[1:6] {
		if g != "" {
			return strings.ToLower(g)
		}
	}
	return ""
}

// Forbidden: die Programme, die eine Frage ausschließt („ohne lsof“,
// „don't use sudo“, „netstat ist nicht installiert“), in Reihenfolge.
func Forbidden(text string) []string {
	var names []string
	add := func(n string) {
		if n == "" || notPrograms[n] {
			return
		}
		for _, x := range names {
			if x == n {
				return
			}
		}
		names = append(names, n)
	}
	for _, re := range []*regexp.Regexp{ruledEN, ruledDE} {
		for _, loc := range re.FindAllStringSubmatchIndex(text, -1) {
			m := subs(text, loc)
			add(ruledOut(m))
			// Listen: „ohne lsof oder netstat“, „without lsof, netstat or ss“
			end := endOfName(text, loc)
			for k := 0; k < 4; k++ {
				ml := listMore.FindStringSubmatchIndex(text[end:])
				if ml == nil {
					break
				}
				add(ruledOut(subs(text[end:], ml)))
				end += ml[1]
			}
		}
	}
	for _, re := range []*regexp.Regexp{ruledDEverb, notInstEN, notInstDE} {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			add(ruledOut(m))
		}
	}
	return names
}

// endOfName: Ende der Namensgruppe im Treffer (vor dem Folgewort).
func endOfName(text string, loc []int) int {
	end := loc[1]
	for g := 1; g <= 5; g++ {
		if loc[2*g] >= 0 {
			end = loc[2*g+1]
			if g <= 4 {
				end++ // schließendes Zeichen
			}
		}
	}
	return min(end, len(text))
}

func subs(s string, loc []int) []string {
	out := make([]string, len(loc)/2)
	for i := range out {
		if loc[2*i] >= 0 {
			out[i] = s[loc[2*i]:loc[2*i+1]]
		}
	}
	return out
}

func sameProgram(ruled, prog string) bool {
	prog = strings.ToLower(prog)
	if prog == ruled {
		return true
	}
	switch ruled {
	case "grep":
		return prog == "egrep" || prog == "fgrep" || prog == "rgrep"
	case "vim", "vi":
		return prog == "vi" || prog == "vim" || prog == "vim.basic" || prog == "vim.tiny" || prog == "view"
	case "python":
		return strings.HasPrefix(prog, "python")
	case "netstat":
		return false
	}
	return false
}

func forbiddenIssues(question, line string) []Issue {
	run := Programs(line)
	var out []Issue
	for _, name := range Forbidden(question) {
		for _, p := range run {
			if sameProgram(name, p) {
				out = append(out, Issue{KindForbidden, fmt.Sprintf(tr("die Frage sagt ohne %s, der Befehl nutzt es"), name)})
				break
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- Angaben aus der Frage

type want struct {
	kind, written string
	nums          []float64 // eine dieser Zahlen muss vorkommen
	text          string    // dieser Text muss vorkommen
}

var (
	reDuration = regexp.MustCompile(`(?i)\b(\d+)[\s-]*(minuten|minute|min|stunden|stunde|std|tagen|tage|tag|wochen|woche|monaten|monate|monat|minutes|minute|mins|hours|hour|hrs|hr|days|day|weeks|week|months|month)\b`)
	reSize     = regexp.MustCompile(`(?i)(?:^|[^\w.])(\d+(?:[.,]\d+)?)\s*(kib|kb|k|mib|mb|m|gib|gb|g|tib|tb|t)\b`)
	rePort     = regexp.MustCompile(`(?i)\bport\s+(\d{2,5})\b`)
	reExt      = regexp.MustCompile(`(?i)(?:\*\.([a-z][a-z0-9]{0,5})\b|(?:^|[\s'"(„])\.([a-z][a-z0-9]{0,5})[-‐ ]?(?:dateien|files|datei)\b|\b(?:ending|end)\s+(?:in|with)\s+['"]?\.([a-z][a-z0-9]{0,5})\b|\b(?:endung|enden(?:d)?\s+auf|endet\s+auf)\s+['"„]?\.?([a-z][a-z0-9]{0,5})\b)`)
	rePath     = regexp.MustCompile(`(?:^|[\s'"(„])((?:/|~/)[\w.@-]+(?:/[\w.@*-]+)*)/?`)
	reExamples = regexp.MustCompile(`(?i)\b(?:like|such as|e\.g\.?|for example|wie|z\.\s?b\.|zum beispiel|etwa)\s+(?:\S+\s+){0,3}$`)
)

func minutesOf(unit string) float64 {
	u := strings.ToLower(unit)
	switch {
	case strings.HasPrefix(u, "min"):
		return 1
	case strings.HasPrefix(u, "st") || strings.HasPrefix(u, "h"):
		return 60
	case strings.HasPrefix(u, "tag") || strings.HasPrefix(u, "day"):
		return 1440
	case strings.HasPrefix(u, "woche") || strings.HasPrefix(u, "week"):
		return 10080
	}
	return 43200 // Monat
}

func durationForms(n float64, unit string) []float64 {
	m := n * minutesOf(unit)
	forms := []float64{n, m, m * 60}
	if m >= 1440 {
		forms = append(forms, math.Floor(m/1440)-1) // find zählt ganze Tage ab
	}
	for _, sz := range []float64{60, 1440, 10080} {
		if math.Mod(m, sz) == 0 {
			forms = append(forms, m/sz)
		}
	}
	if minutesOf(unit) == 43200 {
		forms = append(forms, n*30, n*31, n*4)
	}
	return forms
}

func sizeForms(n float64, unit string) []float64 {
	forms := []float64{n}
	pow := strings.Index("kmgt", strings.ToLower(unit[:1])) + 1
	for _, base := range []float64{1024, 1000} {
		for d := 1; d <= pow; d++ {
			v := n * math.Pow(base, float64(d))
			if v == math.Trunc(v) {
				forms = append(forms, v)
			}
		}
		// auch die nächstkleinere Einheit: 1.5G → 1536M
	}
	return forms
}

func wants(q string) []want {
	var out []want
	for _, m := range reDuration.FindAllStringSubmatch(q, -1) {
		n, _ := strconv.ParseFloat(m[1], 64)
		if n > 1 {
			out = append(out, want{kind: "duration", written: m[0], nums: durationForms(n, m[2])})
		}
	}
	for _, loc := range reSize.FindAllStringSubmatchIndex(q, -1) {
		if reExamples.MatchString(q[:loc[0]]) {
			continue
		}
		m := subs(q, loc)
		n, _ := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
		// „5 m“ als Meter oder Minuten? Nur mit klarer Größenangabe
		u := strings.ToLower(m[2])
		if u == "m" || u == "t" || u == "k" {
			if !regexp.MustCompile(`(?i)\b(groß|größer|kleiner|big|bigger|larger|smaller|size|größe|über|over|under|unter)\b`).MatchString(q) {
				continue
			}
		}
		out = append(out, want{kind: "size", written: strings.TrimSpace(m[0]), nums: sizeForms(n, m[2])})
	}
	for _, m := range rePort.FindAllStringSubmatch(q, -1) {
		n, _ := strconv.ParseFloat(m[1], 64)
		out = append(out, want{kind: "port", written: m[0], nums: []float64{n}})
	}
	for _, m := range reExt.FindAllStringSubmatch(q, -1) {
		for _, g := range m[1:] {
			if g != "" {
				out = append(out, want{kind: "extension", written: "." + g, text: "." + g})
				break
			}
		}
	}
	for _, m := range rePath.FindAllStringSubmatch(q, -1) {
		p := strings.TrimRight(m[1], ".,;:")
		if len(p) > 2 {
			out = append(out, want{kind: "path", written: p, text: p})
		}
	}
	return out
}

var placeholder = regexp.MustCompile(`\b(PID|PIDS|PROCESS_ID|UNIT|RULE_NUMBER)\b|<[^>]+>`)

func hasNumber(cmd string, nums []float64) bool {
	for _, n := range nums {
		var s string
		if n == math.Trunc(n) {
			s = strconv.FormatInt(int64(n), 10)
		} else {
			s = strconv.FormatFloat(n, 'f', -1, 64)
		}
		re := regexp.MustCompile(`(^|[^\d.])` + regexp.QuoteMeta(s) + `([^\d]|$)`)
		if re.MatchString(cmd) {
			return true
		}
	}
	return false
}

func pathPresent(p, cmd string) bool {
	p = strings.TrimRight(p, "/")
	cands := []string{p}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		cands = append(cands, "$HOME/"+rest, "${HOME}/"+rest)
		if regexp.MustCompile(`/home/[\w.-]+/` + regexp.QuoteMeta(rest) + `([^\w.-]|$)`).MatchString(cmd) {
			return true
		}
	}
	for _, c := range cands {
		if regexp.MustCompile(regexp.QuoteMeta(c) + `([^\w.-]|$)`).MatchString(cmd) {
			return true
		}
	}
	// Ordner und Name getrennt: cd /home/a && tar … projekt
	if i := strings.LastIndexByte(p, '/'); i > 0 && i < len(p)-1 {
		return pathPresent(p[:i], cmd) && regexp.MustCompile(`(^|[^\w.-])`+regexp.QuoteMeta(p[i+1:])+`([^\w-]|$)`).MatchString(cmd)
	}
	return false
}

func detailIssues(question, line string) []Issue {
	if placeholder.MatchString(line) {
		return nil // Befehl arbeitet auf einem Platzhalter: Angaben beschrieben die Suche
	}
	loose := strings.ReplaceAll(line, `\`, "")
	low := strings.ToLower(loose)
	var out []Issue
	for _, w := range wants(question) {
		ok := true
		switch {
		case len(w.nums) > 0:
			ok = hasNumber(loose, w.nums)
		case w.kind == "path":
			ok = pathPresent(w.text, loose)
		default:
			ok = strings.Contains(low, strings.ToLower(w.text))
		}
		if !ok {
			out = append(out, Issue{KindDetail, fmt.Sprintf(tr("die Frage nennt %s, der Befehl nicht"), w.written)})
		}
	}
	return out
}

// ---------------------------------------------------------------- Ausrutscher

var (
	headOne     = regexp.MustCompile(`^\s*head\s+(?:-n\s*1|-1|--lines=1)\s*$`)
	psHeaderles = regexp.MustCompile(`--no-headers?\b|(^|\s)h(\s|$)|=[,\s]|=$|-o\s*\S*=(\s|$)`)
	psStart     = regexp.MustCompile(`^\s*(?:sudo\s+)?ps\b`)
	grepRec     = regexp.MustCompile(`(?:^|\s)-[a-zA-Z]*[rR]|--recursive|--dereference-recursive|-d\s*recurse|--directories`)
	knownDir    = regexp.MustCompile(`^(?:/var/log|/etc|/var|/var/lib|/var/www|/home|/srv|/opt|/usr|/tmp|/root|\.|\.\.|~)$`)
	lookOnly    = regexp.MustCompile(`(?i)^\s*(?:bitte\s+|please\s+)?(?:zeig|zeige|liste|list|show|display|print|finde?|find|welche|welcher|welches|which|what|was|wie\s+viele|how\s+many|prüfe?|check|inspect|count|zähle?|view|tell|anzeigen|gib\s+mir)\b`)
	thenDeletes = regexp.MustCompile(`(?i)\b(?:und|dann|danach|anschließend|and|then|before|after)\s+(?:auch\s+|also\s+)?(?:lösch\w*|entfern\w*|delete|remove|rm|erase|wipe|purge|clean|aufräum\w*|shred)\b|,\s*(?:delete|remove|lösch\w*|entfern\w*)\b|\b(?:löschen|entfernen|delete|remove)\b`)
	shutDown    = regexp.MustCompile(`(?i)\b(?:shut\s*down|power\s*off|turn\s+off|halt|herunterfahren|ausschalten|abschalten|runterfahren|herunter|runter)\b`)
	restart     = regexp.MustCompile(`(?i)\b(?:reboot|restart|neustart|neu\s*starten|neustarten)\b`)
)

var deleting = set("rm", "rmdir", "unlink", "shred")

func deletes(line string) bool {
	if strings.Contains(line, "--help") || strings.Contains(line, "--version") {
		return false
	}
	if u, _ := Uses(line); true {
		for _, x := range u {
			if x.Program == "find" {
				for _, a := range x.Args {
					if a.Text == "-delete" && !a.Quoted {
						return true
					}
				}
			}
			if deleting[baseName(x.Program)] {
				return true
			}
		}
	}
	return false
}

// pipeStages: die Stufen einer Pipeline je Abschnitt (an ; && || getrennt).
func pipeStages(line string) [][]string {
	var out [][]string
	for _, part := range regexp.MustCompile(`;|&&|\|\|`).Split(line, -1) {
		var st []string
		for _, s := range strings.Split(part, "|") {
			st = append(st, strings.TrimSpace(s))
		}
		out = append(out, st)
	}
	return out
}

func psHeadOne(first, second string) bool {
	return psStart.MatchString(first) && headOne.MatchString(second) && !psHeaderles.MatchString(first)
}

// grepOnDir: Dateiargumente von grep, die sicher Ordner sind.
func grepOnDir(args []Token) bool {
	var raw []string
	for _, a := range args {
		raw = append(raw, a.Text)
	}
	if grepRec.MatchString(" " + strings.Join(raw, " ")) {
		return false
	}
	var pos []string
	patternGiven, skip := false, false
	for _, a := range args {
		t := a.Text
		switch {
		case skip:
			skip = false
		case !a.Quoted && strings.HasPrefix(t, "--"):
			name, _, hasEq := strings.Cut(t, "=")
			patternGiven = patternGiven || name == "--regexp" || name == "--file"
			skip = !hasEq && set("--regexp", "--file", "--max-count", "--context", "--after-context", "--before-context", "--include", "--exclude", "--exclude-dir", "--label", "--devices", "--binary-files")[name]
		case !a.Quoted && strings.HasPrefix(t, "-") && len(t) > 1:
			patternGiven = patternGiven || strings.ContainsAny(t[1:], "ef")
			skip = strings.ContainsRune("efmABCdD", rune(t[len(t)-1]))
		default:
			pos = append(pos, t)
		}
	}
	if !patternGiven && len(pos) > 0 {
		pos = pos[1:]
	}
	for _, p := range pos {
		if strings.HasSuffix(p, "/") || knownDir.MatchString(p) {
			return true
		}
	}
	return false
}

func slipIssues(question, line string) []Issue {
	var out []Issue
	for _, st := range pipeStages(line) {
		for i := 0; i+1 < len(st); i++ {
			if psHeadOne(st[i], st[i+1]) {
				out = append(out, Issue{KindSlip, tr("ps gibt zuerst eine Kopfzeile aus: head -n 1 zeigt nur sie")})
			}
		}
	}
	if u, ok := Uses(line); ok {
		for _, x := range u {
			if (x.Program == "grep" || x.Program == "egrep" || x.Program == "zgrep") && grepOnDir(x.Args) {
				out = append(out, Issue{KindSlip, tr("grep durchsucht Dateien, keinen Ordner: ein Ordner braucht -r")})
				break
			}
		}
	}
	if question != "" && lookOnly.MatchString(question) && !thenDeletes.MatchString(question) && deletes(line) {
		out = append(out, Issue{KindSlip, tr("die Frage will nur ansehen, der Befehl löscht")})
	}
	switch powerAction(line) {
	case "reboot":
		if shutDown.MatchString(question) && !restart.MatchString(question) {
			out = append(out, Issue{KindSlip, tr("die Frage sagt herunterfahren, der Befehl startet neu")})
		}
	case "poweroff":
		if restart.MatchString(question) && !shutDown.MatchString(question) {
			out = append(out, Issue{KindSlip, tr("die Frage sagt neu starten, der Befehl fährt herunter")})
		}
	}
	return out
}

// powerAction: "reboot", "poweroff" oder "".
func powerAction(line string) string {
	u, _ := Uses(line)
	for _, x := range u {
		var words []string
		for _, a := range x.Args {
			words = append(words, a.Text)
		}
		args := " " + strings.Join(words, " ") + " "
		switch baseName(x.Program) {
		case "systemctl":
			if strings.Contains(args, " reboot ") {
				return "reboot"
			}
			if strings.Contains(args, " poweroff ") || strings.Contains(args, " halt ") {
				return "poweroff"
			}
		case "shutdown":
			if regexp.MustCompile(`\s-\w*c\b`).MatchString(args) {
				return ""
			}
			if regexp.MustCompile(`\s-\w*r|--reboot\b`).MatchString(args) {
				return "reboot"
			}
			return "poweroff"
		case "reboot":
			return "reboot"
		case "poweroff", "halt":
			if strings.Contains(args, "--reboot") {
				return "reboot"
			}
			return "poweroff"
		}
	}
	return ""
}

// ---------------------------------------------------------------- Reparieren

// Sys: was über das Zielsystem bekannt ist (aus dem Systemprofil).
type Sys struct {
	PkgManager string // apt, dnf, yum, zypper, pacman, apk; leer = unbekannt
	Root       bool   // als root angemeldet
}

var (
	headOneRepl  = regexp.MustCompile(`head\s+(?:-n\s*1|-1|--lines=1)`)
	grepCall     = regexp.MustCompile(`(^|[|;&(]\s*|\s)((?:sudo\s+)?[ez]?grep)(\s[^|;&)]*)`)
	leadingSudo  = regexp.MustCompile(`(^|[;&|(]\s*)sudo\s+(?:-[EHiknSs]+\s+)*`)
	sudoAsOther  = regexp.MustCompile(`\bsudo\s+(?:-\S+\s+)*(?:-u|--user|-g|--group)\b`)
	pkgListing   = regexp.MustCompile(`^\s*(?:sudo\s+)?(?:dpkg\s+(?:-l|--list)|apt\s+list\s+--installed|rpm\s+-qa|dnf\s+list\s+installed|yum\s+list\s+installed|pacman\s+-Q|apk\s+(?:info|list\s+--installed)|zypper\s+(?:se|search)\s+(?:-i|--installed-only))\s*$`)
	pkgSearch    = regexp.MustCompile(`^\s*(?:sudo\s+)?(?:apt(?:-cache)?\s+search|dnf\s+search|yum\s+search|pacman\s+-Ss|apk\s+search|zypper\s+(?:se|search))\s+(\S+)\s*$`)
	namesManager = regexp.MustCompile(`(?i)\b(apt|apt-get|dpkg|dnf|yum|rpm|pacman|apk|zypper|debian|ubuntu|fedora|rhel|centos|rocky|alma|arch|alpine|suse)\b`)
)

var listingOf = map[string]string{
	"apt": "dpkg -l", "dnf": "dnf list installed", "yum": "yum list installed", "zypper": "zypper search --installed-only",
	"pacman": "pacman -Q", "apk": "apk info",
}

var searchOf = map[string]string{
	"apt": "apt search", "dnf": "dnf search", "yum": "yum search", "zypper": "zypper search", "pacman": "pacman -Ss", "apk": "apk search",
}

// Repair behebt Ausrutscher mit genau einer richtigen Lösung und nennt sie.
func Repair(question, line string, sys Sys) (string, []string) {
	var notes []string
	if strings.Contains(line, "\n") {
		return line, nil
	}
	// ps … | head -n 1 → head -n 2
	parts := strings.Split(line, "|")
	for i := 0; i+1 < len(parts); i++ {
		before := parts[i]
		for _, sep := range []string{";", "&&"} {
			if k := strings.LastIndex(before, sep); k >= 0 {
				before = before[k+len(sep):]
			}
		}
		if psHeadOne(strings.TrimSpace(before), strings.TrimSpace(parts[i+1])) {
			parts[i+1] = headOneRepl.ReplaceAllString(parts[i+1], "head -n 2")
			notes = append(notes, tr("head -n 2 statt head -n 1 (ps gibt zuerst eine Kopfzeile aus)"))
		}
	}
	line = strings.Join(parts, "|")
	// grep auf Ordner → grep -r
	fixedGrep := false
	line = grepCall.ReplaceAllStringFunc(line, func(m string) string {
		sm := grepCall.FindStringSubmatch(m)
		toks, ok := Tokens(sm[3])
		if !ok || !grepOnDir(toks) {
			return m
		}
		fixedGrep = true
		return sm[1] + sm[2] + " -r" + sm[3]
	})
	if fixedGrep {
		notes = append(notes, tr("grep -r ergänzt (durchsucht einen Ordner)"))
	}
	// „ohne sudo“ und der Befehl nutzt sudo → weg damit
	for _, f := range Forbidden(question) {
		if f == "sudo" && !sudoAsOther.MatchString(line) && leadingSudo.MatchString(line) {
			line = leadingSudo.ReplaceAllString(line, "$1")
			notes = append(notes, tr("sudo entfernt (die Frage sagt ohne sudo)"))
		}
	}
	// macOS-Formen mit genau einer Linux-Entsprechung
	if re := notLinux[0].re; re.MatchString(line) {
		line = regexp.MustCompile(`(-i)\s+(?:''|"")(\s|$)`).ReplaceAllString(line, "$1$2")
		notes = append(notes, tr("sed -i ohne '' (Linux-Form)"))
	}
	// Paketliste/Suche einer anderen Distribution → die dieses Systems
	if pm := sys.PkgManager; pm != "" && !namesManager.MatchString(question) {
		if pkgListing.MatchString(line) && !strings.Contains(line, listingOf[pm]) && listingOf[pm] != "" {
			line = listingOf[pm]
			notes = append(notes, fmt.Sprintf(tr("Paketliste für %s angepasst"), pm))
		} else if m := pkgSearch.FindStringSubmatch(line); m != nil && searchOf[pm] != "" && !strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "sudo ")), searchOf[pm]) {
			line = searchOf[pm] + " " + m[1]
			notes = append(notes, fmt.Sprintf(tr("Paketsuche für %s angepasst"), pm))
		}
	}
	return line, notes
}

// foreignManager: Paketverwaltung einer anderen Distribution.
func foreignManager(question, line string, sys Sys) []Issue {
	if sys.PkgManager == "" || namesManager.MatchString(question) {
		return nil
	}
	family := map[string]string{"apt": "deb", "apt-get": "deb", "dpkg": "deb", "aptitude": "deb", "dnf": "rpm", "yum": "rpm",
		"rpm": "rpm", "zypper": "suse", "pacman": "arch", "apk": "alpine"}
	mine := family[sys.PkgManager]
	if sys.PkgManager == "zypper" {
		family["rpm"] = "suse"
	}
	for _, p := range Programs(line) {
		f, ok := family[p]
		if ok && f != mine {
			return []Issue{{KindSlip, fmt.Sprintf(tr("%s gibt es hier nicht, dieses System nutzt %s"), p, sys.PkgManager)}}
		}
	}
	return nil
}

// ---------------------------------------------------------------- alles zusammen

// Result: Ergebnis einer Prüfung.
type Result struct {
	Command string   // nach den Reparaturen
	Repairs []string // was repariert wurde
	Issues  []Issue  // was offen bleibt
}

// OK: keine Einwände.
func (r Result) OK() bool { return len(r.Issues) == 0 }

// Static prüft ohne Host: Syntax, macOS-Formen, Vorgaben, Angaben aus der
// Frage und Ausrutscher; repariert, was eindeutig ist.
func Static(question, line string, sys Sys) Result {
	line = strings.TrimSpace(line)
	fixed, notes := Repair(question, line, sys)
	r := Result{Command: fixed, Repairs: notes}
	if _, ok := Tokens(fixed); !ok {
		r.Issues = append(r.Issues, Issue{KindSyntax, tr("Anführungszeichen oder Klammer nicht geschlossen")})
		return r
	}
	r.Issues = append(r.Issues, NotLinux(fixed)...)
	if question != "" {
		r.Issues = append(r.Issues, forbiddenIssues(question, fixed)...)
		r.Issues = append(r.Issues, detailIssues(question, fixed)...)
	}
	r.Issues = append(r.Issues, slipIssues(question, fixed)...)
	r.Issues = append(r.Issues, foreignManager(question, fixed, sys)...)
	return r
}

// Full: Static plus die Doku des Hosts.
func Full(question, line string, sys Sys, docs Docs) Result {
	r := Static(question, line, sys)
	if len(docs) > 0 {
		r.Issues = append(r.Issues, CheckDocs(r.Command, docs)...)
	}
	return r
}

// Texts: die Einwände als Sätze.
func (r Result) Texts() []string {
	var out []string
	for _, is := range r.Issues {
		out = append(out, is.Text)
	}
	return out
}
