package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
)

func planSession(t *testing.T) (*App, *session, *syncBuf) {
	t.Helper()
	a := dangerApp(t)
	s, out := typedSession(domain.Connection{Alias: "lab-01"}, "dominik@lab:~$ ", "")
	a.sessions = []*session{s}
	withPanel(s, panelGoal)
	a.panelOpened(s)
	for !s.goal.planFirst { // Tab bis „Erst planen“
		a.panelMsg(s, tea.KeyPressMsg{Code: tea.KeyTab})
	}
	a.startGoal(s, "Logs komprimieren")
	if !s.goal.planning || s.goal.runID == "" {
		t.Fatalf("Planung nicht gestartet: %+v", s.goal)
	}
	return a, s, out
}

func testPlan(dir string) *agent.FullPlan {
	f := agent.FileAction{Name: "rotate.sh", Content: "#!/bin/sh\necho ok\n"}
	return &agent.FullPlan{Summary: "Logs komprimieren", Steps: []agent.PlanStep{
		{Suggestion: agent.ParseStepCommand("du -sh /var/log/app", "unix")},
		{File: &f},
		{Suggestion: agent.ParseStepCommand("bash "+dir+"/rotate.sh", "unix")},
		{Suggestion: agent.ParseStepCommand("rm -f /var/log/app/old-*.gz", "unix")},
	}, Risks: []string{"Logs weg"}, Checks: []string{"du -sh /var/log/app"}}
}

func TestPlanExploreOnlyReadOnly(t *testing.T) {
	a, s, out := planSession(t)
	g := &s.goal
	a.sessionMsg(stepMsg(s, cmdStep("ls -la /var/log/app", "unix")))
	if !strings.Contains(out.String(), "ls -la /var/log/app\r") || g.explored != 1 {
		t.Fatalf("lesender Befehl muss beim Planen laufen: %q", out.String())
	}
	g.waiting = false
	a.sessionMsg(stepMsg(s, cmdStep("mkdir /tmp/x", "unix")))
	if strings.Contains(out.String(), "mkdir") {
		t.Fatal("ändernder Befehl darf beim Planen nicht laufen")
	}
	last := g.steps[len(g.steps)-1]
	if !last.Skipped || last.ExitHint == "" {
		t.Errorf("abgelehnter Befehl nicht vermerkt: %+v", last)
	}
	// Typing/Esc stören die Erkundung nicht als Agent-Pause
	if g.agentRunning() {
		t.Error("Planung zählt nicht als laufender Agent")
	}
}

func TestPlanReviewAndRun(t *testing.T) {
	a, s, out := planSession(t)
	g := &s.goal
	a.sessionMsg(stepMsg(s, agent.Step{Full: testPlan(g.agentDir())}))
	if !g.planReview() || g.planning {
		t.Fatalf("Plan muss zur Freigabe vorliegen: %+v", g)
	}
	var b strings.Builder
	a.viewGoal(&b, s, 60)
	v := b.String()
	for _, want := range []string{"Plan", "rotate.sh", "RISIKEN", "PRÜFUNG", "Agent · Normal"} {
		if !strings.Contains(v, want) {
			t.Errorf("%q fehlt in der Planansicht:\n%s", want, v)
		}
	}
	// Schritt 4 (rm) streichen, Schritt 1 ändern
	for i := 0; i < 3; i++ {
		a.panelMsg(s, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	a.panelMsg(s, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if !g.planOff[3] || len(g.planOn()) != 3 {
		t.Fatalf("Streichen: %v", g.planOff)
	}
	// Runbook aus dem Plan
	a.panelMsg(s, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	rbs := a.st.Runbooks("")
	if len(rbs) != 1 || len(rbs[0].Steps) != 3 || len(rbs[0].Files) != 1 ||
		rbs[0].Steps[2] != "bash "+domain.AgentDirToken+"/rotate.sh" {
		t.Fatalf("Runbook aus Plan: %+v", rbs)
	}
	// freigeben: läuft als Agent, Plan gilt als freigegeben
	a.panelMsg(s, enterKey)
	if g.planning || g.fp != nil || !g.auto || !g.approved || !g.planOK || len(g.recipe) != 3 || len(g.recipeFiles) != 1 {
		t.Fatalf("Ausführung nicht gestartet: %+v", g)
	}
	if !strings.Contains(g.recipe[2], g.agentDir()+"/rotate.sh") {
		t.Errorf("Arbeitsordner nicht eingesetzt: %q", g.recipe)
	}
	if !g.planFirst {
		t.Error("„Erst planen“ soll für das nächste Ziel gewählt bleiben")
	}
	// erster Schritt läuft ohne Plan-Pause
	a.sessionMsg(stepMsg(s, cmdStep("du -sh /var/log/app", "unix")))
	if g.paused || !strings.Contains(out.String(), "du -sh /var/log/app\r") {
		t.Fatalf("freigegebener Plan muss ohne zweite Freigabe starten: %+v", g)
	}
}

func TestPlanReplanAndDiscard(t *testing.T) {
	a, s, _ := planSession(t)
	g := &s.goal
	a.sessionMsg(stepMsg(s, agent.Step{Full: testPlan(g.agentDir())}))
	a.panelMsg(s, runeKey('?'))
	if a.modal == nil {
		t.Fatal("? muss nach der Änderung fragen")
	}
	a.modal.in.SetValue("ohne rm")
	a.modal.onText(a.modal.in.Value())
	if !g.planning || g.fpPrev == nil || g.fp != nil || !strings.Contains(strings.Join(g.answers, " "), "ohne rm") {
		t.Fatalf("Neu planen: %+v", g)
	}
	a.sessionMsg(stepMsg(s, agent.Step{Full: testPlan(g.agentDir())}))
	a.panelMsg(s, runeKey('x'))
	if g.active || g.fp != nil {
		t.Fatal("x muss den Plan verwerfen")
	}
}

func TestPlanRunStaysInPlan(t *testing.T) {
	a, s, out := planSession(t)
	g := &s.goal
	a.sessionMsg(stepMsg(s, agent.Step{Full: testPlan(g.agentDir())}))
	g.planOff[3] = true // rm gestrichen
	a.panelMsg(s, enterKey)
	// lesend: läuft, auch wenn nicht im Plan
	a.sessionMsg(stepMsg(s, cmdStep("ls /var/log/app", "unix")))
	if g.paused {
		t.Fatalf("lesender Befehl muss laufen: %q", g.pauseWhy)
	}
	g.waiting = false
	// ändernd und im Plan: läuft
	a.sessionMsg(stepMsg(s, cmdStep("bash "+g.agentDir()+"/rotate.sh", "unix")))
	if g.paused || !strings.Contains(out.String(), "rotate.sh\r") {
		t.Fatalf("Planschritt muss laufen: %q", g.pauseWhy)
	}
	g.waiting = false
	// ändernd, nicht im Plan: hält an
	a.sessionMsg(stepMsg(s, cmdStep("mkdir -p /srv/neu", "unix")))
	if !g.paused || !strings.Contains(g.pauseWhy, "Plan") {
		t.Fatalf("Befehl außerhalb des Plans muss anhalten: %q", g.pauseWhy)
	}
	// Datei, die der Plan nicht vorsieht
	g.paused, g.cur = false, nil
	f := agent.FileAction{Name: "anders.sh", Content: "echo\n"}
	a.sessionMsg(stepMsg(s, agent.Step{File: &f}))
	if !g.paused {
		t.Fatal("fremde Datei muss anhalten")
	}
}
