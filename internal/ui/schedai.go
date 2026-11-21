package ui

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/broadcast"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// KI-Prüfung als Zeitplan-Art (agent/schedcheck.go): Die KI wählt einmal
// bis zu fünf rein lesende Befehle, sie laufen in einer Zeile auf jedem
// Server (mit Trennmarken, damit ein Fehler die übrigen nicht aufhält),
// dann urteilt die KI je Host. Ergebnis wie bei jedem Zeitplan: Toast,
// Desktop- und Handy-Meldung, Text in der Zeitplanliste.

type schedAIMsg struct {
	id       string
	cmds     []string
	outs     []agent.HostOutput
	verdicts []agent.HostVerdict
	sum      string
	err      error
}

const schedMark = "@@sshtermy-check"

// schedAILine: alle Prüfbefehle in einer Zeile, jeder mit Marke und Exit.
func schedAILine(cmds []string) string {
	var parts []string
	for i, c := range cmds {
		parts = append(parts, fmt.Sprintf("echo '%s %d@@'; { %s; } 2>&1; echo \"%s-exit $?@@\"", schedMark, i, c, schedMark))
	}
	return strings.Join(parts, "; ")
}

var schedMarkRe = regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(schedMark) + ` (\d+)@@\r?$`)
var schedExitRe = regexp.MustCompile(regexp.QuoteMeta(schedMark) + `-exit (\d+)@@`)

// schedAISplit: Ausgabe der Zeile wieder je Befehl.
func schedAISplit(out string, cmds []string) []agent.CheckResult {
	res := make([]agent.CheckResult, len(cmds))
	for i, c := range cmds {
		res[i] = agent.CheckResult{Command: c, Exit: -1}
	}
	locs := schedMarkRe.FindAllStringSubmatchIndex(out, -1)
	for j, l := range locs {
		i, _ := strconv.Atoi(out[l[2]:l[3]])
		if i < 0 || i >= len(cmds) {
			continue
		}
		end := len(out)
		if j+1 < len(locs) {
			end = locs[j+1][0]
		}
		body := out[l[1]:end]
		if m := schedExitRe.FindStringSubmatchIndex(body); m != nil {
			res[i].Exit, _ = strconv.Atoi(body[m[2]:m[3]])
			body = body[:m[0]]
		}
		res[i].Output = strings.Trim(body, "\r\n")
	}
	return res
}

// schedAIContext: Material für die KI (Systeme der Hosts, kurz).
func schedAIContext(sc domain.Schedule, conns []domain.Connection) (agent.Context, []string) {
	c := agent.Context{Host: orDefault(sc.Folder, i18n.T("Zeitplan")), Now: time.Now()}
	count := map[string]int{}
	var hosts, lines []string
	for _, x := range conns {
		hosts = append(hosts, x.Alias)
		if x.Profile != nil && x.Profile.OS != "" {
			count[x.Profile.OS]++
			if len(lines) < 30 {
				lines = append(lines, x.Alias+": "+x.Profile.OS)
			}
		}
	}
	best := 0
	for os, n := range count {
		if n > best || (n == best && os < c.OS) {
			c.OS, best = os, n
		}
	}
	sort.Strings(lines)
	c.Profile = strings.Join(lines, "\n")
	return c, hosts
}

func (a *App) startScheduleAI(sc domain.Schedule, conns []domain.Connection) tea.Cmd {
	id := sc.ID
	ag, err := a.agentClientFor(conns...)
	if err != nil {
		return func() tea.Msg { return schedDoneMsg{id: id, err: err} }
	}
	actx, _ := schedAIContext(sc, conns)
	dial := schedDialer(a)
	prompt := sc.Prompt
	if !sc.ChecksPinned() {
		return func() tea.Msg {
			return schedDoneMsg{id: id, err: errors.New(i18n.T("Prüfbefehle noch nicht bestätigt: Zeitplan öffnen und speichern"))}
		}
	}
	cmds := agent.SafeChecks(sc.Checks, actx)
	if len(cmds) != len(sc.Checks) {
		return func() tea.Msg {
			return schedDoneMsg{id: id, err: errors.New(i18n.T("Ein bestätigter Prüfbefehl gilt nicht mehr als sicher: Zeitplan öffnen und speichern"))}
		}
	}
	a.flash(i18n.T("Zeitplan startet: ") + sc.Name)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		m := schedAIMsg{id: id, cmds: cmds}
		job := broadcast.Job{Conns: conns, Steps: []string{schedAILine(m.cmds)}, Timeout: 2 * time.Minute}
		res := broadcast.Run(ctx, job, dial, func(int, broadcast.Result) {})
		for _, r := range res {
			o := agent.HostOutput{Host: r.Alias}
			if r.Err != "" || r.State != broadcast.Done {
				o.Err = orDefault(r.Err, i18n.T("nicht fertig geworden"))
			} else {
				o.Results = schedAISplit(r.Output, m.cmds)
			}
			m.outs = append(m.outs, o)
		}
		m.verdicts, m.sum, m.err = ag.SchedJudge(ctx, prompt, m.outs, actx)
		return m
	}
}

type schedChecksMsg struct {
	id, prompt string
	cmds       []string
	err        error
}

// schedChooseChecks: Die KI schlägt die Prüfbefehle einmal vor; erst wenn
// du sie bestätigst, laufen sie (dann bei jedem Lauf dieselben).
func (a *App) schedChooseChecks(id string) tea.Cmd {
	sc, ok := a.scheduleByID(id)
	if !ok || sc.Kind != domain.ScheduleAI {
		return nil
	}
	conns := a.schedConns(sc)
	ag, err := a.agentClientFor(conns...)
	if err != nil {
		a.flashErr(err.Error())
		return nil
	}
	actx, hosts := schedAIContext(sc, conns)
	prompt := strings.TrimSpace(sc.Prompt)
	a.flash(i18n.T("KI wählt die Prüfbefehle …"))
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		m := schedChecksMsg{id: id, prompt: prompt}
		m.cmds, m.err = ag.SchedChecks(ctx, prompt, hosts, actx)
		if m.err == nil && len(m.cmds) == 0 {
			m.err = errors.New(i18n.T("Die KI hat keinen Befehl vorgeschlagen, der sicher nur liest"))
		}
		return m
	}
}

func (a *App) onSchedChecks(m schedChecksMsg) tea.Cmd {
	if m.err != nil {
		a.flashErr(i18n.T("Zeitplan: ") + m.err.Error())
		return nil
	}
	var b strings.Builder
	b.WriteString(i18n.T("Diese Befehle laufen bei jedem Lauf auf allen Servern im Ziel, ohne Aufsicht:") + "\n\n")
	for _, c := range m.cmds {
		b.WriteString("  $ " + c + "\n")
	}
	b.WriteString("\n" + i18n.T("Übernehmen? Ohne Bestätigung läuft die KI-Prüfung nicht."))
	a.showAsync(confirmModal(i18n.T("Prüfbefehle bestätigen"), b.String(), func() tea.Cmd {
		a.updateSchedule(m.id, func(x *domain.Schedule) {
			if strings.TrimSpace(x.Prompt) == m.prompt {
				x.Checks, x.ChecksFor = m.cmds, m.prompt
			}
		})
		a.flashOK(i18n.T("Prüfbefehle bestätigt"))
		return nil
	}))
	return nil
}

// schedAIReport: Text für die Zeitplanliste, Zahl der Hosts ok / mit Befund.
func schedAIReport(m schedAIMsg) (text string, okN, badN int) {
	var b strings.Builder
	if m.sum != "" {
		b.WriteString(m.sum + "\n\n")
	}
	judged := map[string]agent.HostVerdict{}
	for _, v := range m.verdicts {
		judged[v.Host] = v
	}
	for _, o := range m.outs {
		v, ok := judged[o.Host]
		switch {
		case o.Err != "":
			badN++
			fmt.Fprintf(&b, "× %s: %s\n", o.Host, o.Err)
		case !ok:
			fmt.Fprintf(&b, "? %s: %s\n", o.Host, i18n.T("kein Urteil der KI"))
		case v.OK:
			okN++
			fmt.Fprintf(&b, "✓ %s: %s\n", o.Host, v.Finding)
		default:
			badN++
			fmt.Fprintf(&b, "× %s: %s\n", o.Host, v.Finding)
		}
	}
	b.WriteString("\n" + i18n.T("Prüfbefehle (nur lesend):") + "\n")
	for _, c := range m.cmds {
		b.WriteString("  $ " + c + "\n")
	}
	text = strings.TrimRight(b.String(), "\n")
	if len(text) > 6000 {
		text = text[:6000] + "\n…"
	}
	return text, okN, badN
}

func (a *App) schedAIMsg(msg tea.Msg) (tea.Cmd, bool) {
	if c, ok := msg.(schedChecksMsg); ok {
		return a.onSchedChecks(c), true
	}
	m, ok := msg.(schedAIMsg)
	if !ok {
		return nil, false
	}
	if m.err != nil {
		return a.scheduleMsg(schedDoneMsg{id: m.id, err: m.err})
	}
	delete(a.schedRun, m.id)
	text, okN, badN := schedAIReport(m)
	a.updateSchedule(m.id, func(x *domain.Schedule) { x.LastOK, x.LastFail, x.LastText = okN, badN, text })
	name := i18n.T("KI-Prüfung")
	if sc, ok := a.scheduleByID(m.id); ok {
		name = sc.Name
	}
	head := fmt.Sprintf(i18n.T("Zeitplan „%s“: KI-Prüfung fertig, %d in Ordnung, %d mit Befund"), name, okN, badN)
	body := orDefault(m.sum, fmt.Sprintf(i18n.T("%d in Ordnung, %d mit Befund"), okN, badN))
	if badN > 0 {
		a.pushToast(toastWarn, head+"\n"+trunc(body, 200))
	} else {
		a.flashOK(head)
	}
	a.schedDesktop(name, body)
	if a.pick != nil && a.pick.title == i18n.T("Zeitpläne") {
		a.pick.items = a.scheduleItems()
	}
	return nil, true
}
