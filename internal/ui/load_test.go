package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/broadcast"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/sshx"
	"github.com/db9979/sshtermy/internal/store"
	"github.com/db9979/sshtermy/internal/syncx"
)

// Lasttest mit 1000 Hosts: make loadtest (SSHTERMY_LOAD=1). Misst die
// Stellen, die mit der Zahl der Verbindungen wachsen, und schlägt fehl,
// wenn eine davon spürbar zäh wird. Die Grenzen sind großzügig (langsame
// CI-Rechner); die gemessenen Zeiten stehen im Testprotokoll.

const loadHosts = 1000

type loadFake struct{}

func (loadFake) NetDevice() bool { return false }
func (loadFake) Learned() bool   { return false }
func (loadFake) Close() error    { return nil }
func (loadFake) Exec(context.Context, string) (sshx.RunResult, error) {
	time.Sleep(2 * time.Millisecond)
	return sshx.RunResult{Output: []byte(strings.Repeat("Filesystem 1K-blocks Used Available Use% Mounted on\n", 20))}, nil
}
func (loadFake) RunInShell(context.Context, []string) (sshx.RunResult, error) {
	return sshx.RunResult{}, nil
}

func TestLoad1000(t *testing.T) {
	if os.Getenv("SSHTERMY_LOAD") == "" {
		t.Skip("Lasttest: make loadtest")
	}
	type row struct {
		what  string
		took  time.Duration
		limit time.Duration
	}
	var rows []row
	measure := func(what string, limit time.Duration, runs int, f func()) {
		t.Helper()
		ds := make([]time.Duration, runs)
		for i := range ds {
			t0 := time.Now()
			f()
			ds[i] = time.Since(t0)
		}
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		rows = append(rows, row{what, ds[len(ds)/2], limit}) // Median
	}

	var a *App
	measure("Store: 1000 Verbindungen anlegen", 5*time.Second, 1, func() { a = manyConnsApp(t, loadHosts) })
	measure("Store: öffnen und entsperren", 5*time.Second, 1, func() {
		st, err := store.Open()
		if err != nil {
			t.Fatal(err)
		}
		if err := st.V.UnlockPassword([]byte("correct horse")); err != nil {
			t.Fatal(err)
		}
		if err := st.AfterUnlock(); err != nil {
			t.Fatal(err)
		}
		if n := len(st.Connections()); n != loadHosts {
			t.Fatalf("%d Verbindungen", n)
		}
	})
	measure("Manager: ein Bild", 60*time.Millisecond, 15, func() { a.View() })
	a.mgr.search.SetValue("srv-9")
	measure("Manager: Bild mit Suchtext", 60*time.Millisecond, 15, func() { a.View() })
	a.mgr.search.SetValue("")
	measure("Palette: öffnen + unscharf suchen", 100*time.Millisecond, 9, func() {
		a.openPalette()
		a.pal.in.SetValue("k5 st1 r3")
		_ = a.paletteItems()
		a.View()
		a.pal = nil
	})
	measure("Verbindung speichern (eine ändern)", 300*time.Millisecond, 5, func() {
		c := a.st.Connections()[500]
		c.Tags = append(c.Tags, "x")
		if _, err := a.st.UpsertConnection(c); err != nil {
			t.Fatal(err)
		}
	})

	// Online-Punkt: 1000 Adressen, Verbindung wird sofort abgewiesen.
	conns := a.st.Connections()
	probe := make([]domain.Connection, len(conns))
	for i, c := range conns {
		c.Host, c.Port = "127.0.0.1", 20000+i
		probe[i] = c
	}
	measure("Online-Prüfung: 1000 Adressen", 15*time.Second, 1, func() {
		if n := len(checkReach(probe)); n != loadHosts {
			t.Fatalf("%d Ergebnisse", n)
		}
	})

	// Broadcast an alle 1000 (Ausführung simuliert) und das Bild dazu.
	var rs []broadcast.Result
	measure("Broadcast: 1000 Hosts (je 2 ms, 32 parallel)", 10*time.Second, 1, func() {
		rs = broadcast.Run(context.Background(), broadcast.Job{Conns: conns, Steps: []string{"df -h"}, Parallel: 32},
			func(context.Context, domain.Connection) (broadcast.Executor, error) { return loadFake{}, nil }, func(int, broadcast.Result) {})
	})
	if ok, failed, _ := broadcast.Summary(rs); ok != loadHosts || failed != 0 {
		t.Fatalf("Broadcast: %d ok, %d Fehler", ok, failed)
	}
	a.openBroadcast()
	a.bc.targets, a.bc.steps, a.bc.results, a.bc.phase = conns, []string{"df -h"}, rs, bcRun
	measure("Broadcast: Ergebnisbild", 100*time.Millisecond, 9, func() { a.View() })
	a.scr = scrManager

	// Sync: zwei Stände mit je 1000 Verbindungen zusammenführen.
	d, _, err := a.st.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	measure("Sync: 1000 + 1000 zusammenführen", 2*time.Second, 3, func() {
		p := syncx.FromData(d, nil)
		q := syncx.FromData(d, nil)
		if m := syncx.Merge(p, q, time.Now()); len(m.Connections) != loadHosts {
			t.Fatalf("Sync: %d", len(m.Connections))
		}
	})

	a.st.UpdateSettings(func(s *domain.Settings) { s.MCP = mcpRead })
	measure("MCP: list_connections", 300*time.Millisecond, 5, func() {
		var out string
		a.onMCP(mcpCallMsg{tool: "list_connections", args: json.RawMessage(`{}`), reply: func(s string, _ error) { out = s }})
		if strings.Count(out, `"alias"`) != loadHosts {
			t.Fatal("MCP-Liste unvollständig")
		}
	})

	var b strings.Builder
	fmt.Fprintf(&b, "\n%-46s %10s %10s\n", "Lasttest mit 1000 Hosts", "Median", "Grenze")
	for _, r := range rows {
		mark := ""
		if r.took > r.limit {
			mark = "  ZU LANGSAM"
			t.Errorf("%s: %v (Grenze %v)", r.what, r.took, r.limit)
		}
		fmt.Fprintf(&b, "%-46s %10s %10s%s\n", r.what, r.took.Round(time.Microsecond*100), r.limit, mark)
	}
	t.Log(b.String())
}

// 1000 nicht antwortende Hosts: mehr gleichzeitig prüfen, längere Pause.
func TestReachScaling(t *testing.T) {
	if reachParFor(10) != reachPar || reachParFor(1000) != 62 || reachParFor(5000) != reachParMax {
		t.Fatalf("parallel: %d %d %d", reachParFor(10), reachParFor(1000), reachParFor(5000))
	}
	round := time.Duration(loadHosts/reachParFor(loadHosts)+1) * reachTimeout // ~34 s
	if p := reachPause(round); p < 3*round || reachPause(time.Second) != reachEvery {
		t.Fatalf("Pause: %v", p)
	}
}
