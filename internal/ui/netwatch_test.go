package ui

import (
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/netwatch"
)

func TestNetEventRestartsWaitingReconnect(t *testing.T) {
	a := newPaletteTestApp(t)
	a.pal = nil
	s := &session{id: 7, conn: domain.Connection{Alias: "web", Host: "web.example.org"}, state: stReconnect}
	s.rc = reconnState{active: true, attempts: 6, due: time.Now().Add(30 * time.Second)}
	a.sessions = append(a.sessions, s)
	if cmd := a.onNetEvent(netwatch.Resumed); cmd == nil {
		t.Fatal("kein Befehl nach dem Aufwachen")
	}
	if s.rc.attempts != 0 || time.Until(s.rc.due) > time.Second {
		t.Fatalf("Neuverbinden nicht sofort: %d %v", s.rc.attempts, time.Until(s.rc.due))
	}

	// ohne Netz: warten, Versuch zählt nicht
	addrs := []string{}
	a.nw = &netwatch.Watcher{Addrs: func() []string { return addrs }}
	a.nw.Check(time.Now())
	if !a.offlineWait(s) || s.rc.lastErr == "" {
		t.Fatal("ohne Netz muss gewartet werden")
	}
	if a.reconnectTick(s) != nil || s.rc.attempts != 0 {
		t.Fatal("Versuch ohne Netz gestartet")
	}
	local := &session{conn: domain.Connection{Host: "127.0.0.1"}}
	if a.offlineWait(local) {
		t.Fatal("localhost braucht kein Netz")
	}
}
