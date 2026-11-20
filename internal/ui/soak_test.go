package ui

import (
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/devfake"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/store"
)

// TestSoak ist der Dauerlauf (make soak, nachts in CI): immer wieder Tabs zu
// einem Test-Switch öffnen, Befehle tippen, Ausgabe abwarten, die Hälfte
// der Verbindungen abreißen lassen und alle Tabs schließen. Am Ende dürfen
// Hintergrundabläufe (Goroutinen) und Speicher nicht gewachsen sein, sonst
// hätte sshTermy nach Tagen im Betrieb ein Leck.
//
//	SSHTERMY_SOAK=30m go test -run TestSoak -v ./internal/ui
func TestSoak(t *testing.T) {
	d := os.Getenv("SSHTERMY_SOAK")
	if d == "" {
		t.Skip("Dauerlauf: make soak")
	}
	dur, err := time.ParseDuration(d)
	if err != nil {
		t.Fatalf("SSHTERMY_SOAK=%q: %v", d, err)
	}
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	sw, err := devfake.StartSwitch("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sw.Close()
	a, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	a.w, a.h, a.scr = 140, 40, scrTerminal
	stop := make(chan struct{})
	defer close(stop)
	go func() { // Rückfragen beantworten, Ausgabe-Meldungen verwerfen
		for {
			select {
			case m := <-a.events:
				if hk, ok := m.(hostKeyAskMsg); ok {
					hk.reply <- true
				}
			case <-stop:
				return
			}
		}
	}()
	host, port, _ := net.SplitHostPort(sw.Addr)
	p, _ := strconv.Atoi(port)
	c, err := st.UpsertConnection(domain.Connection{Alias: "sw-dauer", Host: host, Port: p, Username: "manager",
		Auth: domain.AuthPassword, Password: "x", GuardOff: true})
	if err != nil {
		t.Fatal(err)
	}

	const tabs = 8
	nextID := 1
	round := func() {
		var open []*session
		for i := 0; i < tabs; i++ {
			s := &session{id: nextID, conn: c, chipArmed: -1}
			nextID++
			a.sessions = append(a.sessions, s)
			msg := a.connectCmd(s)()
			if e, ok := msg.(connectErrMsg); ok {
				t.Fatal(e.err)
			}
			a.Update(msg)
			open = append(open, s)
		}
		for _, s := range open {
			waitFor(t, func() bool { return strings.HasSuffix(strings.TrimSpace(s.term.CursorLine()), "#") })
			s.term.SendText("show system\r")
		}
		for _, s := range open {
			waitFor(t, func() bool { return strings.Contains(s.term.PlainLines(40), "Software revision") })
		}
		for i, s := range open {
			if i%2 == 0 && s.client != nil {
				_ = s.client.Close() // Verbindung reißt ab
			}
			a.closeTab(s)
		}
	}
	settle := func() (int, uint64) {
		var ms runtime.MemStats
		for i := 0; i < 3; i++ {
			time.Sleep(200 * time.Millisecond)
			runtime.GC()
		}
		runtime.ReadMemStats(&ms)
		return runtime.NumGoroutine(), ms.HeapAlloc
	}

	for i := 0; i < 3; i++ { // Aufwärmen: Puffer, Caches, Pools
		round()
	}
	g0, h0 := settle()
	rounds := 0
	for end := time.Now().Add(dur); time.Now().Before(end); rounds++ {
		round()
	}
	// Aufräumen braucht einen Moment (Keepalive, Lesen enden nach Close)
	var g1 int
	var h1 uint64
	for i := 0; i < 20; i++ {
		if g1, h1 = settle(); g1 <= g0+tabs {
			break
		}
	}
	t.Logf("Dauerlauf %s: %d Runden à %d Tabs; Goroutinen %d → %d; Heap %.1f → %.1f MB",
		dur, rounds, tabs, g0, g1, float64(h0)/1e6, float64(h1)/1e6)
	if g1 > g0+tabs {
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		t.Fatalf("Goroutinen-Leck: %d → %d\n%s", g0, g1, buf[:n])
	}
	if h1 > 2*h0+32<<20 {
		t.Fatalf("Speicher wächst: %.1f → %.1f MB", float64(h0)/1e6, float64(h1)/1e6)
	}
}
