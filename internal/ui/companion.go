package ui

// KI-Begleiter: liest im Terminal mit und meldet sich von selbst.
//
//   - Stufe Leise (A): eine Zeile über der Vorschlagszeile, nur bei Anlass:
//     Befehl fehlgeschlagen, Gefahrenregel trifft die getippte Zeile (vor ⏎,
//     nach einer Tipp-Pause), gleicher Befehl mehrmals kurz hintereinander.
//   - Stufe Gesprächig (B): dazu ein Kommentar zu jedem Befehl (gebündelt,
//     höchstens alle 20 s). Alles landet in der Begleiter-Leiste (Panel).
//   - Stufe Coach (C): dazu „Absicht erkannt“ (Plan oder Agent übernimmt den
//     Rest) und ein Rückblick beim Schließen des Tabs (Fakten fürs
//     Systemprofil merken, Sitzung als Runbook speichern).
//
// Ob es einen Anlass gibt, entscheiden feste Regeln hier, nie das Modell.
// Grenzen: Anfragen pro Stunde und ein geschätztes Tagesbudget in Tokens.
// Datenschutz: nichts bei Passwortabfragen, in Vollbild-Programmen (vim,
// less, top), bei privaten Befehlen (Leerzeichen am Anfang), bei laufendem
// Agent/Plan, bei Tag „begleiter-aus“; KI-Tags (ki-aus/ki-lokal) gelten.
// „Filter aus“ (Geheimnisse nicht entfernen) wirkt nur, wenn jede Anfrage an
// eine eigene KI geht (lokal oder als eigener KI-Server eingetragen).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/llm"
	"github.com/db9979/sshtermy/internal/safety"
	"github.com/db9979/sshtermy/internal/sessionlog"
)

const (
	compFeedMax    = 60
	compRunsMax    = 60
	compChattyGap  = 20 * time.Second
	compTopicGap   = 10 * time.Minute // gleicher Anlass erst danach wieder
	compTypePause  = 1500 * time.Millisecond
	compRepeatWin  = 2 * time.Minute
	compRepeatN    = 3
	compIntentCmds = 4
	compIntentGap  = 3 * time.Minute
	compRecapMin   = 5 // so viele Befehle, damit sich ein Rückblick lohnt
)

// Eintragsarten im Verlauf der Leiste (neben agent.Event…).
const (
	compKindIntent = "absicht"
	compKindNote   = "hinweis" // nur Text von sshTermy selbst (z. B. Passwort, Grenze)
)

type compEntry struct {
	at        time.Time
	kind      string
	cmd       string
	text      string
	cmds      []agent.Suggestion
	fact      string
	factSaved bool
	topic     string
	local     bool // ohne KI entstanden (feste Regel)
}

type compRun struct {
	at     time.Time
	cmd    string
	out    string
	failed bool
}

// companionState: Begleiter einer Sitzung.
type companionState struct {
	feed    []compEntry
	remark  *compEntry // steht in der Begleiter-Zeile
	runs    []compRun
	pending []string // gesprächig: Befehle seit dem letzten Kommentar
	due     *agent.CompanionEvent
	dueTop  string

	busy, intentBusy bool
	seq, intentSeq   int
	cancel           context.CancelFunc

	asked map[string]time.Time
	muted map[string]bool

	lastChatty time.Time

	typeSeq     int
	typedSeen   string // diese getippte Zeile ist schon geprüft
	dangerTyped string // Zeile, zu der die Gefahren-Bemerkung gehört

	intent      *agent.Intent
	intentAt    time.Time
	sinceIntent int

	sel, armed int
	limitNoted string // Grenze schon gemeldet (Stunde/Tag)
	pausedWhy  string
}

// companionUsage: Anfragen und geschätzte Tokens (alle Sitzungen).
type companionUsage struct {
	hits     []time.Time
	day      string
	tokens   int
	requests int
}

func (u *companionUsage) roll(now time.Time) {
	if d := now.Format("2006-01-02"); d != u.day {
		u.day, u.tokens, u.requests = d, 0, 0
	}
	cut := now.Add(-time.Hour)
	i := 0
	for i < len(u.hits) && u.hits[i].Before(cut) {
		i++
	}
	u.hits = u.hits[i:]
}

// ---------------------------------------------------------------- Einstellung

func (a *App) compCfg() domain.CompanionConfig {
	if a.st == nil {
		return domain.CompanionConfig{} // Tests ohne Store
	}
	return a.st.Settings().Companion
}
func (a *App) compLevel() int { return clamp(a.compCfg().Level, 0, domain.CompanionCoach) }

func companionLevelLabel(l int) string {
	switch l {
	case domain.CompanionQuiet:
		return i18n.T("Leise")
	case domain.CompanionChatty:
		return i18n.T("Gesprächig")
	case domain.CompanionCoach:
		return i18n.T("Coach")
	}
	return i18n.T("Aus")
}

func (a *App) setCompanion(f func(*domain.CompanionConfig)) {
	if err := a.st.UpdateSettings(func(s *domain.Settings) { f(&s.Companion) }); err != nil {
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
	}
}

// cycleCompanion: Stufe durchschalten (Taste und Palette).
func (a *App) cycleCompanion(dir int) tea.Cmd {
	before := a.compLevel()
	next := cycleIndex(before, domain.CompanionCoach+1, dir)
	a.setCompanion(func(c *domain.CompanionConfig) { c.Level = next })
	for _, s := range a.sessions {
		a.resize(s) // Begleiter-Zeile kommt oder geht
		if next == domain.CompanionOff {
			s.comp.stop()
			s.comp.remark = nil
		}
	}
	msg := i18n.T("Begleiter: ") + companionLevelLabel(next)
	switch next {
	case domain.CompanionQuiet:
		msg += i18n.T(" · meldet sich bei Fehler, Gefahr, Wiederholung")
	case domain.CompanionChatty:
		msg += fmt.Sprintf(i18n.T(" · kommentiert jeden Befehl · Leiste mit %s"), a.keys.label(actCompanionBar))
	case domain.CompanionCoach:
		msg += i18n.T(" · erkennt, was du vorhast, Rückblick beim Schließen")
	}
	a.flash(msg)
	return nil
}

// ---------------------------------------------------------------- Zustand

func (c *companionState) stop() {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.busy, c.due = false, nil
}

func (c *companionState) add(e compEntry) *compEntry {
	if e.at.IsZero() {
		e.at = time.Now()
	}
	c.feed = append(c.feed, e)
	if len(c.feed) > compFeedMax {
		c.feed = c.feed[len(c.feed)-compFeedMax:]
	}
	return &c.feed[len(c.feed)-1]
}

// note: Hinweis von sshTermy selbst (nicht von der KI), höchstens einmal in Folge.
func (c *companionState) note(text string) {
	if n := len(c.feed); n > 0 && c.feed[n-1].kind == compKindNote && c.feed[n-1].text == text {
		return
	}
	c.add(compEntry{kind: compKindNote, text: text, local: true})
}

// topicOf: Thema eines Anlasses (zum Stummschalten und gegen Wiederholung).
func topicOf(kind, cmd string) string {
	w := strings.Fields(cmd)
	first := ""
	for _, x := range w {
		if x == "sudo" || strings.HasPrefix(x, "-") {
			continue
		}
		first = x
		break
	}
	return kind + ":" + first
}

// companionOff: Gründe, gerade nichts zu senden ("" = darf).
func (a *App) companionOff(s *session) string {
	if a.compLevel() == domain.CompanionOff {
		return i18n.T("aus")
	}
	if hasTag(s.conn, "begleiter-aus", "companion-off") {
		return i18n.T("für diese Verbindung aus (Tag begleiter-aus)")
	}
	if a.aiLevel(s.conn) == llm.PrivacyOff {
		return i18n.T("KI für diese Verbindung aus")
	}
	if s.goal.active {
		return i18n.T("pausiert, solange Ziel/Agent/Plan läuft")
	}
	if s.term == nil || s.state != stConnected {
		return i18n.T("nicht verbunden")
	}
	if s.term.AltScreen() {
		return i18n.T("pausiert im Vollbild-Programm")
	}
	if agent.LooksLikeSecretPrompt(s.term.CursorLine()) {
		return i18n.T("Passworteingabe, nichts gesendet")
	}
	return ""
}

func hasTag(c domain.Connection, tags ...string) bool {
	for _, t := range c.Tags {
		t = strings.ToLower(strings.TrimSpace(t))
		for _, x := range tags {
			if t == x {
				return true
			}
		}
	}
	return false
}

// companionBudget: Grenze erreicht? ("" = nein)
func (a *App) companionBudget() string {
	cfg := a.compCfg()
	u := &a.compUse
	u.roll(time.Now())
	if len(u.hits) >= cfg.Hour() {
		return fmt.Sprintf(i18n.T("Grenze erreicht: %d Anfragen in der letzten Stunde"), cfg.Hour())
	}
	if u.tokens >= cfg.DayTokens() {
		return fmt.Sprintf(i18n.T("Tagesbudget erreicht: etwa %s Tokens"), kTok(cfg.DayTokens()))
	}
	return ""
}

func kTok(n int) string {
	if n >= 10000 {
		return fmt.Sprintf("%dk", n/1000)
	}
	return fmt.Sprintf("%d", n)
}

func (a *App) companionSpent(tokens int) {
	u := &a.compUse
	u.roll(time.Now())
	u.tokens += tokens
}

// companionClient: KI-Zugang des Begleiters. raw = Filter dürfen aus.
func (a *App) companionClient(s *session) (*agent.Agent, bool, error) {
	cfg := a.compCfg()
	level := a.aiLevel(s.conn)
	if cfg.OnlyLocal || cfg.Raw {
		level = llm.Stricter(level, llm.PrivacyLocal) // Filter aus nur mit eigener KI
	}
	c, err := llm.NewWithPrivacy(a.st.LLM(), level)
	if errors.Is(err, llm.ErrAIOff) {
		return nil, false, errors.New(i18n.T("KI für diese Verbindung aus"))
	}
	if err != nil {
		return nil, false, err
	}
	return &agent.Agent{C: c}, cfg.Raw && c.AllOwn(), nil
}

func (a *App) companionContext(s *session, raw bool) agent.Context {
	c := a.agentContext(s, true)
	if raw {
		c.Raw = true
		if c.Screen == "" && s.term != nil { // Filter aus: Bildschirm immer mit
			n := a.st.LLM().ContextLines
			if n <= 0 {
				n = 80
			}
			c.Screen = s.term.PlainLines(n)
		}
	}
	return c
}

// ---------------------------------------------------------------- Auslöser

// companionAfter: ein Befehl ist fertig (aus finishCommand).
func (a *App) companionAfter(s *session, r *cmdRun, f agent.Failure, failed bool) {
	level := a.compLevel()
	if level == domain.CompanionOff || r.private || r.agent {
		return
	}
	c := &s.comp
	if c.remark != nil && c.remark.kind == agent.EventDanger {
		c.remark = nil // die Zeile ist abgeschickt
	}
	now := time.Now()
	c.runs = append(c.runs, compRun{at: now, cmd: r.cmd, out: sessionlog.Clip(f.Output), failed: failed})
	if len(c.runs) > compRunsMax {
		c.runs = c.runs[len(c.runs)-compRunsMax:]
	}
	c.sinceIntent++
	if s.goal.active {
		return
	}
	ev := agent.CompanionEvent{Command: r.cmd, Output: f.Output, Exit: f.Exit, Reason: f.Reason}
	switch {
	case failed && !r.interrupted:
		ev.Kind = agent.EventFail
	case !r.interrupted && c.repeats(r.cmd, now) >= compRepeatN:
		ev.Kind, ev.Count = agent.EventRepeat, c.repeats(r.cmd, now)
	case level >= domain.CompanionChatty && !r.interrupted:
		c.pending = append(c.pending, r.cmd)
		if time.Since(c.lastChatty) < compChattyGap {
			return
		}
		ev.Kind = agent.EventCommand
		if len(c.pending) > 1 {
			ev.More = append([]string(nil), c.pending[:len(c.pending)-1]...)
			if len(ev.More) > 6 {
				ev.More = ev.More[len(ev.More)-6:]
			}
		}
	default:
		return
	}
	topic := topicOf(ev.Kind, r.cmd)
	if ev.Kind != agent.EventCommand {
		if c.muted[topic] || now.Sub(c.asked[topic]) < compTopicGap {
			return
		}
	}
	c.due, c.dueTop = &ev, topic
}

// repeats: wie oft lief cmd im Fenster der letzten 2 Minuten?
func (c *companionState) repeats(cmd string, now time.Time) int {
	n := 0
	for i := len(c.runs) - 1; i >= 0; i-- {
		if now.Sub(c.runs[i].at) > compRepeatWin {
			break
		}
		if c.runs[i].cmd == cmd {
			n++
		}
	}
	return n
}

type compRemarkMsg struct {
	sid, seq int
	ev       agent.CompanionEvent
	topic    string
	r        agent.Remark
	err      error
}

type compIntentMsg struct {
	sid, seq int
	in       agent.Intent
	err      error
}

type compTypeMsg struct{ sid, seq int }

// companionTick (jede Sekunde): fällige Anfrage abschicken, Absicht prüfen.
func (a *App) companionTick(s *session) tea.Cmd {
	c := &s.comp
	if a.compLevel() == domain.CompanionOff {
		return nil
	}
	c.pausedWhy = a.companionOff(s)
	var cmds []tea.Cmd
	if c.due != nil && !c.busy {
		if why := c.pausedWhy; why != "" {
			if c.due.Kind != agent.EventCommand {
				c.note(why)
			}
			c.due = nil
		} else {
			ev, topic := *c.due, c.dueTop
			c.due = nil
			cmds = append(cmds, a.companionAsk(s, ev, topic))
		}
	}
	if a.compLevel() >= domain.CompanionCoach && !c.intentBusy && c.pausedWhy == "" &&
		c.sinceIntent >= compIntentCmds && time.Since(c.intentAt) >= compIntentGap && c.intent == nil {
		cmds = append(cmds, a.companionIntent(s))
	}
	return tea.Batch(cmds...)
}

func (a *App) companionAsk(s *session, ev agent.CompanionEvent, topic string) tea.Cmd {
	c := &s.comp
	if why := a.companionBudget(); why != "" {
		if c.limitNoted != why {
			c.limitNoted = why
			c.note(why)
		}
		return nil
	}
	ag, raw, err := a.companionClient(s)
	if err != nil {
		c.note(err.Error())
		return nil
	}
	if c.asked == nil {
		c.asked = map[string]time.Time{}
	}
	c.asked[topic] = time.Now()
	if ev.Kind == agent.EventCommand {
		c.lastChatty, c.pending = time.Now(), nil
	}
	a.compUse.hits = append(a.compUse.hits, time.Now())
	a.compUse.requests++
	c.stop()
	c.busy = true
	c.seq++
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	c.cancel = cancel
	sid, seq := s.id, c.seq
	ctxt := a.companionContext(s, raw)
	return func() tea.Msg {
		defer cancel()
		r, err := ag.Comment(ctx, ev, ctxt)
		return compRemarkMsg{sid, seq, ev, topic, r, err}
	}
}

func (a *App) companionIntent(s *session) tea.Cmd {
	c := &s.comp
	c.intentAt, c.sinceIntent = time.Now(), 0
	if a.companionBudget() != "" {
		return nil
	}
	ag, raw, err := a.companionClient(s)
	if err != nil {
		return nil
	}
	a.compUse.hits = append(a.compUse.hits, time.Now())
	a.compUse.requests++
	c.intentBusy = true
	c.intentSeq++
	sid, seq := s.id, c.intentSeq
	ctxt := a.companionContext(s, raw)
	ctxt.Commands = lastN(s.history, 15)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		in, err := ag.GuessIntent(ctx, ctxt)
		return compIntentMsg{sid, seq, in, err}
	}
}

// companionTyped: nach jeder Taste am Prompt; prüft nach einer Pause die
// getippte Zeile gegen die Gefahrenregeln.
func (a *App) companionTyped(s *session) tea.Cmd {
	if a.compLevel() == domain.CompanionOff {
		return nil
	}
	s.comp.typeSeq++
	sid, seq := s.id, s.comp.typeSeq
	return tea.Tick(compTypePause, func(time.Time) tea.Msg { return compTypeMsg{sid, seq} })
}

func (a *App) companionTypePause(s *session) tea.Cmd {
	c := &s.comp
	if s.term == nil || a.companionOff(s) != "" {
		return nil
	}
	typed, ok := s.term.TypedInput()
	typed = strings.TrimSpace(typed)
	if !ok || typed == "" || typed == c.typedSeen {
		return nil
	}
	c.typedSeen = typed
	found := safety.Risky(typed, a.safetyKind(s))
	if len(found) == 0 {
		if c.remark != nil && c.remark.kind == agent.EventDanger {
			c.remark = nil // Zeile entschärft
		}
		return nil
	}
	why := found[0].Reason
	e := c.add(compEntry{kind: agent.EventDanger, cmd: typed, text: "! " + why, local: true, topic: topicOf(agent.EventDanger, typed)})
	c.remark, c.dangerTyped = e, typed
	topic := topicOf(agent.EventDanger, typed)
	if c.muted[topic] || c.busy {
		return nil
	}
	return a.companionAsk(s, agent.CompanionEvent{Kind: agent.EventDanger, Command: typed, Reason: why}, topic)
}

// companionMsg verteilt die Antworten.
func (a *App) companionMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case compTypeMsg:
		if s := a.byID(m.sid); s != nil && m.seq == s.comp.typeSeq {
			return a.companionTypePause(s), true
		}
		return nil, true
	case compRemarkMsg:
		s := a.byID(m.sid)
		if s == nil || m.seq != s.comp.seq {
			return nil, true
		}
		c := &s.comp
		c.busy, c.cancel = false, nil
		a.companionSpent(m.r.Tokens)
		if m.err != nil {
			c.note(i18n.T("KI-Fehler: ") + trunc(m.err.Error(), 120))
			return nil, true
		}
		if m.r.Quiet {
			return nil, true
		}
		if m.ev.Kind == agent.EventDanger {
			typed, _ := s.term.TypedInput()
			if strings.TrimSpace(typed) != m.ev.Command {
				return nil, true // Zeile ist schon abgeschickt oder geändert
			}
		}
		ne := compEntry{kind: m.ev.Kind, cmd: m.ev.Command, text: m.r.Text, cmds: m.r.Commands, fact: m.r.Fact, topic: m.topic, at: time.Now()}
		var e *compEntry
		if n := len(c.feed); n > 0 && c.feed[n-1].local && c.feed[n-1].kind == m.ev.Kind && c.feed[n-1].cmd == m.ev.Command {
			c.feed[n-1] = ne // Hinweis der festen Regel durch die Erklärung ersetzen
			e = &c.feed[n-1]
		} else {
			e = c.add(ne)
		}
		for i, x := range e.cmds { // was der Admin hier korrigiert hat, gleich richtig
			if fix, ok := s.learnedFix(x.Command); ok {
				fresh := agent.ParseStepCommand(fix, a.safetyKind(s))
				e.cmds[i].Command, e.cmds[i].Danger, e.cmds[i].Elevated = fix, fresh.Danger, fresh.Elevated
			}
		}
		c.remark = e
		if len(e.cmds) > 0 {
			s.chips, s.chipArmed = append([]agent.Suggestion(nil), e.cmds...), -1
			s.chipReplace, s.compChips = m.ev.Kind == agent.EventDanger, true
		}
		return nil, true
	case compIntentMsg:
		s := a.byID(m.sid)
		if s == nil || m.seq != s.comp.intentSeq {
			return nil, true
		}
		c := &s.comp
		c.intentBusy = false
		a.companionSpent(m.in.Tokens)
		if m.err != nil || m.in.Goal == "" || c.muted["absicht:"+strings.ToLower(m.in.Goal)] || s.goal.active {
			return nil, true
		}
		in := m.in
		c.intent = &in
		text := fmt.Sprintf(i18n.T("Sieht aus, als willst du: %s. Plan oder Agent kann den Rest übernehmen (%s)."), in.Goal, a.keys.label(actCompanionBar))
		e := c.add(compEntry{kind: compKindIntent, text: text, cmds: in.Steps, topic: "absicht:" + strings.ToLower(in.Goal)})
		c.remark = e
		s.chips, s.chipArmed, s.chipReplace, s.compChips = append([]agent.Suggestion(nil), in.Steps[:min(4, len(in.Steps))]...), -1, false, true
		return nil, true
	case compRecapMsg:
		a.recapDone(m)
		return nil, true
	}
	return nil, false
}

// ---------------------------------------------------------------- Aktionen

// companionRemember: Tatsachen in die Notizen des Systemprofils (gehen der KI
// künftig mit).
func (a *App) companionRemember(s *session, facts ...string) int {
	n := 0
	for _, f := range facts {
		f = strings.TrimSpace(f)
		if f == "" || strings.Contains(strings.ToLower(s.prof.Notes), strings.ToLower(f)) {
			continue
		}
		if s.prof.Notes != "" && !strings.HasSuffix(s.prof.Notes, "\n") {
			s.prof.Notes += "\n"
		}
		s.prof.Notes += "- " + f
		n++
	}
	if n > 0 {
		a.saveProfile(s)
		a.flashOK(fmt.Sprintf(i18n.T("Gemerkt für %s (Systemprofil, Notizen): %s"), s.conn.Alias, plural(n, i18n.T("Tatsache"), i18n.T("Tatsachen"))))
	}
	return n
}

// companionTakeIntent: Absicht übernehmen. plan = erst planen, sonst Agent
// „Vorsichtig“.
func (a *App) companionTakeIntent(s *session, plan bool) tea.Cmd {
	in := s.comp.intent
	if in == nil {
		return nil
	}
	s.comp.intent, s.comp.remark = nil, nil
	g := &s.goal
	if plan {
		g.planFirst, g.auto, g.level = true, true, lvlNormal
	} else {
		g.planFirst, g.auto, g.level = false, true, lvlCareful
	}
	return a.startGoal(s, in.Goal)
}

func (a *App) companionDropIntent(s *session, mute bool) {
	if in := s.comp.intent; in != nil && mute {
		s.comp.mute("absicht:" + strings.ToLower(in.Goal))
	}
	s.comp.intent = nil
	if s.comp.remark != nil && s.comp.remark.kind == compKindIntent {
		s.comp.remark = nil
	}
}

func (c *companionState) mute(topic string) {
	if c.muted == nil {
		c.muted = map[string]bool{}
	}
	c.muted[topic] = true
}

// ---------------------------------------------------------------- Rückblick

type compRecapMsg struct {
	sid int
	r   agent.Recap
	err error
}

type recapState struct {
	sid     int
	closing bool
	after   func() // Tab schließen
	r       *agent.Recap
	err     string
}

// companionLog: Befehle der Sitzung für den Rückblick.
func (c *companionState) log() string {
	var b strings.Builder
	for _, r := range c.runs {
		mark := ""
		if r.failed {
			mark = " [fehlgeschlagen]"
		}
		fmt.Fprintf(&b, "$ %s%s\n", r.cmd, mark)
		out := strings.TrimSpace(r.out)
		if len(out) > 600 {
			out = out[len(out)-600:]
		}
		if out != "" {
			b.WriteString(out + "\n")
		}
	}
	return b.String()
}

// wantsRecap: Rückblick beim Schließen anbieten?
func (a *App) wantsRecap(s *session) bool {
	return a.compLevel() >= domain.CompanionCoach && !a.compCfg().NoRecap && len(s.comp.runs) >= compRecapMin &&
		s.state == stConnected && !hasTag(s.conn, "begleiter-aus", "companion-off") && a.aiLevel(s.conn) != llm.PrivacyOff
}

// openRecap startet den Rückblick. closing: danach den Tab schließen (after).
func (a *App) openRecap(s *session, closing bool, after func()) tea.Cmd {
	if len(s.comp.runs) == 0 {
		a.flash(i18n.T("Begleiter: noch keine Befehle in dieser Sitzung"))
		return nil
	}
	a.recap = &recapState{sid: s.id, closing: closing, after: after}
	a.modal = a.recapModal(s)
	ag, raw, err := a.companionClient(s)
	if err != nil {
		a.recap.err = err.Error()
		a.modal = a.recapModal(s)
		return nil
	}
	a.compUse.hits = append(a.compUse.hits, time.Now())
	a.compUse.requests++
	log := s.comp.log()
	ctxt := a.companionContext(s, raw)
	sid := s.id
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		r, err := ag.RecapSession(ctx, log, ctxt)
		return compRecapMsg{sid, r, err}
	}
}

func (a *App) recapDone(m compRecapMsg) {
	rc := a.recap
	s := a.byID(m.sid)
	a.companionSpent(m.r.Tokens)
	if rc == nil || rc.sid != m.sid || s == nil {
		return
	}
	if m.err != nil {
		rc.err = m.err.Error()
	} else {
		r := m.r
		rc.r = &r
	}
	if a.modal != nil && a.modal.kind == mMenu && a.modal.recap {
		a.modal = a.recapModal(s)
	}
}

// recapRunbook: erfolgreiche Befehle der Sitzung als Runbook speichern.
func (a *App) recapRunbook(s *session, name string) {
	var steps []string
	for _, r := range s.comp.runs {
		if r.failed || (len(steps) > 0 && steps[len(steps)-1] == r.cmd) {
			continue
		}
		steps = append(steps, r.cmd)
	}
	if len(steps) > 25 {
		steps = steps[len(steps)-25:]
	}
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf(i18n.T("Sitzung auf %s"), s.conn.Alias)
	}
	rb := domain.Runbook{Name: name, Goal: name, Steps: steps}
	if s.client != nil {
		rb.OS = s.client.Remote.OS
	}
	saved, err := a.st.UpsertRunbook(rb)
	if err != nil {
		a.flashErr(i18n.T("Runbook: ") + err.Error())
		return
	}
	a.flashOK(i18n.T("Runbook gespeichert: ") + trunc(saved.Name, 40) + " (" + plural(len(saved.Steps), i18n.T("Schritt"), i18n.T("Schritte")) + ")")
}

// recapModal: Rückblick als Menü mit Text darüber.
func (a *App) recapModal(s *session) *modal {
	rc := a.recap
	st := a.s
	var b strings.Builder
	n := len(s.comp.runs)
	fails := 0
	for _, r := range s.comp.runs {
		if r.failed {
			fails++
		}
	}
	fmt.Fprintf(&b, "%s · %s · %s\n\n", s.conn.Alias, plural(n, i18n.T("Befehl"), i18n.T("Befehle")), plural(fails, i18n.T("Fehler"), i18n.T("Fehler")))
	close := func() tea.Cmd {
		a.recap = nil
		if rc.closing && rc.after != nil {
			rc.after()
		}
		return nil
	}
	var items []menuItem
	switch {
	case rc.err != "":
		b.WriteString(st.Danger.Render("× "+trunc(rc.err, 200)) + "\n")
	case rc.r == nil:
		b.WriteString(st.AI.Render(i18n.T("◆ fasst zusammen …")) + "\n")
	default:
		r := rc.r
		b.WriteString(st.Accent.Render(i18n.T("Was passiert ist")) + "\n")
		for _, p := range r.Points {
			b.WriteString("  · " + p + "\n")
		}
		if len(r.Facts) > 0 {
			b.WriteString("\n" + st.Accent.Render(fmt.Sprintf(i18n.T("Für %s merken"), s.conn.Alias)) + st.Muted.Render(i18n.T("  (Systemprofil, geht der KI künftig mit)")) + "\n")
			for _, f := range r.Facts {
				b.WriteString("  " + st.OK.Render("+") + " " + f + "\n")
			}
			facts := r.Facts
			items = append(items, menuItem{label: i18n.T("Merken"), hint: plural(len(facts), i18n.T("Tatsache"), i18n.T("Tatsachen")), run: func() tea.Cmd {
				a.companionRemember(s, facts...)
				return close()
			}})
		}
		name := r.Runbook
		items = append(items, menuItem{label: i18n.T("Als Runbook speichern"), hint: trunc(orDefault(name, i18n.T("Sitzung")), 30), run: func() tea.Cmd {
			a.recapRunbook(s, name)
			return close()
		}})
		if len(r.Facts) > 0 {
			facts := r.Facts
			items = append(items, menuItem{label: i18n.T("Merken + Runbook"), run: func() tea.Cmd {
				a.companionRemember(s, facts...)
				a.recapRunbook(s, name)
				return close()
			}})
		}
	}
	if rc.closing {
		items = append(items,
			menuItem{label: i18n.T("Tab schließen ohne Übernehmen"), run: close},
			menuItem{label: i18n.T("Tab offen lassen"), hint: "Esc", run: func() tea.Cmd { a.recap = nil; return nil }})
	} else {
		items = append(items, menuItem{label: i18n.T("Schließen"), hint: "Esc", run: func() tea.Cmd { a.recap = nil; return nil }})
	}
	return &modal{kind: mMenu, title: i18n.T("◆ Begleiter · Rückblick"), body: strings.TrimRight(b.String(), "\n"), items: items, recap: true}
}

// ---------------------------------------------------------------- Einstellungen (Palette)

var (
	compHourSteps   = []int{10, 20, 30, 60, 120}
	compBudgetSteps = []int{50, 150, 500, 2000} // in 1000 Tokens
)

func stepIndex(steps []int, v int) int {
	for i, x := range steps {
		if x == v {
			return i
		}
	}
	return -1
}

func (a *App) cycleCompanionHour(dir int) tea.Cmd {
	next := compHourSteps[cycleIndex(stepIndex(compHourSteps, a.compCfg().Hour()), len(compHourSteps), dir)]
	a.setCompanion(func(c *domain.CompanionConfig) { c.PerHour = next })
	return nil
}

func (a *App) cycleCompanionBudget(dir int) tea.Cmd {
	next := compBudgetSteps[cycleIndex(stepIndex(compBudgetSteps, a.compCfg().DayTokens()/1000), len(compBudgetSteps), dir)]
	a.setCompanion(func(c *domain.CompanionConfig) { c.DayKTok = next })
	return nil
}

func (a *App) toggleCompanionLocal() tea.Cmd {
	cfg := a.compCfg()
	on := !(cfg.OnlyLocal || cfg.Raw)
	a.setCompanion(func(c *domain.CompanionConfig) {
		c.OnlyLocal = on
		if !on {
			c.Raw = false // Filter aus gibt es nur mit eigener KI
		}
	})
	return nil
}

func (a *App) toggleCompanionRaw() tea.Cmd {
	on := !a.compCfg().Raw
	a.setCompanion(func(c *domain.CompanionConfig) {
		c.Raw = on
		if on {
			c.OnlyLocal = true
		}
	})
	if on {
		a.flash(i18n.T("Begleiter: Filter aus, aber nur solange jede Anfrage an eine eigene KI geht (lokal oder als eigener KI-Server eingetragen)"))
	}
	return nil
}

func ownHostsHint(h []string) string {
	if len(h) == 0 {
		return i18n.T("keine")
	}
	return trunc(strings.Join(h, ", "), 24)
}

// askOwnHosts: eigene KI-Server eintragen (Namen, durch Komma getrennt).
// Sie zählen wie Endpunkte im eigenen Netz, auch wenn sie extern stehen.
func (a *App) askOwnHosts() tea.Cmd {
	cur := strings.Join(a.st.LLM().OwnHosts, ", ")
	a.modal = inputModal(i18n.T("Eigene KI-Server"),
		i18n.T("Namen eigener KI-Server, die außerhalb des eigenen Netzes stehen (z. B. llm.firma.de), durch Komma getrennt. Sie zählen wie lokale Endpunkte: für „nur eigene KI“ und den Filter-Schalter des Begleiters. Ein Name gilt auch für seine Unternamen."),
		cur, func(x string) tea.Cmd {
			var hosts []string
			for _, h := range strings.FieldsFunc(x, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
				if h = strings.TrimSpace(h); h != "" {
					hosts = append(hosts, h)
				}
			}
			cfg := a.st.LLM()
			cfg.OwnHosts = hosts
			if err := a.st.SetLLM(cfg); err != nil {
				a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
				return nil
			}
			a.flashOK(i18n.T("Eigene KI-Server: ") + ownHostsHint(hosts))
			return nil
		})
	a.modal.in.Placeholder = "llm.firma.de, ki.example.org"
	return nil
}
