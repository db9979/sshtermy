package ui

// Ziel-Modus im Einsatz (2026-11-12, Vorschläge 1–8 und 10):
//
//   - ? gibt der KI mitten im Ziel eine Anweisung (wie im Agent-Modus),
//     ⇧S überspringt einen Schritt mit Grund.
//   - Wartet ein Befehl auf eine Eingabe ([Y/n], Pager, --More--, „Press any
//     key“), zeigt das Panel Knöpfe dafür; der Agent hält dort an.
//   - Wartet das Ziel auf dich und der Tab ist nicht zu sehen, gibt es eine
//     Einblendung, im Hintergrund eine Desktop-Meldung; der Tab zeigt den
//     Zustand des Ziels und die Schrittzahl.
//   - f startet nach „Ziel erreicht“ ein Folgeziel, das die Schritte davor kennt.
//   - Vor Schritten, die Dateien ändern, sichert sshTermy sie per SFTP;
//     u holt am Ende alles zurück.
//   - Zielverlauf je Verbindung im Eingabebildschirm (goalhistory.go).
//   - Reißt die Verbindung ab, hält das Ziel an; nach dem Neuverbinden setzt
//     ⏎ es fort.
//   - Was du während des Ziels selbst tippst, bekommt die KI als Schritt mit
//     Ausgabe.

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sessionlog"
	"github.com/db9979/sshtermy/internal/sshx"
)

// ---------------------------------------------------------------- 1 Anweisung

// goalInstruct: ? im Ziel-Modus (Schritt für Schritt). Die Anweisung gilt ab
// dem nächsten Vorschlag; ein offener Vorschlag wird verworfen.
func (a *App) goalInstruct(s *session) tea.Cmd {
	g := &s.goal
	a.modal = inputModal(i18n.T("KI anweisen"), i18n.T("Was soll die KI anders machen oder beachten? Sie schlägt danach neu vor."), "", func(x string) tea.Cmd {
		x = strings.TrimSpace(x)
		if x == "" || !g.active {
			return nil
		}
		g.answers = append(g.answers, agent.Instruction(x))
		if g.waiting || g.writing || g.backingUp {
			a.flash(i18n.T("Anweisung gilt ab dem nächsten Schritt"))
			return nil
		}
		g.cur, g.armed, g.editing = nil, false, false
		return a.goalNext(s)
	})
	a.modal.in.Placeholder = i18n.T("z. B. nimm docker compose statt docker-compose")
	return nil
}

// goalSkipWhy: ⇧S überspringt den Vorschlag und sagt der KI, warum.
func (a *App) goalSkipWhy(s *session) tea.Cmd {
	g := &s.goal
	if g.cur == nil {
		return nil
	}
	cur := g.cur
	a.modal = inputModal(i18n.T("Überspringen mit Grund"), i18n.T("Warum soll dieser Schritt nicht laufen? Die KI richtet sich danach."), "", func(x string) tea.Cmd {
		if g.cur != cur {
			return nil
		}
		g.steps = append(g.steps, agent.DoneStep{Command: cur.CommandOrFile(), Skipped: true, Note: strings.TrimSpace(x)})
		g.cur, g.armed = nil, false
		if g.auto {
			g.paused, g.pauseWhy, g.planOK = false, "", true
		}
		return a.goalNext(s)
	})
	a.modal.in.Placeholder = i18n.T("z. B. der Dienst läuft schon, nicht neu starten")
	return nil
}

// ---------------------------------------------------------------- 2 Eingabe erwartet

// inputWaitTick (aus goalTick): steht ein laufender Schritt an einer Rückfrage?
func (a *App) inputWaitTick(s *session, quiet bool) {
	g := &s.goal
	if !g.waiting || s.term == nil {
		g.inputWait = ""
		return
	}
	if !quiet {
		g.inputWait = ""
		return
	}
	if s.term.AtIdlePrompt() {
		return
	}
	line := s.term.CursorLine()
	if agent.LooksLikeSecretPrompt(line) {
		return // Passwort: agentTick hält an, der Mensch tippt selbst
	}
	k := agent.InputWait(line, s.term.AltScreen())
	if k == agent.InputNone || k == g.inputWait {
		return
	}
	g.inputWait, g.inputLine = k, strings.TrimSpace(line)
	if g.auto {
		a.agentPause(s, i18n.T("Befehl wartet auf eine Eingabe"))
	}
}

// inputWaitKey: Knöpfe j/n/q/Leertaste/⏎/Strg+C, solange ein Befehl fragt.
func (a *App) inputWaitKey(s *session, key string) (tea.Cmd, bool) {
	g := &s.goal
	if key == "y" {
		key = "j"
	}
	for _, b := range agent.InputKeys(g.inputWait) {
		if b.Key == key {
			a.sendInputAnswer(s, b.Send)
			return nil, true
		}
	}
	switch key {
	case "esc", "x", "?", "ctrl+s":
		return nil, false
	}
	return nil, true // andere Tasten nicht als Ziel-Tasten deuten
}

func (a *App) sendInputAnswer(s *session, send string) {
	g := &s.goal
	if s.term == nil {
		return
	}
	s.term.SendText(send)
	g.inputWait, g.inputLine = "", ""
	g.waitSince = time.Now() // neue Ausgabe abwarten
	if g.auto && g.paused && g.pauseWhy == i18n.T("Befehl wartet auf eine Eingabe") {
		g.paused, g.pauseWhy = false, ""
	}
}

func inputKeyLabel(k string) string {
	switch k {
	case "space":
		return i18n.T("Leertaste")
	case "enter":
		return "⏎"
	case "ctrl+c":
		return i18n.T("Strg+C")
	}
	return k
}

func (a *App) inputWaitHints(s *session) [][2]string {
	var out [][2]string
	for _, b := range agent.InputKeys(s.goal.inputWait) {
		out = append(out, [2]string{inputKeyLabel(b.Key), i18n.T(b.Label)})
	}
	return append(out, [2]string{"Esc", i18n.T("selbst im Terminal")})
}

func (a *App) viewInputWait(b *strings.Builder, s *session, w int) {
	st := a.s
	g := &s.goal
	b.WriteString(st.Warn.Render(i18n.T("▸ Der Befehl wartet auf eine Eingabe")) + "\n")
	if g.inputLine != "" {
		b.WriteString(st.Panel.Width(w).Render(wrap(st.Text.Render(g.inputLine), w-4)) + "\n")
	}
	var row []string
	for _, k := range agent.InputKeys(g.inputWait) {
		send := k.Send
		lbl := st.Accent.Render("["+inputKeyLabel(k.Key)+"]") + " " + st.Text.Render(i18n.T(k.Label))
		row = append(row, a.z.click(lbl, func(tea.Mouse, bool) tea.Cmd {
			a.sendInputAnswer(s, send)
			return nil
		}))
	}
	b.WriteString(strings.Join(row, "   ") + "\n")
	b.WriteString(st.Muted.Render(i18n.T("Oder Esc und selbst im Terminal antworten.")) + "\n")
}

// ---------------------------------------------------------------- 3 + 10 Wartet auf dich

// goalAttention: wartet das Ziel gerade auf dich? key ändert sich mit jedem
// neuen Anlass (für die Meldung), why ist der Text dazu.
func (a *App) goalAttention(s *session) (key, why string) {
	g := &s.goal
	switch {
	case g.inputWait != "" && g.waiting:
		return "in" + string(g.inputWait) + g.inputLine, i18n.T("Befehl wartet auf eine Eingabe")
	case g.lost && g.active:
		if s.state == stConnected {
			return "lost", i18n.T("Verbindung wieder da, ⏎ setzt das Ziel fort")
		}
		return "", ""
	case g.done && !g.active && !g.doneSeen:
		return fmt.Sprintf("done%d", g.seq), i18n.T("Ziel erreicht")
	case !g.active || g.busy || g.waiting || g.writing || g.backingUp || g.manualPending:
		return "", ""
	case g.undoing:
		return "undo", i18n.T("Zurückholen wartet auf ⏎")
	case g.planReview():
		return "plan", i18n.T("Plan wartet auf Freigabe")
	case g.cur != nil && g.cur.Question != "":
		return fmt.Sprintf("q%d", g.seq), i18n.T("Rückfrage der KI")
	case g.auto && g.paused:
		return "p" + g.pauseWhy + strconv.Itoa(g.seq), i18n.T("Agent angehalten: ") + g.pauseWhy
	case g.auto:
		return "", ""
	case g.err != "":
		return "e" + g.err, i18n.T("Fehler im Ziel-Modus")
	case g.cur != nil:
		return fmt.Sprintf("s%d", g.seq), i18n.T("nächster Schritt wartet auf deine Freigabe")
	}
	return "", ""
}

// goalNotifyTick (aus goalTick): meldet einen neuen Anlass, wenn der Tab
// nicht zu sehen ist oder sshTermy im Hintergrund läuft.
func (a *App) goalNotifyTick(s *session) {
	g := &s.goal
	key, why := a.goalAttention(s)
	if key == g.attnKey {
		return
	}
	g.attnKey = key
	if key == "" || !a.notifyOn() {
		return
	}
	if a.sessionVisible(s) && !a.blurred {
		return
	}
	if !a.sessionVisible(s) {
		a.flash(ic("ai") + " " + s.tabName() + ": " + why)
		s.flashTab(a.s.T.Warn)
	}
	if (a.blurred || a.lock != nil) && a.desktopNotifyOn() && time.Since(g.attnDesk) > 20*time.Second {
		g.attnDesk = time.Now()
		body := why
		if a.lock != nil {
			body = i18n.T("Ziel-Modus wartet auf dich") // gesperrt: nichts verraten
		}
		a.sendDesktop("sshTermy · "+s.tabName(), body)
	}
}

// goalGlyph: Zeichen im Tab für ein Ziel ("" = keins). ◎ Ziel-Modus,
// ◆ Agent, ◇ Plan; violett = läuft, gelb = wartet auf dich, grün = fertig
// (bis du den Tab ansiehst).
func (a *App) goalGlyph(x *session) string {
	st := a.s
	g := &x.goal
	glyph := "◎"
	switch {
	case g.planning || g.planReview():
		glyph = "◇"
	case g.auto:
		glyph = "◆"
	}
	switch {
	case g.active || g.lost:
		if key, _ := a.goalAttention(x); key != "" {
			return st.Warn.Render(glyph)
		}
		return st.AI.Render(glyph)
	case g.done && !g.doneSeen:
		return st.OK.Render(glyph)
	}
	return ""
}

// goalTabCount: Schrittzahl hinter dem Tab-Namen, solange ein Ziel läuft.
func goalTabCount(x *session) string {
	if g := &x.goal; g.active && len(g.steps) > 0 {
		return " ·" + strconv.Itoa(len(g.steps))
	}
	return ""
}

// goalTip: Zeile für den Tooltip des Tabs.
func (a *App) goalTip(x *session) string {
	g := &x.goal
	if !g.active && !(g.done && !g.doneSeen) {
		return ""
	}
	state := i18n.T("läuft")
	if g.done && !g.active {
		state = i18n.T("erreicht")
	} else if _, why := a.goalAttention(x); why != "" {
		state = why
	}
	return fmt.Sprintf(i18n.T("Ziel: %s · %s · %s"), trunc(g.goal, 30), plural(len(g.steps), i18n.T("Schritt"), i18n.T("Schritte")), state)
}

// ---------------------------------------------------------------- 4 Folgeziel

type goalCarry struct {
	prior     []agent.DoneStep
	notes     []string
	backups   []fileBackup
	priorGoal string
}

// startFollowGoal: f nach „Ziel erreicht“.
func (a *App) startFollowGoal(s *session) tea.Cmd {
	g := &s.goal
	if !g.done || g.goal == "" {
		return nil
	}
	prevGoal, prevReason := g.goal, g.doneReason
	prior := append(append([]agent.DoneStep(nil), g.prior...), g.steps...)
	var notes []string
	for _, x := range g.answers {
		if agent.IsPriorGoal(x) {
			notes = append(notes, x)
		}
	}
	notes = append(notes, agent.PriorGoal(prevGoal, prevReason, len(prior)))
	backups := append([]fileBackup(nil), g.backups...)
	a.modal = inputModal(i18n.T("Folgeziel"), i18n.T("Was soll als Nächstes passieren? Die KI kennt das Ziel davor und seine Schritte."), "", func(x string) tea.Cmd {
		x = strings.TrimSpace(x)
		if x == "" {
			return nil
		}
		return a.startGoalCarry(s, x, nil, &goalCarry{prior: prior, notes: notes, backups: backups, priorGoal: prevGoal})
	})
	a.modal.in.Placeholder = i18n.T("z. B. jetzt noch HTTPS mit Let's Encrypt einrichten")
	return nil
}

// ---------------------------------------------------------------- 5 Sicherungen

// fileBackup: Zustand einer Datei, bevor ein Schritt des Ziels sie änderte.
type fileBackup struct {
	Path    string
	Existed bool
	Content []byte
	Sudo    bool
	Skip    string // warum es keine Sicherung gibt ("" = gesichert)
}

func (f fileBackup) restorable() bool { return f.Skip == "" }

const (
	maxBackups    = 20
	maxBackupSize = 1 << 20
	backupTimeout = 10 * time.Second
)

type goalBackupMsg struct {
	sid, seq int
	cmd      string
	insert   bool
	saved    []fileBackup
}

type goalUndoMsg struct {
	sid, seq int
	list     []fileBackup
	err      error
}

func (g *goalState) hasBackup(p string) bool {
	for _, b := range g.backups {
		if b.Path == p {
			return true
		}
	}
	return false
}

// execProposal führt einen Vorschlag aus; ändert er Dateien, werden sie
// vorher gesichert (nur über SFTP auf Unix, wie die Dateien des Agenten).
func (a *App) execProposal(s *session, cmd string, insert bool) tea.Cmd {
	g := &s.goal
	var need []agent.FileChange
	if a.agentFilesOK(s) {
		for _, f := range agent.ChangedFiles(cmd) {
			if g.hasBackup(f.Path) || strings.HasPrefix(f.Path, "~/"+agentDirBase+"/") || len(g.backups)+len(need) >= maxBackups {
				continue
			}
			need = append(need, f)
		}
	}
	if len(need) == 0 {
		a.runProposal(s, cmd, insert)
		return nil
	}
	g.backingUp = true
	sid, seq := s.id, g.seq
	local, cl := s.agentFSOf()
	return func() tea.Msg {
		return goalBackupMsg{sid, seq, cmd, insert, readBackups(local, cl, need)}
	}
}

// readBackups liest die Dateien; was nicht klappt, steht mit Grund in Skip.
func readBackups(local bool, cl *sshx.Client, need []agent.FileChange) []fileBackup {
	out := make([]fileBackup, len(need))
	for i, f := range need {
		out[i] = fileBackup{Path: f.Path, Sudo: f.Sudo, Skip: i18n.T("keine Antwort")}
	}
	done := make(chan []fileBackup, 1)
	go func() {
		res := make([]fileBackup, len(need))
		copy(res, out)
		fsys, closeFn, err := agentFS(local, cl)
		if err != nil {
			for i := range res {
				res[i].Skip = err.Error()
			}
			done <- res
			return
		}
		defer closeFn()
		for i, f := range need {
			res[i] = readBackup(fsys, f)
		}
		done <- res
	}()
	select {
	case r := <-done:
		return r
	case <-time.After(backupTimeout):
		return out
	}
}

func readBackup(fsys sshx.FS, f agent.FileChange) fileBackup {
	b := fileBackup{Path: f.Path, Sudo: f.Sudo}
	p := f.Path
	if strings.HasPrefix(p, "~/") {
		p = fsys.Join(fsys.Home(), p[2:])
	}
	e, err := fsys.Stat(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return b // gab es noch nicht: Zurückholen löscht sie
	case err != nil:
		b.Skip = i18n.T("nicht lesbar")
		return b
	case e.IsDir:
		b.Skip = i18n.T("ist ein Ordner")
		return b
	case e.Size > maxBackupSize:
		b.Skip = i18n.T("zu groß")
		return b
	}
	r, err := fsys.Open(p)
	if err != nil {
		b.Skip = i18n.T("nicht lesbar")
		return b
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, maxBackupSize+1))
	if err != nil || len(data) > maxBackupSize {
		b.Skip = i18n.T("nicht lesbar")
		return b
	}
	b.Existed, b.Content = true, data
	return b
}

func (a *App) goalBackupDone(m goalBackupMsg) tea.Cmd {
	s := a.byID(m.sid)
	if s == nil || m.seq != s.goal.seq {
		return nil
	}
	g := &s.goal
	g.backingUp = false
	for _, b := range m.saved {
		if !g.hasBackup(b.Path) {
			g.backups = append(g.backups, b)
		}
	}
	if !g.active || s.state != stConnected {
		return nil
	}
	a.runProposal(s, m.cmd, m.insert)
	return nil
}

// startUndo: u legt die Sicherungen in den Arbeitsordner und schlägt den
// Befehl vor, der sie zurückkopiert (bzw. neu angelegte Dateien löscht).
func (a *App) startUndo(s *session) tea.Cmd {
	g := &s.goal
	var list []fileBackup
	for _, b := range g.backups {
		if b.restorable() {
			list = append(list, b)
		}
	}
	if len(list) == 0 {
		a.flash(i18n.T("Nichts zum Zurückholen"))
		return nil
	}
	if s.state != stConnected || !a.agentFilesOK(s) {
		a.flashErr(i18n.T("Zurückholen braucht eine Verbindung mit SFTP"))
		return nil
	}
	if g.runID == "" {
		g.runID = newRunID()
	}
	g.backingUp = true
	g.seq++
	sid, seq, run := s.id, g.seq, g.runID
	local, cl := s.agentFSOf()
	return func() tea.Msg {
		fsys, closeFn, err := agentFS(local, cl)
		if err != nil {
			return goalUndoMsg{sid, seq, list, err}
		}
		defer closeFn()
		dir := fsys.Join(fsys.Home(), agentDirBase, run, "undo")
		if err := fsys.Mkdir(dir); err != nil {
			return goalUndoMsg{sid, seq, list, err}
		}
		_ = fsys.Chmod(fsys.Join(fsys.Home(), agentDirBase, run), 0o700) // Sicherungen können Geheimnisse enthalten
		for i, b := range list {
			if !b.Existed {
				continue
			}
			w, err := fsys.Create(fsys.Join(dir, strconv.Itoa(i)), 0o600)
			if err != nil {
				return goalUndoMsg{sid, seq, list, err}
			}
			if _, err := w.Write(b.Content); err != nil {
				_ = w.Close()
				return goalUndoMsg{sid, seq, list, err}
			}
			if err := w.Close(); err != nil {
				return goalUndoMsg{sid, seq, list, err}
			}
		}
		return goalUndoMsg{sid, seq, list, nil}
	}
}

// undoCommand: eine Zeile, die alle Dateien zurückholt.
func undoCommand(run string, list []fileBackup) string {
	var parts []string
	for i, b := range list {
		sudo := ""
		if b.Sudo {
			sudo = "sudo "
		}
		target := shQuote(b.Path)
		if strings.HasPrefix(b.Path, "~/") {
			target = "~/" + shQuote(b.Path[2:]) // ~ bleibt außerhalb der Anführungszeichen
		}
		if b.Existed {
			parts = append(parts, fmt.Sprintf("%scp ~/%s/%s/undo/%d %s", sudo, agentDirBase, run, i, target))
		} else {
			parts = append(parts, sudo+"rm -f "+target)
		}
	}
	return strings.Join(parts, " && ")
}

func (a *App) goalUndoDone(m goalUndoMsg) tea.Cmd {
	s := a.byID(m.sid)
	if s == nil || m.seq != s.goal.seq {
		return nil
	}
	g := &s.goal
	g.backingUp = false
	if m.err != nil {
		a.flashErr(i18n.T("Zurückholen: ") + m.err.Error())
		return nil
	}
	cmd := undoCommand(g.runID, m.list)
	g.undoing, g.undoBack = true, g.done
	g.active, g.done, g.paused, g.pauseWhy = true, false, false, ""
	st := agent.ParseStepCommand(cmd, a.safetyKind(s))
	st.Command = cmd
	st.Reason = fmt.Sprintf(i18n.T("stellt die geänderten Dateien (%d) wieder so her, wie sie vor dem Ziel waren"), len(m.list))
	g.cur = &agent.Step{Suggestion: st}
	g.armed = false
	s.panelFocus = true
	return nil
}

// undoKey: Tasten, solange der Zurückholen-Befehl wartet.
func (a *App) undoKey(s *session, key string) tea.Cmd {
	g := &s.goal
	switch key {
	case "enter":
		if g.cur == nil || g.waiting {
			return nil
		}
		if s.state != stConnected {
			a.flashErr(i18n.T("keine Verbindung: erst neu verbinden"))
			return nil
		}
		a.runProposal(s, g.cur.Command, false)
	case "x":
		if g.waiting {
			return nil
		}
		g.undoing, g.cur, g.active = false, nil, false
		g.done = g.undoBack
	}
	return nil
}

// undoFinished (aus goalTick): der Zurückholen-Befehl ist durch.
func (a *App) undoFinished(s *session) {
	g := &s.goal
	g.undoing, g.active, g.done, g.cur = false, false, true, nil
	failed := false
	if n := len(g.steps); n > 0 {
		last := g.steps[n-1]
		failed = last.ExitHint != "" || (last.Exit != nil && *last.Exit != 0)
	}
	if failed {
		g.doneReason = i18n.T("Zurückholen hat nicht geklappt, siehe Terminal. Die Sicherungen bleiben, u versucht es erneut.")
		return
	}
	var paths []string
	keep := g.backups[:0]
	for _, b := range g.backups {
		if b.restorable() {
			paths = append(paths, b.Path)
		} else {
			keep = append(keep, b)
		}
	}
	g.backups = keep
	g.doneReason = i18n.T("Änderungen zurückgenommen: ") + strings.Join(paths, ", ")
	a.flashOK(i18n.T("Änderungen des Ziels zurückgenommen"))
}

// viewBackups: geänderte Dateien unter „Ziel erreicht“ bzw. im Eingabebild.
func (a *App) viewBackups(b *strings.Builder, s *session, w int, key string) {
	st := a.s
	g := &s.goal
	if len(g.backups) == 0 {
		return
	}
	n := 0
	for _, x := range g.backups {
		if x.restorable() {
			n++
		}
	}
	head := st.Warn.Render("← " + plural(len(g.backups), i18n.T("Datei geändert"), i18n.T("Dateien geändert")))
	if n > 0 {
		head += st.Muted.Render(" · ") + a.z.click(st.Accent.Render(key)+" "+st.Text.Render(i18n.T("zurückholen")), func(tea.Mouse, bool) tea.Cmd {
			s.panelFocus = true
			return a.startUndo(s)
		})
	}
	if strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	} else {
		b.WriteString("\n\n")
	}
	b.WriteString(head + "\n")
	for i, x := range g.backups {
		if i >= 5 {
			b.WriteString(st.Muted.Render(fmt.Sprintf(i18n.T("  … %d weitere"), len(g.backups)-5)) + "\n")
			break
		}
		meta := ""
		switch {
		case x.Skip != "":
			meta = i18n.T("ohne Sicherung: ") + x.Skip
		case !x.Existed:
			meta = i18n.T("neu angelegt")
		}
		line := "  " + trunc(x.Path, max(8, w-lipgloss.Width(meta)-4))
		if meta != "" {
			line = st.Text.Render(line) + "  " + st.Muted.Render(meta)
		} else {
			line = st.Text.Render(line)
		}
		b.WriteString(line + "\n")
	}
}

// ---------------------------------------------------------------- 7 Verbindung weg

// connLost (aus suspend): das Ziel hält an, die laufende Anfrage verfällt.
func (g *goalState) connLost() {
	if !g.active {
		return
	}
	g.seq++ // späte Antworten der KI und Sicherungen verfallen
	g.busy, g.waiting, g.writing, g.backingUp, g.manualPending = false, false, false, false, false
	g.inputWait, g.inputLine = "", ""
	g.lost = true
	if g.auto {
		g.paused, g.pauseWhy = true, i18n.T("Verbindung unterbrochen")
		g.needStep = g.cur == nil
	}
}

// resumeLost: ⏎ nach dem Neuverbinden.
func (a *App) resumeLost(s *session) tea.Cmd {
	g := &s.goal
	if s.state != stConnected {
		a.flash(i18n.T("Noch nicht wieder verbunden"))
		return nil
	}
	g.lost = false
	if g.undoing {
		return nil // Zurückholen-Befehl steht noch da, ⏎ führt ihn aus
	}
	if g.cur != nil { // der Vorschlag steht noch; nichts lief
		if g.auto {
			g.pauseWhy = i18n.T("Verbindung wieder da, ⏎ führt den Schritt aus")
		}
		return nil
	}
	g.answers = append(g.answers, agent.Notice("Die Verbindung war unterbrochen und ist wieder da (neue Shell). Der letzte Befehl ist eventuell nicht fertig geworden; prüfe zuerst den Stand, bevor du weitermachst."))
	if g.auto {
		g.paused, g.pauseWhy, g.needStep = false, "", false
		return a.agentNext(s)
	}
	return a.requestStep(s)
}

func (a *App) viewLost(b *strings.Builder, s *session, w int) {
	st := a.s
	if s.state == stConnected {
		b.WriteString(st.Warn.Render(i18n.T("▲ Die Verbindung war unterbrochen")) + "\n" +
			wrap(st.Text.Render(i18n.T("Das Ziel ist angehalten. Die KI prüft zuerst den Stand, der letzte Befehl ist eventuell nicht fertig geworden.")), w) + "\n\n" +
			a.hintsIn(s, st.Muted, [2]string{"⏎", i18n.T("Ziel fortsetzen")}, [2]string{"x", i18n.T("Ziel beenden")}))
		return
	}
	b.WriteString(st.Warn.Render(i18n.T("▲ Verbindung unterbrochen")) + "\n" +
		wrap(st.Muted.Render(i18n.T("Das Ziel ist angehalten und wartet auf die neue Verbindung.")), w))
}

// ---------------------------------------------------------------- 8 Eigene Befehle

// noteManualStep (aus finishCommand): Befehle, die du während des Ziels
// selbst ausführst, bekommt die KI als Schritt mit Ausgabe.
func (a *App) noteManualStep(s *session, r *cmdRun, out string, f agent.Failure, failed bool) bool {
	g := &s.goal
	if !g.active || g.undoing || r.sent || r.agent || r.private || g.lost {
		return false
	}
	if n := len(g.steps); n > 0 && !g.steps[n-1].Manual && strings.HasPrefix(r.cmd, g.steps[n-1].Command) {
		return false // der zuletzt vorgeschlagene (z. B. mit i eingefügt)
	}
	d := agent.DoneStep{Command: r.cmd, Manual: true, Output: sessionlog.Clip(out), Exit: f.Exit}
	if failed && !r.interrupted {
		d.ExitHint = orDefault(f.Reason, i18n.T("Fehlermeldung in der Ausgabe"))
	}
	g.steps = append(g.steps, d)
	if g.cur != nil && !g.auto {
		g.manualNew = true
	}
	return true
}

// goalUXKeys: Fußzeile für die Zustände aus dieser Datei (nil = sonst).
func (a *App) goalUXKeys(s *session) [][2]string {
	g := &s.goal
	switch {
	case g.lost && g.active:
		if s.state == stConnected {
			return [][2]string{{"⏎", i18n.T("Ziel fortsetzen")}, {"x", i18n.T("Ziel beenden")}, {"Esc", "Terminal"}}
		}
		return [][2]string{{"x", i18n.T("Ziel beenden")}, {"Esc", "Terminal"}}
	case g.undoing:
		return [][2]string{{"⏎", i18n.T("zurückholen")}, {"x", i18n.T("abbrechen")}, {"Esc", "Terminal"}}
	case g.inputWait != "" && g.waiting:
		return a.inputWaitHints(s)
	case g.backingUp:
		return [][2]string{{"Esc", "Terminal"}}
	}
	return nil
}
