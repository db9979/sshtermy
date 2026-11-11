package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
)

// agentSession: Agent-Lauf auf einem Lab-Host, Ziel gesetzt, wartet auf den ersten Schritt.
func agentSession(t *testing.T, c domain.Connection, level int) (*App, *session, *syncBuf) {
	t.Helper()
	a := dangerApp(t)
	s, out := typedSession(c, "dominik@lab:~$ ", "")
	a.sessions = []*session{s}
	withPanel(s, panelGoal)
	a.panelOpened(s)
	s.goal.auto, s.goal.level = true, level
	a.startGoal(s, "Logrotation einrichten")
	if !s.goal.auto || s.goal.runID == "" || s.goal.maxSteps != agentSteps {
		t.Fatalf("Agent-Lauf nicht vorbereitet: %+v", s.goal)
	}
	return a, s, out
}

func stepMsg(s *session, st agent.Step) goalStepMsg {
	return goalStepMsg{sid: s.id, seq: s.goal.seq, step: st}
}

func cmdStep(cmd, kind string) agent.Step {
	return agent.Step{Suggestion: agent.ParseStepCommand(cmd, kind), Plan: []string{"prüfen", "einrichten"}}
}

var enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}

func runeKey(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func TestAgentPlanThenAuto(t *testing.T) {
	a, s, out := agentSession(t, domain.Connection{GuardOff: true, Alias: "lab-01"}, lvlNormal)
	g := &s.goal
	a.sessionMsg(stepMsg(s, cmdStep("df -h /var", "unix")))
	if !g.paused || g.planOK || g.cur == nil {
		t.Fatalf("erster Schritt muss auf den Plan warten: %+v", g)
	}
	if strings.Contains(out.String(), "df -h") {
		t.Fatal("vor der Freigabe des Plans darf nichts laufen")
	}
	a.panelMsg(s, enterKey)
	if !g.planOK || g.paused || !g.waiting || !out.has("df -h /var\r") {
		t.Fatalf("⏎ muss den Agenten starten: %+v / %q", g, out.String())
	}
	if s.run == nil || !s.run.agent {
		t.Error("Befehl nicht als Agent-Befehl markiert")
	}
	// nächster Schritt: ändert etwas, kein sudo ⇒ läuft in Normal allein
	g.waiting = false
	a.sessionMsg(stepMsg(s, cmdStep("mkdir -p /tmp/rot", "unix")))
	if g.paused || !out.has("mkdir -p /tmp/rot\r") {
		t.Fatalf("Normal: Änderung ohne sudo muss allein laufen: %+v", g)
	}
}

func TestAgentGates(t *testing.T) {
	cases := []struct {
		name  string
		conn  domain.Connection
		level int
		cmd   string
		auto  bool
	}{
		{"lesen vorsichtig", domain.Connection{GuardOff: true, Alias: "lab"}, lvlCareful, "ls -la /etc", true},
		{"ändern vorsichtig", domain.Connection{GuardOff: true, Alias: "lab"}, lvlCareful, "mkdir /tmp/x", false},
		{"sudo normal", domain.Connection{GuardOff: true, Alias: "lab"}, lvlNormal, "sudo systemctl restart nginx", false},
		{"sudo lesen normal", domain.Connection{GuardOff: true, Alias: "lab"}, lvlNormal, "sudo du -sh /var/log", true},
		{"sudo voll", domain.Connection{GuardOff: true, Alias: "lab"}, lvlFull, "sudo systemctl restart nginx", true},
		{"gefahr voll", domain.Connection{GuardOff: true, Alias: "lab"}, lvlFull, "sudo rm -rf /var/log/app/old", false},
		{"prod ändern voll", domain.Connection{Alias: "web-prod"}, lvlFull, "mkdir /tmp/x", false},
		{"prod lesen", domain.Connection{Alias: "web-prod"}, lvlNormal, "df -h", true},
		{"switch config", domain.Connection{Alias: "sw", Device: domain.DeviceNetwork}, lvlFull, "ntp server 10.0.0.1", false},
		{"sudo verkettet normal", domain.Connection{GuardOff: true, Alias: "lab"}, lvlNormal, "cd /etc && sudo systemctl restart nginx", false},
		{"inline normal", domain.Connection{GuardOff: true, Alias: "lab"}, lvlNormal, `python3 -c "import os; os.remove('x')"`, false},
		{"inline voll", domain.Connection{GuardOff: true, Alias: "lab"}, lvlFull, "bash -c 'mkdir -p /tmp/x'", true},
		{"skript normal", domain.Connection{GuardOff: true, Alias: "lab"}, lvlNormal, "bash /tmp/x.sh", true},
		{"switch show", domain.Connection{Alias: "sw", Device: domain.DeviceNetwork}, lvlCareful, "show ntp status", true},
		// Sicherheitsprüfung 2026-11-10: root/Admin hat keine sudo-Bremse
		{"root normal ändern", domain.Connection{GuardOff: true, Alias: "lab", Username: "root"}, lvlNormal, "systemctl restart nginx", false},
		{"root normal lesen", domain.Connection{GuardOff: true, Alias: "lab", Username: "root"}, lvlNormal, "df -h", true},
		{"root voll", domain.Connection{GuardOff: true, Alias: "lab", Username: "root"}, lvlFull, "systemctl restart nginx", true},
		{"winrm normal", domain.Connection{Alias: "win", Protocol: domain.ProtoWinRM}, lvlNormal, "Restart-Service Spooler", false},
		{"root voll fstab", domain.Connection{GuardOff: true, Alias: "lab", Username: "root"}, lvlFull, "rm /etc/fstab", false},
		{"falscher switch", domain.Connection{GuardOff: true, Alias: "lab"}, lvlFull, "erase startup-config", false},
		{"geheimnis voll", domain.Connection{GuardOff: true, Alias: "lab"}, lvlFull, "sudo cat /etc/shadow", false},
		{"switch config lesen", domain.Connection{Alias: "sw", Device: domain.DeviceNetwork}, lvlFull, "show running-config", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := dangerApp(t)
			s, _ := typedSession(c.conn, "$ ", "")
			s.goal.auto, s.goal.level = true, c.level
			st := cmdStep(c.cmd, a.safetyKind(s))
			ok, why := a.agentGate(s, &st)
			if ok != c.auto {
				t.Errorf("%q: automatisch=%v, erwartet %v (%s)", c.cmd, ok, c.auto, why)
			}
			if !ok && why == "" {
				t.Error("Pause ohne Grund")
			}
		})
	}
}

func TestAgentDangerNeedsTwoEnters(t *testing.T) {
	a, s, out := agentSession(t, domain.Connection{GuardOff: true, Alias: "lab-01"}, lvlFull)
	g := &s.goal
	g.planOK = true
	a.sessionMsg(stepMsg(s, cmdStep("rm -rf /tmp/rot/old", "unix")))
	if !g.paused || !strings.Contains(g.pauseWhy, "löscht") {
		t.Fatalf("Gefahr muss anhalten: %+v", g)
	}
	a.panelMsg(s, enterKey)
	if strings.Contains(out.String(), "rm -rf") || !g.armed {
		t.Fatal("erstes ⏎ darf nur scharf schalten")
	}
	a.panelMsg(s, enterKey)
	if !out.has("rm -rf /tmp/rot/old\r") {
		t.Fatal("zweites ⏎ muss ausführen")
	}
}

func TestAgentTypingAndEscPause(t *testing.T) {
	a, s, _ := agentSession(t, domain.Connection{GuardOff: true, Alias: "lab-01"}, lvlNormal)
	g := &s.goal
	g.planOK, g.paused = true, false
	a.terminalKey(s, runeKey('l'))
	if !g.paused {
		t.Fatal("Tippen im Terminal muss den Agenten anhalten")
	}
	g.paused = false
	a.panelMsg(s, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !g.paused || !s.panelFocus {
		t.Fatalf("Esc muss anhalten (und das Panel behalten): %+v", g)
	}
}

func TestAgentLimits(t *testing.T) {
	a, s, _ := agentSession(t, domain.Connection{GuardOff: true, Alias: "lab-01"}, lvlNormal)
	g := &s.goal
	g.planOK, g.paused = true, false
	for i := 0; i < agentSteps; i++ {
		g.steps = append(g.steps, agent.DoneStep{Command: "true"})
	}
	if a.agentNext(s) != nil || !g.paused || !g.needStep {
		t.Fatalf("Schrittgrenze muss anhalten: %+v", g)
	}
	a.agentResume(s)
	if g.maxSteps != agentSteps+agentStepsMore || g.paused {
		t.Fatalf("⏎ muss weitere Schritte erlauben: %d", g.maxSteps)
	}
	// Fehler in Folge
	g.paused = false
	for i := 0; i < agentFailsInRow; i++ {
		g.steps = append(g.steps, agent.DoneStep{Command: "false", ExitHint: "Exit-Code 1"})
		a.agentStepFinished(s)
		g.paused, g.busy = false, false
	}
	if g.fails < agentFailsInRow {
		t.Fatalf("Fehler nicht gezählt: %d", g.fails)
	}
	// Zeit
	g.fails, g.deadline = 0, time.Now().Add(-time.Second)
	a.agentNext(s)
	if !g.paused || !strings.Contains(g.pauseWhy, "Zeit") {
		t.Fatalf("Zeitgrenze muss anhalten: %q", g.pauseWhy)
	}
}

func TestAgentFileStepAndRunbook(t *testing.T) {
	a, s, _ := agentSession(t, domain.Connection{GuardOff: true, Alias: "lab-01"}, lvlNormal)
	g := &s.goal
	g.planOK = true
	f := agent.FileAction{Name: "rotate.sh", Content: "#!/bin/sh\necho ok\n"}
	a.sessionMsg(stepMsg(s, agent.Step{File: &f}))
	// ohne SSH-Client keine Dateien: der Schritt wird als nicht möglich vermerkt
	if n := len(g.steps); n != 1 || g.steps[0].File != "rotate.sh" || g.steps[0].ExitHint == "" {
		t.Fatalf("Datei ohne SFTP: %+v", g.steps)
	}
	// Ergebnis einer erfolgreichen Ablage (wie nach SFTP)
	g.steps, g.writing = nil, true
	a.agentFileDone(agentFileMsg{sid: s.id, seq: g.seq, file: f})
	if len(g.files) != 1 || g.steps[0].File != "rotate.sh" || !strings.Contains(g.steps[0].Command, g.agentDir()+"/rotate.sh") {
		t.Fatalf("Datei nicht vermerkt: %+v", g.steps)
	}
	g.steps = append(g.steps, agent.DoneStep{Command: "bash " + g.agentDir() + "/rotate.sh"})
	g.done, g.active, g.doneReason = true, false, "fertig"
	a.saveRunbook(s)
	rbs := a.st.Runbooks("")
	if len(rbs) != 1 {
		t.Fatalf("Runbook fehlt: %+v", rbs)
	}
	rb := rbs[0]
	if len(rb.Steps) != 1 || rb.Steps[0] != "bash "+domain.AgentDirToken+"/rotate.sh" {
		t.Errorf("Schritte: %q", rb.Steps)
	}
	if len(rb.Files) != 1 || rb.Files[0].Content != f.Content {
		t.Errorf("Dateien: %+v", rb.Files)
	}
	// erneut starten: Platzhalter wird zum neuen Arbeitsordner, Datei geht als Vorlage mit
	a.startRunbook(s, rb)
	if !s.goal.auto || len(s.goal.recipeFiles) != 1 || s.goal.recipe[0] != "bash "+s.goal.agentDir()+"/rotate.sh" {
		t.Errorf("Runbook-Start: %+v / %q", s.goal.recipeFiles, s.goal.recipe)
	}
}

func TestAgentViewRenders(t *testing.T) {
	a, s, _ := agentSession(t, domain.Connection{GuardOff: true, Alias: "lab-01"}, lvlNormal)
	g := &s.goal
	render := func() string {
		var b strings.Builder
		a.viewGoal(&b, s, 46)
		return b.String()
	}
	a.sessionMsg(stepMsg(s, cmdStep("df -h /var", "unix")))
	if v := render(); !strings.Contains(v, "Agent") || !strings.Contains(v, "Plan prüfen") {
		t.Errorf("Plan-Pause fehlt:\n%s", v)
	}
	f := agent.FileAction{Name: "rotate.sh", Content: strings.Repeat("echo x\n", 10)}
	g.cur = &agent.Step{File: &f}
	if v := render(); !strings.Contains(v, "rotate.sh") || !strings.Contains(v, "weitere Zeilen") {
		t.Errorf("Dateivorschau fehlt:\n%s", v)
	}
	g.files = []agent.FileAction{f}
	g.cur, g.done, g.active, g.doneReason = nil, true, false, "Logrotation läuft."
	if v := render(); !strings.Contains(v, "Runbook") || !strings.Contains(v, "rotate.sh") {
		t.Errorf("Abschluss fehlt:\n%s", v)
	}
	// Startansicht mit Moduswahl
	g.done = false
	if v := render(); !strings.Contains(v, "Schritt für Schritt") || !strings.Contains(v, "Vorsichtig") {
		t.Errorf("Moduswahl fehlt:\n%s", v)
	}
	a.panelMsg(s, tea.KeyPressMsg{Code: tea.KeyTab})
	if g.level != lvlFull {
		t.Errorf("Tab: Stufe %d", g.level)
	}
	a.panelMsg(s, tea.KeyPressMsg{Code: tea.KeyTab})
	if !g.planFirst {
		t.Error("Tab nach Voll muss zu „Erst planen“ wechseln")
	}
	a.panelMsg(s, tea.KeyPressMsg{Code: tea.KeyTab})
	if g.auto || g.planFirst {
		t.Error("Tab nach „Erst planen“ muss zu Schritt für Schritt wechseln")
	}
}
