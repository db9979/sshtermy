package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/agent"
)

func TestReplacedRunsDate(t *testing.T) {
	from := `sudo journalctl -u sudo --since "10.11. 12:00:00" --until "10.11. 12:10:00"`
	to := `sudo journalctl -u sudo --since "2026-11-10 12:00:00" --until "2026-11-10 12:10:00"`
	pairs := replacedRuns(from, to)
	if len(pairs) != 1 || strings.Join(pairs[0][0], " ") != `"10.11.` || strings.Join(pairs[0][1], " ") != `"2026-11-10` {
		t.Fatalf("Paare: %q", pairs)
	}
	other := `sudo journalctl -p err --since "10.11. 08:00"`
	if got := applyRuns(other, pairs); got != `sudo journalctl -p err --since "2026-11-10 08:00"` {
		t.Fatalf("mitgezogen: %q", got)
	}
	if got := applyRuns("ls -la /home", pairs); got != "ls -la /home" {
		t.Fatalf("ohne Treffer unverändert: %q", got)
	}
	// Programmname wird nie ersetzt
	if p := replacedRuns("ifconfig -a", "ip addr"); len(p) != 0 {
		t.Fatalf("Programmwechsel zählt nicht: %q", p)
	}
}

func TestCardFailAdoptLearn(t *testing.T) {
	a := newPaletteTestApp(t)
	s := &session{id: 7}
	a.sessions = append(a.sessions, s)
	q := &s.ask
	q.question = "wer hat heute ab 12 Uhr sudo benutzt?"
	q.answer.WriteString("Prüfen:\n\n```sh\nsudo grep wenzroot /etc/sudoers\n```\n\n" +
		"```sh\nsudo journalctl -u sudo --since \"10.11. 12:00:00\" --until \"10.11. 12:10:00\"\n```\n\n" +
		"```sh\njournalctl -p err --since \"10.11. 12:00:00\"\n```\n")
	f := agent.Failure{Command: `sudo journalctl -u sudo --since "10.11. 12:00:00" --until "10.11. 12:10:00"`,
		Output: "Failed to parse timestamp: 10.11. 12:00:00"}
	a.cardFailed(s, f)
	cf := q.fixes[1]
	if cf == nil || cf.errLine != "Failed to parse timestamp: 10.11. 12:00:00" || !cf.want {
		t.Fatalf("Karte 2 nicht markiert: %+v", cf)
	}
	// Antwort der KI kommt an
	cf.want = false
	a.cardFixDone(cardFixMsg{sid: 7, seq: q.seq, idx: 1, r: agent.CardFix{
		Command: agent.ParseStepCommand(`sudo journalctl -u sudo --since "2026-11-10 12:00:00" --until "2026-11-10 12:10:00"`, ""),
		Reason:  "journalctl erwartet ISO 8601"}})
	var b strings.Builder
	a.viewAnswer(&b, s, agent.VisibleAnswer(q.answer.String()), 70)
	out := ansi.Strip(b.String())
	for _, want := range []string{"fehlgeschlagen: Failed to parse", "Fix:", "2026-11-10 12:00:00"} {
		if !strings.Contains(out, want) {
			t.Fatalf("%q fehlt:\n%s", want, out)
		}
	}
	if len(s.chips) != 1 {
		t.Fatalf("Fix auch per Alt+1: %+v", s.chips)
	}
	// u übernimmt, Karte 3 wird mitkorrigiert
	q.sel = 1
	a.askCardKey(s, "u")
	if c, _ := a.cardCode(s, 1); !strings.Contains(c.code, "2026-11-10 12:10:00") {
		t.Fatalf("Karte 2: %q", c.code)
	}
	if c, _ := a.cardCode(s, 2); c.code != `journalctl -p err --since "2026-11-10 12:00:00"` {
		t.Fatalf("Karte 3: %q", c.code)
	}
	if c, _ := a.cardCode(s, 0); c.code != "sudo grep wenzroot /etc/sudoers" {
		t.Fatalf("Karte 1 darf sich nicht ändern: %q", c.code)
	}
	b.Reset()
	a.viewAnswer(&b, s, agent.VisibleAnswer(q.answer.String()), 70)
	out = ansi.Strip(b.String())
	if strings.Contains(out, "fehlgeschlagen") || !strings.Contains(out, "korrigiert") || !strings.Contains(out, "angepasst wie Karte 2") {
		t.Fatalf("nach u:\n%s", out)
	}
	// eingefügt und mit Enter abgeschickt: Korrektur wird zum Lernen vorgemerkt
	c, _ := a.cardCode(s, 1)
	s.noteAI(c.code, true)
	s.aiPend.learnFrom = q.learnSource(1)
	if from := s.correctionFrom(c.code); from != f.Command {
		t.Fatalf("lernt von %q", from)
	}
	// neue Frage: alles zurück
	q.begin("neu")
	if q.fixes != nil || q.codes != nil {
		t.Fatal("Zustand muss mit der neuen Frage weg")
	}
}

func TestCardFailNotFromCard(t *testing.T) {
	s := &session{}
	s.ask.question = "x"
	s.ask.answer.WriteString("```sh\ndf -h\n```")
	if i := s.ask.failedCard("ls /nix"); i != -1 {
		t.Fatal(i)
	}
	if i := s.ask.failedCard("df   -h"); i != 0 {
		t.Fatal(i)
	}
}
