package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
	"github.com/db9979/sshtermy/internal/sessionlog"
)

// ---------------------------------------------------------------- Agent-Modus
//
// Der Agent-Modus ist der Ziel-Modus ohne Bestätigung pro Schritt: sshTermy
// führt jeden Schritt selbst aus, solange die Freigabestufe ihn erlaubt, legt
// Skripte per SFTP in einem eigenen Arbeitsordner an und hält an, sobald
// etwas Gefährliches, eine Grenze oder eine Passwortabfrage kommt.
//
// Feste Grenzen, die keine Stufe aufhebt:
//   - Gefahrenbefehle (safety-Regeln, auch jede Zeile eines Skripts) fragen immer.
//   - Verbindungen mit Schutz (Standard, siehe isProduction) und
//     Netzwerkgeräte: nur Lesendes läuft allein.
//   - Esc im Panel oder Tippen im Terminal hält den Agenten sofort an.

const (
	lvlCareful = iota // nur Lesendes läuft allein
	lvlNormal         // alles außer Gefahr, sudo, root/Admin und Inline-Code (python -c …)
	lvlFull           // auch sudo; Gefahr fragt trotzdem
)

const (
	agentSteps       = 50 // dann hält der Agent an; ⏎ erlaubt agentStepsMore weitere
	agentStepsMore   = 50
	agentTime        = 15 * time.Minute
	agentFailsInRow  = 3
	agentCmdTimeout  = 5 * time.Minute
	agentDirBase     = ".sshtermy-agent"
	agentPreviewRows = 6
)

func levelName(l int) string {
	switch l {
	case lvlCareful:
		return i18n.T("Vorsichtig")
	case lvlFull:
		return i18n.T("Voll")
	}
	return i18n.T("Normal")
}

func levelHint(l int) string {
	switch l {
	case lvlCareful:
		return i18n.T("nur Lesendes läuft allein")
	case lvlFull:
		return i18n.T("auch sudo; Gefahr fragt immer")
	}
	return i18n.T("alles außer Gefahr, sudo, root und Code in der Zeile")
}

// agentRunning: läuft gerade ein Agent (nicht angehalten)?
func (g *goalState) agentRunning() bool {
	return g.auto && g.active && !g.paused && !g.planning && g.fp == nil
}

// agentDir: Arbeitsordner dieses Laufs, so wie die Shell ihn versteht.
func (g *goalState) agentDir() string { return "~/" + agentDirBase + "/" + g.runID }

func newRunID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// agentFilesOK: kann der Agent hier Dateien anlegen? Nur über SSH mit SFTP
// auf Unix-Systemen; Switches, Telnet und serielle Konsolen bekommen nur Befehle.
func (a *App) agentFilesOK(s *session) bool {
	return (s.client != nil || s.isLocal()) && !s.netDevice() && a.safetyKind(s) == "unix"
}

// agentInit: neuer Lauf (aus startGoalWith).
func (a *App) agentInit(s *session, rb *domain.Runbook) {
	g := &s.goal
	g.runID = newRunID()
	g.started = time.Now()
	g.deadline = g.started.Add(agentTime)
	g.maxSteps = agentSteps
	g.planOK, g.paused, g.pauseWhy, g.fails = false, false, "", 0
	if rb != nil {
		for _, f := range rb.Files {
			g.recipeFiles = append(g.recipeFiles, agent.FileAction{Name: f.Name, Content: f.Content})
		}
		for i := range g.recipe {
			g.recipe[i] = strings.ReplaceAll(g.recipe[i], domain.AgentDirToken, g.agentDir())
		}
	}
}

// agentGate: darf dieser Schritt ohne Rückfrage laufen? Sonst der Grund.
func (a *App) agentGate(s *session, st *agent.Step) (bool, string) {
	g := &s.goal
	if len(st.Danger) > 0 {
		return false, i18n.T("Gefahr: ") + st.Danger[0].Reason
	}
	prod, net := isProduction(s.conn), s.netDevice()
	if st.File != nil {
		switch {
		case g.approved && !g.planHasFile(st.File.Name):
			return false, i18n.T("nicht im freigegebenen Plan")
		case g.level == lvlCareful:
			return false, i18n.T("Stufe Vorsichtig: Datei anlegen fragt nach")
		case prod:
			return false, i18n.T("Schutz an: Datei anlegen fragt nach")
		case st.Elevated && g.level < lvlFull:
			return false, i18n.T("Skript nutzt sudo")
		}
		return true, ""
	}
	if why := safety.Sensitive(st.Command); why != "" {
		// liest nur, aber die Ausgabe ginge an die KI: immer fragen
		return false, why + i18n.T(" (Ausgabe ginge an die KI)")
	}
	if safety.ReadOnly(st.Command, a.safetyKind(s)) && !(st.Elevated && g.level == lvlCareful) {
		return true, ""
	}
	switch {
	case g.approved && !g.planHasCmd(st.Command):
		return false, i18n.T("nicht im freigegebenen Plan")
	case g.level == lvlCareful:
		return false, i18n.T("Stufe Vorsichtig: Befehl ändert etwas")
	case net:
		return false, i18n.T("Netzwerkgerät: Konfiguration fragt immer nach")
	case prod:
		return false, i18n.T("Schutz an: Änderungen fragen nach")
	case st.Elevated && g.level < lvlFull:
		return false, i18n.T("braucht sudo")
	case g.level < lvlFull && a.agentAdmin(s):
		return false, i18n.T("angemeldet als root/Administrator: Änderungen fragen nach")
	case g.level < lvlFull && safety.InlineCode(st.Command):
		return false, i18n.T("startet Code aus der Zeile (python -c, bash -c, eval …)")
	}
	return true, ""
}

// agentAdmin: läuft die Sitzung ohnehin mit vollen Rechten (root, Windows-
// Administrator)? Dann fehlt die sudo-Bremse; „Normal“ fragt deshalb bei
// allem nach, was etwas ändert. Windows zählt immer dazu: Fernzugang per
// WinRM oder SSH hat dort fast immer Administratorrechte.
func (a *App) agentAdmin(s *session) bool {
	return s.rootNow() || s.conn.Protocol == domain.ProtoWinRM || a.safetyKind(s) == "windows"
}

// agentPause hält den Agenten an. Ein laufender Befehl läuft weiter; sein
// Ergebnis bekommt das Modell, sobald es weitergeht.
func (a *App) agentPause(s *session, why string) {
	g := &s.goal
	if g.paused {
		return
	}
	g.paused, g.pauseWhy = true, why
	if why != "" {
		a.flash(i18n.T("Agent angehalten: ") + why)
	}
}

// agentAfterStep: ein Befehl oder eine Datei ist vorgeschlagen.
func (a *App) agentAfterStep(s *session) tea.Cmd {
	g := &s.goal
	if g.cur == nil {
		return nil
	}
	if !g.planOK {
		g.paused, g.pauseWhy = true, i18n.T("Plan prüfen, ⏎ startet den Agenten")
		return nil
	}
	if g.paused {
		return nil
	}
	if ok, why := a.agentGate(s, g.cur); !ok {
		g.paused, g.pauseWhy = true, why
		a.flash(i18n.T("Agent wartet auf dich: ") + why)
		return nil
	}
	return a.agentExec(s)
}

// agentExec führt den aktuellen Vorschlag aus (nach Freigabe oder automatisch).
func (a *App) agentExec(s *session) tea.Cmd {
	g := &s.goal
	if g.cur == nil {
		return nil
	}
	if s.state != stConnected || s.term == nil {
		g.paused, g.pauseWhy = true, i18n.T("keine Verbindung: erst neu verbinden")
		return nil
	}
	if g.cur.File != nil {
		return a.writeAgentFile(s)
	}
	return a.execProposal(s, g.cur.Command, false) // ändert er Dateien, erst sichern (goalux.go)
}

// agentNext holt den nächsten Schritt, wenn keine Grenze greift.
func (a *App) agentNext(s *session) tea.Cmd {
	g := &s.goal
	if g.paused {
		g.needStep = true
		return nil
	}
	switch {
	case len(g.steps) >= g.maxSteps:
		g.needStep = true
		a.agentPause(s, fmt.Sprintf(i18n.T("%d Schritte erreicht. ⏎ erlaubt %d weitere"), g.maxSteps, agentStepsMore))
		return nil
	case time.Now().After(g.deadline):
		g.needStep = true
		a.agentPause(s, fmt.Sprintf(i18n.T("Zeitgrenze erreicht. ⏎ erlaubt %d weitere Minuten"), int(agentTime.Minutes())))
		return nil
	case g.fails >= agentFailsInRow:
		g.needStep = true
		a.agentPause(s, fmt.Sprintf(i18n.T("%d Fehler in Folge. Lage prüfen, ⏎ versucht es weiter"), g.fails))
		return nil
	}
	g.needStep = false
	return a.requestStep(s)
}

// agentResume: ⏎ im angehaltenen Zustand.
func (a *App) agentResume(s *session) tea.Cmd {
	g := &s.goal
	g.planOK = true
	if len(g.steps) >= g.maxSteps {
		g.maxSteps = min(maxGoalSteps, g.maxSteps+agentStepsMore)
	}
	if time.Now().After(g.deadline) {
		g.deadline = time.Now().Add(agentTime)
	}
	if g.fails >= agentFailsInRow {
		g.fails = 0
	}
	g.paused, g.pauseWhy = false, ""
	switch {
	case g.cur != nil && g.cur.Question == "":
		return a.agentExec(s) // dein ⏎ ist die Freigabe für genau diesen Schritt
	case g.waiting || g.busy || g.writing:
		return nil // läuft noch; goalTick macht weiter
	case g.needStep || g.err != "":
		g.err = ""
		return a.agentNext(s)
	}
	return nil
}

// agentTick (aus goalTick): Zeitlimit pro Befehl und Passwortabfragen.
func (a *App) agentTick(s *session) {
	g := &s.goal
	if !g.agentRunning() || !g.waiting || s.term == nil {
		return
	}
	if time.Since(s.lastOutput) > time.Second && agent.LooksLikeSecretPrompt(s.term.CursorLine()) {
		a.agentPause(s, i18n.T("Passwortabfrage: bitte selbst im Terminal eingeben"))
		return
	}
	if time.Since(g.waitSince) > agentCmdTimeout {
		a.agentPause(s, fmt.Sprintf(i18n.T("Befehl läuft seit über %d Minuten (Strg+C im Terminal bricht ihn ab)"), int(agentCmdTimeout.Minutes())))
	}
}

// agentStepFinished (aus goalTick): der letzte Befehl ist fertig.
func (a *App) agentStepFinished(s *session) tea.Cmd {
	g := &s.goal
	if n := len(g.steps); n > 0 && g.steps[n-1].ExitHint != "" {
		g.fails++
	} else {
		g.fails = 0
	}
	return a.agentNext(s)
}

// agentTyped: der Mensch tippt ins Terminal => sofort anhalten.
func (a *App) agentTyped(s *session) {
	if s.goal.agentRunning() {
		a.agentPause(s, i18n.T("du hast ins Terminal getippt"))
	}
}

// ---------------------------------------------------------------- Dateien

type agentFileMsg struct {
	sid, seq int
	file     agent.FileAction
	err      error
}

type agentCleanMsg struct {
	sid int
	dir string
	err error
}

// writeAgentFile legt die vorgeschlagene Datei per SFTP an (Rechte 0700).
func (a *App) writeAgentFile(s *session) tea.Cmd {
	g := &s.goal
	if g.cur == nil || g.cur.File == nil {
		return nil
	}
	f := *g.cur.File
	g.cur, g.armed = nil, false
	if !a.agentFilesOK(s) {
		g.steps = append(g.steps, agent.DoneStep{Command: i18n.T("Datei ") + f.Name, File: f.Name, ExitHint: i18n.T("Dateien gehen hier nicht")})
		return a.goalNext(s)
	}
	if len(g.files) >= agent.MaxFiles && g.fileIndex(f.Name) < 0 {
		if !g.auto {
			g.err = fmt.Sprintf(i18n.T("höchstens %d Dateien pro Lauf"), agent.MaxFiles)
			return nil
		}
		a.agentPause(s, fmt.Sprintf(i18n.T("höchstens %d Dateien pro Lauf"), agent.MaxFiles))
		return nil
	}
	g.writing = true
	sid, seq, run := s.id, g.seq, g.runID
	local, cl := s.agentFSOf()
	return func() tea.Msg {
		fs, done, err := agentFS(local, cl)
		if err != nil {
			return agentFileMsg{sid, seq, f, err}
		}
		defer done()
		dir := fs.Join(fs.Home(), agentDirBase, run)
		if err := fs.Mkdir(dir); err != nil {
			return agentFileMsg{sid, seq, f, err}
		}
		w, err := fs.Create(fs.Join(dir, f.Name), 0o700)
		if err != nil {
			return agentFileMsg{sid, seq, f, err}
		}
		if _, err := io.WriteString(w, f.Content); err != nil {
			_ = w.Close()
			return agentFileMsg{sid, seq, f, err}
		}
		return agentFileMsg{sid, seq, f, w.Close()}
	}
}

func (g *goalState) fileIndex(name string) int {
	for i, f := range g.files {
		if f.Name == name {
			return i
		}
	}
	return -1
}

func lineCount(s string) int { return strings.Count(strings.TrimRight(s, "\n"), "\n") + 1 }

func (a *App) agentFileDone(m agentFileMsg) tea.Cmd {
	s := a.byID(m.sid)
	if s == nil || m.seq != s.goal.seq {
		return nil
	}
	g := &s.goal
	g.writing = false
	path := g.agentDir() + "/" + m.file.Name
	if m.err != nil {
		g.steps = append(g.steps, agent.DoneStep{Command: i18n.T("Datei ") + path, File: m.file.Name, ExitHint: m.err.Error()})
		if !g.auto { // Ziel-Modus: Fehler zeigen, r fragt neu
			g.err = i18n.T("Datei konnte nicht angelegt werden: ") + m.err.Error()
			return nil
		}
		g.fails++
		a.agentPause(s, i18n.T("Datei konnte nicht angelegt werden: ")+m.err.Error())
		g.needStep = true
		return nil
	}
	desc := fmt.Sprintf(i18n.T("Datei %s angelegt (%d Zeilen)"), path, lineCount(m.file.Content))
	g.steps = append(g.steps, agent.DoneStep{Command: desc, File: m.file.Name})
	if i := g.fileIndex(m.file.Name); i >= 0 {
		g.files[i] = m.file
	} else {
		g.files = append(g.files, m.file)
	}
	if s.log != nil {
		s.log.Add(sessionlog.Entry{At: time.Now(), Command: "# " + desc, Output: m.file.Content, Agent: true})
		a.saveLog(s)
	}
	g.fails = 0
	return a.goalNext(s)
}

// goalNext: nächster Schritt, im Agent-Modus mit dessen Grenzen.
func (a *App) goalNext(s *session) tea.Cmd {
	if s.goal.auto {
		return a.agentNext(s)
	}
	return a.requestStep(s)
}

// agentCleanup löscht den Arbeitsordner des Laufs auf dem Host.
func (a *App) agentCleanup(s *session) tea.Cmd {
	g := &s.goal
	if (s.client == nil && !s.isLocal()) || len(g.files) == 0 || g.cleaned {
		return nil
	}
	sid, run, show := s.id, g.runID, g.agentDir()
	local, cl := s.agentFSOf()
	return func() tea.Msg {
		fs, done, err := agentFS(local, cl)
		if err != nil {
			return agentCleanMsg{sid, show, err}
		}
		defer done()
		return agentCleanMsg{sid, show, fs.RemoveAll(fs.Join(fs.Home(), agentDirBase, run))}
	}
}

func (a *App) agentCleanDone(m agentCleanMsg) {
	s := a.byID(m.sid)
	if s == nil {
		return
	}
	if m.err != nil {
		a.flashErr(i18n.T("Aufräumen: ") + m.err.Error())
		return
	}
	s.goal.cleaned = true
	a.flash(i18n.T("Gelöscht: ") + m.dir)
}

// agentFinished: Ziel erreicht => Laufbericht ins Sitzungsprotokoll.
func (a *App) agentFinished(s *session) {
	g := &s.goal
	if s.log == nil {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, i18n.T("Stufe: %s · %d Schritte · %s\n"), levelName(g.level), len(g.steps), time.Since(g.started).Round(time.Second))
	b.WriteString(g.doneReason + "\n")
	for _, f := range g.files {
		b.WriteString(i18n.T("Datei: ") + g.agentDir() + "/" + f.Name + "\n")
	}
	s.log.Add(sessionlog.Entry{At: time.Now(), Command: i18n.T("# Agent: Ziel erreicht: ") + g.goal, Output: b.String(), Agent: true})
	a.saveLog(s)
}

// agentInstruct: dem Agenten eine Anweisung geben (hält ihn dabei an).
func (a *App) agentInstruct(s *session) tea.Cmd {
	g := &s.goal
	a.agentPause(s, "")
	a.modal = inputModal(i18n.T("Agent anweisen"), i18n.T("Was soll der Agent anders machen? Er plant danach neu."), "", func(x string) tea.Cmd {
		x = strings.TrimSpace(x)
		if x == "" {
			return nil
		}
		g.answers = append(g.answers, i18n.T("Anweisung: ")+x)
		g.cur, g.armed, g.paused, g.pauseWhy, g.planOK = nil, false, false, "", true
		if g.waiting || g.busy || g.writing {
			return nil
		}
		return a.agentNext(s)
	})
	return nil
}

// agentView zeigt eine Datei (aktueller Vorschlag oder zuletzt angelegte).
func (a *App) agentView(s *session) {
	g := &s.goal
	var f *agent.FileAction
	if g.cur != nil && g.cur.File != nil {
		f = g.cur.File
	} else if n := len(g.files); n > 0 {
		f = &g.files[n-1]
	}
	if f == nil {
		return
	}
	var lines []string
	for i, l := range strings.Split(strings.TrimRight(f.Content, "\n"), "\n") {
		lines = append(lines, fmt.Sprintf("%3d  %s", i+1, l))
	}
	a.modal = textModal(g.agentDir()+"/"+f.Name, lines)
}

// cycleMode: Tab im Startfeld: Schritt für Schritt → Agent (3 Stufen) → …
func (g *goalState) cycleMode() {
	switch {
	case g.planFirst:
		g.planFirst, g.auto = false, false
	case !g.auto:
		g.auto, g.level = true, lvlCareful
	case g.level < lvlFull:
		g.level++
	default:
		g.planFirst, g.auto, g.level = true, true, lvlNormal // Plan läuft danach als Agent (Normal)
	}
}

// agentKey: Tasten im Panel, solange ein Agent-Lauf aktiv ist. ok=false =>
// die normale Ziel-Modus-Behandlung übernimmt.
func (a *App) agentKey(s *session, key string) (tea.Cmd, bool) {
	g := &s.goal
	if !g.auto || g.planning {
		return nil, false
	}
	if g.done && !g.active {
		switch key {
		case "s":
			a.saveRunbook(s)
			return nil, true
		case "r":
			return a.agentCleanup(s), true
		case "v":
			a.agentView(s)
			return nil, true
		case "f":
			return a.startFollowGoal(s), true
		case "u":
			return a.startUndo(s), true
		}
		return nil, false
	}
	if !g.active || g.editing || (g.cur != nil && g.cur.Question != "") {
		return nil, false
	}
	switch key {
	case "?":
		return a.agentInstruct(s), true
	case "v":
		a.agentView(s)
		return nil, true
	case "x", "r":
		return nil, false
	}
	if !g.paused {
		return nil, true // läuft: nur Esc, x, ?, v
	}
	switch key {
	case "enter":
		if g.cur != nil && len(g.cur.Danger) > 0 && !g.armed {
			g.armed = true
			return nil, true
		}
		return a.agentResume(s), true
	case "j", "y":
		if g.cur != nil {
			return a.agentResume(s), true
		}
	case "S":
		if g.cur != nil {
			return a.goalSkipWhy(s), true
		}
	case "s", "n":
		if g.cur != nil {
			g.steps = append(g.steps, agent.DoneStep{Command: g.cur.CommandOrFile(), Skipped: true})
			g.cur, g.armed = nil, false
			g.paused, g.pauseWhy, g.planOK = false, "", true
			return a.agentNext(s), true
		}
	case "e":
		if g.cur != nil && g.cur.File != nil {
			return nil, true // Dateien: ansehen (v) oder anweisen (?), nicht in einer Zeile bearbeiten
		}
		return nil, false
	}
	return nil, true
}
