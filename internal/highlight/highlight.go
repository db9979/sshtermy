// Package highlight färbt Ausgabe im Terminal nach Regeln ein: Fehler rot,
// Warnungen gelb, IP- und MAC-Adressen blau/lila, Syslog-Meldungen von
// Switches nach Schwere. Eingefärbt wird nur Text, dem das Programm auf dem
// Server selbst keine Farbe gegeben hat (ls --color, grep --color bleiben,
// wie sie sind), und nie im Vollbild (vim, htop).
//
// Eine Regel ist eine Zeile „[!]farbe muster“. Das Muster ist ein regulärer
// Ausdruck (Go-Syntax, Groß/Klein egal, außer es enthält Großbuchstaben).
// „!“ davor heißt: Taucht das Muster in einem verdeckten Tab auf, meldet sich
// sshTermy (Tab-Markierung, Einblendung, auf Wunsch Desktop-Meldung).
package highlight

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Color: Bedeutung statt RGB, damit die Regeln in jedem Theme passen.
type Color uint8

const (
	None Color = iota
	Red
	Yellow
	Green
	Blue
	Purple
	Grey
)

// colorNames: deutsche und englische Namen (erster deutscher Name wird
// beim Speichern geschrieben).
var colorNames = map[string]Color{
	"rot": Red, "red": Red,
	"gelb": Yellow, "yellow": Yellow, "orange": Yellow,
	"grün": Green, "gruen": Green, "green": Green,
	"blau": Blue, "blue": Blue,
	"lila": Purple, "violett": Purple, "purple": Purple, "magenta": Purple,
	"grau": Grey, "grey": Grey, "gray": Grey,
}

// Name: Farbname zum Speichern (deutsch, wie in der Hilfe).
func (c Color) Name() string {
	return [...]string{"", "rot", "gelb", "grün", "blau", "lila", "grau"}[c]
}

// Rule: eine Farbregel.
type Rule struct {
	Color  Color
	Notify bool
	Expr   string
	rx     *regexp.Regexp
}

// String: die Regel als Zeile („!rot %LINK-3-UPDOWN“).
func (r Rule) String() string {
	p := ""
	if r.Notify {
		p = "!"
	}
	return p + r.Color.Name() + " " + r.Expr
}

// Parse liest eine Regelzeile. Leere Zeilen und Kommentare (#) ergeben ok = false
// ohne Fehler.
func Parse(line string) (r Rule, ok bool, err error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return Rule{}, false, nil
	}
	if strings.HasPrefix(line, "!") {
		r.Notify = true
		line = strings.TrimSpace(line[1:])
	}
	name, expr, found := strings.Cut(line, " ")
	expr = strings.TrimSpace(expr)
	c, known := colorNames[strings.ToLower(name)]
	if !found || !known || expr == "" {
		return Rule{}, false, errors.New(i18n.T("Regel: „farbe muster“, Farben: rot, gelb, grün, blau, lila, grau"))
	}
	r.Color, r.Expr = c, expr
	if r.rx, err = compile(expr); err != nil {
		return Rule{}, false, err
	}
	return r, true, nil
}

// compile: smartcase wie bei der Suche: ohne Großbuchstaben egal.
func compile(expr string) (*regexp.Regexp, error) {
	src := expr
	if strings.IndexFunc(expr, unicode.IsUpper) < 0 && !strings.HasPrefix(expr, "(?") {
		src = "(?i)" + expr
	}
	rx, err := regexp.Compile(src)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("Muster „%s“: %v"), expr, err)
	}
	if rx.MatchString("") {
		return nil, fmt.Errorf(i18n.T("Muster „%s“ passt auf leeren Text"), expr)
	}
	return rx, nil
}

// Defaults: eingebaute Regeln. Spätere Regeln überschreiben frühere nicht;
// wer zuerst färbt, gewinnt (eigene Regeln stehen deshalb vorn).
var Defaults = []string{
	// Syslog von Cisco/Aruba: %FACILITY-SCHWERE-NAME, 0–3 rot, 4 gelb
	`rot %[A-Z0-9_]+-[0-3]-[A-Z0-9_]+`,
	`gelb %[A-Z0-9_]+-4-[A-Z0-9_]+`,
	`rot \b(error|errors|fatal|failed|failure|critical|panic|denied|refused|unreachable|fehler|fehlgeschlagen|verweigert|err-disabled)\b`,
	`rot \b(down|notconnect|blocking)\b`,
	`rot \b(inactive \(dead\)|failed \(Result: [a-z-]+\))`,
	`gelb \b(warn|warning|warnings|warnung|deprecated|veraltet|timeout|timed out|retrying)\b`,
	`grün \b(ok|success|successful|succeeded|erfolgreich|connected|forwarding|up)\b`,
	`grün \bactive \(running\)`,
	`blau \b(?:(?:25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|1?\d?\d)(?:/\d{1,2})?\b`,
	`lila \b[0-9a-f]{2}(?:[:-][0-9a-f]{2}){5}\b`,
	`lila \b[0-9a-f]{4}\.[0-9a-f]{4}\.[0-9a-f]{4}\b`,
	`lila \b[0-9a-f]{6}-[0-9a-f]{6}\b`,
}

// Set: übersetzte Regeln mit Zwischenspeicher pro Zeilentext.
type Set struct {
	rules []Rule
	mu    sync.Mutex
	cache map[string][]Span
}

// Span: Bereich [Start, End) in Bytes des Zeilentexts.
type Span struct {
	Start, End int
	Color      Color
}

// New baut ein Set aus Regelzeilen (eigene zuerst). Fehlerhafte Zeilen
// werden übersprungen; der erste Fehler kommt mit zurück.
func New(own []string, defaults bool) (*Set, error) {
	var first error
	s := &Set{cache: map[string][]Span{}}
	lines := append([]string{}, own...)
	if defaults {
		lines = append(lines, Defaults...)
	}
	for _, l := range lines {
		r, ok, err := Parse(l)
		if err != nil && first == nil {
			first = err
		}
		if ok {
			s.rules = append(s.rules, r)
		}
	}
	return s, first
}

// Rules: die übersetzten Regeln.
func (s *Set) Rules() []Rule { return s.rules }

// HasNotify: gibt es Regeln mit „!“?
func (s *Set) HasNotify() bool {
	for _, r := range s.rules {
		if r.Notify {
			return true
		}
	}
	return false
}

const cacheMax = 2048

// Spans: farbige Bereiche einer Zeile, nicht überlappend, sortiert.
func (s *Set) Spans(line string) []Span {
	if s == nil || len(s.rules) == 0 || strings.TrimSpace(line) == "" {
		return nil
	}
	s.mu.Lock()
	if sp, ok := s.cache[line]; ok {
		s.mu.Unlock()
		return sp
	}
	s.mu.Unlock()
	var out []Span
	taken := func(a, b int) bool {
		for _, x := range out {
			if a < x.End && b > x.Start {
				return true
			}
		}
		return false
	}
	for _, r := range s.rules {
		for _, m := range r.rx.FindAllStringIndex(line, 64) {
			if m[1] > m[0] && !taken(m[0], m[1]) {
				out = append(out, Span{m[0], m[1], r.Color})
			}
		}
	}
	s.mu.Lock()
	if len(s.cache) >= cacheMax {
		s.cache = map[string][]Span{}
	}
	s.cache[line] = out
	s.mu.Unlock()
	return out
}

// Alert: erste Regel mit „!“, die auf die Zeile passt.
func (s *Set) Alert(line string) (Rule, bool) {
	if s == nil {
		return Rule{}, false
	}
	for _, r := range s.rules {
		if r.Notify && r.rx.MatchString(line) {
			return r, true
		}
	}
	return Rule{}, false
}
