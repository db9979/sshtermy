package ui

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/highlight"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/termemu"
)

// Farbregeln für die Ausgabe
//
// Text ohne eigene Farbe wird nach Regeln eingefärbt (Fehler rot, Warnungen
// gelb, IP blau, MAC lila, Syslog-Meldungen von Switches nach Schwere). Eigene
// Regeln „[!]farbe muster“ stehen vor den eingebauten. Regeln mit „!“ melden
// sich, wenn ihr Muster in einem verdeckten Tab auftaucht (wie ein langer
// Befehl, der fertig ist: Tab-Markierung, Einblendung, Desktop-Meldung).

const maxHLRules = 12

// hlState: übersetzte Regeln, neu gebaut, wenn sich die Einstellungen ändern.
type hlState struct {
	key string
	set *highlight.Set
	err error
}

func hlKey(c domain.HighlightConfig) string {
	return fmt.Sprintf("%v|%v|%s", c.Off, c.NoDefaults, strings.Join(c.Rules, "\n"))
}

// hlSet: die aktiven Regeln (nil = Farbregeln aus).
func (a *App) hlSet() *highlight.Set {
	c := a.st.Settings().Highlight
	if k := hlKey(c); k != a.hl.key {
		a.hl.key = k
		a.hl.set, a.hl.err = nil, nil
		if !c.Off {
			a.hl.set, a.hl.err = highlight.New(c.Rules, !c.NoDefaults)
		}
	}
	return a.hl.set
}

// termHL: Farbregeln mit den Farben des Themes für das Zeichnen.
func (a *App) termHL() *termemu.HL {
	set := a.hlSet()
	if set == nil {
		return nil
	}
	t := a.s.T
	h := &termemu.HL{Set: set}
	h.Colors[highlight.Red] = t.Danger
	h.Colors[highlight.Yellow] = t.Warn
	h.Colors[highlight.Green] = t.OK
	h.Colors[highlight.Blue] = t.Accent
	h.Colors[highlight.Purple] = t.AI
	h.Colors[highlight.Grey] = t.Muted
	return h
}

func (a *App) highlightOn() bool { return !a.st.Settings().Highlight.Off }

func (a *App) toggleHighlight() tea.Cmd {
	on := !a.highlightOn()
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Highlight.Off = !on })
	return nil
}

// ---------------------------------------------------------------- Bearbeiten

func (a *App) openHighlightForm() tea.Cmd {
	c := a.st.Settings().Highlight
	f := &form{title: i18n.T("Farbregeln für die Ausgabe")}
	f.toggle("defaults", i18n.T("Eingebaute Regeln"), !c.NoDefaults,
		i18n.T("Fehler rot, Warnungen gelb, ok/up grün, IP blau, MAC lila, %SYSLOG-Meldungen nach Schwere"))
	for i := 0; i < maxHLRules; i++ {
		v := ""
		if i < len(c.Rules) {
			v = c.Rules[i]
			if r, ok, err := highlight.Parse(v); ok && err == nil {
				v = r.String() // Farbname in der Sprache der Oberfläche
			}
		}
		h := i18n.T("„farbe muster“ · Farben: rot gelb grün blau lila grau · ! davor = melden, wenn es in einem verdeckten Tab auftaucht")
		if i == 0 {
			h = i18n.T("z. B. „!rot %LINK-3-UPDOWN“ oder „gelb \\bvlan ?\\d+“ · Muster = regulärer Ausdruck, Groß/Klein egal ohne Großbuchstaben")
		}
		f.text(fmt.Sprintf("r%d", i), fmt.Sprintf(i18n.T("Regel %d"), i+1), v, h)
	}
	a.returnToTerminal = a.scr == scrTerminal
	a.form, a.scr = f, scrHighlightForm
	return f.setFocus(1)
}

func (a *App) saveHighlight() error {
	f := a.form
	var rules []string
	for i := 0; i < maxHLRules; i++ {
		v := f.str(fmt.Sprintf("r%d", i))
		if v == "" {
			continue
		}
		r, ok, err := highlight.Parse(v)
		if err != nil {
			return fmt.Errorf(i18n.T("Regel %d: %v"), i+1, err)
		}
		if ok {
			rules = append(rules, r.String())
		}
	}
	return a.st.UpdateSettings(func(s *domain.Settings) {
		s.Highlight.Rules = rules
		s.Highlight.NoDefaults = !f.flag("defaults")
		s.Highlight.Off = false
	})
}

// ---------------------------------------------------------------- Melden

// alertScan liest im Ausgabestrom mit und merkt sich Zeilen, auf die eine
// Regel mit „!“ passt. Läuft im Lese-Goroutine der Sitzung.
type alertScan struct {
	set  *highlight.Set
	mu   sync.Mutex
	part []byte
	hits []alertHit
}

type alertHit struct {
	line string
	rule highlight.Rule
}

const alertMaxLine = 2048

func (sc *alertScan) feed(p []byte) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.part = append(sc.part, p...)
	for {
		i := bytes.IndexByte(sc.part, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimSpace(ansi.Strip(string(sc.part[:i])))
		sc.part = sc.part[i+1:]
		if line == "" {
			continue
		}
		if r, ok := sc.set.Alert(line); ok && len(sc.hits) < 20 {
			sc.hits = append(sc.hits, alertHit{line, r})
		}
	}
	if len(sc.part) > alertMaxLine {
		sc.part = sc.part[len(sc.part)-alertMaxLine:]
	}
}

func (sc *alertScan) take() []alertHit {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	h := sc.hits
	sc.hits = nil
	return h
}

// alertTick: Scanner an die Regeln anpassen und Treffer melden.
func (a *App) alertTick(s *session) {
	set := a.hlSet()
	cur := s.alerts.Load()
	if set == nil || !set.HasNotify() {
		if cur != nil {
			s.alerts.Store(nil)
		}
		return
	}
	if cur == nil || cur.set != set {
		s.alerts.Store(&alertScan{set: set})
		return
	}
	hits := cur.take()
	if len(hits) == 0 || !a.notifyOn() {
		return
	}
	if a.sessionVisible(s) && !a.blurred {
		return // steht gerade vor einem
	}
	if !a.sessionVisible(s) {
		s.notice = noticeAlert
	}
	if time.Since(s.alertAt) < 20*time.Second {
		return // ein Gerät, das dauernd meldet, nicht jedes Mal
	}
	s.alertAt = time.Now()
	h := hits[0]
	msg := "! " + s.tabName() + ": " + trunc(h.line, 70)
	if len(hits) > 1 {
		msg += fmt.Sprintf(i18n.T(" (+%d weitere)"), len(hits)-1)
	}
	a.pushToast(toastWarn, msg)
	if (a.blurred || a.lock != nil) && a.desktopNotifyOn() {
		body := trunc(h.line, 100)
		if a.lock != nil {
			body = i18n.T("Farbregel hat angeschlagen") // gesperrt: keinen Inhalt verraten
		}
		a.sendDesktop("sshTermy · "+s.tabName(), body)
	}
}

func hlHint(on bool) string {
	if on {
		return i18n.T("an · bearbeiten")
	}
	return i18n.T("aus · bearbeiten")
}
