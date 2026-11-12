package agent

import (
	"path"
	"strings"
)

// Welche Dateien ändert ein Befehl? Für die Sicherung vor einem Schritt im
// Ziel-/Agent-Modus (Rückgängig). Eine Heuristik für die üblichen Wege:
// Umleitung (> / >>), tee, sed -i, cp/mv/install auf eine Datei, rm ohne -r,
// truncate. Nur absolute Pfade und ~/…, ohne Platzhalter und Variablen –
// alles andere lässt sich vorher nicht sicher bestimmen.

// FileChange: eine Datei, die ein Befehl ändern oder löschen wird.
type FileChange struct {
	Path string
	Sudo bool // der Befehl läuft mit sudo (Wiederherstellen braucht es dann auch)
}

// ChangedFiles listet die Zieldateien eines Befehls (ohne Doppelte).
func ChangedFiles(cmd string) []FileChange {
	var out []FileChange
	seen := map[string]bool{}
	add := func(p string, sudo bool) {
		p = cleanTarget(p)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, FileChange{Path: p, Sudo: sudo})
	}
	for _, part := range splitParts(cmd) {
		toks := shellTokens(part)
		sudo := false
		// Umleitungen gelten für den ganzen Teil
		var args []string
		for i := 0; i < len(toks); i++ {
			t := toks[i]
			switch {
			case t == ">" || t == ">>" || t == "1>" || t == "1>>" || t == "&>" || t == "&>>":
				if i+1 < len(toks) {
					add(toks[i+1], false) // Umleitung macht die Shell des Nutzers, nicht sudo
					i++
				}
				continue
			case strings.HasPrefix(t, ">>") && len(t) > 2:
				add(t[2:], false)
				continue
			case strings.HasPrefix(t, ">") && len(t) > 1 && t[1] != '&' && t[1] != '(':
				add(t[1:], false)
				continue
			case strings.HasPrefix(t, "2>") || strings.HasPrefix(t, "<"):
				if (t == "2>" || t == "2>>" || t == "<" || t == "<<<") && i+1 < len(toks) {
					i++
				}
				continue
			}
			args = append(args, t)
		}
		// Vorspann: sudo [-u x] [-E], env VAR=…, VAR=…
		for len(args) > 0 {
			a := args[0]
			if a == "sudo" || a == "doas" {
				sudo = true
				args = args[1:]
				for len(args) > 0 && strings.HasPrefix(args[0], "-") {
					if args[0] == "-u" || args[0] == "-g" {
						args = args[1:]
					}
					if len(args) > 0 {
						args = args[1:]
					}
				}
				continue
			}
			if a == "env" || a == "nohup" || a == "command" || (strings.Contains(a, "=") && !strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "/")) {
				args = args[1:]
				continue
			}
			break
		}
		if len(args) == 0 {
			continue
		}
		prog := path.Base(args[0])
		rest := args[1:]
		plain, opts := splitOpts(prog, rest)
		switch prog {
		case "tee":
			for _, p := range plain {
				add(p, sudo)
			}
		case "sed", "perl":
			if !hasOpt(opts, "-i", "--in-place") {
				continue
			}
			files := plain
			if !hasOpt(opts, "-e", "--expression", "-f", "--file") && len(files) > 0 {
				files = files[1:] // erstes Argument ist das Skript
			}
			for _, p := range files {
				add(p, sudo)
			}
		case "cp", "mv", "install", "ln":
			if prog == "ln" && !hasOpt(opts, "-f", "--force") {
				continue
			}
			if hasOpt(opts, "-t", "--target-directory") || len(plain) < 2 {
				continue
			}
			dst := plain[len(plain)-1]
			if !strings.HasSuffix(dst, "/") {
				add(dst, sudo)
			}
			if prog == "mv" {
				for _, p := range plain[:len(plain)-1] {
					add(p, sudo) // Quelle verschwindet
				}
			}
		case "rm", "unlink", "truncate", "shred":
			if prog == "rm" && hasOpt(opts, "-r", "-R", "--recursive") {
				continue
			}
			for _, p := range plain {
				add(p, sudo)
			}
		case "dd":
			for _, a := range rest {
				if strings.HasPrefix(a, "of=") {
					add(a[3:], sudo)
				}
			}
		}
	}
	return out
}

// splitOpts trennt Optionen von den übrigen Argumenten. Kurze Optionen wie
// -i.bak oder -ni zählen als -i.
func splitOpts(prog string, args []string) (plain, opts []string) {
	val := map[string]bool{}
	switch prog {
	case "sed", "perl":
		val = map[string]bool{"-e": true, "-f": true, "--expression": true, "--file": true}
	case "cp", "mv", "install", "ln":
		val = map[string]bool{"-t": true, "-S": true, "-m": true, "-o": true, "-g": true}
	case "truncate":
		val = map[string]bool{"-s": true, "-r": true}
	case "shred":
		val = map[string]bool{"-n": true, "-s": true}
	}
	end := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if end || !strings.HasPrefix(a, "-") || a == "-" {
			plain = append(plain, a)
			continue
		}
		if a == "--" {
			end = true
			continue
		}
		opts = append(opts, a)
		if val[a] && i+1 < len(args) {
			opts = append(opts, args[i+1])
			i++
		}
	}
	return plain, opts
}

func hasOpt(opts []string, names ...string) bool {
	for _, o := range opts {
		for _, n := range names {
			if o == n || strings.HasPrefix(o, n+"=") {
				return true
			}
			// kurze Optionen zusammengefasst: -ni, -i.bak, -rf
			if len(n) == 2 && n[0] == '-' && len(o) > 1 && o[0] == '-' && o[1] != '-' {
				flags := o[1:]
				if n == "-i" { // sed -i.bak: alles nach i ist die Endung
					if strings.ContainsRune(flags, 'i') && !strings.ContainsAny(flags[:strings.IndexRune(flags, 'i')], "=") {
						return true
					}
					continue
				}
				if strings.ContainsRune(flags, rune(n[1])) {
					return true
				}
			}
		}
	}
	return false
}

// cleanTarget: nur Pfade, die sich sicher auflösen lassen.
func cleanTarget(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || strings.ContainsAny(p, "*?[]{}$`\\") {
		return ""
	}
	if !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "~/") {
		return ""
	}
	for _, pre := range []string{"/dev/", "/proc/", "/sys/", "/run/"} {
		if strings.HasPrefix(p, pre) {
			return ""
		}
	}
	if strings.HasSuffix(p, "/") || p == "/" || p == "~/" {
		return ""
	}
	if strings.HasPrefix(p, "~/") {
		return "~/" + strings.TrimPrefix(path.Clean("/"+p[2:]), "/")
	}
	return path.Clean(p)
}

// splitParts teilt an ; && || | und Zeilenumbrüchen (außerhalb von Anführungszeichen).
func splitParts(cmd string) []string {
	var parts []string
	var b strings.Builder
	var q rune
	rs := []rune(cmd)
	flush := func() {
		if s := strings.TrimSpace(b.String()); s != "" {
			parts = append(parts, s)
		}
		b.Reset()
	}
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		if q != 0 {
			b.WriteRune(c)
			if c == q {
				q = 0
			} else if c == '\\' && q == '"' && i+1 < len(rs) {
				i++
				b.WriteRune(rs[i])
			}
			continue
		}
		switch c {
		case '\'', '"':
			q = c
			b.WriteRune(c)
		case '\\':
			b.WriteRune(c)
			if i+1 < len(rs) {
				i++
				b.WriteRune(rs[i])
			}
		case ';', '\n':
			flush()
		case '&':
			if i+1 < len(rs) && rs[i+1] == '&' {
				flush()
				i++
			} else if i+1 < len(rs) && rs[i+1] == '>' {
				b.WriteRune(c) // &> Umleitung
			} else if i > 0 && rs[i-1] == '>' {
				b.WriteRune(c) // 2>&1
			} else {
				flush()
			}
		case '|':
			flush()
			if i+1 < len(rs) && rs[i+1] == '|' {
				i++
			}
		default:
			b.WriteRune(c)
		}
	}
	flush()
	return parts
}

// shellTokens: einfache Wortzerlegung mit Anführungszeichen. Umleitungen
// stehen als eigene Wörter, wenn sie mit Leerzeichen geschrieben sind;
// „>datei“ bleibt zusammen (siehe ChangedFiles).
func shellTokens(s string) []string {
	var out []string
	var b strings.Builder
	var q rune
	has := false
	for _, c := range s {
		switch {
		case q != 0:
			if c == q {
				q = 0
			} else {
				b.WriteRune(c)
			}
		case c == '\'' || c == '"':
			q, has = c, true
		case c == ' ' || c == '\t':
			if has || b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
				has = false
			}
		default:
			b.WriteRune(c)
		}
	}
	if has || b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}
