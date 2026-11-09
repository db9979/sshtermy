package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
)

func companionApp(t *testing.T, level int) *App {
	t.Helper()
	a := dangerApp(t)
	a.setCompanion(func(c *domain.CompanionConfig) { c.Level = level })
	return a
}

func finish(a *App, s *session, cmd string, failed bool) {
	r := &cmdRun{cmd: cmd, at: time.Now()}
	f := agent.Failure{Command: cmd, Output: "out"}
	a.companionAfter(s, r, f, failed)
}

func TestCompanionTriggers(t *testing.T) {
	web := domain.Connection{Alias: "web01"}
	// aus: nichts
	a := companionApp(t, domain.CompanionOff)
	s, _ := typedSession(web, "$ ", "")
	finish(a, s, "systemctl status nginx", true)
	if s.comp.due != nil || len(s.comp.runs) != 0 || a.companionRows() != 0 {
		t.Fatal("Begleiter aus, trotzdem aktiv")
	}

	// leise: nur Fehler und Wiederholung
	a = companionApp(t, domain.CompanionQuiet)
	s, _ = typedSession(web, "$ ", "")
	finish(a, s, "ls /etc", false)
	if s.comp.due != nil {
		t.Fatal("leise meldet sich bei erfolgreichem Befehl")
	}
	finish(a, s, "sudo nginx -t", true)
	if s.comp.due == nil || s.comp.due.Kind != agent.EventFail || a.companionRows() != 2 {
		t.Fatalf("Fehler nicht erkannt: %+v", s.comp.due)
	}
	s.comp.due = nil
	finish(a, s, "ps aux", false)
	finish(a, s, "ps aux", false)
	finish(a, s, "ps aux", false)
	if s.comp.due == nil || s.comp.due.Kind != agent.EventRepeat || s.comp.due.Count != 3 {
		t.Fatalf("Wiederholung nicht erkannt: %+v", s.comp.due)
	}
	// stummes Thema und gerade gefragtes Thema: nicht noch einmal
	s.comp.due = nil
	s.comp.mute(topicOf(agent.EventFail, "sudo nginx -t"))
	finish(a, s, "sudo nginx -t", true)
	if s.comp.due != nil {
		t.Fatal("stummes Thema gemeldet")
	}
	// private Befehle (Leerzeichen am Anfang) bleiben draußen
	r := &cmdRun{cmd: "mysql -p", at: time.Now(), private: true}
	n := len(s.comp.runs)
	a.companionAfter(s, r, agent.Failure{}, true)
	if len(s.comp.runs) != n {
		t.Fatal("privater Befehl mitgeschrieben")
	}

	// gesprächig: jeder Befehl, gebündelt
	a = companionApp(t, domain.CompanionChatty)
	s, _ = typedSession(web, "$ ", "")
	finish(a, s, "df -h", false)
	if s.comp.due == nil || s.comp.due.Kind != agent.EventCommand {
		t.Fatal("gesprächig kommentiert nicht")
	}
	s.comp.due, s.comp.lastChatty = nil, time.Now()
	finish(a, s, "free -m", false)
	if s.comp.due != nil {
		t.Fatal("gesprächig ohne Pause")
	}
}

func TestCompanionGates(t *testing.T) {
	a := companionApp(t, domain.CompanionQuiet)
	s, _ := typedSession(domain.Connection{Alias: "kunde", Tags: []string{"begleiter-aus"}}, "$ ", "")
	if a.companionOff(s) == "" {
		t.Fatal("Tag begleiter-aus ignoriert")
	}
	s2, _ := typedSession(domain.Connection{Alias: "web"}, "[sudo] Passwort für dominik: ", "")
	if !strings.Contains(a.companionOff(s2), "Passwort") {
		t.Fatalf("Passwortabfrage nicht erkannt: %q", a.companionOff(s2))
	}
	s3, _ := typedSession(domain.Connection{Alias: "web"}, "$ ", "")
	s3.goal.active = true
	if a.companionOff(s3) == "" {
		t.Fatal("läuft trotz Agent/Ziel")
	}
	// Grenze pro Stunde
	for i := 0; i < a.compCfg().Hour(); i++ {
		a.compUse.hits = append(a.compUse.hits, time.Now())
	}
	if a.companionBudget() == "" {
		t.Fatal("Stundengrenze greift nicht")
	}
	a.compUse.hits = nil
	a.compUse.roll(time.Now())
	a.compUse.tokens = a.compCfg().DayTokens()
	if a.companionBudget() == "" {
		t.Fatal("Tagesbudget greift nicht")
	}
}

func TestCompanionRawOnlyOwnAI(t *testing.T) {
	a := companionApp(t, domain.CompanionQuiet)
	s, _ := typedSession(domain.Connection{Alias: "web"}, "$ ", "")
	a.setCompanion(func(c *domain.CompanionConfig) { c.Raw = true })
	cfg := a.st.LLM()
	cfg.BaseURL = "http://localhost:11434/v1"
	_ = a.st.SetLLM(cfg)
	if _, raw, err := a.companionClient(s); err != nil || !raw {
		t.Fatalf("lokale KI: raw=%v err=%v", raw, err)
	}
	cfg.BaseURL, cfg.Model = "https://llm.firma.de/v1", "m"
	_ = a.st.SetLLM(cfg)
	if _, _, err := a.companionClient(s); err == nil {
		t.Fatal("Filter aus mit fremdem Server erlaubt")
	}
	cfg.OwnHosts = []string{"llm.firma.de"}
	_ = a.st.SetLLM(cfg)
	if _, raw, err := a.companionClient(s); err != nil || !raw {
		t.Fatalf("eingetragener eigener Server: raw=%v err=%v", raw, err)
	}
}

func TestCompanionDangerBeforeEnter(t *testing.T) {
	a := companionApp(t, domain.CompanionQuiet)
	s, out := typedSession(domain.Connection{Alias: "web"}, "$ ", "sudo chmod -R 777 /")
	a.sessions = []*session{s}
	a.companionTypePause(s)
	if s.comp.remark == nil || s.comp.remark.kind != agent.EventDanger {
		t.Fatalf("keine Bemerkung vor ⏎: %+v", s.comp.remark)
	}
	// Antwort der KI: sicherere Variante ersetzt die Zeile per Alt+1
	cmd, _ := a.companionMsg(compRemarkMsg{sid: s.id, seq: s.comp.seq, topic: "x",
		ev: agent.CompanionEvent{Kind: agent.EventDanger, Command: "sudo chmod -R 777 /"},
		r:  agent.Remark{Text: "777 auf / macht das System unsicher.", Commands: []agent.Suggestion{{Command: "sudo chown -R www-data: /var/www"}}}})
	_ = cmd
	if !s.chipReplace || len(s.chips) != 1 || !strings.Contains(s.comp.remark.text, "777") {
		t.Fatalf("Vorschlag fehlt: chips=%v replace=%v", s.chips, s.chipReplace)
	}
	if line := a.companionLine(s); !strings.Contains(line, "777") {
		t.Fatalf("Zeile zeigt die Bemerkung nicht: %q", line)
	}
	a.terminalKeyOne(s, tea.KeyPressMsg{Code: '1', Mod: tea.ModAlt})
	if !out.has("\x15sudo chown") {
		t.Fatalf("Zeile nicht ersetzt: %q", out.String())
	}
}

func TestCompanionRecapRunbook(t *testing.T) {
	a := companionApp(t, domain.CompanionCoach)
	s, _ := typedSession(domain.Connection{Alias: "web01"}, "$ ", "")
	for _, c := range []struct {
		cmd  string
		fail bool
	}{{"sudo nginx -t", true}, {"sudo sed -i s/a/b/ x", false}, {"sudo nginx -t", false}, {"sudo nginx -t", false}, {"curl -sI localhost", false}} {
		finish(a, s, c.cmd, c.fail)
	}
	if !a.wantsRecap(s) {
		t.Fatal("Coach bietet keinen Rückblick an")
	}
	a.recapRunbook(s, "nginx reparieren")
	var rb *domain.Runbook
	for _, x := range a.st.Runbooks("") {
		if x.Name == "nginx reparieren" {
			rb = &x
		}
	}
	if rb == nil || len(rb.Steps) != 3 || rb.Steps[1] != "sudo nginx -t" {
		t.Fatalf("Runbook: %+v", rb)
	}
	if a.companionRemember(s, "Backend ist php8.3-fpm", "Backend ist php8.3-fpm") != 1 || !strings.Contains(s.prof.Notes, "php8.3-fpm") {
		t.Fatalf("Notizen: %q", s.prof.Notes)
	}
}
