package hostinfo

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Regeln für Switch-Configs
//
// Eine Regel pro Zeile: „+muster“ muss in der Konfiguration vorkommen,
// „-muster“ darf nicht vorkommen. Das Muster ist ein regulärer Ausdruck und
// gilt zeilenweise; Groß/Klein ist egal, solange das Muster keine
// Großbuchstaben enthält. Beispiele:
//
//	+^ntp server        NTP muss eingetragen sein
//	+^logging           Syslog-Ziel muss gesetzt sein
//	-telnet-server      Telnet darf nicht an sein
//	-community "public" keine Standard-SNMP-Community

// ConfigRule: eine geprüfte Regel.
type ConfigRule struct {
	Must    bool
	Pattern string
	rx      *regexp.Regexp
}

// String: wieder als „+muster“ / „-muster“.
func (r ConfigRule) String() string {
	if r.Must {
		return "+" + r.Pattern
	}
	return "-" + r.Pattern
}

// ParseConfigRule liest eine Zeile (leer oder # = keine Regel, ok=false).
func ParseConfigRule(line string) (r ConfigRule, ok bool, err error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return r, false, nil
	}
	switch line[0] {
	case '+':
		r.Must = true
	case '-':
	default:
		return r, false, errors.New(i18n.T("mit + (muss vorkommen) oder - (darf nicht vorkommen) beginnen"))
	}
	r.Pattern = strings.TrimSpace(line[1:])
	if r.Pattern == "" {
		return r, false, errors.New(i18n.T("Muster fehlt"))
	}
	expr := r.Pattern
	if strings.ToLower(expr) == expr {
		expr = "(?i)" + expr
	}
	r.rx, err = regexp.Compile("(?m)" + expr)
	if err != nil {
		return r, false, fmt.Errorf(i18n.T("Muster ist kein gültiger regulärer Ausdruck: %v"), err)
	}
	return r, true, nil
}

// ParseConfigRules: alle Zeilen, ungültige werden übersprungen.
func ParseConfigRules(lines []string) []ConfigRule {
	var out []ConfigRule
	for _, l := range lines {
		if r, ok, err := ParseConfigRule(l); ok && err == nil {
			out = append(out, r)
		}
	}
	return out
}

// Violation: verletzte Regel, bei „-“ mit der ersten passenden Zeile.
type Violation struct {
	Rule ConfigRule
	Line string
}

// Text für Meldungen.
func (v Violation) Text() string {
	if v.Rule.Must {
		return fmt.Sprintf(i18n.T("fehlt: %s"), v.Rule.Pattern)
	}
	return fmt.Sprintf(i18n.T("verboten: %s"), strings.TrimSpace(v.Line))
}

// CheckConfig prüft eine Konfiguration gegen die Regeln.
func CheckConfig(cfg string, rules []ConfigRule) []Violation {
	cfg = strings.ReplaceAll(cfg, "\r", "")
	var out []Violation
	for _, r := range rules {
		if r.rx == nil {
			continue
		}
		loc := r.rx.FindStringIndex(cfg)
		switch {
		case r.Must && loc == nil:
			out = append(out, Violation{Rule: r})
		case !r.Must && loc != nil:
			start := strings.LastIndexByte(cfg[:loc[0]], '\n') + 1
			end := strings.IndexByte(cfg[loc[0]:], '\n')
			line := cfg[start:]
			if end >= 0 {
				line = cfg[start : loc[0]+end]
			}
			out = append(out, Violation{Rule: r, Line: line})
		}
	}
	return out
}
