package ui

import (
	"fmt"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/store"
)

// Leistung mit vielen Verbindungen: go test -run '^$' -bench . ./internal/ui

// manyConnsApp: App mit n Verbindungen in 30 Ordnern, Manager offen.
func manyConnsApp(tb testing.TB, n int) *App {
	tb.Setenv("SSHTERMY_HOME", tb.TempDir())
	st, err := store.Open()
	if err != nil {
		tb.Fatal(err)
	}
	if err := st.Init([]byte("correct horse"), false); err != nil {
		tb.Fatal(err)
	}
	cs := make([]domain.Connection, n)
	for i := range cs {
		cs[i] = domain.Connection{
			Alias: fmt.Sprintf("srv-%03d", i), Host: fmt.Sprintf("10.%d.%d.%d", i/65536, i/256%256, i%256), Port: 22,
			Username: "admin", Auth: domain.AuthPassword, Group: fmt.Sprintf("Kunde %d / Standort %d", i%10, i%3),
			Tags: []string{"linux", fmt.Sprintf("rack%d", i%7)},
		}
	}
	if _, err := st.AddConnections(cs); err != nil {
		tb.Fatal(err)
	}
	a, err := New(st)
	if err != nil {
		tb.Fatal(err)
	}
	a.intro = nil
	a.w, a.h, a.scr = 160, 45, scrManager
	return a
}

// Ein Bild des Managers mit 500 Verbindungen (läuft bei jeder Taste).
func BenchmarkManagerView500(b *testing.B) {
	a := manyConnsApp(b, 500)
	a.View()
	b.ReportAllocs()
	for b.Loop() {
		a.View()
	}
}

// Manager mit Suchtext: jede Taste filtert alle Verbindungen neu.
func BenchmarkManagerSearch500(b *testing.B) {
	a := manyConnsApp(b, 500)
	a.mgr.search.SetValue("srv-4")
	a.View()
	b.ReportAllocs()
	for b.Loop() {
		a.View()
	}
}

// Die Zeilen müssen auch mit vielen Ordnern stimmen: Suche zeigt nur
// Ordner mit Treffern, zugeklappte Ordner verstecken ihre Einträge.
func TestManagerRowsMany(t *testing.T) {
	a := manyConnsApp(t, 500)
	all := a.mgrRows()
	conns := 0
	for _, r := range all {
		if !r.isFolder() {
			conns++
		}
	}
	if conns != 500 {
		t.Fatalf("%d Verbindungen sichtbar, erwartet 500", conns)
	}
	a.mgr.search.SetValue("srv-123")
	rows := a.mgrRows()
	var names []string
	for _, r := range rows {
		if r.isFolder() {
			names = append(names, "/"+r.node.Path)
		} else {
			names = append(names, r.conn.Alias)
		}
	}
	if len(rows) != 3 || rows[2].conn.Alias != "srv-123" {
		t.Fatalf("Suche: %v", names)
	}
	a.mgr.search.SetValue("")
	if err := a.st.SetCollapsed(true, "Kunde 3"); err != nil {
		t.Fatal(err)
	}
	if got := len(a.mgrRows()); got >= len(all) {
		t.Fatalf("zugeklappt: %d Zeilen, vorher %d", got, len(all))
	}
}
