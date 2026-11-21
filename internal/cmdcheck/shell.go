// Package cmdcheck prüft Shell-Befehle, ohne sie auszuführen: Gibt es das
// Programm, kennt es die Optionen, hält der Befehl die Vorgaben aus der Frage
// ein („ohne lsof“, „.log-Dateien“, „Port 8080“), und steckt einer der
// bekannten Ausrutscher darin (grep auf einen Ordner ohne -r, ps | head -n 1)?
// Was genau eine richtige Lösung hat, wird repariert. Dazu erklärt es einen
// Befehl Teil für Teil aus der man-Seite des Hosts, ganz ohne KI.
//
// Die Idee und viele Regeln stammen aus lxask von Aaron Thomas
// (github.com/arxhr007/lxask, MIT-Lizenz); hier neu geschrieben, auf Deutsch
// und Englisch, und gegen die Doku des echten Zielsystems statt eines
// mitgelieferten Index.
package cmdcheck

import (
	"strings"
)

// Token: ein Wort der Befehlszeile nach dem Entfernen der Anführungszeichen.
type Token struct {
	Text   string
	Quoted bool // ganz oder teilweise in Anführungszeichen (dann nie eine Option)
	Op     bool // Verkettung: | || && ; & |& ( )
}

// Tokens zerlegt eine Zeile wie die Shell (vereinfacht). Umleitungen samt
// Ziel fallen weg; ok=false bei offenen Anführungszeichen.
func Tokens(line string) (out []Token, ok bool) {
	r := []rune(line)
	var cur strings.Builder
	have, quoted := false, false
	flush := func() {
		if have {
			out = append(out, Token{Text: cur.String(), Quoted: quoted})
		}
		cur.Reset()
		have, quoted = false, false
	}
	for i := 0; i < len(r); i++ {
		c := r[i]
		switch {
		case c == '\\' && i+1 < len(r):
			if r[i+1] == '\n' {
				i++
				continue
			}
			cur.WriteRune(r[i+1])
			have = true
			i++
		case c == '\'':
			j := i + 1
			for j < len(r) && r[j] != '\'' {
				j++
			}
			if j >= len(r) {
				return out, false
			}
			cur.WriteString(string(r[i+1 : j]))
			have, quoted = true, true
			i = j
		case c == '"':
			j := i + 1
			for j < len(r) && r[j] != '"' {
				if r[j] == '\\' && j+1 < len(r) {
					j++
				}
				j++
			}
			if j >= len(r) {
				return out, false
			}
			s := string(r[i+1 : j])
			s = strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\$`, `$`, "\\`", "`").Replace(s)
			cur.WriteString(s)
			have, quoted = true, true
			i = j
		case c == '$' && i+1 < len(r) && r[i+1] == '(':
			// Befehlsersetzung bleibt ein Wort (Inhalt prüft Substitutions)
			depth, j := 0, i+1
			for ; j < len(r); j++ {
				if r[j] == '(' {
					depth++
				} else if r[j] == ')' {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			if j >= len(r) {
				return out, false
			}
			cur.WriteString(string(r[i : j+1]))
			have, quoted = true, true
			i = j
		case c == '`':
			j := i + 1
			for j < len(r) && r[j] != '`' {
				j++
			}
			if j >= len(r) {
				return out, false
			}
			cur.WriteString(string(r[i : j+1]))
			have, quoted = true, true
			i = j
		case c == ' ' || c == '\t' || c == '\r':
			flush()
		case c == '\n':
			flush()
			out = append(out, Token{Text: ";", Op: true})
		case c == '#' && !have:
			// Kommentar bis Zeilenende
			for i < len(r) && r[i] != '\n' {
				i++
			}
			i--
		case c == '|' || c == '&' || c == ';' || c == '(' || c == ')':
			// &> / &>> ist eine Umleitung
			if c == '&' && i+1 < len(r) && r[i+1] == '>' {
				flush()
				i++
				if i+1 < len(r) && r[i+1] == '>' {
					i++
				}
				i = skipTarget(r, i+1) - 1
				continue
			}
			// 2>&1: & nach > gehört zur Umleitung (oben behandelt)
			flush()
			op := string(c)
			if i+1 < len(r) && ((c == '|' && (r[i+1] == '|' || r[i+1] == '&')) || (c == '&' && r[i+1] == '&') || (c == ';' && r[i+1] == ';')) {
				op += string(r[i+1])
				i++
			}
			out = append(out, Token{Text: op, Op: true})
		case c == '>' || c == '<':
			// Umleitung: führende Ziffer des Worts gehört dazu (2>)
			if have && isDigits(cur.String()) && !quoted {
				cur.Reset()
				have = false
			}
			flush()
			j := i + 1
			if j < len(r) && (r[j] == '>' || r[j] == '<') {
				j++
			}
			if c == '<' && j < len(r) && r[j] == '<' { // <<< here-string
				j++
			}
			if j < len(r) && r[j] == '&' { // >&2, 2>&1
				j++
				for j < len(r) && (r[j] >= '0' && r[j] <= '9' || r[j] == '-') {
					j++
				}
				i = j - 1
				continue
			}
			if j < len(r) && r[j] == '(' { // <(…) Prozessersetzung: als Wort behalten
				i = j - 1
				cur.WriteRune(c)
				have, quoted = true, true
				continue
			}
			i = skipTarget(r, j) - 1
		default:
			cur.WriteRune(c)
			have = true
		}
	}
	flush()
	return out, true
}

// skipTarget überspringt Leerzeichen und das Ziel einer Umleitung.
func skipTarget(r []rune, j int) int {
	for j < len(r) && (r[j] == ' ' || r[j] == '\t') {
		j++
	}
	for j < len(r) {
		c := r[j]
		if c == ' ' || c == '\t' || c == '\n' || c == '|' || c == ';' || c == '&' || c == ')' {
			break
		}
		if c == '\'' || c == '"' {
			k := j + 1
			for k < len(r) && r[k] != c {
				k++
			}
			j = k + 1
			continue
		}
		j++
	}
	return j
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Simple zerlegt in einfache Befehle (an | && || ; & und Klammern).
func Simple(toks []Token) [][]Token {
	var out [][]Token
	var cur []Token
	for _, t := range toks {
		if t.Op {
			if len(cur) > 0 {
				out = append(out, cur)
			}
			cur = nil
			continue
		}
		cur = append(cur, t)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// Use: ein Programmaufruf mit seinen Argumenten (Wrapper wie sudo, xargs,
// env, timeout und find -exec aufgelöst).
type Use struct {
	Program string
	Args    []Token
	Wrapped bool // stand hinter sudo, xargs, find -exec …
}

// Shell-Schlüsselwörter, nach denen ein Befehl folgt, und solche ohne.
var (
	kwBefore = set("if", "then", "else", "elif", "do", "while", "until", "!", "{", "time", "exec", "command", "builtin", "nohup", "nice", "ionice", "stdbuf", "chrt", "taskset", "unbuffer")
	kwNone   = set("for", "case", "select", "function", "done", "fi", "esac", "}", "in", "[[", "]]")
)

// Wrapper mit Optionen, die ein Argument verbrauchen.
var wrapperValueOpts = map[string]map[string]bool{
	"sudo":    set("-u", "-g", "-C", "-h", "-p", "-U", "-r", "-t", "-D", "--user", "--group", "--host", "--prompt", "--other-user", "--role", "--type", "--chdir"),
	"doas":    set("-u", "-C"),
	"timeout": set("-s", "-k", "--signal", "--kill-after"),
	"xargs":   set("-I", "-i", "-n", "-P", "-d", "-a", "-L", "-l", "-s", "-E", "-e", "--max-args", "--max-procs", "--delimiter", "--arg-file", "--max-lines", "--max-chars", "--replace"),
	"nice":    set("-n", "--adjustment"),
	"ionice":  set("-c", "-n", "-p", "--class", "--classdata"),
	"watch":   set("-n", "--interval", "-d"),
	"env":     set("-u", "--unset", "-C", "--chdir", "-S"),
	"stdbuf":  set("-i", "-o", "-e"),
	"taskset": set(),
	"chrt":    set(),
}

// Wrappers: Programme, die einen anderen Befehl ausführen.
var wrappers = set("sudo", "doas", "env", "timeout", "xargs", "nice", "nohup", "watch", "time", "ionice", "stdbuf", "command", "exec", "builtin", "unbuffer", "chrt", "taskset", "strace", "ltrace", "flock", "setsid", "runuser", "su")

// Uses liefert alle Programmaufrufe einer Zeile, auch in $(…).
func Uses(line string) ([]Use, bool) {
	toks, ok := Tokens(line)
	if !ok {
		return nil, false
	}
	var out []Use
	for _, sc := range Simple(toks) {
		resolve(sc, false, &out, 0)
	}
	for _, t := range toks {
		for _, sub := range substitutions(t.Text) {
			if u, ok := Uses(sub); ok {
				out = append(out, u...)
			}
		}
	}
	return out, true
}

// substitutions: Inhalte von $(…) und `…` in einem Wort.
func substitutions(s string) []string {
	var out []string
	for {
		i := strings.Index(s, "$(")
		if i < 0 {
			break
		}
		depth, j := 0, i+1
		for ; j < len(s); j++ {
			if s[j] == '(' {
				depth++
			} else if s[j] == ')' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		if j >= len(s) {
			break
		}
		if !strings.HasPrefix(s[i:], "$((") { // Rechnen, kein Befehl
			out = append(out, s[i+2:j])
		}
		s = s[j+1:]
	}
	for {
		i := strings.IndexByte(s, '`')
		if i < 0 {
			break
		}
		j := strings.IndexByte(s[i+1:], '`')
		if j < 0 {
			break
		}
		out = append(out, s[i+1:i+1+j])
		s = s[i+2+j:]
	}
	return out
}

func resolve(sc []Token, wrapped bool, out *[]Use, depth int) {
	if depth > 6 {
		return
	}
	// Zuweisungen am Anfang (LANG=C …) und Schlüsselwörter überspringen
	for len(sc) > 0 && !sc[0].Quoted && (isAssign(sc[0].Text) || kwBefore[sc[0].Text] && !wrappers[sc[0].Text]) {
		sc = sc[1:]
	}
	if len(sc) == 0 || kwNone[sc[0].Text] || sc[0].Quoted && strings.HasPrefix(sc[0].Text, "$") {
		return
	}
	name := sc[0].Text
	args := sc[1:]
	if wrappers[name] {
		*out = append(*out, Use{Program: name, Args: wrapperOwnArgs(name, args), Wrapped: wrapped})
		rest := afterWrapper(name, args)
		if len(rest) > 0 {
			resolve(rest, true, out, depth+1)
		}
		return
	}
	if name == "find" {
		own, inner := splitFindExec(args)
		*out = append(*out, Use{Program: name, Args: own, Wrapped: wrapped})
		for _, in := range inner {
			resolve(in, true, out, depth+1)
		}
		return
	}
	if (name == "sh" || name == "bash" || name == "zsh") && len(args) >= 2 && args[0].Text == "-c" {
		*out = append(*out, Use{Program: name, Args: args[:1], Wrapped: wrapped})
		if u, ok := Uses(args[1].Text); ok {
			for _, x := range u {
				x.Wrapped = true
				*out = append(*out, x)
			}
		}
		return
	}
	*out = append(*out, Use{Program: name, Args: args, Wrapped: wrapped})
}

func isAssign(s string) bool {
	i := strings.IndexByte(s, '=')
	if i <= 0 {
		return false
	}
	for j, c := range s[:i] {
		if !(c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || j > 0 && c >= '0' && c <= '9' || j == i-1 && c == '+') {
			return false
		}
	}
	return true
}

// afterWrapper: der Befehl hinter sudo/xargs/timeout …
func afterWrapper(name string, args []Token) []Token {
	vals := wrapperValueOpts[name]
	i := 0
	switch name {
	case "su", "runuser":
		// su -c 'befehl' [user]
		for k := 0; k < len(args); k++ {
			if args[k].Text == "-c" && k+1 < len(args) {
				toks, ok := Tokens(args[k+1].Text)
				if ok {
					return toks
				}
			}
		}
		return nil
	case "flock":
		for i < len(args) && strings.HasPrefix(args[i].Text, "-") {
			i++
		}
		i++ // Sperrdatei
	}
	for i < len(args) {
		t := args[i].Text
		if args[i].Quoted || !strings.HasPrefix(t, "-") || t == "-" {
			break
		}
		if t == "--" {
			i++
			break
		}
		i++
		base := t
		if k := strings.IndexByte(t, '='); k > 0 {
			continue
		} else if vals[base] {
			i++
		}
	}
	if name == "env" {
		for i < len(args) && isAssign(args[i].Text) {
			i++
		}
	}
	if name == "timeout" && i < len(args) {
		i++ // Dauer
	}
	if (name == "taskset" || name == "chrt") && i < len(args) {
		i++ // Maske oder Priorität
	}
	if i >= len(args) {
		return nil
	}
	return args[i:]
}

// wrapperOwnArgs: die Optionen des Wrappers selbst (ohne den Befehl dahinter).
func wrapperOwnArgs(name string, args []Token) []Token {
	rest := afterWrapper(name, args)
	n := len(args) - len(rest)
	if rest == nil {
		n = len(args)
	}
	if name == "su" || name == "runuser" {
		return nil
	}
	return args[:max(0, n)]
}

// splitFindExec trennt find-Ausdrücke von den Befehlen in -exec … ; / +.
func splitFindExec(args []Token) (own []Token, inner [][]Token) {
	for i := 0; i < len(args); i++ {
		t := args[i].Text
		if !args[i].Quoted && (t == "-exec" || t == "-execdir" || t == "-ok" || t == "-okdir") {
			own = append(own, args[i])
			j := i + 1
			for j < len(args) && args[j].Text != ";" && args[j].Text != "+" && args[j].Text != `\;` {
				j++
			}
			if j > i+1 {
				inner = append(inner, args[i+1:j])
			}
			i = j
			continue
		}
		own = append(own, args[i])
	}
	return own, inner
}

// Programs: die Namen aller aufgerufenen Programme, ohne Doppelte.
func Programs(line string) []string {
	u, _ := Uses(line)
	seen := map[string]bool{}
	var out []string
	for _, x := range u {
		p := baseName(x.Program)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func baseName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 && i < len(p)-1 {
		return p[i+1:]
	}
	return p
}

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
