package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

// termBenchApp: großes Fenster mit voller, bunter Terminalfläche und
// offener Host-Übersicht (wie im eigenen Fenster unter Windows).
func termBenchApp(tb testing.TB, panel bool) *App {
	tb.Setenv("SSHTERMY_HOME", tb.TempDir())
	st, err := store.Open()
	if err != nil {
		tb.Fatal(err)
	}
	if err := st.Init([]byte("correct horse"), false); err != nil {
		tb.Fatal(err)
	}
	a, err := New(st)
	if err != nil {
		tb.Fatal(err)
	}
	a.intro = nil
	a.w, a.h, a.scr = 200, 55, scrTerminal
	s := liveSession(a)
	a.sessions = []*session{s}
	if panel {
		s.panel = panelHost
	}
	a.resize(s)
	var out []byte
	for i := 0; i < 80; i++ {
		out = fmt.Appendf(out, "\x1b[1;34mdrwxr-xr-x\x1b[0m  2 root root 4096 Nov  9 11:%02d \x1b[01;32mdatei-%03d.log\x1b[0m \x1b[33m%d Bytes\x1b[0m\r\n", i%60, i, i*937)
	}
	_, _ = s.term.Write(out)
	return a
}

// Ein Bild im Terminal (läuft bei jeder Ausgabe).
func BenchmarkTerminalView(b *testing.B) {
	a := termBenchApp(b, false)
	a.View()
	b.ReportAllocs()
	for b.Loop() {
		a.View()
	}
}

// Dasselbe mit offener Host-Übersicht und Leistungsanzeige.
func BenchmarkTerminalViewPanelPerf(b *testing.B) {
	a := termBenchApp(b, true)
	a.perf.on = true
	a.View()
	b.ReportAllocs()
	for b.Loop() {
		a.View()
	}
}

// Wie oben, aber vor jedem Bild kommt eine Zeile neue Ausgabe (cat, tail -f).
func BenchmarkTerminalViewOutput(b *testing.B) {
	a := termBenchApp(b, true)
	a.perf.on = true
	s := a.current()
	a.View()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		i++
		_, _ = fmt.Fprintf(s.term, "\x1b[33mZeile %d\x1b[0m mit etwas Text und \x1b[1;32mFarbe\x1b[0m\r\n", i)
		a.View()
	}
}

// stamp legt ein Feld über das Bild, ohne die Zeile zu verschieben, auch
// wenn ein breites Zeichen an der Kante steht; Farben rechts bleiben.
func TestStampKeepsWidth(t *testing.T) {
	base := "\x1b[31mabc\x1b[32mdefgh\x1b[0mij 漢字xyz\nzweite Zeile\nkurz"
	cases := []struct {
		x    int
		want string
	}{
		{2, "abXYefghij 漢字xyz"},
		{11, "abcdefghij XY字xyz"},
		{12, " XY 字xyz"},
		{13, "abcdefghij  XYxyz"},
	}
	for _, c := range cases {
		out := stamp(base, "XY\nXY", c.x, 0, 40)
		lines := strings.Split(out, "\n")
		got := ansi.Strip(lines[0])
		if ansi.StringWidth(got) != ansi.StringWidth(ansi.Strip(strings.Split(base, "\n")[0])) {
			t.Errorf("x=%d: Breite geändert: %q", c.x, got)
		}
		if c.x == 2 && got != c.want {
			t.Errorf("x=%d: %q, erwartet %q", c.x, got, c.want)
		}
		if c.x == 2 && !strings.Contains(lines[0], "\x1b[32m") {
			t.Errorf("Farbe rechts vom Feld verloren: %q", lines[0])
		}
	}
	out := ansi.Strip(stamp(base, "XY\nXY\nXY\nXY", 6, 1, 40))
	if want := "zweiteXYeile\nkurz  XY"; !strings.HasSuffix(out, want) || strings.Count(out, "\n") != 2 {
		t.Errorf("Zeilen 2/3: %q", out)
	}
}

// Das Terminalbild wird gemerkt und erst bei neuer Ausgabe neu gebaut.
func TestTermViewMemo(t *testing.T) {
	a := termBenchApp(t, false)
	s := a.current()
	v1 := a.termView(s, "")
	if !strings.Contains(v1, "datei-079") {
		t.Fatal("Ausgabe fehlt")
	}
	if a.termView(s, "") != v1 {
		t.Error("ohne Ausgabe anderes Bild")
	}
	_, _ = s.term.Write([]byte("neue-zeile\r\n"))
	if v2 := a.termView(s, ""); !strings.Contains(ansi.Strip(v2), "neue-zeile") {
		t.Error("neue Ausgabe nicht im Bild (Gedächtnis veraltet)")
	}
}

// Die Anzeige erklärt sich: Bewertung in Worten, steht neben der F6-Leiste.
func TestPerfBoxReadable(t *testing.T) {
	a := termBenchApp(t, true)
	a.perf.on = true
	now := time.Now()
	for i := 0; i < 50; i++ {
		a.perf.record(now.Add(-time.Duration(50-i)*300*time.Millisecond), 3*time.Millisecond)
	}
	box := ansi.Strip(a.perfBox(now))
	for _, want := range []string{"flüssig", "möglich", "nur bei Änderung", "▂"} {
		if !strings.Contains(box, want) {
			t.Errorf("%q fehlt:\n%s", want, box)
		}
	}
	v := a.View().Content
	if w := lipgloss.Width(v); w > a.w {
		t.Errorf("Bild %d breit, Fenster %d", w, a.w)
	}
	cols, _ := a.paneSize(a.current())
	for _, l := range strings.Split(ansi.Strip(v), "\n") {
		if i := strings.Index(l, "Leistung"); i >= 0 && ansi.StringWidth(l[:i]) > cols {
			t.Errorf("Anzeige liegt über der Seitenleiste: %q", l)
		}
	}
}
