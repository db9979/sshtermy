package cmdcheck

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Doc: was der Host über ein Programm weiß.
type Doc struct {
	Missing bool   // command -v findet es nicht
	Builtin bool   // Shell-Befehl, Alias oder Funktion (keine eigene Doku)
	Path    string // wo es liegt
	Source  string // "man", "help" oder ""
	Text    string // Doku als Klartext (ohne Überdruck)
}

// Known: Es gibt brauchbare Doku zum Prüfen der Optionen.
func (d *Doc) Known() bool {
	if d == nil || d.Missing || d.Builtin || len(d.Text) < 120 {
		return false
	}
	if d.Source == "help" && partialHelp.MatchString(d.Text) {
		return false // gekürzte Hilfe (curl, ps): nennt nicht alle Optionen
	}
	return len(optLine.FindAllStringIndex(d.Text, 4)) >= 3
}

var (
	partialHelp = regexp.MustCompile(`(?i)not the full help|--help all|--help <|try '[^']*--help [a-z]`)
	optLine     = regexp.MustCompile(`(?m)^\s{0,12}--?[A-Za-z0-9]`)
)

// Docs: Doku je Programmname.
type Docs map[string]*Doc

const marker = "@@SSHTERMY-DOC "

// MaxProbe: so viele Programme fragt ein Prüflauf höchstens ab.
const MaxProbe = 10

// ProbeScript: ein sh-Skript, das für jedes Programm meldet, ob es da ist,
// und seine man-Seite (sonst --help, nur für Programme aus den System-
// ordnern, mit Zeitlimit und ohne Eingabe) ausgibt. Es liest nur.
func ProbeScript(programs []string) string {
	var b strings.Builder
	b.WriteString("export LC_ALL=C MANWIDTH=120 MANPAGER=cat PAGER=cat GROFF_NO_SGR=1; ")
	b.WriteString("to=; command -v timeout >/dev/null 2>&1 && to='timeout 3'; ")
	b.WriteString("for p in")
	n := 0
	for _, p := range programs {
		if !safeName(p) {
			continue
		}
		b.WriteString(" " + p)
		n++
		if n >= MaxProbe {
			break
		}
	}
	b.WriteString(`; do printf '\n` + marker + `%s\n' "$p"; t=$(command -v "$p" 2>/dev/null); `)
	b.WriteString(`if [ -z "$t" ]; then echo '@@missing'; continue; fi; `)
	b.WriteString(`case "$t" in /*) ;; *) echo '@@builtin'; continue;; esac; echo "@@path $t"; `)
	b.WriteString(`d=$(man "$p" 2>/dev/null | head -c 150000); `)
	b.WriteString(`case "$d" in *"has been minimized"*) d=;; esac; `) // Ubuntu minimal: man ist nur ein Hinweis
	b.WriteString(`if [ -n "$d" ]; then echo '@@man'; printf '%s\n' "$d"; else `)
	b.WriteString(`case "$t" in /usr/bin/*|/bin/*|/usr/sbin/*|/sbin/*|/usr/local/bin/*|/usr/local/sbin/*) `)
	b.WriteString(`echo '@@help'; $to "$p" --help </dev/null 2>&1 | head -c 60000;; esac; fi; done`)
	return b.String()
}

var nameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+-]{0,60}$`)

func safeName(p string) bool { return nameRE.MatchString(p) }

var overstrike = regexp.MustCompile(".\x08")

// ParseProbe liest die Ausgabe von ProbeScript.
func ParseProbe(out string) Docs {
	docs := Docs{}
	parts := strings.Split(out, "\n"+marker)
	for i, part := range parts {
		if i == 0 && !strings.HasPrefix(part, marker) {
			continue
		}
		part = strings.TrimPrefix(part, marker)
		nl := strings.IndexByte(part, '\n')
		if nl < 0 {
			continue
		}
		name := strings.TrimSpace(part[:nl])
		body := part[nl+1:]
		d := &Doc{}
		lines := strings.SplitN(body, "\n", 3)
		for len(lines) > 0 && strings.HasPrefix(lines[0], "@@") {
			l := lines[0]
			switch {
			case l == "@@missing":
				d.Missing = true
			case l == "@@builtin":
				d.Builtin = true
			case strings.HasPrefix(l, "@@path "):
				d.Path = strings.TrimPrefix(l, "@@path ")
			case l == "@@man":
				d.Source = "man"
			case l == "@@help":
				d.Source = "help"
			}
			body = ""
			if len(lines) > 1 {
				body = strings.Join(lines[1:], "\n")
			}
			lines = strings.SplitN(body, "\n", 3)
		}
		if d.Source != "" {
			d.Text = overstrike.ReplaceAllString(body, "")
			d.Text = strings.ReplaceAll(d.Text, "‐", "-") // man: Silbentrenner als Bindestrich
			d.Text = strings.ReplaceAll(d.Text, "−", "-")
		}
		docs[name] = d
	}
	return docs
}

// Merge übernimmt neue Einträge.
func (d Docs) Merge(o Docs) {
	for k, v := range o {
		d[k] = v
	}
}

// Missing: Programme, die noch nicht abgefragt sind.
func (d Docs) Missing(programs []string) []string {
	var out []string
	for _, p := range programs {
		if _, ok := d[p]; !ok && safeName(p) {
			out = append(out, p)
		}
	}
	return out
}

// Builtins der Shell: Optionen nicht prüfen, keine eigene man-Seite.
var shellBuiltins = set("cd", "echo", "printf", "export", "source", ".", "alias", "unalias", "set", "unset", "read", "ulimit",
	"umask", "type", "hash", "eval", "exec", "exit", "return", "shift", "trap", "wait", "jobs", "fg", "bg", "kill", "test", "[",
	"true", "false", "pwd", "local", "declare", "typeset", "let", "history", "builtin", "command", "times", "getopts", ":", "break",
	"continue", "readonly", "shopt", "complete", "disown", "suspend", "logout", "dirs", "pushd", "popd", "mapfile", "readarray")

// Unterbefehle, die in der Doku stehen müssen.
var subcommandStyle = set("systemctl", "apt", "apt-get", "dnf", "yum", "zypper", "docker", "podman", "nmcli", "snap",
	"flatpak", "kubectl", "loginctl", "timedatectl", "hostnamectl", "resolvectl", "networkctl", "ufw", "apk", "busctl",
	"localectl", "coredumpctl", "machinectl", "bootctl", "journalctl")

// Optionen, die jedes Programm versteht.
var alwaysOptions = set("--help", "--version", "-h", "--")

// Programme, bei denen Wörter mit Bindestrich keine Optionen sind (oder sehr frei).
var freeArgs = set("echo", "printf", "test", "[", "[[", "expr", "awk", "gawk", "mawk", "sed", "perl", "python", "python3",
	"ruby", "node", "java", "kill", "pkill", "killall", "chmod", "date", "seq", "bc", "dc", "printenv", "true", "false",
	"ssh", "scp", "rsync", "git", "ffmpeg", "gcc", "g++", "clang", "cc", "ld", "make", "cmake", "go", "cargo", "npm", "npx",
	"pip", "pip3", "mysql", "psql", "su", "sh", "bash", "zsh", "dash", "ksh", "fish", "pwsh", "powershell", "openssl", "iptables",
	"ip6tables", "nft", "tc", "ip", "busybox", "dd")

var (
	longOpt  = regexp.MustCompile(`^--[A-Za-z0-9][\w.-]*`)
	shortOpt = regexp.MustCompile(`^-[A-Za-z0-9?@#][\w.:+,/=%{}-]*$`)
	numeric  = regexp.MustCompile(`^-\d+[\w.]*$`)
	sigName  = regexp.MustCompile(`^-(SIG)?[A-Z][A-Z0-9+]*$`)
)

// documented: steht die Option in der Doku?
func documented(doc, opt string) bool {
	re := regexp.MustCompile(`(^|[^\w-])` + regexp.QuoteMeta(opt) + `([^\w-]|$)`)
	return re.MatchString(doc)
}

// prefixOfLong: GNU erlaubt eindeutige Abkürzungen (--max-d für --max-depth).
func prefixOfLong(doc, opt string) bool {
	if len(opt) < 5 {
		return false
	}
	re := regexp.MustCompile(`(^|[^\w-])` + regexp.QuoteMeta(opt) + `[\w-]+`)
	return re.MatchString(doc)
}

// takesValue: Nimmt die kurze Option ein Argument? (-k KEY, -kKEY, -k=…)
func takesValue(doc, opt string) bool {
	re := regexp.MustCompile(`(^|[^\w-])` + regexp.QuoteMeta(opt) + `(\s?[A-Z<\[{][\w>\]}]*|=|\s[a-z]+\b)`)
	for _, m := range re.FindAllString(doc, 6) {
		if !strings.HasSuffix(strings.TrimSpace(m), ",") {
			return true
		}
	}
	// -k, --key=KEYDEF
	re2 := regexp.MustCompile(regexp.QuoteMeta(opt) + `,\s*--[\w-]+[= ][A-Z<\[]`)
	return re2.MatchString(doc)
}

// CheckDocs prüft Programme, Optionen und Unterbefehle gegen die Doku des
// Hosts. Was die Doku nicht kennt, wird nicht beanstandet.
func CheckDocs(line string, docs Docs) []Issue {
	uses, ok := Uses(line)
	if !ok {
		return nil
	}
	var out []Issue
	seen := map[string]bool{}
	add := func(is Issue) {
		if !seen[is.Text] {
			seen[is.Text] = true
			out = append(out, is)
		}
	}
	for _, u := range uses {
		name := baseName(u.Program)
		d := docs[name]
		if d == nil {
			continue
		}
		if d.Missing {
			if !shellBuiltins[name] && !strings.Contains(u.Program, "$") && !strings.HasPrefix(u.Program, "./") {
				add(Issue{Kind: KindMissing, Text: fmt.Sprintf(tr("%s: auf diesem System nicht installiert"), name)})
			}
			continue
		}
		if !d.Known() || shellBuiltins[name] || freeArgs[name] {
			continue
		}
		for _, o := range undocumentedOptions(name, u.Args, d.Text) {
			add(Issue{Kind: KindOption, Text: fmt.Sprintf(tr("%s: %s steht nicht in der Doku"), name, o)})
		}
		if subcommandStyle[name] && d.Source != "" {
			if sub := firstOperand(u.Args); sub != "" && name != "journalctl" {
				if !documented(d.Text, sub) {
					add(Issue{Kind: KindSubcmd, Text: fmt.Sprintf(tr("%s: Unterbefehl %s steht nicht in der Doku"), name, sub)})
				}
			}
		}
	}
	return out
}

func firstOperand(args []Token) string {
	for _, a := range args {
		if a.Quoted {
			return ""
		}
		if strings.HasPrefix(a.Text, "-") {
			continue
		}
		if regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(a.Text) {
			return a.Text
		}
		return ""
	}
	return ""
}

// undocumentedOptions: Optionen der Argumente, die die Doku nicht nennt.
func undocumentedOptions(name string, args []Token, doc string) []string {
	var out []string
	for _, a := range args {
		t := a.Text
		if a.Quoted || t == "-" || !strings.HasPrefix(t, "-") {
			continue
		}
		if t == "--" {
			break
		}
		if alwaysOptions[t] || numeric.MatchString(t) || sigName.MatchString(t) {
			continue
		}
		if strings.HasPrefix(t, "--") {
			m := longOpt.FindString(t)
			if m == "" || documented(doc, m) || prefixOfLong(doc, m) {
				continue
			}
			// --[no-]foo
			if strings.HasPrefix(m, "--no-") && documented(doc, "--"+m[5:]) {
				continue
			}
			out = append(out, m)
			continue
		}
		if !shortOpt.MatchString(t) {
			continue
		}
		whole := t
		if i := strings.IndexByte(whole, '='); i > 0 {
			whole = whole[:i]
		}
		if documented(doc, whole) {
			continue // -name, -mtime, -xdev …
		}
		// gebündelte kurze Optionen: -czf, -k2nr (ab der Ziffer der Wert)
		for k, c := range t[1:] {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
				break
			}
			o := "-" + string(c)
			if !documented(doc, o) {
				if k == 0 && len(whole) > 2 {
					o = whole // eher eine lange Option mit einem Strich (find -iname)
				}
				out = append(out, o)
				break
			}
			if takesValue(doc, o) {
				break // Rest ist der Wert (-ofile, -Wall …)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- Erklären

// Part: eine Zeile der Erklärung.
type Part struct {
	Program bool   // Programmzeile (sonst Option)
	Name    string // Programm oder Option
	Text    string // ein Satz aus der Doku; leer = Doku sagt nichts
}

// Explain zerlegt einen Befehl in Programme und Optionen und holt zu jedem
// den passenden Satz aus der Doku des Hosts. Erfunden wird nichts.
func Explain(line string, docs Docs) []Part {
	uses, ok := Uses(line)
	if !ok {
		return nil
	}
	var out []Part
	for _, u := range uses {
		name := baseName(u.Program)
		d := docs[name]
		p := Part{Program: true, Name: name}
		switch {
		case d == nil:
		case d.Missing:
			p.Text = tr("nicht installiert")
		case d.Builtin || shellBuiltins[name]:
			if s, ok := builtinText[name]; ok {
				p.Text = tr(s)
			} else {
				p.Text = tr("Befehl der Shell")
			}
		default:
			p.Text = summary(name, d.Text)
		}
		out = append(out, p)
		if d == nil || !d.Known() {
			continue
		}
		seen := map[string]bool{}
		for _, o := range optionsOf(u.Args, d.Text) {
			if seen[o] {
				continue
			}
			seen[o] = true
			if t := optionText(d.Text, o); t != "" {
				out = append(out, Part{Name: o, Text: t})
			}
		}
		if subcommandStyle[name] {
			if sub := firstOperand(u.Args); sub != "" {
				if t := optionText(d.Text, sub); t != "" {
					out = append(out, Part{Name: sub, Text: t})
				}
			}
		}
	}
	return out
}

var builtinText = map[string]string{
	"cd": "wechselt den Ordner", "echo": "gibt Text aus", "export": "setzt eine Umgebungsvariable",
	"source": "führt ein Skript in dieser Shell aus", ".": "führt ein Skript in dieser Shell aus",
	"alias": "legt einen Kurznamen an", "read": "liest eine Zeile ein", "kill": "schickt einem Prozess ein Signal",
	"test": "prüft eine Bedingung", "[": "prüft eine Bedingung", "pwd": "zeigt den aktuellen Ordner",
	"printf": "gibt formatierten Text aus", "ulimit": "setzt Grenzen für Prozesse", "umask": "setzt die Rechte-Maske für neue Dateien",
	"exit": "beendet die Shell", "history": "zeigt den Befehlsverlauf", "type": "zeigt, was ein Name ist",
}

// optionsOf: die Optionen der Argumente, gebündelte einzeln.
func optionsOf(args []Token, doc string) []string {
	var out []string
	for _, a := range args {
		t := a.Text
		if a.Quoted || t == "-" || t == "--" || !strings.HasPrefix(t, "-") || numeric.MatchString(t) {
			continue
		}
		if strings.HasPrefix(t, "--") {
			if m := longOpt.FindString(t); m != "" {
				out = append(out, m)
			}
			continue
		}
		if !shortOpt.MatchString(t) {
			continue
		}
		whole := t
		if i := strings.IndexByte(whole, '='); i > 0 {
			whole = whole[:i]
		}
		if documented(doc, whole) {
			out = append(out, whole)
			continue
		}
		for _, c := range t[1:] {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
				break
			}
			o := "-" + string(c)
			out = append(out, o)
			if takesValue(doc, o) {
				break
			}
		}
	}
	return out
}

var nameLine = regexp.MustCompile(`(?m)^\s*([\w.+-]+(?:,\s*[\w.+-]+)*)\s+\\?[-–—]+\s+(.+)$`)

// summary: der Satz aus dem Abschnitt NAME (sonst die erste Zeile von --help).
func summary(name, doc string) string {
	i := strings.Index(doc, "\nNAME")
	if strings.HasPrefix(doc, "NAME") {
		i = -1
	}
	if i >= 0 || strings.HasPrefix(doc, "NAME") {
		sec := doc[i+1:]
		sec = sec[:min(len(sec), 600)]
		lines := strings.Split(sec, "\n")
		var joined []string
		for _, l := range lines[1:] {
			l = strings.TrimSpace(l)
			if l == "" {
				if len(joined) > 0 {
					break
				}
				continue
			}
			if strings.ToUpper(l) == l && len(l) > 3 && !strings.Contains(l, " - ") {
				break // nächster Abschnitt
			}
			joined = append(joined, l)
		}
		s := strings.Join(joined, " ")
		if m := nameLine.FindStringSubmatch(s); m != nil {
			return cleanSentence(m[2])
		}
		if k := strings.Index(s, " - "); k > 0 {
			return cleanSentence(s[k+3:])
		}
	}
	// --help: erste Zeile, die kein Usage ist
	for _, l := range strings.Split(doc, "\n") {
		l = strings.TrimSpace(l)
		low := strings.ToLower(l)
		if l == "" || strings.HasPrefix(low, "usage") || strings.HasPrefix(low, "or:") || strings.HasPrefix(l, "-") || strings.HasPrefix(low, name+" ") && strings.Contains(l, "[") {
			continue
		}
		return cleanSentence(l)
	}
	return ""
}

func cleanSentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSuffix(s, ".")
	if r := []rune(s); len(r) > 160 {
		s = string(r[:157]) + "…"
	}
	return s
}

// optionText: der erste Satz zur Option (oder zum Unterbefehl) in der Doku.
func optionText(doc, opt string) string {
	lines := strings.Split(doc, "\n")
	optRE := regexp.MustCompile(`^\s*(?:(?:-[\w?]|--?[\w][\w-]*)(?:[ =]\S+)?\s*,\s*)*` + regexp.QuoteMeta(opt) + `(?:[ =\[<][^\s,]*)?(?:,|\s|$)`)
	for i, l := range lines {
		if !optRE.MatchString(l) {
			continue
		}
		ind := indentOf(l)
		// Text in derselben Zeile nach zwei Leerzeichen (--help-Stil)?
		rest := strings.TrimSpace(l)
		if k := regexp.MustCompile(`\S\s{2,}(\S.*)$`).FindStringSubmatchIndex(rest); k != nil {
			txt := rest[k[2]:]
			// Folgezeilen mit größerem Einzug anhängen
			for j := i + 1; j < len(lines) && j < i+4; j++ {
				nl := lines[j]
				if strings.TrimSpace(nl) == "" || indentOf(nl) <= ind || strings.HasPrefix(strings.TrimSpace(nl), "-") {
					break
				}
				txt += " " + strings.TrimSpace(nl)
			}
			if s := firstSentence(txt); s != "" {
				return s
			}
		}
		// man-Stil: Beschreibung in den folgenden, tiefer eingerückten Zeilen
		var txt []string
		for j := i + 1; j < len(lines) && j < i+8; j++ {
			nl := lines[j]
			if strings.TrimSpace(nl) == "" {
				if len(txt) > 0 {
					break
				}
				continue
			}
			if indentOf(nl) <= ind {
				break
			}
			txt = append(txt, strings.TrimSpace(nl))
		}
		if s := firstSentence(strings.Join(txt, " ")); s != "" {
			return s
		}
	}
	return ""
}

func indentOf(l string) int {
	n := 0
	for _, c := range l {
		switch c {
		case ' ':
			n++
		case '\t':
			n += 8
		default:
			return n
		}
	}
	return n
}

func firstSentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return ""
	}
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '.' && s[i+1] == ' ' && i > 12 {
			// nicht nach Abkürzungen wie „e.g.“ oder „i.e.“
			if i >= 3 && (s[i-2] == '.' || s[i-3:i] == " eg" || s[i-3:i] == " ie") {
				continue
			}
			s = s[:i]
			break
		}
	}
	return cleanSentence(s)
}

// SortedNames: Programmnamen sortiert (für Tests und Anzeige).
func (d Docs) SortedNames() []string {
	var out []string
	for k := range d {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
