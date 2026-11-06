package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/broadcast"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/runbook"
)

// Runbooks im Broadcast
//
//   - Variablen: {{name}} oder {{name=Vorschlag}} in einem Schritt fragt vor dem
//     Start nach einem Wert; {{host}}, {{alias}}, {{user}}, {{port}} füllt
//     sshTermy pro Host selbst aus.
//   - Bearbeiten: Quelle auf ein Runbook, e lädt die Schritte ins Befehlsfeld;
//     Strg+R speichert das Befehlsfeld als (neues oder bearbeitetes) Runbook,
//     Strg+D löscht das gewählte Runbook.
//   - KI-Prüfung (im Bestätigen-Schritt, k): pro Host prüft das Sprachmodell
//     anhand des Systemprofils, ob jeder Schritt dort läuft, und schlägt eine
//     Entsprechung vor. Mit a laufen die angepassten Befehle. Gefährlich oder
//     nicht entscheiden weiter die festen Regeln, auch für die Vorschläge.

// hostReview: KI-Urteil für einen Host.
type hostReview struct {
	busy  bool
	err   string
	steps []string // geprüfte Schritte (ausgefüllt)
	r     agent.Review
}

type bcReviewMsg struct {
	seq    int
	connID string
	steps  []string
	r      agent.Review
	err    error
}

// varsOf: Variablen der aktuellen Schritte.
func (b *bcState) varsOf() []runbook.Var { return runbook.Vars(b.stepList()) }

// hostSteps: die Schritte für einen Host (Variablen ausgefüllt, auf Wunsch
// mit den Vorschlägen der KI).
func (b *bcState) hostSteps(c domain.Connection) []string {
	steps := runbook.Fill(b.stepList(), b.vals, c)
	if !b.useAdapted {
		return steps
	}
	if hr := b.review[c.ID]; hr != nil && !hr.busy && hr.err == "" && sameSteps(hr.steps, steps) {
		if out, _ := hr.r.Apply(steps); len(out) > 0 {
			return out
		}
	}
	return steps
}

func sameSteps(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// perHost: Schritte für alle gewählten Hosts.
func (b *bcState) perHost(hosts []domain.Connection) [][]string {
	out := make([][]string, len(hosts))
	for i, c := range hosts {
		out[i] = b.hostSteps(c)
	}
	return out
}

// allDangers: Gefahrentreffer über alle Hosts (gleiche Befehle nur einmal).
func allDangers(per [][]string) []broadcast.Danger {
	var out []broadcast.Danger
	seen := map[string]bool{}
	for _, steps := range per {
		for _, d := range broadcast.Dangers(steps) {
			if !seen[d.Command] {
				seen[d.Command] = true
				out = append(out, d)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- Variablen abfragen

func (a *App) bcToVars() tea.Cmd {
	b := a.bc
	vars := b.varsOf()
	if len(vars) == 0 {
		return nil
	}
	if b.vals == nil {
		b.vals = map[string]string{}
	}
	b.vars, b.varIn, b.varCur = vars, nil, 0
	for _, v := range vars {
		in := newInput(b.vals[v.Name], false)
		in.Placeholder = v.Default
		in.SetWidth(40)
		b.varIn = append(b.varIn, in)
	}
	b.phase = bcVars
	b.cmd.Blur()
	return b.varIn[0].Focus()
}

func (a *App) bcVarsKey(k tea.KeyPressMsg) tea.Cmd {
	b := a.bc
	move := func(d int) tea.Cmd {
		b.varIn[b.varCur].Blur()
		b.varCur = (b.varCur + d + len(b.varIn)) % len(b.varIn)
		return b.varIn[b.varCur].Focus()
	}
	switch k.String() {
	case "esc":
		b.phase = bcSetup
		return b.setFocus(b.focus)
	case "tab", "down":
		return move(1)
	case "shift+tab", "up":
		return move(-1)
	case "enter", "ctrl+s":
		for i, v := range b.vars {
			b.vals[v.Name] = strings.TrimSpace(b.varIn[i].Value())
		}
		if k.String() == "enter" && b.varCur < len(b.varIn)-1 {
			return move(1)
		}
		if miss := runbook.Missing(b.stepList(), b.vals); len(miss) > 0 {
			b.err = i18n.T("Wert fehlt: ") + strings.Join(miss, ", ")
			return nil
		}
		b.err = ""
		b.varIn[b.varCur].Blur()
		b.armed, b.phase = false, bcConfirm
		return nil
	}
	var cmd tea.Cmd
	b.varIn[b.varCur], cmd = b.varIn[b.varCur].Update(k)
	b.err = ""
	return cmd
}

func (a *App) viewBcVars() string {
	b, s := a.bc, a.s
	w := min(90, a.w-4)
	var o strings.Builder
	title := i18n.T("eigener Befehl")
	if rb := b.book(); rb != nil {
		title = "▶ " + rb.Name
	}
	o.WriteString(s.Accent.Render(i18n.T("» Variablen ausfüllen")) + s.Muted.Render("  "+trunc(title, w-30)) + "\n\n")
	for i, st := range b.stepList() {
		o.WriteString(s.Muted.Render(fmt.Sprintf("%d. ", i+1)) + s.Code.Render(trunc(st, w-10)) + "\n")
	}
	o.WriteString("\n")
	for i, v := range b.vars {
		lbl := "  " + lipgloss.NewStyle().Foreground(s.T.Muted).Width(18).Render(trunc(v.Name, 17))
		if i == b.varCur {
			lbl = s.Accent.Render("▌ ") + lipgloss.NewStyle().Foreground(s.T.Text).Bold(true).Width(18).Render(trunc(v.Name, 17))
		}
		hint := ""
		if v.Default != "" {
			hint = s.Muted.Render(i18n.T("  leer = ") + v.Default)
		}
		o.WriteString(lbl + b.varIn[i].View() + hint + "\n")
	}
	if runbook.HasBuiltins(b.stepList()) {
		o.WriteString("\n" + s.Muted.Render(i18n.T("{{host}}, {{alias}}, {{user}} und {{port}} setzt sshTermy pro Host ein.")) + "\n")
	}
	if b.err != "" {
		o.WriteString("\n" + s.Danger.Render("× "+b.err) + "\n")
	}
	o.WriteString("\n" + a.hints(s.Muted, [2]string{"⏎", i18n.T("nächstes Feld")}, [2]string{"Strg+S", i18n.T("weiter")}, [2]string{"Esc", i18n.T("zurück")}))
	return lipgloss.Place(a.w, a.h-2, lipgloss.Center, lipgloss.Center, s.Panel.Width(w).Render(o.String()))
}

// ---------------------------------------------------------------- Runbooks bearbeiten

// book: gewähltes Runbook (Quelle) oder nil.
func (b *bcState) book() *domain.Runbook {
	if b.src > 0 && b.src <= len(b.books) {
		return &b.books[b.src-1]
	}
	return nil
}

// bcEditBook lädt das gewählte Runbook ins Befehlsfeld.
func (a *App) bcEditBook() tea.Cmd {
	b := a.bc
	rb := b.book()
	if rb == nil {
		return nil
	}
	b.editID, b.editName = rb.ID, rb.Name
	b.cmd.SetValue(strings.Join(rb.Steps, "\n"))
	b.src = 0
	a.flash(i18n.T("Bearbeite „") + rb.Name + i18n.T("“ · Strg+R speichert"))
	return b.setFocus(bcFocusCmd)
}

// bcSaveBook speichert das Befehlsfeld als Runbook (fragt nach dem Namen).
func (a *App) bcSaveBook() tea.Cmd {
	b := a.bc
	if b.book() != nil {
		return a.bcEditBook()
	}
	steps := b.stepList()
	if len(steps) == 0 {
		b.err = i18n.T("Kein Befehl")
		return nil
	}
	name := b.editName
	if name == "" {
		name = trunc(steps[0], 40)
	}
	body := i18n.T("Name für das Runbook. Variablen: {{name}} oder {{name=Vorschlag}}, pro Host: {{host}} {{alias}} {{user}} {{port}}.")
	a.modal = inputModal(i18n.T("Als Runbook speichern"), body, name, func(n string) tea.Cmd {
		if n == "" {
			return nil
		}
		rb := domain.Runbook{ID: b.editID, Name: n, Goal: n, Steps: steps}
		for _, x := range a.st.Runbooks("") {
			if x.ID == b.editID {
				rb.Goal, rb.ConnectionID, rb.OS, rb.CreatedAt, rb.LastUsed, rb.Uses = x.Goal, x.ConnectionID, x.OS, x.CreatedAt, x.LastUsed, x.Uses
				if x.Goal == x.Name {
					rb.Goal = n
				}
			}
		}
		saved, err := a.st.UpsertRunbook(rb)
		if err != nil {
			b.err = err.Error()
			return nil
		}
		b.editID, b.editName = saved.ID, saved.Name
		b.books = a.st.Runbooks("")
		a.flash(i18n.T("Runbook gespeichert: ") + trunc(saved.Name, 40) + " (" + plural(len(saved.Steps), i18n.T("Schritt"), i18n.T("Schritte")) + ")")
		return nil
	})
	return nil
}

func (a *App) bcDeleteBook() tea.Cmd {
	b := a.bc
	rb := b.book()
	if rb == nil {
		return nil
	}
	id, name := rb.ID, rb.Name
	a.modal = confirmModal(i18n.T("Runbook löschen?"), "„"+name+i18n.T("“ löschen?"), func() tea.Cmd {
		_ = a.st.DeleteRunbook(id)
		b.books = a.st.Runbooks("")
		b.src = 0
		if b.editID == id {
			b.editID, b.editName = "", ""
		}
		return nil
	})
	return nil
}

// ---------------------------------------------------------------- KI-Prüfung

// connContext: was das Modell über eine gespeicherte Verbindung weiß (ohne
// offene Sitzung: Systemprofil und Gerätetyp).
func connContext(c domain.Connection) agent.Context {
	ctx := agent.Context{Host: c.Alias, Net: c.Device == domain.DeviceNetwork}
	if p := c.Profile; p != nil && !p.Empty() {
		ctx.OS = p.Summary()
		ctx.Shell = p.Shell
		ctx.Profile = p.Prompt()
		ctx.Net = ctx.Net || p.Family == domain.FamilyNetwork
	}
	if ctx.OS == "" && ctx.Net {
		ctx.OS = i18n.T("Netzwerkgerät (Switch/Router)")
	}
	return ctx
}

func hasProfile(c domain.Connection) bool {
	return (c.Profile != nil && !c.Profile.Empty()) || c.Device == domain.DeviceNetwork
}

// bcReview startet die KI-Prüfung für alle gewählten Hosts.
func (a *App) bcReview() tea.Cmd {
	b := a.bc
	ag, err := a.agentClient()
	if err != nil {
		a.flashErr(i18n.T("KI-Prüfung: ") + err.Error())
		return nil
	}
	if b.review == nil {
		b.review = map[string]*hostReview{}
	}
	b.reviewSeq++
	seq := b.reviewSeq
	var cmds []tea.Cmd
	for _, c := range b.chosen() {
		steps := runbook.Fill(b.stepList(), b.vals, c)
		if hr := b.review[c.ID]; hr != nil && hr.err == "" && !hr.busy && sameSteps(hr.steps, steps) {
			continue // schon geprüft
		}
		b.review[c.ID] = &hostReview{busy: true, steps: steps}
		c, cctx := c, connContext(c)
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			r, err := ag.Review(ctx, steps, cctx)
			return bcReviewMsg{seq: seq, connID: c.ID, steps: steps, r: r, err: err}
		})
	}
	if len(cmds) == 0 {
		a.flash(i18n.T("Alle gewählten Hosts sind schon geprüft."))
		return nil
	}
	return tea.Batch(cmds...)
}

func (a *App) bcReviewDone(m bcReviewMsg) {
	b := a.bc
	if b == nil || b.review == nil || m.seq != b.reviewSeq {
		return
	}
	hr := &hostReview{steps: m.steps, r: m.r}
	if m.err != nil {
		hr.err = reviewErr(m.err)
	}
	b.review[m.connID] = hr
	b.armed = false // Befehle können sich geändert haben
}

func reviewErr(err error) string {
	switch {
	case errors.Is(err, agent.ErrUnreadable), errors.Is(err, agent.ErrReviewMismatch):
		return i18n.T("Antwort der KI unbrauchbar")
	case errors.Is(err, context.DeadlineExceeded):
		return i18n.T("Zeitlimit überschritten")
	}
	return trunc(oneLine(err.Error()), 80)
}

// reviewLines: Ergebnis der KI-Prüfung für die Bestätigung.
func (a *App) reviewLines(hosts []domain.Connection, w int) []string {
	b, s := a.bc, a.s
	if b.review == nil {
		return nil
	}
	var out []string
	changed := 0
	for _, c := range hosts {
		hr := b.review[c.ID]
		name := s.Text.Bold(true).Render(trunc(c.Alias, 18))
		pad := strings.Repeat(" ", max(0, 19-lipgloss.Width(name)))
		switch {
		case hr == nil:
			out = append(out, s.Muted.Render("○ ")+name+pad+s.Muted.Render(i18n.T("nicht geprüft (k)")))
		case hr.busy:
			out = append(out, s.AI.Render("◐ ")+name+pad+s.AI.Render(i18n.T("KI prüft …")))
		case hr.err != "":
			out = append(out, s.Danger.Render("! ")+name+pad+s.Danger.Render(trunc(hr.err, w-26)))
		case !sameSteps(hr.steps, runbook.Fill(b.stepList(), b.vals, c)):
			out = append(out, s.Muted.Render("○ ")+name+pad+s.Muted.Render(i18n.T("Schritte geändert, nochmal prüfen (k)")))
		case hr.r.Fits():
			txt := i18n.T("passt zum Systemprofil")
			if !hasProfile(c) {
				txt += i18n.T(" (ohne Profil, nur geschätzt)")
			}
			out = append(out, s.OK.Render("✓ ")+name+pad+s.OK.Render(txt))
		default:
			changed++
			sum := hr.r.Summary
			if sum == "" {
				sum = fmt.Sprintf(i18n.T("%d Schritte passen nicht"), hr.r.Changed())
			}
			out = append(out, s.Warn.Render("↔ ")+name+pad+s.Warn.Render(trunc(sum, w-26)))
			shown := 0
			for i, st := range hr.r.Steps {
				if st.OK || i >= len(hr.steps) {
					continue
				}
				if shown == 3 {
					out = append(out, s.Muted.Render(strings.Repeat(" ", 21)+"…"))
					break
				}
				shown++
				to := s.Danger.Render(i18n.T("gibt es dort nicht, fällt weg"))
				if st.Adapted.Command != "" {
					to = s.Code.Render(trunc(st.Adapted.Command, max(10, (w-30)/2)))
					if len(st.Adapted.Danger) > 0 {
						to = s.Danger.Render(trunc(st.Adapted.Command, max(10, (w-30)/2)) + " ▲")
					}
				}
				from := s.Muted.Render(fmt.Sprintf("%d. ", i+1) + trunc(hr.steps[i], max(10, (w-30)/2)))
				out = append(out, strings.Repeat(" ", 21)+from+s.Muted.Render(" → ")+to)
				if st.Reason != "" {
					out = append(out, strings.Repeat(" ", 24)+s.Muted.Render(trunc(st.Reason, w-30)))
				}
			}
		}
	}
	if changed > 0 {
		state := i18n.T("aus: es laufen die Original-Schritte")
		if b.useAdapted {
			state = i18n.T("an: angepasste Befehle laufen")
		}
		out = append(out, "", s.Text.Bold(true).Render(i18n.T("a = KI-Vorschläge "))+s.Text.Render(state))
	}
	return out
}
