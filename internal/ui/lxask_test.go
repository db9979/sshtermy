package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/cmdcheck"
)

const testManTop = `NAME
       top - display Linux processes

OPTIONS
       -b  :Batch-mode operation
            Starts top in Batch mode.
       -n  :Number-of-iterations limit as:  -n number
            Specifies the maximum number of iterations.
       -o  :Override-sort-field as:  -o fieldname
            Specifies the name of the field.
`

func TestCardChecksRepairDocsRecheck(t *testing.T) {
	a := newPaletteTestApp(t)
	s := &session{id: 3}
	a.sessions = append(a.sessions, s)
	q := &s.ask
	q.question = "zeige Fehler in /var/log und die Prozesse, ohne lsof"
	q.answer.WriteString("So:\n\n```sh\ngrep ERROR /var/log\n```\n\n```sh\ntop -b -n 1 -l 1\n```\n\n```sh\nlsof -i :80\n```\n")
	a.startCardChecks(s)
	if c, _ := a.cardCode(s, 0); c.code != "grep -r ERROR /var/log" {
		t.Fatalf("grep nicht repariert: %q", c.code)
	}
	if ck := q.checks[2]; ck == nil || len(ck.res.Issues) != 1 || ck.res.Issues[0].Kind != cmdcheck.KindForbidden {
		t.Fatalf("lsof trotz „ohne lsof“: %+v", ck)
	}
	// Doku vom Host ist da
	s.docs = cmdcheck.Docs{"top": {Source: "man", Text: testManTop, Path: "/usr/bin/top"}, "grep": {Missing: false}}
	a.finishCardChecks(s, q.seq)
	ck := q.checks[1]
	if ck == nil || ck.res.OK() || !strings.Contains(ck.res.Issues[0].Text, "-l") || !ck.retried {
		t.Fatalf("top -l nicht beanstandet: %+v", ck)
	}
	// zweiter Versuch der KI ist besser
	a.recheckDone(recheckMsg{sid: 3, seq: q.seq, idx: 1, cmd: "top -b -n 1"})
	if c, _ := a.cardCode(s, 1); c.code != "top -b -n 1" || ck.improved != "top -b -n 1 -l 1" {
		t.Fatalf("Nachbesserung nicht übernommen: %q / %+v", c.code, ck)
	}
	var b strings.Builder
	a.viewAnswer(&b, s, agent.VisibleAnswer(q.answer.String()), 80)
	out := ansi.Strip(b.String())
	for _, want := range []string{"angepasst: grep -r ergänzt", "KI hat nachgebessert", "unbestätigt: die Frage sagt ohne lsof"} {
		if !strings.Contains(out, want) {
			t.Fatalf("%q fehlt:\n%s", want, out)
		}
	}
	// schlechterer zweiter Versuch bleibt draußen
	ck0 := q.checks[2]
	ck0.busy = true
	a.recheckDone(recheckMsg{sid: 3, seq: q.seq, idx: 2, cmd: "sudo lsof -i :80 -Z"})
	if c, _ := a.cardCode(s, 2); c.code != "lsof -i :80" || ck0.busy {
		t.Fatalf("schlechterer Vorschlag übernommen: %q", c.code)
	}
	// abgeschaltet: keine Prüfung
	a.toggleCmdCheck()
	q.checks = nil
	a.startCardChecks(s)
	if q.checks != nil {
		t.Fatal("Prüfung trotz „aus“")
	}
}

func TestAITiersHideFeatures(t *testing.T) {
	a := newPaletteTestApp(t)
	s := &session{id: 1, state: stConnected}
	a.sessions = append(a.sessions, s)
	if a.aiTier(s) != tierFull {
		t.Fatalf("Standard sollte volle KI sein: %v", a.aiTier(s))
	}
	no, yes := false, true
	a.aiDownTest, a.offTest = true, &no
	if a.aiTier(s) != tierNone {
		t.Fatal("ohne erreichbare KI und ohne Offline-KI: keine KI")
	}
	keys := a.terminalKeys(s)
	for _, k := range keys {
		if k[1] == "Ziel" || k[1] == "KI" || k[1] == "Warum? & Fix" {
			t.Fatalf("KI-Taste trotz keiner KI: %v", keys)
		}
	}
	if a.needAI(s, false) {
		t.Fatal("needAI ohne KI")
	}
	if _, err := a.agentClientFor(s.conn); err == nil || !strings.Contains(err.Error(), "Keine KI") {
		t.Fatalf("agentClientFor: %v", err)
	}
	items := a.aiHide(s, []palItem{{label: "Ziel", area: "ai"}, {label: "KI fragen", area: "ai", small: true}, {label: "Dateien", area: "session"}})
	if len(items) != 1 || items[0].label != "Dateien" {
		t.Fatalf("Palette ohne KI: %+v", items)
	}

	a.offTest = &yes
	if a.aiTier(s) != tierSmall {
		t.Fatal("mit Offline-KI: kleine Stufe")
	}
	keys = a.terminalKeys(s)
	var labels []string
	for _, k := range keys {
		labels = append(labels, k[1])
	}
	joined := strings.Join(labels, "|")
	if strings.Contains(joined, "|Ziel|") || !strings.Contains(joined, "|KI|") || !strings.Contains(joined, "Satz→Befehl") {
		t.Fatalf("Tasten mit Offline-KI: %v", labels)
	}
	if !a.needAI(s, false) || a.needAI(s, true) {
		t.Fatal("needAI mit Offline-KI")
	}
	if _, err := a.agentClientFor(s.conn); err == nil || !strings.Contains(err.Error(), "größere KI") {
		t.Fatalf("große Funktionen mit Offline-KI: %v", err)
	}
	ag, err := a.cmdAgent(s)
	if err != nil || !ag.Small {
		t.Fatalf("cmdAgent: %+v %v", ag, err)
	}
	items = a.aiHide(s, []palItem{{label: "Ziel", area: "ai"}, {label: "KI fragen", area: "ai", small: true}})
	if len(items) != 1 || items[0].label != "KI fragen" {
		t.Fatalf("Palette mit Offline-KI: %+v", items)
	}
	if a.companionOff(s) == "" {
		t.Fatal("Begleiter mit Offline-KI")
	}
}

func TestExplainModalFromDocs(t *testing.T) {
	a := newPaletteTestApp(t)
	s := &session{id: 2}
	s.docs = cmdcheck.Docs{"top": {Source: "man", Text: testManTop, Path: "/usr/bin/top"}}
	m := a.explainModal(s, "top -b -n 1 -l 1", "")
	out := m.copy
	for _, want := range []string{"top  display Linux processes", "-b", "Starts top in Batch mode", "? top: -l steht nicht in der Doku"} {
		if !strings.Contains(out, want) {
			t.Fatalf("%q fehlt:\n%s", want, out)
		}
	}
	m = a.explainModal(s, "rm -rf /tmp/x", "")
	if !strings.Contains(m.copy, "! ") {
		t.Fatalf("Gefahr fehlt:\n%s", m.copy)
	}
}

func TestCheckBackgroundStatic(t *testing.T) {
	res, _ := checkBackground(context.Background(), nil, "ohne sudo nginx neu starten", "sudo systemctl restart nginx", cmdcheck.Sys{}, cmdcheck.Docs{}, nil, agent.Context{})
	if res.Command != "systemctl restart nginx" || !res.OK() {
		t.Fatalf("%+v", res)
	}
}

// Absturz 2026-11-21: nach einem fehlgeschlagenen Download stand ein langer
// Fehlertext als Hinweis neben „Kleine Offline-KI herunterladen“; trunc bekam
// eine negative Länge und die Palette stürzte ab.
func TestPaletteLongHintNoPanic(t *testing.T) {
	if trunc("abc", -6) != "" || trunc("abc", 0) != "" || trunc("abc", 1) != "a" {
		t.Fatal("trunc mit wenig Platz")
	}
	a := newPaletteTestApp(t)
	a.offDL.err = strings.Repeat("Get \"https://api.github.com/repos/ggml-org/llama.cpp/releases/latest\": dial tcp: lookup failed ", 3)
	a.openPalette()
	a.pal.menu = "settings/ki"
	for _, w := range []int{200, 82, 60, 40, 20} {
		a.w = w
		out := ansiStrip(a.viewPalette())
		if w >= 82 && !strings.Contains(out, "Offline-KI") {
			t.Fatalf("Breite %d: Eintrag fehlt:\n%s", w, out)
		}
	}
}
