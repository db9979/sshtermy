package ui

// Befehle vorab prüfen (Ideen aus lxask): Bevor ein KI-Befehl eingefügt
// oder ausgeführt wird, prüft sshTermy ihn, ohne ihn auszuführen
// (internal/cmdcheck):
//
//   - ohne KI und ohne Host: Vorgaben aus der Frage („ohne lsof“, „Port
//     8080“, „.log-Dateien“), macOS-Formen, bekannte Ausrutscher; was genau
//     eine richtige Lösung hat (grep -r, head -n 2, Paketliste dieses
//     Systems), wird gleich repariert;
//   - auf dem echten Host: Gibt es das Programm (command -v), stehen die
//     Optionen und Unterbefehle in seiner man-Seite (sonst --help)? Das läuft
//     unsichtbar in einem eigenen SSH-Kanal, liest nur, und die Doku bleibt
//     für die Sitzung im Speicher.
//
// Bleiben Einwände, bekommt die KI einen zweiten Versuch mit den
// Einwänden; behalten wird der Befehl mit weniger Einwänden. Was offen
// bleibt, steht als „unbestätigt“ an der Karte.

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/cmdcheck"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// cardCheck: Prüfstand einer Antwortkarte.
type cardCheck struct {
	busy     bool            // Doku wird gelesen oder die KI bessert nach
	res      cmdcheck.Result // Ergebnis für res.Command
	withDocs bool            // die man-Seiten des Hosts flossen ein
	retried  bool            // die KI hatte ihren zweiten Versuch
	improved string          // vorheriger Befehl, wenn die KI nachgebessert hat
}

// docsMsg: Doku vom Host ist da.
type docsMsg struct {
	sid  int
	docs cmdcheck.Docs
	err  error
	next func(s *session) tea.Cmd // läuft danach im UI-Takt
}

// recheckMsg: zweiter Versuch der KI für eine Karte.
type recheckMsg struct {
	sid, seq, idx int
	cmd           string
	docs          cmdcheck.Docs
	err           error
}

func (a *App) cmdCheckOn() bool { return a.st == nil || !a.st.Settings().NoCmdCheck }

func (a *App) toggleCmdCheck() tea.Cmd {
	on := !a.cmdCheckOn()
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.NoCmdCheck = !on }); err != nil {
		a.flash(i18n.T("Fehler: ") + err.Error())
		return nil
	}
	if on {
		a.flashOK(i18n.T("KI-Befehle werden vorab geprüft"))
	} else {
		a.flash(i18n.T("Prüfung vorab aus"))
	}
	return nil
}

// checkable: Unix-Shell (über SSH oder lokal), dort lassen sich Befehle prüfen.
func (a *App) checkable(s *session) bool {
	return s != nil && a.safetyKind(s) == "unix" && !s.netDevice()
}

// canProbe: Lässt sich die Doku vom Host holen (eigener Kanal, nur lesend)?
func (a *App) canProbe(s *session) bool {
	if !a.checkable(s) || s.state != stConnected {
		return false
	}
	if s.isLocal() {
		return runtime.GOOS != "windows"
	}
	return s.client != nil && s.conn.Protocol != domain.ProtoWinRM && s.conn.Protocol != domain.ProtoTelnet && s.conn.Protocol != domain.ProtoSerial
}

// probeRunner führt das Prüfskript aus: über SSH in einem eigenen Kanal
// oder auf dem eigenen Rechner.
func (a *App) probeRunner(s *session) func(ctx context.Context, script string) (string, error) {
	if s.isLocal() {
		return func(ctx context.Context, script string) (string, error) {
			out, err := exec.CommandContext(ctx, "sh", "-c", script).CombinedOutput()
			return string(out), err
		}
	}
	cl := s.client
	return func(ctx context.Context, script string) (string, error) {
		r, err := cl.Exec(ctx, script)
		return string(r.Output), err
	}
}

// checkSys: was das Systemprofil über den Host weiß.
func (a *App) checkSys(s *session) cmdcheck.Sys {
	pm := s.prof.PkgMgr
	switch pm {
	case "apt-get", "aptitude":
		pm = "apt"
	}
	return cmdcheck.Sys{PkgManager: pm, Root: s.conn.Username == "root"}
}

// fetchDocs holt die Doku zu den noch unbekannten Programmen und ruft
// danach next auf (im UI-Takt). Ohne Host oder ohne Neues gleich next.
func (a *App) fetchDocs(s *session, programs []string, next func(s *session) tea.Cmd) tea.Cmd {
	if s.docs == nil {
		s.docs = cmdcheck.Docs{}
	}
	missing := s.docs.Missing(programs)
	if len(missing) == 0 || !a.canProbe(s) {
		return next(s)
	}
	run, sid := a.probeRunner(s), s.id
	return func() tea.Msg {
		docs, err := probeDocs(run, missing)
		return docsMsg{sid: sid, docs: docs, err: err, next: next}
	}
}

// probeDocs: Prüfskript in Paketen zu je cmdcheck.MaxProbe Programmen.
func probeDocs(run func(context.Context, string) (string, error), programs []string) (cmdcheck.Docs, error) {
	docs := cmdcheck.Docs{}
	for i := 0; i < len(programs); i += cmdcheck.MaxProbe {
		part := programs[i:min(i+cmdcheck.MaxProbe, len(programs))]
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		out, err := run(ctx, cmdcheck.ProbeScript(part))
		cancel()
		got := cmdcheck.ParseProbe(out)
		if len(got) == 0 && err != nil {
			return docs, err
		}
		docs.Merge(got)
	}
	return docs, nil
}

func (a *App) cmdCheckMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case docsMsg:
		s := a.byID(m.sid)
		if s == nil {
			return nil, true
		}
		if s.docs == nil {
			s.docs = cmdcheck.Docs{}
		}
		s.docs.Merge(m.docs)
		if m.next != nil {
			return m.next(s), true
		}
		return nil, true
	case recheckMsg:
		a.recheckDone(m)
		return nil, true
	case wishMsg:
		return a.wishDone(m), true
	}
	return nil, false
}

// ---------------------------------------------------------------- Karten

// startCardChecks (nach der Antwort): alle einzeiligen Shell-Karten prüfen.
func (a *App) startCardChecks(s *session) tea.Cmd {
	q := &s.ask
	if !a.cmdCheckOn() || !a.checkable(s) || q.question == "" {
		return nil
	}
	cards := answerCards(agent.VisibleAnswer(q.answer.String()))
	sys := a.checkSys(s)
	var progs []string
	for i, cd := range cards {
		cd = q.cardText(i, cd)
		if !cd.runnable() || i >= 9 {
			continue
		}
		res := cmdcheck.Static(q.question, cd.code, sys)
		if res.Command != cd.code {
			if q.codes == nil {
				q.codes = map[int]string{}
			}
			q.codes[i] = res.Command
		}
		if q.checks == nil {
			q.checks = map[int]*cardCheck{}
		}
		q.checks[i] = &cardCheck{res: res, busy: a.canProbe(s)}
		progs = append(progs, cmdcheck.Programs(res.Command)...)
	}
	if len(q.checks) == 0 {
		return nil
	}
	for _, ck := range q.checks {
		ck.res = q.cardResult(ck.res)
	}
	seq := q.seq
	return a.fetchDocs(s, progs, func(s *session) tea.Cmd { return a.finishCardChecks(s, seq) })
}

// finishCardChecks: mit der Doku des Hosts nachprüfen, bei Einwänden die
// KI einmal nachbessern lassen.
func (a *App) finishCardChecks(s *session, seq int) tea.Cmd {
	q := &s.ask
	if q.seq != seq {
		return nil
	}
	sys := a.checkSys(s)
	var cmds []tea.Cmd
	for i, ck := range q.checks {
		ck.busy = false
		ck.withDocs = len(s.docs) > 0 && a.canProbe(s)
		prevRepairs := ck.res.Repairs
		ck.res = q.cardResult(cmdcheck.Full(q.question, ck.res.Command, sys, s.docs))
		ck.res.Repairs = append(prevRepairs, ck.res.Repairs...)
		if ck.res.OK() || ck.retried {
			continue
		}
		if c := a.requestRecheck(s, i); c != nil {
			cmds = append(cmds, c)
		}
	}
	return tea.Batch(cmds...)
}

// requestRecheck: zweiter Versuch der KI mit den Einwänden.
func (a *App) requestRecheck(s *session, i int) tea.Cmd {
	q := &s.ask
	ck := q.checks[i]
	ag, err := a.cmdAgent(s)
	if err != nil {
		return nil
	}
	ensure := a.offlineEnsure(ag)
	ck.busy, ck.retried = true, true
	sid, seq, question, cmd := s.id, q.seq, q.question, ck.res.Command
	issues := ck.res.Texts()
	c := a.agentContext(s, false)
	var run func(context.Context, string) (string, error)
	if a.canProbe(s) {
		run = a.probeRunner(s)
	}
	known := cmdcheck.Docs{}
	known.Merge(s.docs)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := ensure(ctx); err != nil {
			return recheckMsg{sid: sid, seq: seq, idx: i, err: err}
		}
		next, err := ag.RecheckCommand(ctx, question, cmd, issues, c)
		m := recheckMsg{sid: sid, seq: seq, idx: i, cmd: next, err: err}
		if err == nil && run != nil {
			if miss := known.Missing(cmdcheck.Programs(next)); len(miss) > 0 {
				m.docs, _ = probeDocs(run, miss)
			}
		}
		return m
	}
}

func (a *App) recheckDone(m recheckMsg) {
	s := a.byID(m.sid)
	if s == nil || s.ask.seq != m.seq {
		return
	}
	q := &s.ask
	ck := q.checks[m.idx]
	if ck == nil {
		return
	}
	ck.busy = false
	if s.docs == nil {
		s.docs = cmdcheck.Docs{}
	}
	s.docs.Merge(m.docs)
	if m.err != nil || m.cmd == "" {
		return
	}
	res := q.cardResult(cmdcheck.Full(q.question, m.cmd, a.checkSys(s), s.docs))
	if len(res.Issues) >= len(ck.res.Issues) {
		return // nicht besser: der erste Befehl bleibt, mit seinen Einwänden
	}
	if q.codes == nil {
		q.codes = map[int]string{}
	}
	ck.improved = ck.res.Command
	q.codes[m.idx] = res.Command
	ck.res = res
}

// cardResult: Bei mehreren Karten deckt jede nur einen Teil der Frage ab;
// dann zählt nicht, ob jede Angabe der Frage in jedem Befehl steht.
func (q *askState) cardResult(r cmdcheck.Result) cmdcheck.Result {
	if len(q.checks) <= 1 {
		return r
	}
	var keep []cmdcheck.Issue
	for _, is := range r.Issues {
		if is.Kind != cmdcheck.KindDetail {
			keep = append(keep, is)
		}
	}
	r.Issues = keep
	return r
}

// checkLines: die Zeilen unter dem Befehl einer geprüften Karte.
func (a *App) checkLines(ck *cardCheck, code string, w int) []string {
	st := a.s
	if ck == nil || ck.res.Command != code {
		return nil // inzwischen geändert (Fix übernommen)
	}
	var out []string
	if ck.improved != "" {
		out = append(out, wrap(st.AI.Render("» "+i18n.T("KI hat nachgebessert, war: ")+trunc(ck.improved, w)), w))
	}
	if len(ck.res.Repairs) > 0 {
		out = append(out, wrap(st.AI.Render("» "+i18n.T("angepasst: ")+strings.Join(ck.res.Repairs, " · ")), w))
	}
	switch {
	case ck.busy && ck.retried:
		out = append(out, st.Muted.Render(i18n.T("◌ KI bessert nach …")))
	case ck.busy:
		out = append(out, st.Muted.Render(i18n.T("◌ prüfe gegen die man-Seiten des Hosts …")))
	case !ck.res.OK():
		out = append(out, wrap(st.Warn.Render("? "+i18n.T("unbestätigt: ")+strings.Join(ck.res.Texts(), " · ")), w))
	case ck.withDocs:
		out = append(out, st.OK.Render("✓ "+i18n.T("geprüft (man-Seiten des Hosts)"))+st.Muted.Render("  e "+i18n.T("erklärt")))
	}
	return out
}

// checkOpen: Karte hat offene Einwände (Rahmen gelb).
func (q *askState) checkOpen(i int, code string) bool {
	ck := q.checks[i]
	return ck != nil && !ck.busy && ck.res.Command == code && !ck.res.OK()
}
