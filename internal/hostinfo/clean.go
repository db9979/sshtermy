package hostinfo

import (
	"regexp"
	"strings"
)

var (
	csiRe = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	// Cursor auf Zeile/Spalte setzen: ProCurve/ArubaOS-Switch beginnt so
	// neue Zeilen statt mit "\r\n" (sonst klebt der Prompt am Echo).
	cupRe  = regexp.MustCompile(`\x1b\[[0-9]*(?:;[0-9]*)?[Hf]`)
	oscRe  = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	escRe  = regexp.MustCompile(`\x1b[()][0-9A-Za-z]|\x1b[0-9=>A-DF-Za-z]`)
	moreRe = regexp.MustCompile(`(?i)-+\s*more\s*-+(?:[^\n]*?control-c)?|--more--|<--- more --->|---\(more[^)\n]*\)---|-- \[Q quit\|D dump\|down\]`)
)

// Clean macht aus dem rohen PTY-Strom Klartextzeilen: ANSI-Sequenzen raus
// (ProCurve positioniert den Cursor bei jeder Zeile), "\eE" als Zeilenumbruch,
// Rückschritte angewandt, Blätter-Hinweise ("-- MORE --") entfernt, leere
// Zeilen und Leerzeichen am Ende weg.
func Clean(raw string) []string {
	s := strings.ReplaceAll(raw, "\x1bE", "\n")
	s = oscRe.ReplaceAllString(s, "")
	s = cupRe.ReplaceAllString(s, "\n")
	s = csiRe.ReplaceAllString(s, "")
	s = escRe.ReplaceAllString(s, "")
	s = moreRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out []string
	for _, l := range strings.Split(s, "\n") {
		// "\r" ohne "\n": die Zeile wurde von vorn überschrieben.
		if i := strings.LastIndex(l, "\r"); i >= 0 {
			if rest := l[i+1:]; strings.TrimSpace(rest) != "" {
				l = rest
			} else {
				l = strings.ReplaceAll(l, "\r", "")
			}
		}
		l = applyBackspace(l)
		l = strings.Map(func(r rune) rune {
			if r < 0x20 && r != '\t' {
				return -1
			}
			return r
		}, l)
		l = strings.TrimRight(l, " \t")
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func applyBackspace(s string) string {
	if !strings.Contains(s, "\b") {
		return s
	}
	r := []rune{}
	for _, c := range s {
		if c == '\b' {
			if len(r) > 0 {
				r = r[:len(r)-1]
			}
			continue
		}
		r = append(r, c)
	}
	return string(r)
}

// HasMore: wartet das Gerät am Ende der Ausgabe auf eine Taste zum Blättern?
func HasMore(raw string) bool {
	tail := raw
	if len(tail) > 300 {
		tail = tail[len(tail)-300:]
	}
	tail = csiRe.ReplaceAllString(tail, "")
	loc := moreRe.FindAllStringIndex(tail, -1)
	if len(loc) == 0 {
		return false
	}
	// Nach dem letzten Hinweis darf nichts Sichtbares mehr kommen.
	return strings.TrimSpace(tail[loc[len(loc)-1][1]:]) == ""
}

// IsPrompt: sieht die Zeile wie ein Geräte-Prompt aus ("switch#", "sw01>",
// "core(config)#")?
func IsPrompt(line string) bool {
	l := strings.TrimSpace(line)
	if l == "" || len(l) > 80 {
		return false
	}
	end := l[len(l)-1]
	if (end == '#' || end == '>') && !strings.ContainsAny(l[:len(l)-1], " \t#>") {
		return true
	}
	// RouterOS „[admin@MikroTik] >“ / „[admin@MikroTik] /interface>“,
	// FortiOS „FGT60F #“ / „FGT60F (root) #“
	return vendorPrompt.MatchString(l)
}

var vendorPrompt = regexp.MustCompile(`^\[[\w.@-]+\]\s*(/[\w/-]*)?>$|^[\w.-]+(\s+\([\w.-]+\))?\s+#$`)

// Output: Ausgabe eines Befehls ohne Echo der Befehlszeile und ohne den
// abschließenden Prompt. done meldet, ob der Prompt schon wieder da ist.
func Output(raw, cmd, prompt string) (lines []string, done bool) {
	ls := Clean(raw)
	// Echo: erste Zeile, die den Befehl enthält (oft "switch# show vlans").
	for i, l := range ls {
		if i > 2 {
			break
		}
		if strings.Contains(l, cmd) {
			ls = ls[i+1:]
			break
		}
	}
	if n := len(ls); n > 0 && strings.TrimSpace(ls[n-1]) == prompt {
		return ls[:n-1], true
	}
	return ls, false
}

// volatile: Zeilen, die sich bei jedem Abruf ändern, ohne dass jemand die
// Konfiguration angefasst hat. Für Vergleich und Diff ignoriert.
var volatile = regexp.MustCompile(`(?i)^\s*(?:!\s*last configuration change|!\s*nvram config last updated|!\s*time:|current configuration\s*:|building configuration|ntp clock-period|running configuration:\s*$)`)

// ConfigLines: Konfiguration ohne flüchtige Zeilen.
func ConfigLines(cfg string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(cfg, "\r", ""), "\n") {
		if strings.TrimSpace(l) == "" || volatile.MatchString(l) {
			continue
		}
		out = append(out, strings.TrimRight(l, " \t"))
	}
	return out
}

// SameConfig: inhaltlich gleich (flüchtige Zeilen ausgenommen)?
func SameConfig(a, b string) bool {
	x, y := ConfigLines(a), ConfigLines(b)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
