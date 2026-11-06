package termemu

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Kompakter Verlauf: jede Zelle kommt so zurück, wie sie hineinging.
func TestScrollbackRoundTrip(t *testing.T) {
	red := uv.Style{Fg: ansi.Red, Attrs: uv.AttrBold}
	rgb := uv.Style{Fg: ansi.IndexedColor(208), Bg: ansi.Black, Underline: uv.UnderlineCurly}
	link := uv.Link{URL: "https://example.org", Params: "id=1"}
	line := uv.Line{
		{Content: "a", Width: 1},
		{Content: "ä", Width: 1, Style: red},
		{Content: "漢", Width: 2, Style: rgb}, {},
		{Content: "👍🏽", Width: 2, Link: link}, {},
		{Content: " ", Width: 1, Style: red},
		{Content: "é", Width: 1},
		{Content: "\x00", Width: 1},
	}
	sb := newScrollback(10)
	sb.push(line)
	for x, want := range line {
		got, ok := sb.cell(x, 0)
		if !ok || got != want {
			t.Fatalf("Spalte %d: %+v statt %+v", x, got, want)
		}
	}
	if _, ok := sb.cell(len(line), 0); ok {
		t.Fatal("hinter dem Zeilenende darf keine Zelle sein")
	}
	if got := sb.line(0).String(); got != line.String() {
		t.Fatalf("Text %q statt %q", got, line.String())
	}
}

// Mehr als 255 Stile in einer Zeile: Ausweichweg mit vollen Zellen.
func TestScrollbackManyStyles(t *testing.T) {
	var line uv.Line
	for i := 0; i < 300; i++ {
		line = append(line, uv.Cell{Content: "x", Width: 1, Style: uv.Style{Fg: ansi.RGBColor{R: uint8(i), G: uint8(i >> 8)}}})
	}
	sb := newScrollback(10)
	sb.push(line)
	for x := range line {
		if got, _ := sb.cell(x, 0); got != line[x] {
			t.Fatalf("Spalte %d: %+v", x, got)
		}
	}
}

// Ringpuffer: die ältesten Zeilen fallen heraus, Reihenfolge bleibt.
func TestScrollbackRingAndTerm(t *testing.T) {
	tm := New(40, 5, io.Discard)
	defer tm.Close()
	var b strings.Builder
	for i := 0; i < ScrollbackLines+1000; i++ {
		fmt.Fprintf(&b, "\x1b[3%dmzeile %d\x1b[0m\r\n", i%8, i)
	}
	tm.Write([]byte(b.String()))
	if n := tm.ScrollbackLen(); n != ScrollbackLines {
		t.Fatalf("Verlauf %d Zeilen, erwartet %d", n, ScrollbackLines)
	}
	// 5 Bildschirmzeilen: die letzte ist leer (Cursor), davor stehen 4
	first := ScrollbackLines + 1000 - 4 - ScrollbackLines
	if got := tm.PlainLine(0); got != fmt.Sprintf("zeile %d", first) {
		t.Fatalf("älteste Zeile %q, erwartet zeile %d", got, first)
	}
	if got := tm.PlainLine(ScrollbackLines - 1); got != fmt.Sprintf("zeile %d", first+ScrollbackLines-1) {
		t.Fatalf("jüngste Verlaufszeile %q", got)
	}
	if c, ok := tm.cellAt(tm.ScrollbackLen(), Pos{0, 0}); !ok || c.Style.Fg != ansi.BasicColor((first)%8) {
		t.Fatalf("Farbe im Verlauf verloren: %+v", c)
	}
	if ms := tm.Search(fmt.Sprintf("zeile %d", first+10), false); len(ms) != 1 || ms[0].Line != 10 {
		t.Fatalf("Suche im Verlauf: %+v", ms)
	}
	if v := tm.Render(ScrollbackLines, nil); !strings.Contains(v, fmt.Sprintf("zeile %d", first)) {
		t.Fatalf("Zurückblättern zeigt nicht die älteste Zeile: %q", strings.Split(ansi.Strip(v), "\n")[0])
	}
	if !strings.Contains(tm.PlainLines(3), fmt.Sprintf("zeile %d", ScrollbackLines+999)) {
		t.Fatal("PlainLines ohne letzte Zeile")
	}

	// ESC[3J (z. B. „clear“ in neueren Shells) leert auch den Verlauf
	tm.Write([]byte("\x1b[H\x1b[2J\x1b[3J"))
	if n := tm.ScrollbackLen(); n != 0 {
		t.Fatalf("nach ESC[3J noch %d Zeilen", n)
	}
	// ESC[2J allein schiebt den Bildschirm in den Verlauf (wie xterm)
	tm.Write([]byte("eins\r\nzwei\x1b[2J"))
	if n := tm.ScrollbackLen(); n == 0 {
		t.Fatal("ESC[2J soll den Bildschirm in den Verlauf schieben")
	}
}

// Speicher: ein voller Verlauf (5000 Zeilen à ~150 Zeichen, farbig) darf
// nur wenige MB kosten. Vorher waren es rund 59 MB pro Tab.
func TestScrollbackMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("misst Speicher")
	}
	mb := sessionHeapMB(t, 2)
	t.Logf("%.1f MB pro Tab mit vollem Verlauf", mb)
	if mb > 20 {
		t.Fatalf("%.1f MB pro Tab, erwartet unter 20", mb)
	}
}

// sessionHeapMB: Heap pro Terminal mit vollem Verlauf.
func sessionHeapMB(tb testing.TB, n int) float64 {
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	big := []byte(strings.Repeat(benchLine, ScrollbackLines+100))
	terms := make([]*Term, n)
	for i := range terms {
		terms[i] = New(160, 42, io.Discard)
		terms[i].Write(big)
	}
	runtime.GC()
	runtime.ReadMemStats(&m1)
	for _, tm := range terms {
		if tm.ScrollbackLen() != ScrollbackLines {
			tb.Fatalf("Verlauf %d", tm.ScrollbackLen())
		}
		tm.Close()
	}
	return float64(m1.HeapAlloc-m0.HeapAlloc) / 1e6 / float64(n)
}

// typische farbige Ausgabezeile (ls --color, Logs): ~150 Zeichen
var benchLine = strings.Repeat("\x1b[32mhello\x1b[0m world \x1b[1;34m12345\x1b[0m ", 6) + "\r\n"

// BenchmarkWrite: Durchsatz bei durchlaufender Ausgabe (cat großer Datei).
func BenchmarkWrite(b *testing.B) {
	big := []byte(strings.Repeat(benchLine, 1000))
	tm := New(160, 42, io.Discard)
	defer tm.Close()
	b.SetBytes(int64(len(big)))
	b.ReportAllocs()
	for b.Loop() {
		tm.Write(big)
	}
}

// BenchmarkSessionMemory: MB pro Tab mit vollem Verlauf (Metrik MB/tab).
func BenchmarkSessionMemory(b *testing.B) {
	var mb float64
	for b.Loop() {
		mb = sessionHeapMB(b, 4)
	}
	b.ReportMetric(mb, "MB/tab")
}

// BenchmarkRenderScrolled: ein Bild beim Zurückblättern im Verlauf.
func BenchmarkRenderScrolled(b *testing.B) {
	tm := New(160, 42, io.Discard)
	defer tm.Close()
	tm.Write([]byte(strings.Repeat(benchLine, ScrollbackLines)))
	b.ReportAllocs()
	for b.Loop() {
		_ = tm.Render(2000, nil)
	}
}
