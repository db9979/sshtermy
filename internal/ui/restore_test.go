package ui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestRestoreTabsAfterUpdate(t *testing.T) {
	a := dangerApp(t)
	saved, err := a.st.UpsertConnection(domain.Connection{Alias: "web", Host: "127.0.0.1", Port: 1, Username: "admin", Auth: domain.AuthPassword, Password: "streng-geheim"})
	if err != nil {
		t.Fatal(err)
	}
	adhoc := domain.Connection{Alias: "10.9.9.9", Host: "10.9.9.9", Port: 22, Username: "root", Auth: domain.AuthPassword, Password: "auch-geheim"}
	a.sessions = []*session{
		{id: 1, conn: saved, state: stConnected, mux: muxState{kind: "tmux", name: "arbeit"}},
		{id: 2, conn: domain.Connection{Alias: "zu"}, state: stClosed},
		{id: 3, conn: adhoc, state: stConnected},
	}
	a.cur = 2
	a.saveRestore()

	b, err := os.ReadFile(restorePath(a.st.Dir))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "geheim") {
		t.Fatalf("Geheimnis in der Datei: %s", b)
	}

	// „neue Version“: frische Oberfläche auf demselben Store
	n, err := New(a.st)
	if err != nil {
		t.Fatal(err)
	}
	n.w, n.h = 120, 40
	if cmd := n.restoreTabs(); cmd == nil {
		t.Fatal("keine Tabs wiederhergestellt")
	}
	if len(n.sessions) != 2 || n.cur != 1 || n.scr != scrTerminal {
		t.Fatalf("Tabs %d, aktiv %d", len(n.sessions), n.cur)
	}
	s0, s1 := n.sessions[0], n.sessions[1]
	if s0.conn.ID != saved.ID || s0.conn.Password.Reveal() != "streng-geheim" {
		t.Fatalf("gespeicherte Verbindung nicht aus dem Store: %+v", s0.conn)
	}
	if !s0.muxPending || s0.mux.attachCmd() != "tmux attach -d -t arbeit" {
		t.Fatalf("tmux nicht vorgemerkt: %+v", s0.mux)
	}
	if s1.conn.Host != "10.9.9.9" || s1.conn.Username != "root" || s1.conn.Password.IsSet() {
		t.Fatalf("Ad-hoc-Verbindung falsch: %+v", s1.conn)
	}
	if _, err := os.Stat(restorePath(a.st.Dir)); !os.IsNotExist(err) {
		t.Fatal("Datei nach dem Wiederherstellen nicht gelöscht")
	}
	if n.restoreTabs() != nil {
		t.Fatal("zweimal wiederhergestellt")
	}
}

func TestRestoreIgnoresOldFile(t *testing.T) {
	dir := t.TempDir()
	b, _ := json.Marshal(restoreState{Saved: time.Now().Add(-2 * time.Hour), Tabs: []restoreTab{{ConnID: "x"}}})
	_ = os.WriteFile(restorePath(dir), b, 0o600)
	if loadRestore(dir, time.Now()) != nil {
		t.Fatal("alte Datei benutzt")
	}
}

// Beim Start dort weitermachen: normales Beenden merkt die Tabs, der nächste
// Start fragt (Standard) und öffnet sie nach „j“.
func TestRestoreSessionAsksOnNextStart(t *testing.T) {
	a := dangerApp(t)
	saved, err := a.st.UpsertConnection(domain.Connection{Alias: "web", Host: "127.0.0.1", Port: 1, Username: "admin", Auth: domain.AuthPassword, Password: "geheim"})
	if err != nil {
		t.Fatal(err)
	}
	a.restoreArmed = true
	a.sessions = []*session{{id: 1, conn: saved, state: stConnected}}
	a.restoreOnQuit()

	n, err := New(a.st)
	if err != nil {
		t.Fatal(err)
	}
	n.w, n.h = 120, 40
	if cmd := n.restoreTabs(); cmd != nil || len(n.sessions) != 0 {
		t.Fatal("ohne Rückfrage geöffnet")
	}
	if n.modal == nil || n.modal.kind != mConfirm || !strings.Contains(n.modal.body, "web") {
		t.Fatalf("keine Rückfrage: %+v", n.modal)
	}
	if cmd := n.modal.onYes(); cmd == nil || len(n.sessions) != 1 || n.sessions[0].conn.ID != saved.ID {
		t.Fatalf("nach j nicht geöffnet: %d Tabs", len(n.sessions))
	}
}

func TestRestoreSessionModes(t *testing.T) {
	a := dangerApp(t)
	saved, _ := a.st.UpsertConnection(domain.Connection{Alias: "db", Host: "127.0.0.1", Port: 1, Username: "u"})
	a.restoreArmed = true
	a.sessions = []*session{{id: 1, conn: saved, state: stConnected}}

	// „immer“: ohne Rückfrage
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.RestoreTabs = restoreAlways })
	a.restoreOnQuit()
	n, _ := New(a.st)
	n.w, n.h = 120, 40
	if n.restoreTabs() == nil || len(n.sessions) != 1 || n.modal != nil {
		t.Fatal("„immer“ hat nicht direkt geöffnet")
	}

	// „nie“: keine Datei, nichts geöffnet
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.RestoreTabs = restoreNever })
	a.restoreSig = ""
	a.restoreOnQuit()
	if _, err := os.Stat(restorePath(a.st.Dir)); !os.IsNotExist(err) {
		t.Fatal("„nie“ schreibt trotzdem")
	}
}

// Die Sicherung im Takt schreibt nur bei Änderungen und erst, wenn die Tabs
// vom letzten Mal angeboten wurden (sonst löscht die leere Liste sie).
func TestRestoreTickWaitsAndSkipsUnchanged(t *testing.T) {
	a := dangerApp(t)
	saved, _ := a.st.UpsertConnection(domain.Connection{Alias: "web", Host: "127.0.0.1", Port: 1, Username: "u"})
	p := restorePath(a.st.Dir)
	b, _ := json.Marshal(restoreState{Reason: restoreSession, Saved: time.Now(), Tabs: []restoreTab{{ConnID: saved.ID}}})
	_ = os.WriteFile(p, b, 0o600)

	a.restoreTick(time.Now()) // noch nicht angeboten: Datei bleibt
	if _, err := os.Stat(p); err != nil {
		t.Fatal("Datei vor dem Angebot gelöscht")
	}
	a.restoreArmed = true
	a.sessions = []*session{{id: 1, conn: saved, state: stConnected}}
	a.restoreTick(time.Now())
	st1, _ := os.Stat(p)
	a.restoreAt = time.Time{}
	time.Sleep(10 * time.Millisecond)
	a.restoreTick(time.Now())
	st2, _ := os.Stat(p)
	if !st1.ModTime().Equal(st2.ModTime()) {
		t.Fatal("unverändert und trotzdem neu geschrieben")
	}
	if loadRestore(a.st.Dir, time.Now().Add(7*24*time.Hour)) == nil {
		t.Fatal("eine Woche alte Sitzung nicht mehr angeboten")
	}
}
