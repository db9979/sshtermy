package metrics

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestAddSeriesFull(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	var h History
	start := now.Add(-9 * 24 * time.Hour)
	const gb = 1 << 30
	for ts := start; !ts.After(now); ts = ts.Add(5 * time.Minute) {
		days := ts.Sub(start).Hours() / 24
		used := uint64((50 + 4*days) * gb) // 4 GB je Tag
		h.Add(Sample{T: ts.Unix(), CPU: int(ts.Hour() * 4), RAM: 60, Disks: []Disk{{Mount: "/", Used: used, Total: 100 * gb}}})
	}
	if first := time.Unix(h.Samples[0].T, 0); now.Sub(first) > Keep {
		t.Fatalf("alte Punkte nicht weg: %v", first)
	}
	// ein Punkt je 10 Minuten
	if d := h.Samples[1].T - h.Samples[0].T; d != 600 {
		t.Fatalf("Abstand %d", d)
	}
	days, ok := h.Full("/", now)
	// heute: 50 + 4*9 = 86 GB → 14 GB frei → 3,5 Tage
	if !ok || math.Abs(days-3.5) > 0.3 {
		t.Fatalf("%v %v", days, ok)
	}
	s := h.Series(now, 24*time.Hour, 24, CPU)
	if math.IsNaN(s[0]) || Max(s) < 80 {
		t.Fatalf("%v", s)
	}
	sp := Spark(s, 100)
	if len([]rune(sp)) != 24 || !strings.ContainsRune(sp, '█') && !strings.ContainsRune(sp, '▇') {
		t.Fatalf("%q", sp)
	}
	if _, ok := h.Full("/data", now); ok {
		t.Fatal("unbekannter Einhängepunkt")
	}
	// gleichbleibend: keine Vorwarnung
	var flat History
	for ts := now.Add(-3 * 24 * time.Hour); !ts.After(now); ts = ts.Add(time.Hour) {
		flat.Add(Sample{T: ts.Unix(), CPU: -1, RAM: -1, Disks: []Disk{{Mount: "/", Used: 10, Total: 100}}})
	}
	if _, ok := flat.Full("/", now); ok {
		t.Fatal("flach meldet")
	}
	if !math.IsNaN(Last(flat.Series(now, time.Hour, 4, CPU))) {
		t.Fatal("CPU unbekannt zählt")
	}
}

func TestFill(t *testing.T) {
	n := math.NaN()
	got := Fill([]float64{n, 1, n, 2, n, n, n, 3, n}, 2)
	want := []float64{n, 1, 1, 2, n, n, n, 3, n}
	for i := range want {
		if math.IsNaN(want[i]) != math.IsNaN(got[i]) || !math.IsNaN(want[i]) && want[i] != got[i] {
			t.Fatalf("%v", got)
		}
	}
}
