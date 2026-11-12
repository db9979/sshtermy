package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
)

// goalSession: Ziel-Modus (Schritt für Schritt) mit einem Ziel, ohne KI.
func goalSession(t *testing.T, c domain.Connection) (*App, *session, *syncBuf) {
	t.Helper()
	a := dangerApp(t)
	if c.ID == "" {
		c.ID = "c1"
	}
	s, out := typedSession(c, "dominik@lab:~$ ", "")
	a.sessions = []*session{s}
	withPanel(s, panelGoal)
	a.panelOpened(s)
	a.startGoal(s, "Nginx einrichten")
	s.goal.busy, s.goal.err = false, "" // keine KI im Test
	s.panelFocus = true
	return a, s, out
}

func toastHas(a *App, sub string) bool {
	for _, t := range a.toasts {
		if strings.Contains(t.text, sub) {
			return true
		}
	}
	return false
}

func TestGoalInstructAndSkipWhy(t *testing.T) {
	a, s, _ := goalSession(t, domain.Connection{GuardOff: true, Alias: "lab"})
	g := &s.goal
	a.sessionMsg(stepMsg(s, cmdStep("docker-compose up -d", "unix")))
	if g.cur == nil {
		t.Fatal("kein Vorschlag")
	}
	a.panelMsg(s, runeKey('?'))
	if a.modal == nil || a.modal.onText == nil {
		t.Fatal("? öffnet keine Eingabe für die Anweisung")
	}
	a.modal.onText("nimm docker compose")
	a.modal = nil
	if g.cur != nil || len(g.answers) == 0 {
		t.Fatalf("Anweisung nicht übernommen: cur=%v answers=%q", g.cur, g.answers)
	}
	if x, ok := agent.IsInstruction(g.answers[len(g.answers)-1]); !ok || x != "nimm docker compose" {
		t.Errorf("Anweisung falsch markiert: %q", g.answers)
	}
	// Überspringen mit Grund
	g.busy, g.err = false, ""
	a.sessionMsg(stepMsg(s, cmdStep("systemctl restart nginx", "unix")))
	a.panelMsg(s, runeKey('S'))
	if a.modal == nil {
		t.Fatal("⇧S öffnet keine Eingabe")
	}
	a.modal.onText("läuft schon")
	a.modal = nil
	last := g.steps[len(g.steps)-1]
	if !last.Skipped || last.Note != "läuft schon" {
		t.Errorf("Überspringen mit Grund: %+v", last)
	}
}

func TestGoalInputWait(t *testing.T) {
	a, s, out := goalSession(t, domain.Connection{GuardOff: true, Alias: "lab"})
	g := &s.goal
	a.sessionMsg(stepMsg(s, cmdStep("sudo apt install nginx", "unix")))
	if cmd := a.panelMsg(s, enterKey); cmd != nil {
		t.Fatal("ohne SFTP darf es keine Sicherung geben")
	}
	if !g.waiting || !out.has("sudo apt install nginx\r") {
		t.Fatalf("Schritt läuft nicht: %+v", g)
	}
	_, _ = s.term.Write([]byte("\r\nDo you want to continue? [Y/n] "))
	s.lastOutput = time.Now().Add(-3 * time.Second)
	g.waitSince = time.Now().Add(-3 * time.Second)
	a.goalTick(s)
	if g.inputWait != agent.InputYesNo {
		t.Fatalf("Rückfrage nicht erkannt: %q (%q)", g.inputWait, s.term.CursorLine())
	}
	if k, _ := a.goalAttention(s); k == "" {
		t.Error("Eingabe erwartet zählt nicht als „wartet auf dich“")
	}
	if !strings.Contains(a.viewPanel(s, 60, 30), "[j]") {
		t.Error("Panel zeigt keine Knöpfe")
	}
	a.panelMsg(s, runeKey('j'))
	if !out.has("y\r") || g.inputWait != "" {
		t.Errorf("j schickt kein y⏎: %q", out.String())
	}
}

func TestAgentPausesOnInputWait(t *testing.T) {
	a, s, _ := agentSession(t, domain.Connection{GuardOff: true, Alias: "lab-01"}, lvlNormal)
	g := &s.goal
	g.planOK, g.busy = true, false
	g.startWait()
	_, _ = s.term.Write([]byte("\r\n--More--"))
	s.lastOutput = time.Now().Add(-3 * time.Second)
	g.waitSince = time.Now().Add(-3 * time.Second)
	a.goalTick(s)
	if g.inputWait != agent.InputPager || !g.paused {
		t.Fatalf("Agent muss am Pager anhalten: wait=%q paused=%v", g.inputWait, g.paused)
	}
	a.panelMsg(s, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if g.paused || g.inputWait != "" {
		t.Errorf("Leertaste muss weiterblättern und den Agenten weiterlaufen lassen: paused=%v", g.paused)
	}
}

func TestGoalNotifyHiddenTab(t *testing.T) {
	a, s, _ := goalSession(t, domain.Connection{GuardOff: true, Alias: "lab"})
	other, _ := typedSession(domain.Connection{ID: "c2", Alias: "web"}, "$ ", "")
	other.id = 2
	a.sessions = []*session{s, other}
	a.cur = 1 // der Ziel-Tab ist verdeckt
	var sent []string
	old := desktopSend
	desktopSend = func(title, body string) { sent = append(sent, body) }
	defer func() { desktopSend = old }()

	a.sessionMsg(stepMsg(s, agent.Step{Question: "Welcher Port?"}))
	a.goalNotifyTick(s)
	if !toastHas(a, "Rückfrage") {
		t.Errorf("keine Einblendung für die Rückfrage: %+v", a.toasts)
	}
	if !strings.Contains(a.goalGlyph(s), "◎") {
		t.Errorf("Tab zeigt den Zustand des Ziels nicht: %q", a.goalGlyph(s))
	}
	a.blurred = true
	a.sessionMsg(stepMsg(s, cmdStep("ls", "unix")))
	a.goalNotifyTick(s)
	if len(sent) != 1 {
		t.Errorf("im Hintergrund muss eine Desktop-Meldung kommen: %q", sent)
	}
	a.goalNotifyTick(s)
	if len(sent) != 1 {
		t.Error("derselbe Anlass darf nur einmal melden")
	}
}

func TestGoalTabCount(t *testing.T) {
	_, s, _ := goalSession(t, domain.Connection{GuardOff: true, Alias: "lab"})
	if goalTabCount(s) != "" {
		t.Error("ohne Schritte keine Zahl")
	}
	s.goal.steps = append(s.goal.steps, agent.DoneStep{Command: "ls"}, agent.DoneStep{Command: "df"})
	if got := goalTabCount(s); got != " ·2" {
		t.Errorf("Schrittzahl: %q", got)
	}
}

func TestFollowGoalAndHistory(t *testing.T) {
	a, s, _ := goalSession(t, domain.Connection{GuardOff: true, Alias: "lab"})
	g := &s.goal
	g.steps = append(g.steps, agent.DoneStep{Command: "apt install nginx", Output: "ok"})
	a.sessionMsg(stepMsg(s, agent.Step{Done: true, Suggestion: agent.Suggestion{Reason: "Nginx läuft auf Port 80"}}))
	if !g.done || g.active {
		t.Fatal("Ziel nicht erreicht")
	}
	runs := a.st.GoalRuns("c1")
	if len(runs) != 1 || runs[0].State != "done" || len(runs[0].Steps) != 1 || runs[0].Result == "" {
		t.Fatalf("Zielverlauf: %+v", runs)
	}
	a.panelMsg(s, runeKey('f'))
	if a.modal == nil {
		t.Fatal("f öffnet kein Folgeziel")
	}
	a.modal.onText("HTTPS einrichten")
	a.modal = nil
	g = &s.goal
	if !g.active || g.goal != "HTTPS einrichten" || len(g.prior) != 1 || g.priorGoal != "Nginx einrichten" {
		t.Fatalf("Folgeziel ohne Vorgeschichte: %+v", g)
	}
	found := false
	for _, x := range g.answers {
		found = found || agent.IsPriorGoal(x)
	}
	if !found {
		t.Error("das Ziel davor fehlt für die KI")
	}
	// x beendet: Verlauf hat zwei Einträge (neueste zuerst)
	g.busy = false
	g.steps = append(g.steps, agent.DoneStep{Command: "certbot --nginx"})
	a.panelMsg(s, runeKey('x'))
	runs = a.st.GoalRuns("c1")
	if len(runs) != 2 || runs[0].State != "stopped" || runs[0].Goal != "HTTPS einrichten" {
		t.Fatalf("Verlauf nach x: %+v", runs)
	}
	// Auswahl im Eingabebild: ↓ wählt den neuesten Eintrag
	a.panelMsg(s, tea.KeyPressMsg{Code: tea.KeyDown})
	if g.input.Value() != "HTTPS einrichten" {
		t.Errorf("↓ übernimmt das Ziel aus dem Verlauf nicht: %q", g.input.Value())
	}
	if !strings.Contains(a.viewPanel(s, 70, 40), "HTTPS einrichten") {
		t.Error("Verlauf nicht zu sehen")
	}
	a.panelMsg(s, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if len(a.st.GoalRuns("c1")) != 1 {
		t.Errorf("Strg+D entfernt den Eintrag nicht: sel=%d key=%q active=%v done=%v", g.rbSel, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}.String(), g.active, g.done)
	}
}

func TestGoalBackupAndUndo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	conf := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(conf, []byte("port=80\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(dir, "neu.conf")
	a, s, out := goalSession(t, domain.Connection{Alias: "lokal", Protocol: domain.ProtoLocal, GuardOff: true})
	if !a.agentFilesOK(s) {
		t.Skip("lokale Shell hier ohne Dateien (kein Unix)")
	}
	g := &s.goal
	run := func(cmd string) {
		t.Helper()
		g.busy, g.waiting = false, false
		a.sessionMsg(stepMsg(s, cmdStep(cmd, "unix")))
		c := a.panelMsg(s, enterKey)
		if c == nil {
			t.Fatalf("%s: keine Sicherung angestoßen", cmd)
		}
		if !g.backingUp {
			t.Fatal("Sicherung läuft nicht")
		}
		a.sessionMsg(c())
		if !out.has(cmd + "\r") {
			t.Fatalf("%s lief nach der Sicherung nicht", cmd)
		}
	}
	run("sed -i 's/80/8080/' " + conf)
	run("echo x | sudo tee " + fresh)
	if len(g.backups) != 2 || string(g.backups[0].Content) != "port=80\n" || g.backups[1].Existed || !g.backups[1].Sudo {
		t.Fatalf("Sicherungen: %+v", g.backups)
	}
	// derselbe Pfad wird nur einmal gesichert (Zustand vor dem Ziel)
	g.busy, g.waiting = false, false
	a.sessionMsg(stepMsg(s, cmdStep("sed -i 's/8080/9090/' "+conf, "unix")))
	if c := a.panelMsg(s, enterKey); c != nil {
		t.Error("schon gesicherte Datei wird nochmal gesichert")
	}
	g.waiting = false
	a.sessionMsg(stepMsg(s, agent.Step{Done: true, Suggestion: agent.Suggestion{Reason: "fertig"}}))
	if !strings.Contains(a.viewPanel(s, 80, 40), "2") {
		t.Error("Liste der geänderten Dateien fehlt")
	}
	c := a.panelMsg(s, runeKey('u'))
	if c == nil {
		t.Fatal("u holt nichts zurück")
	}
	a.sessionMsg(c())
	if !g.undoing || g.cur == nil {
		t.Fatalf("Zurückholen-Befehl fehlt: %+v", g)
	}
	cmd := g.cur.Command
	if !strings.Contains(cmd, "cp ~/.sshtermy-agent/"+g.runID+"/undo/0 '"+conf+"'") || !strings.Contains(cmd, "sudo rm -f '"+fresh+"'") {
		t.Errorf("Befehl: %s", cmd)
	}
	saved, err := os.ReadFile(filepath.Join(home, ".sshtermy-agent", g.runID, "undo", "0"))
	if err != nil || string(saved) != "port=80\n" {
		t.Errorf("Sicherung nicht im Arbeitsordner: %q %v", saved, err)
	}
	a.panelMsg(s, enterKey)
	if !out.has(cmd + "\r") {
		t.Fatal("⏎ führt das Zurückholen nicht aus")
	}
	g.waiting = false
	a.undoFinished(s)
	if g.undoing || !g.done || len(g.backups) != 0 || !strings.Contains(g.doneReason, conf) {
		t.Errorf("nach dem Zurückholen: %+v", g)
	}
	if runs := a.st.GoalRuns("c1"); len(runs) != 1 {
		t.Errorf("Zurückholen darf keinen zweiten Verlaufseintrag schreiben: %d", len(runs))
	}
}

func TestGoalConnLostResume(t *testing.T) {
	a, s, _ := goalSession(t, domain.Connection{GuardOff: true, Alias: "lab"})
	g := &s.goal
	a.sessionMsg(stepMsg(s, cmdStep("apt update", "unix")))
	a.panelMsg(s, enterKey)
	seq := g.seq
	s.state = stReconnect
	g.connLost()
	if !g.lost || g.waiting || g.seq == seq {
		t.Fatalf("Ziel hält nicht an: %+v", g)
	}
	a.panelMsg(s, enterKey)
	if !g.lost {
		t.Error("ohne Verbindung darf ⏎ nicht fortsetzen")
	}
	s.state = stConnected
	if k, _ := a.goalAttention(s); k != "lost" {
		t.Errorf("wieder verbunden muss als „wartet auf dich“ gelten: %q", k)
	}
	a.panelMsg(s, enterKey)
	if g.lost {
		t.Fatal("⏎ setzt nicht fort")
	}
	last := g.answers[len(g.answers)-1]
	if !strings.HasPrefix(last, "\x02") {
		t.Errorf("KI erfährt nichts vom Abbruch: %q", g.answers)
	}
}

func TestGoalManualStep(t *testing.T) {
	a, s, _ := goalSession(t, domain.Connection{GuardOff: true, Alias: "lab"})
	g := &s.goal
	a.sessionMsg(stepMsg(s, cmdStep("systemctl status nginx", "unix")))
	a.beginCommand(s, "uptime")
	_, _ = s.term.Write([]byte("uptime\r\n 10:00 up 3 days\r\ndominik@lab:~$ "))
	a.finishCommand(s)
	if n := len(g.steps); n != 1 || !g.steps[0].Manual || g.steps[0].Command != "uptime" {
		t.Fatalf("selbst getippter Befehl fehlt: %+v", g.steps)
	}
	if !g.manualNew {
		t.Error("Hinweis auf neuen Vorschlag fehlt")
	}
	// mit i eingefügter Vorschlag ist kein eigener Schritt
	a.panelMsg(s, runeKey('i'))
	a.beginCommand(s, "systemctl status nginx")
	a.finishCommand(s)
	if len(g.steps) != 2 || g.steps[1].Manual {
		t.Errorf("eingefügter Vorschlag doppelt oder als eigener gezählt: %+v", g.steps)
	}
}

func TestCmdBlocksJumpAndList(t *testing.T) {
	a := dangerApp(t)
	s, _ := typedSession(domain.Connection{ID: "c1", Alias: "lab"}, "$ ", "")
	a.sessions = []*session{s}
	for i := 0; i < 15; i++ {
		cmd := "echo nummer" + string(rune('a'+i))
		a.beginCommand(s, cmd)
		_, _ = s.term.Write([]byte(cmd + "\r\nzeile1\r\nzeile2\r\n$ "))
		a.finishCommand(s)
	}
	if len(s.blocks) != 15 {
		t.Fatalf("Blöcke: %d", len(s.blocks))
	}
	a.jumpBlock(s, -1)
	if s.blockNav != 15 {
		t.Fatalf("Alt+Bild↑ springt nicht zum letzten Befehl: %d", s.blockNav)
	}
	for i := 0; i < 12; i++ {
		a.jumpBlock(s, -1)
	}
	if s.blockNav != 3 || s.scroll == 0 {
		t.Fatalf("zurückblättern: nav=%d scroll=%d", s.blockNav, s.scroll)
	}
	ln := s.locateBlock(s.blocks[2])
	if !strings.Contains(s.term.PlainLine(ln), "echo nummerc") {
		t.Errorf("falsche Zeile: %q", s.term.PlainLine(ln))
	}
	if first := s.term.ScrollbackLen() - s.scroll; first != ln-1 {
		t.Errorf("Befehl steht nicht oben: erste Zeile %d, Befehl %d", first, ln)
	}
	for i := 0; i < 13; i++ {
		a.jumpBlock(s, 1)
	}
	if s.blockNav != 0 || s.scroll != 0 {
		t.Errorf("Alt+Bild↓ am Ende muss live zeigen: nav=%d scroll=%d", s.blockNav, s.scroll)
	}
	a.openBlocks(s)
	if a.pick == nil || len(a.pick.items) != 15 || !strings.Contains(a.pick.items[0].label, "nummero") {
		t.Fatalf("Liste: %+v", a.pick)
	}
	if c := a.pickerKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); c == nil || a.pick != nil {
		t.Error("Strg+C kopiert die Ausgabe nicht")
	}
}
