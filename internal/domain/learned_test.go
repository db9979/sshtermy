package domain

import (
	"strings"
	"testing"
	"time"
)

func TestAddLearned(t *testing.T) {
	var p SystemProfile
	now := time.Now()
	if !p.AddLearned("show interface brief", "show interfaces brief", now) {
		t.Fatal("neu muss gemerkt werden")
	}
	if p.AddLearned("show interface brief", "show interfaces brief", now) {
		t.Fatal("gleiche Korrektur ist keine Änderung")
	}
	if p.AddLearned("x", "x", now) || p.AddLearned("", "y", now) {
		t.Fatal("leer oder gleich zählt nicht")
	}
	// neuere Korrektur desselben Befehls ersetzt die alte
	p.AddLearned("show vlan", "show vlans", now)
	p.AddLearned("show interface brief", "show int brief", now)
	if len(p.Learned) != 2 || p.Learned[1].Fixed != "show int brief" {
		t.Fatalf("%+v", p.Learned)
	}
	for i := 0; i < 20; i++ {
		p.AddLearned("a"+strings.Repeat("x", i), "b", now)
	}
	if len(p.Learned) != MaxLearned {
		t.Fatalf("Grenze: %d", len(p.Learned))
	}
	if p.Empty() {
		t.Fatal("Profil mit Korrekturen ist nicht leer")
	}
	if pr := p.Prompt(); !strings.Contains(pr, "Vom Admin korrigiert") || !strings.Contains(pr, "→  b") {
		t.Fatalf("Prompt: %s", pr)
	}
}

func TestMonitorConfig(t *testing.T) {
	c, r, d := MonitorConfig{}.Limits()
	if c != 90 || r != 90 || d != 90 || (MonitorConfig{}).Every() != 5*time.Minute {
		t.Fatal("Standardwerte")
	}
	m := MonitorConfig{CPU: 70, RAM: 150, EveryMin: -1}
	c, r, _ = m.Limits()
	if c != 70 || r != 100 || m.Every() != 0 {
		t.Fatalf("eigene Werte: %d %d %v", c, r, m.Every())
	}
}
