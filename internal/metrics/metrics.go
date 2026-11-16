// Package metrics: Verlauf der stillen Host-Abfragen (CPU, RAM, Platten)
// über bis zu 8 Tage, verdichtet auf einen Punkt je 10 Minuten. Daraus
// entstehen kleine Kurven für F6 und die Vorwarnung „Platte voll in ~N
// Tagen“ (lineare Schätzung über die letzten 7 Tage).
package metrics

import (
	"math"
	"sort"
	"strings"
	"time"
)

const (
	// Step: höchstens ein Punkt je Step (spätere ersetzen frühere).
	Step = 10 * time.Minute
	// Keep: so lange bleiben Punkte.
	Keep = 8 * 24 * time.Hour
)

// Disk: Belegung eines Dateisystems.
type Disk struct {
	Mount string `json:"m"`
	Used  uint64 `json:"u"`
	Total uint64 `json:"t"`
}

// Sample: ein Messpunkt.
type Sample struct {
	T     int64  `json:"t"` // Unix-Sekunden
	CPU   int    `json:"c"` // %, -1 = unbekannt
	RAM   int    `json:"r"` // %, -1 = unbekannt
	Disks []Disk `json:"d,omitempty"`
}

// History: Messpunkte, älteste zuerst.
type History struct {
	Samples []Sample `json:"s"`
}

// Add hängt einen Punkt an (ersetzt den letzten, wenn er weniger als Step
// zurückliegt) und wirft Punkte älter als Keep weg.
func (h *History) Add(s Sample) {
	n := len(h.Samples)
	if n > 0 && s.T < h.Samples[n-1].T {
		return // Uhr zurückgestellt: nicht durcheinanderbringen
	}
	if n > 0 && time.Duration(s.T-h.Samples[n-1].T)*time.Second < Step {
		prev := h.Samples[n-1]
		// Spitzen nicht verlieren: höchste Auslastung im Fenster behalten
		if prev.CPU > s.CPU {
			s.CPU = prev.CPU
		}
		s.T = prev.T
		h.Samples[n-1] = s
	} else {
		h.Samples = append(h.Samples, s)
	}
	cut := s.T - int64(Keep/time.Second)
	i := sort.Search(len(h.Samples), func(i int) bool { return h.Samples[i].T >= cut })
	if i > 0 {
		h.Samples = append([]Sample(nil), h.Samples[i:]...)
	}
}

// Series: Mittelwert je Abschnitt über [now-span, now]; NaN = keine Daten.
func (h *History) Series(now time.Time, span time.Duration, buckets int, val func(Sample) (float64, bool)) []float64 {
	out := make([]float64, buckets)
	cnt := make([]int, buckets)
	from := now.Add(-span).Unix()
	w := float64(span/time.Second) / float64(buckets)
	for _, s := range h.Samples {
		if s.T < from || s.T > now.Unix() {
			continue
		}
		v, ok := val(s)
		if !ok {
			continue
		}
		b := int(float64(s.T-from) / w)
		if b >= buckets {
			b = buckets - 1
		}
		out[b] += v
		cnt[b]++
	}
	for i := range out {
		if cnt[i] == 0 {
			out[i] = math.NaN()
		} else {
			out[i] /= float64(cnt[i])
		}
	}
	return out
}

// CPU, RAM: Werte für Series.
func CPU(s Sample) (float64, bool) { return float64(s.CPU), s.CPU >= 0 }
func RAM(s Sample) (float64, bool) { return float64(s.RAM), s.RAM >= 0 }

// DiskPct: Belegung eines Einhängepunkts in Prozent.
func DiskPct(mount string) func(Sample) (float64, bool) {
	return func(s Sample) (float64, bool) {
		for _, d := range s.Disks {
			if d.Mount == mount && d.Total > 0 {
				return 100 * float64(d.Used) / float64(d.Total), true
			}
		}
		return 0, false
	}
}

// Mounts: Einhängepunkte im letzten Punkt („/“ zuerst).
func (h *History) Mounts() []string {
	if len(h.Samples) == 0 {
		return nil
	}
	var out []string
	for _, d := range h.Samples[len(h.Samples)-1].Disks {
		out = append(out, d.Mount)
	}
	return out
}

// Full: wann ist die Platte voll? Lineare Schätzung über die letzten 7
// Tage. ok nur bei mindestens 24 h Daten, wachsender Belegung und unter
// einem Jahr.
func (h *History) Full(mount string, now time.Time) (days float64, ok bool) {
	from := now.Add(-7 * 24 * time.Hour).Unix()
	var xs, ys []float64
	var total uint64
	for _, s := range h.Samples {
		if s.T < from {
			continue
		}
		for _, d := range s.Disks {
			if d.Mount == mount && d.Total > 0 {
				xs = append(xs, float64(s.T)/86400)
				ys = append(ys, float64(d.Used))
				total = d.Total
			}
		}
	}
	if len(xs) < 6 || xs[len(xs)-1]-xs[0] < 1 {
		return 0, false
	}
	var mx, my float64
	for i := range xs {
		mx += xs[i]
		my += ys[i]
	}
	mx /= float64(len(xs))
	my /= float64(len(ys))
	var num, den float64
	for i := range xs {
		num += (xs[i] - mx) * (ys[i] - my)
		den += (xs[i] - mx) * (xs[i] - mx)
	}
	if den == 0 {
		return 0, false
	}
	slope := num / den // Bytes je Tag
	if slope <= 0 {
		return 0, false
	}
	last := ys[len(ys)-1]
	days = (float64(total) - last) / slope
	if days < 0 {
		days = 0
	}
	return days, days < 365
}

// Spark: Kurve aus Blockzeichen; Lücken als Leerzeichen. max: 100 für Prozent.
func Spark(vals []float64, max float64) string {
	const bars = "▁▂▃▄▅▆▇█"
	rs := []rune(bars)
	var b strings.Builder
	for _, v := range vals {
		if math.IsNaN(v) {
			b.WriteByte(' ')
			continue
		}
		i := int(v / max * float64(len(rs)))
		if i >= len(rs) {
			i = len(rs) - 1
		}
		if i < 0 {
			i = 0
		}
		b.WriteRune(rs[i])
	}
	return b.String()
}

// Last: letzter Wert einer Reihe (NaN = keiner).
func Last(vals []float64) float64 {
	for i := len(vals) - 1; i >= 0; i-- {
		if !math.IsNaN(vals[i]) {
			return vals[i]
		}
	}
	return math.NaN()
}

// Max: höchster Wert einer Reihe (NaN = keiner).
func Max(vals []float64) float64 {
	m := math.NaN()
	for _, v := range vals {
		if !math.IsNaN(v) && (math.IsNaN(m) || v > m) {
			m = v
		}
	}
	return m
}

// Fill: kurze Lücken (bis n Abschnitte) mit dem Wert davor füllen, damit
// seltene Abfragen keine gestrichelte Kurve ergeben.
func Fill(vals []float64, n int) []float64 {
	out := append([]float64(nil), vals...)
	last, gap := math.NaN(), 0
	for i, v := range out {
		if !math.IsNaN(v) {
			last, gap = v, 0
			continue
		}
		gap++
		if math.IsNaN(last) {
			continue
		}
		// nur füllen, wenn danach innerhalb der Grenze wieder ein Wert kommt
		j := i
		for j < len(out) && math.IsNaN(out[j]) {
			j++
		}
		if j < len(out) && j-i+gap-1 <= n {
			out[i] = last
		}
	}
	return out
}
