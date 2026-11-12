package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
)

// Gefahren-Check beim Tippen
//
// Auf jeder Verbindung (alle gelten als produktiv, siehe isProduction; nur der
// Schalter „Schutz“ im Formular nimmt einzelne aus) prüft sshTermy jede Zeile, bevor Enter an den Server geht. Trifft
// eine der festen Gefahrenregeln (internal/safety, auch für Switches: reload,
// erase, write erase, delete …), kommt erst ein roter Dialog. Ausführen nur mit
// „j“ oder ⏎ gedrückt halten (1,2 s, ein Balken füllt sich); ein kurzes ⏎
// reicht nicht (Gewohnheit). Esc lässt die Zeile stehen, damit man sie
// korrigieren kann.
//
// Einen globalen Schalter gibt es nicht mehr (Settings.DangerAll bleibt nur
// für ältere Sync-Stände im Format): abschalten nur je Verbindung.

type dangerAsk struct {
	sid     int
	cmd     string // getippte Zeile, wie sie am Prompt steht
	reasons []string
	key     tea.KeyPressMsg // das zurückgehaltene Enter
	prod    bool
	root    bool
	hold    holdState
	opened  time.Time
}

// ---------------------------------------------------------------- ⏎ halten
//
// Terminals melden meist kein Loslassen, nur die Wiederholung einer
// gehaltenen Taste (nach ~0,3–0,7 s, dann etwa 30-mal pro Sekunde). Gehalten
// ist ⏎, solange Wiederholungen kommen; das Loslassen erkennt sshTermy an der
// Pause (oder am Loslass-Ereignis, wo das Terminal es schickt). Wer schnell
// mehrmals tippt, startet jedes Mal von vorn. Eine Taste, die schon beim
// Öffnen des Dialogs gehalten war, zählt nicht.

const (
	holdFill     = 1200 * time.Millisecond // so lange halten
	holdFirstGap = 750 * time.Millisecond  // Verzögerung bis zur ersten Wiederholung
	holdGap      = 250 * time.Millisecond  // Abstand der Wiederholungen
	holdFresh    = 150 * time.Millisecond  // so lange Ruhe vor einem neuen Druck
)

type holdState struct {
	start, last time.Time // Beginn, letztes ⏎
	held        bool      // ⏎ ist unten
	rep         bool      // Wiederholungen kommen
	relLevel    float64   // Stand beim Loslassen (läuft dann zurück)
	relAt       time.Time
}

// press: ⏎ (auch Wiederholung). Ergebnis: voll = ausführen.
func (h *holdState) press(now time.Time) bool {
	gap := now.Sub(h.last)
	h.last = now
	if h.held && !h.within(gap) {
		h.letGo(now.Add(-gap)) // Pause zu lang: war schon losgelassen
	}
	if !h.held {
		if gap < holdFresh {
			return false // Taste war schon beim Öffnen des Dialogs unten
		}
		h.held, h.rep, h.start, h.relLevel = true, false, now, 0
		return false
	}
	h.rep = true
	return h.level(now) >= 1
}

// within: kam das ⏎ noch als Wiederholung der gehaltenen Taste?
func (h *holdState) within(gap time.Duration) bool {
	if h.rep {
		return gap <= holdGap
	}
	return gap <= holdFirstGap
}

func (h *holdState) letGo(at time.Time) {
	if !h.held {
		return
	}
	h.relLevel = math.Min(1, float64(at.Sub(h.start))/float64(holdFill))
	h.relAt = at
	h.held, h.rep = false, false
}

// release: Loslass-Ereignis vom Terminal.
func (h *holdState) release(now time.Time) { h.letGo(now) }

// update: Loslassen an der Pause erkennen.
func (h *holdState) update(now time.Time) {
	if h.held && !h.within(now.Sub(h.last)) {
		h.letGo(h.last)
	}
}

// level: Füllstand 0 … 1.
func (h *holdState) level(now time.Time) float64 {
	if h.held {
		return math.Min(1, float64(now.Sub(h.start))/float64(holdFill))
	}
	return math.Max(0, h.relLevel-2*now.Sub(h.relAt).Seconds())
}

func (h *holdState) active(now time.Time) bool { return h.held || h.level(now) > 0 }

// netDevice: Switch/Router (Verbindung, Systemprofil oder Erkennung beim Aufbau).
func (s *session) netDevice() bool {
	return s.netDev() || s.prof.Family == domain.FamilyNetwork || s.conn.Device == domain.DeviceNetwork
}

// safetyKind: Regelgruppe für die Sitzung (unix, windows, network).
func (a *App) safetyKind(s *session) string {
	return safety.KindFor(a.remoteText(s), s.netDevice())
}

// dangerGuardOn: wird auf dieser Verbindung beim Tippen geprüft?
func (a *App) dangerGuardOn(c domain.Connection) bool { return isProduction(c) }

// dangerCheck: Enter am Prompt. true = zurückgehalten, Dialog ist offen.
func (a *App) dangerCheck(s *session, k tea.KeyPressMsg) bool {
	if s.term == nil || s.term.AltScreen() || !a.dangerGuardOn(s.conn) {
		return false
	}
	typed, ok := s.term.TypedInput()
	cmd := strings.TrimSpace(typed)
	if !ok || cmd == "" || agent.LooksLikeSecretPrompt(s.term.CursorLine()) {
		return false
	}
	if s.dangerOK != "" && s.dangerOK == cmd {
		s.dangerOK = ""
		return false
	}
	s.dangerOK = ""
	f := safety.Check(cmd, a.safetyKind(s))
	if len(f) == 0 {
		return false
	}
	d := &dangerAsk{sid: s.id, cmd: cmd, key: k, prod: domain.LooksProduction(s.conn), root: s.state == stConnected && s.rootNow(), opened: time.Now()}
	d.hold.last = d.opened // das ⏎, das den Dialog geöffnet hat
	for _, x := range f {
		d.reasons = append(d.reasons, x.Reason)
	}
	a.modal = &modal{kind: mDanger, title: i18n.T("▲ Riskanter Befehl"), danger: d}
	return true
}

func (a *App) dangerKey(k tea.KeyPressMsg) tea.Cmd {
	d := a.modal.danger
	yn := yesNo(k)
	switch {
	case yn == 1:
		a.heldEat.arm(k.String(), time.Now(), false) // j gehalten: kein jjj im Terminal
		return a.dangerRun(d)
	case yn == -1 || k.String() == "q":
		a.modal = nil
		a.flash(i18n.T("Nicht ausgeführt. Die Zeile steht noch am Prompt."))
	case k.String() == "enter":
		if now := time.Now(); d.hold.press(now) {
			a.heldEat.arm("enter", now, true) // Rest des Haltens nicht ins Terminal
			return a.dangerRun(d)
		}
		if !d.hold.held || !d.hold.rep {
			a.modal.body = "again" // Hinweis: halten oder j
		}
	}
	return nil
}

// heldKeyEat: die Taste, die den Dialog bestätigt hat, ist danach meist noch
// unten, und das Terminal schickt weiter Wiederholungen. Die gehören zum
// Bestätigen und dürfen nicht als weitere Enter (oder jjj) beim Server
// ankommen. Erst nach dem Loslassen (Loslass-Ereignis oder Pause wie beim
// Halten, siehe holdState) zählt die Taste wieder.
type heldKeyEat struct {
	key  string
	last time.Time
	rep  bool // Wiederholungen kommen schon (kürzere Pause reicht als Ende)
}

func (h *heldKeyEat) arm(key string, now time.Time, rep bool) {
	*h = heldKeyEat{key: key, last: now, rep: rep}
}

// eat: true = Wiederholung der Bestätigungstaste, verschlucken.
func (h *heldKeyEat) eat(k tea.KeyPressMsg, now time.Time) bool {
	if h.key == "" {
		return false
	}
	if k.String() != h.key {
		*h = heldKeyEat{} // andere Taste: Halten ist vorbei
		return false
	}
	gap := holdFirstGap
	if h.rep {
		gap = holdGap
	}
	if k.IsRepeat || now.Sub(h.last) <= gap {
		h.last, h.rep = now, true
		return true
	}
	*h = heldKeyEat{} // Pause: losgelassen, das ist ein neuer Druck
	return false
}

// release: Loslass-Ereignis vom Terminal.
func (h *heldKeyEat) release(k tea.KeyReleaseMsg) {
	if h.key != "" && k.String() == h.key {
		*h = heldKeyEat{}
	}
}

// dangerRun: bestätigt, die zurückgehaltene Zeile geht an den Server.
func (a *App) dangerRun(d *dangerAsk) tea.Cmd {
	a.modal = nil
	s := a.byID(d.sid)
	if s == nil || s.term == nil || s.state != stConnected {
		return nil
	}
	s.dangerOK = d.cmd
	return a.terminalKey(s, d.key)
}

// dangerHoldTick: Loslassen erkennen (Takt aus motion.go).
func (a *App) dangerHoldTick() tea.Cmd {
	if a.modal == nil || a.modal.kind != mDanger || a.modal.danger == nil {
		return nil
	}
	a.modal.danger.hold.update(time.Now())
	return nil
}

func (a *App) dangerView(m *modal) string {
	st := a.s
	d := m.danger
	w := dialog{maxW: 80}.innerMax(a.w)
	s := a.byID(d.sid)
	host := ""
	if s != nil {
		host = s.conn.Alias + " (" + s.conn.Username + "@" + s.conn.ShowAddr() + ")"
	}
	var b strings.Builder
	if d.prod {
		b.WriteString(st.ChipDanger.Bold(true).Render(i18n.T("PRODUKTIV")) + " ")
	}
	if d.root {
		b.WriteString(st.ChipDanger.Bold(true).Render(i18n.T("▲ root")) + " ")
	}
	b.WriteString(st.Text.Render(host) + "\n\n")
	b.WriteString(st.Muted.Render("$ ") + st.Danger.Render(trunc(d.cmd, w-2)) + "\n\n")
	for _, r := range d.reasons {
		b.WriteString(st.Danger.Render("▲ "+r) + "\n")
	}
	// Balken: füllt sich, solange ⏎ gehalten wird
	now := time.Now()
	lvl := d.hold.level(now)
	barW := max(10, min(w, 60))
	fill := st.T.Danger
	if motionLevel() > 0 {
		fill = mixColor(st.T.Warn, st.T.Danger, lvl)
	}
	b.WriteString("\n" + smoothBar(lvl, barW, fill, st.T.Surface2) + "\n")
	switch {
	case d.hold.held:
		b.WriteString(st.Warn.Bold(true).Render(i18n.T("weiter halten …")) + "\n")
	case lvl > 0:
		b.WriteString(st.Muted.Render(i18n.T("losgelassen · nichts passiert")) + "\n")
	case m.body == "again":
		b.WriteString(st.Warn.Render(fmt.Sprintf(i18n.T("Ein kurzes ⏎ reicht hier nicht: ⏎ gedrückt halten oder %s drücken."), yesKey())) + "\n")
	default:
		b.WriteString(st.Muted.Render(fmt.Sprintf(i18n.T("⏎ gedrückt halten oder %s drücken zum Ausführen"), yesKey())) + "\n")
	}
	tone := st.T.Danger
	if motionLevel() == 2 {
		tone = mixColor(mixColor(st.T.Border, st.T.Danger, 0.45), st.T.Danger, pulse(now, 1600*time.Millisecond))
	}
	keys := [][2]string{{i18n.T("⏎ halten"), i18n.T("ausführen")}, {yesKey(), i18n.T("trotzdem ausführen")}, {"Esc", i18n.T("abbrechen (Zeile bleibt stehen)")}}
	return st.dialog(dialog{icon: "▲", title: strings.TrimPrefix(m.title, "▲ "), tone: tone, body: b.String(), keys: keys, width: w, maxW: 80}, a.w)
}
