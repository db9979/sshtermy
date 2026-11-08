package netwatch

import (
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	addrs := []string{"192.168.1.5"}
	w := &Watcher{Addrs: func() []string { return addrs }}
	t0 := time.Now()
	if ev, on := w.Check(t0); ev != None || !on {
		t.Fatal("erster Aufruf")
	}
	if ev, _ := w.Check(t0.Add(3 * time.Second)); ev != None {
		t.Fatal("normaler Takt")
	}
	// zu später Takt (Prozess eingefroren / Schlaf ohne Wanduhr-Sprung)
	if ev, _ := w.Check(t0.Add(60 * time.Second)); ev != Resumed {
		t.Fatal("später Takt nicht als Schlaf erkannt")
	}
	// Wanduhr springt, monotone Uhr kaum: Ruhezustand
	last := w.last
	now := last.Add(3 * time.Second)
	addrs = []string{"10.0.0.7"}
	if ev, _ := w.Check(now); ev != NetChanged {
		t.Fatal("Netzwechsel nicht erkannt")
	}
	addrs = nil
	if ev, on := w.Check(now.Add(3 * time.Second)); ev != NetChanged || on {
		t.Fatalf("offline: %v %v", ev, on)
	}
	if w.Online() {
		t.Fatal("Online trotz keiner Adresse")
	}
}

func TestWallJump(t *testing.T) {
	w := &Watcher{Addrs: func() []string { return []string{"a"} }}
	t0 := time.Now()
	w.Check(t0)
	// Simulierter Schlaf: monotone Uhr +3 s, Wanduhr +2 h. Ohne echten
	// Schlaf lässt sich das nur über w.last nachbilden: last ohne monotone
	// Uhr und 2 h früher → Wanduhr-Differenz groß, Sub nimmt dann die Wanduhr.
	w.last = t0.Round(0).Add(-2 * time.Hour)
	if ev, _ := w.Check(t0); ev != Resumed {
		t.Fatal("Wanduhr-Sprung nicht erkannt")
	}
}

func TestLocalAddrs(t *testing.T) {
	for _, a := range LocalAddrs() {
		if a == "127.0.0.1" || a == "::1" {
			t.Fatalf("Loopback in %v", LocalAddrs())
		}
	}
}
