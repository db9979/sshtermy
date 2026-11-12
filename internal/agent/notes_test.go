package agent

import (
	"strings"
	"testing"
)

func TestAnswerLines(t *testing.T) {
	def := "Antwort des Menschen: %s\n"
	if got := answerLine("Port 8080", def); got != "Antwort des Menschen: Port 8080\n" {
		t.Errorf("normale Antwort: %q", got)
	}
	if got := answerLine(Instruction("nimm apt"), def); !strings.HasPrefix(got, "Anweisung des Menschen") || !strings.Contains(got, "nimm apt") {
		t.Errorf("Anweisung: %q", got)
	}
	if got := answerLine(Notice("Verbindung war weg"), def); !strings.HasPrefix(got, "Hinweis von sshTermy: ") {
		t.Errorf("Hinweis: %q", got)
	}
	p := PriorGoal("Nginx installieren", "läuft auf Port 80", 7)
	if !IsPriorGoal(p) {
		t.Fatal("PriorGoal nicht erkennbar")
	}
	if got := answerLine(p, def); !strings.Contains(got, "Schritte 1 bis 7") || !strings.Contains(got, "Nginx installieren") || !strings.Contains(got, "läuft auf Port 80") {
		t.Errorf("früheres Ziel: %q", got)
	}
	if got := answerLine(Instruction("password=geheim123"), def); strings.Contains(got, "geheim123") {
		t.Error("Anweisung wird nicht geschwärzt")
	}
}

func TestManualAndNoteStatus(t *testing.T) {
	var b strings.Builder
	one := 1
	writeHist(&b, []DoneStep{
		{Command: "uptime", Manual: true, Output: "up 3 days"},
		{Command: "false", Manual: true, Exit: &one},
		{Command: "systemctl restart nginx", Skipped: true, Note: "läuft schon"},
	})
	s := b.String()
	for _, want := range []string{"1. uptime (vom Menschen selbst ausgeführt)", "up 3 days", "2. false (vom Menschen selbst ausgeführt, Exit 1)", "übersprungen, Grund: läuft schon"} {
		if !strings.Contains(s, want) {
			t.Errorf("fehlt %q in\n%s", want, s)
		}
	}
}
