package ui

// Vorgang über mehrere Fenster (Alt+V)
//
// Manche Arbeit betrifft mehrere Systeme zugleich: VLAN am Switch, IP am
// Server, Freigabe in der Datenbank. Ein Vorgang verbindet die Fenster
// (Tabs/Bereiche), die dazugehören. Dann sieht die KI sie zusammen und sagt
// zu jedem Schritt, in welches Fenster welcher Befehl gehört.
//
//   1. Fensterkennung: Jeder Tab hat eine feste Nummer (winNo), solange er
//      offen ist. Nummer, Name und Farbe stehen in Tab-Leiste, Bereichstitel
//      und in jeder Antwort der KI.
//   2. Vorgang verbinden: welche Fenster dazugehören und was die KI je
//      Fenster sieht (Profil · + Befehle · + Ausgabe, gefiltert).
//   3. KI fragen über alle Fenster: Befund je Fenster, Schritte mit
//      Zielfenster. Einfügen springt hin und tippt nur vor.
//   4. Fahrplan: Schritt für Schritt, Werte aus einer Ausgabe ({{name}})
//      gehen in spätere Schritte anderer Fenster.
//   5. Plan in Bahnen: eine Spalte je Fenster; der Agent führt nur Lesendes
//      allein aus, Änderungen tippt er vor.
//   6. Begleiter über alle Fenster: passt ein Fehler zu einem anderen
//      Fenster, sagt er, wo was einzugeben ist.
//   7. Zielfenster-Schutz (vorgang_guard.go): vor jedem Vortippen.
//   8. Rückblick über den Vorgang: Zeitleiste, Rückweg, Offenes.
//
// Ausgeführt wird nie ohne ⏎ im Zielfenster (außer Lesendes im Agent).
// Dort gelten alle bisherigen Schutzregeln (Gefahren-Check bei produktiven
// Verbindungen) unverändert.

import (
	"context"
	"image/color"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/llm"
)

type vgMode int

const (
	vgSetup vgMode = iota // Fenster wählen
	vgAsk                 // fragen, Antwort mit Schritten
	vgRun                 // Fahrplan läuft
	vgLanes               // Plan in Bahnen (eine Spalte je Fenster)
	vgRecap               // Rückblick
)

// Was die KI von einem Fenster sieht.
const (
	seeProfile  = iota // nur Systemprofil
	seeCommands        // + letzte eigene Befehle
	seeOutput          // + Terminal-Ausgabe (gefiltert)
)

func seeLabel(n int) string {
	switch n {
	case seeProfile:
		return i18n.T("nur Profil")
	case seeCommands:
		return i18n.T("Profil + Befehle")
	}
	return i18n.T("Profil + Ausgabe")
}

type vgMember struct {
	sid     int
	see     int
	hostKey string    // Fingerprint des Host-Keys beim Aufnehmen
	prompt  string    // Rechnername aus dem Prompt beim Aufnehmen ("" = noch keiner gesehen)
	connAt  time.Time // Verbindungsaufbau beim Aufnehmen
}

type vgStepState int

const (
	vsPending vgStepState = iota
	vsArmed               // Gefahr: wartet auf ⏎ im Fahrplan, dann wird vorgetippt
	vsTyped               // im Zielfenster vorgetippt, wartet auf ⏎ dort
	vsRunning             // läuft (Agent: lesend, allein)
	vsDone
	vsFailed
	vsSkipped
)

type vgStep struct {
	agent.MultiStep
	off   bool // im Plan abgewählt (Bahnen: Leertaste)
	state vgStepState
	cmd   string // tatsächlich vorgetippt/ausgeführt
	note  string
	at    time.Time
}

type vgSetupRow struct {
	sid int
	on  bool
	see int
}

type vgHint struct {
	from, win int
	text, cmd string
	danger    bool
	at        time.Time
}

type vorgang struct {
	name        string
	members     []vgMember
	started     time.Time
	active      bool // Vorgang steht (sonst: wird eingerichtet)
	open, focus bool // Leiste unten sichtbar / hat die Tasten
	mode        vgMode

	// Einrichten
	rows   []vgSetupRow
	sel    int
	inList bool
	goalIn textinput.Model

	// Fragen
	in         aiInput
	question   string
	busy       bool
	seq        int
	cancelFn   context.CancelFunc
	err        string
	ans        *agent.MultiAnswer
	history    []agent.Turn
	stepSel    int
	followNext bool // nächste Frage ist eine Folgefrage (f)
	armedIns   int  // Einfügen einer Gefahr: wartet auf den zweiten Druck (-1 = keiner)

	// Fahrplan / Bahnen
	steps      []vgStep
	cur        int
	vals       map[string]string
	auto       bool   // Agent: Lesendes läuft allein
	why        string // angehalten, warum
	advanceDue bool   // nächster Schritt beim nächsten Takt

	// Zeitleiste aller Fenster, Begleiter-Hinweis, Rückblick
	events    []agent.MultiEvent
	hint      *vgHint
	crossDue  *vgCross
	crossBusy bool
	crossAt   time.Time
	recap     *agent.MultiRecap
	recapBusy bool
	recapErr  string
}

func (g *vorgang) cancel() {
	if g.cancelFn != nil {
		g.cancelFn()
		g.cancelFn = nil
	}
}

// ---------------------------------------------------------------- 1. Fensterkennung

// winNo: feste Nummer des Tabs (kleinste freie, bleibt bis zum Schließen).
func (a *App) winNo(s *session) int {
	if s == nil {
		return 0
	}
	if s.winNo == 0 {
		// alle noch offenen ohne Nummer in Tab-Reihenfolge nummerieren
		used := map[int]bool{}
		for _, x := range a.sessions {
			used[x.winNo] = true
		}
		next := func() int {
			n := 1
			for used[n] {
				n++
			}
			used[n] = true
			return n
		}
		for _, x := range a.sessions {
			if x.winNo == 0 {
				x.winNo = next()
			}
		}
		if s.winNo == 0 { // (noch) nicht in der Liste
			s.winNo = next()
		}
	}
	return s.winNo
}

// byWin: Sitzung mit dieser Fensternummer.
func (a *App) byWin(n int) *session {
	for _, s := range a.sessions {
		if n > 0 && a.winNo(s) == n {
			return s
		}
	}
	return nil
}

// winAuto: Farben für Fenster ohne eigene Akzentfarbe. Rot fehlt mit
// Absicht: Rot heißt immer produktiv.
var winAuto = []string{"blue", "green", "yellow", "purple", "cyan", "orange"}

// winColor: Farbe des Fensters (Akzent der Verbindung, sonst je Nummer).
func (a *App) winColor(s *session) color.Color {
	if c := a.accentOf(s.conn); c != nil {
		return c
	}
	return accentColor(a.s.T, winAuto[(a.winNo(s)-1+len(winAuto))%len(winAuto)])
}

// winBadge: „ 2 “ farbig hinterlegt, mit Namen dahinter (name=true).
func (a *App) winBadge(s *session, name bool) string {
	if s == nil {
		return ""
	}
	col := a.winColor(s)
	out := lipgloss.NewStyle().Background(col).Foreground(a.s.T.Bg).Bold(true).Render(" " + itoa(a.winNo(s)) + " ")
	if name {
		out += lipgloss.NewStyle().Foreground(col).Bold(true).Render(" " + trunc(s.tabName(), 18))
		if domain.LooksProduction(s.conn) {
			out += a.s.Danger.Render(" ▲")
		}
	}
	return out
}

// winBadgeN: Abzeichen für eine Nummer (Fenster evtl. schon zu).
func (a *App) winBadgeN(n int, name bool) string {
	if s := a.byWin(n); s != nil {
		return a.winBadge(s, name)
	}
	return a.s.Muted.Render(" " + itoa(n) + " " + i18n.T("(zu)"))
}

// ---------------------------------------------------------------- 2. Vorgang verbinden

// toggleVorgang: Alt+V. Ohne Vorgang: einrichten. Sonst Leiste zeigen,
// fokussieren bzw. wieder zum Terminal.
func (a *App) toggleVorgang() tea.Cmd {
	g := a.vg
	switch {
	case g == nil:
		return a.vgOpenSetup()
	case !g.open:
		g.open, g.focus = true, true
	case g.focus:
		g.focus = false
	default:
		g.focus = true
	}
	a.resizeAll()
	return a.vgFocusInput()
}

func (a *App) vgFocusInput() tea.Cmd {
	g := a.vg
	if g == nil || !g.focus {
		return nil
	}
	if g.mode == vgSetup && !g.inList {
		return g.goalIn.Focus()
	}
	if g.mode == vgAsk && g.ans == nil && !g.busy {
		return g.in.Focus()
	}
	return nil
}

// vgOpenSetup: Fenster für den Vorgang wählen. Vorausgewählt sind die
// sichtbaren Bereiche der geteilten Ansicht, sonst der aktuelle Tab.
func (a *App) vgOpenSetup() tea.Cmd {
	if len(a.sessions) == 0 {
		a.flash(i18n.T("Erst Verbindungen öffnen, dann den Vorgang verbinden"))
		return nil
	}
	old := a.vg
	g := &vorgang{open: true, focus: true, mode: vgSetup, armedIns: -1}
	g.in = newAIInput(i18n.T("Frage an alle Fenster, z. B. Warum bekommt web02 keine IP?"))
	if old != nil {
		*g = *old
		g.open, g.focus, g.mode = true, true, vgSetup
	}
	g.goalIn = newInput(g.name, false)
	g.goalIn.Placeholder = i18n.T("z. B. web02 in Betrieb nehmen: Switchport, IP, DB-Zugang")
	g.goalIn.SetWidth(60)
	visible := map[int]bool{}
	if a.split != splitOff {
		for _, id := range a.panes {
			visible[id] = true
		}
	} else if s := a.current(); s != nil {
		visible[s.id] = true
	}
	g.rows = nil
	for _, s := range a.sessions {
		r := vgSetupRow{sid: s.id, on: visible[s.id], see: seeOutput}
		if old != nil {
			r.on = false
			for _, m := range old.members {
				if m.sid == s.id {
					r.on, r.see = true, m.see
				}
			}
		}
		if !a.st.LLM().SendScreen && r.see == seeOutput {
			r.see = seeCommands // Terminal-Inhalt an die KI ist ausgeschaltet
		}
		g.rows = append(g.rows, r)
	}
	sort.SliceStable(g.rows, func(i, j int) bool {
		return a.winNo(a.byID(g.rows[i].sid)) < a.winNo(a.byID(g.rows[j].sid))
	})
	g.sel, g.inList = 0, false
	a.vg = g
	a.resizeAll()
	return g.goalIn.Focus()
}

// vgStart: Auswahl übernehmen.
func (a *App) vgStart() tea.Cmd {
	g := a.vg
	var members []vgMember
	for _, r := range g.rows {
		s := a.byID(r.sid)
		if !r.on || s == nil {
			continue
		}
		m := vgMember{sid: s.id, see: r.see}
		for _, old := range g.members { // schon dabei: Grundlage für den Schutz behalten
			if old.sid == s.id {
				m.hostKey, m.prompt, m.connAt = old.hostKey, old.prompt, old.connAt
			}
		}
		if m.connAt.IsZero() {
			a.vgBaseline(s, &m)
		}
		members = append(members, m)
	}
	if len(members) == 0 {
		a.flash(i18n.T("Mindestens ein Fenster auswählen (Leertaste)"))
		return nil
	}
	if len(members) == 1 {
		a.flash(i18n.T("Ein Fenster: geht, aber der Vorgang lohnt sich ab zwei"))
	}
	g.members = members
	g.name = strings.TrimSpace(g.goalIn.Value())
	if !g.active {
		g.started = time.Now()
		g.vals = map[string]string{}
	}
	g.active = true
	g.mode = vgAsk
	g.goalIn.Blur()
	a.resizeAll()
	return g.in.Focus()
}

// vgBaseline: Host-Key, Verbindungszeit und Rechnername im Prompt merken.
func (a *App) vgBaseline(s *session, m *vgMember) {
	m.connAt = s.connectedAt
	if s.client != nil {
		m.hostKey = s.client.HostKey
	}
	if s.term != nil && s.term.AtIdlePrompt() {
		m.prompt = promptHost(s.term.CursorLine())
	}
}

// vgEnd: Vorgang beenden (Leiste zu, Fahrplan weg).
func (a *App) vgEnd() {
	if a.vg != nil {
		a.vg.cancel()
	}
	a.vg = nil
	a.resizeAll()
	a.flash(i18n.T("Vorgang beendet"))
}

// member: Eintrag der Sitzung im Vorgang (nil = nicht dabei).
func (g *vorgang) member(sid int) *vgMember {
	if g == nil || !g.active {
		return nil
	}
	for i := range g.members {
		if g.members[i].sid == sid {
			return &g.members[i]
		}
	}
	return nil
}

// vgForget: Tab geschlossen.
func (a *App) vgForget(s *session) {
	g := a.vg
	if g == nil {
		return
	}
	for i, m := range g.members {
		if m.sid == s.id {
			g.members = append(g.members[:i], g.members[i+1:]...)
			break
		}
	}
	for i, r := range g.rows {
		if r.sid == s.id {
			g.rows = append(g.rows[:i], g.rows[i+1:]...)
			break
		}
	}
}

// vgJoin: Sitzung zum laufenden Vorgang nehmen (z. B. fehlendes System geöffnet).
func (a *App) vgJoin(s *session) {
	g := a.vg
	if g == nil || g.member(s.id) != nil {
		return
	}
	see := seeOutput
	if !a.st.LLM().SendScreen {
		see = seeCommands
	}
	m := vgMember{sid: s.id, see: see}
	a.vgBaseline(s, &m)
	g.members = append(g.members, m)
}

// vgSessions: Sitzungen im Vorgang, nach Fensternummer.
func (a *App) vgSessions() []*session {
	g := a.vg
	if g == nil {
		return nil
	}
	var out []*session
	for _, m := range g.members {
		if s := a.byID(m.sid); s != nil {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return a.winNo(out[i]) < a.winNo(out[j]) })
	return out
}

// vgWindows: Material für die KI. Fenster mit KI aus (Tag ki-aus) fehlen.
func (a *App) vgWindows() ([]agent.Window, []domain.Connection) {
	var wins []agent.Window
	var conns []domain.Connection
	for _, s := range a.vgSessions() {
		m := a.vg.member(s.id)
		if tagPrivacy(s.conn) == llm.PrivacyOff {
			continue
		}
		c := a.agentContext(s, m.see >= seeOutput)
		c.Net = c.Net || s.netDevice()
		if m.see < seeCommands {
			c.Commands = nil
		}
		wins = append(wins, agent.Window{N: a.winNo(s), Name: s.tabName(), Prod: isProduction(s.conn), Ctx: c})
		conns = append(conns, s.conn)
	}
	return wins, conns
}

// vgTick (jede Sekunde): Prompt nachträglich merken, fällige Schritte.
func (a *App) vgTick() tea.Cmd {
	g := a.vg
	if g == nil || !g.active {
		return nil
	}
	for i := range g.members {
		m := &g.members[i]
		s := a.byID(m.sid)
		if s == nil || s.term == nil || s.state != stConnected {
			continue
		}
		if m.connAt.IsZero() {
			m.connAt = s.connectedAt
		}
		if m.hostKey == "" && s.client != nil {
			m.hostKey = s.client.HostKey
		}
		if m.prompt == "" && s.term.AtIdlePrompt() {
			m.prompt = promptHost(s.term.CursorLine())
		}
	}
	var cmds []tea.Cmd
	if g.advanceDue {
		g.advanceDue = false
		cmds = append(cmds, a.vgAdvance())
	}
	if c := a.vgCrossTick(); c != nil {
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
