package ui

import (
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/agent"
)

func TestSimilarCmd(t *testing.T) {
	yes := [][2]string{
		{"show interface brief", "show interfaces brief"},
		{"systemctl status nginx", "service nginx status"},
		{"docker ps", "sudo docker ps"},
		{"apt install htop", "apt-get install htop"},
	}
	no := [][2]string{
		{"show vlans", "show version"},
		{"ip a", "uptime"},
		{"df -h", "df -h"},
	}
	for _, c := range yes {
		if !similarCmd(c[0], c[1]) {
			t.Errorf("%q → %q sollte als Korrektur gelten", c[0], c[1])
		}
	}
	for _, c := range no {
		if similarCmd(c[0], c[1]) {
			t.Errorf("%q → %q ist keine Korrektur", c[0], c[1])
		}
	}
}

func TestCorrectionFrom(t *testing.T) {
	s := &session{}
	// eingefügt und vor dem Enter verändert
	s.noteAI("show interface brief", true)
	if got := s.correctionFrom("show interfaces brief"); got != "show interface brief" {
		t.Fatalf("eingefügt+verändert: %q", got)
	}
	// nur ergänzt: keine Korrektur
	s.noteAI("ls -la", true)
	if got := s.correctionFrom("ls -la /var/log"); got != "" {
		t.Fatalf("ergänzt: %q", got)
	}
	// unverändert ausgeführt und geklappt: nächster Befehl ist keine Korrektur
	s.noteAI("show vlan", true)
	if s.correctionFrom("show vlan") != "" {
		t.Fatal("unverändert")
	}
	s.fail = nil
	if got := s.correctionFrom("show vlans"); got != "" {
		t.Fatalf("ohne Fehlschlag: %q", got)
	}
	// ausgeführt und fehlgeschlagen: eigener ähnlicher Befehl lernt
	s.noteAI("show vlan", false)
	s.fail = &agent.Failure{Command: "show vlan"}
	if got := s.correctionFrom("show vlans"); got != "show vlan" {
		t.Fatalf("nach Fehlschlag: %q", got)
	}
	// zu alt
	s.noteAI("show vlan", true)
	s.aiPend.at = time.Now().Add(-learnWindow - time.Second)
	if got := s.correctionFrom("show vlans"); got != "" {
		t.Fatalf("zu alt: %q", got)
	}
}

func TestFavWorthy(t *testing.T) {
	for _, c := range []string{"docker ps -a", "show interfaces brief", "journalctl -u nginx -n 50"} {
		if !favWorthy(c) {
			t.Errorf("%q sollte taugen", c)
		}
	}
	for _, c := range []string{"ls", "cd /etc", "exit", "clear", "mysql -u root -pGeheim123", "a\nb"} {
		if favWorthy(c) {
			t.Errorf("%q taugt nicht", c)
		}
	}
}
