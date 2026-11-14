package ui

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/metrics"
)

// Verlaufskurven in F6: jede stille Abfrage (auch die Überwachung im
// Hintergrund) legt einen Messpunkt ab (internal/metrics, metrics.enc, nur
// dieser Rechner). w schaltet zwischen 24 Stunden und 7 Tagen. Wächst eine
// Platte gleichmäßig, steht „voll in ~N Tagen“ dabei; unter 14 Tagen kommt
// einmal eine Meldung.

// fullWarnDays: ab hier meldet sich „Platte voll in ~N Tagen“.
const fullWarnDays = 14

type trendState struct {
	hist   map[string]*metrics.History // Verbindungs-ID → Verlauf
	warned map[string]bool             // „ID|Mount“ schon gemeldet
}

func (a *App) metricsHistory(connID string) *metrics.History {
	t := &a.trend
	if t.hist == nil {
		t.hist = map[string]*metrics.History{}
	}
	if h := t.hist[connID]; h != nil {
		return h
	}
	h := &metrics.History{}
	if raw := a.st.Metrics(connID); len(raw) > 0 {
		_ = json.Unmarshal(raw, h)
	}
	t.hist[connID] = h
	return h
}

// sampleOf: Messpunkt aus einer Server-Abfrage. Gesamtgröße wie bei df
// (belegt + frei), damit „voll“ heißt: nichts mehr frei für Benutzer.
func sampleOf(srv hostinfo.Server) metrics.Sample {
	s := metrics.Sample{T: srv.At.Unix(), CPU: srv.CPUPct, RAM: -1}
	if srv.MemTotal > 0 {
		s.RAM = srv.MemPct()
	}
	for i, d := range srv.DiskList() {
		if i >= maxDisks {
			break
		}
		tot := d.Total
		if d.Avail > 0 && d.Used+d.Avail < tot {
			tot = d.Used + d.Avail
		}
		s.Disks = append(s.Disks, metrics.Disk{Mount: d.Mount, Used: d.Used, Total: tot})
	}
	return s
}

// recordMetrics: Messpunkt ablegen. Gespeichert wird nur, wenn ein neuer
// Punkt dazukommt (höchstens alle 10 Minuten je Host).
func (a *App) recordMetrics(connID, alias string, srv hostinfo.Server) {
	if srv.At.IsZero() {
		return
	}
	h := a.metricsHistory(connID)
	n := len(h.Samples)
	var lastT int64
	if n > 0 {
		lastT = h.Samples[n-1].T
	}
	h.Add(sampleOf(srv))
	if m := len(h.Samples); m != n || m > 0 && h.Samples[m-1].T != lastT {
		if raw, err := json.Marshal(h); err == nil {
			_ = a.st.SetMetrics(connID, raw)
		}
	}
	now := srv.At
	t := &a.trend
	if t.warned == nil {
		t.warned = map[string]bool{}
	}
	for _, m := range h.Mounts() {
		days, ok := h.Full(m, now)
		key := connID + "|" + m
		if !ok || days >= fullWarnDays {
			delete(t.warned, key)
			continue
		}
		if !t.warned[key] {
			t.warned[key] = true
			a.pushToast(toastWarn, alias+": "+fullText(m, days))
		}
	}
}

func fullText(mount string, days float64) string {
	if days < 1 {
		return fmt.Sprintf(i18n.T("Platte %s voll in unter einem Tag"), mount)
	}
	return fmt.Sprintf(i18n.T("Platte %s voll in ~%d Tagen"), mount, int(math.Round(days)))
}

// viewTrend: Kurven für CPU, RAM und Platten.
func (a *App) viewTrend(b *strings.Builder, s *session, w int) {
	h := a.metricsHistory(s.conn.ID)
	if len(h.Samples) < 2 {
		return
	}
	st := a.s
	now := time.Now()
	span, name, other := 24*time.Hour, i18n.T("24 h"), i18n.T("7 Tage")
	if s.host.week {
		span, name, other = 7*24*time.Hour, i18n.T("7 Tage"), i18n.T("24 h")
	}
	buckets := min(48, w-10-18)
	if buckets < 8 {
		return
	}
	b.WriteString(st.Muted.Render(fmt.Sprintf(i18n.T("Verlauf %s · w: %s"), name, other)) + "\n")
	line := func(label string, vals []float64, extra string, warn bool) {
		if math.IsNaN(metrics.Max(vals)) {
			return
		}
		tail := fmt.Sprintf(i18n.T(" max %.0f %%"), metrics.Max(vals))
		style := st.Muted
		if extra != "" {
			tail = " " + extra
			if warn {
				style = st.Warn
			}
		}
		b.WriteString(st.Muted.Render(fmt.Sprintf("%-9s ", trunc(label, 9))) + st.Accent.Render(metrics.Spark(vals, 100)) + style.Render(trunc(tail, w-10-buckets)) + "\n")
	}
	line("CPU", metrics.Fill(h.Series(now, span, buckets, metrics.CPU), 3), "", false)
	line("RAM", metrics.Fill(h.Series(now, span, buckets, metrics.RAM), 3), "", false)
	for i, m := range h.Mounts() {
		if i >= 4 {
			break
		}
		vals := metrics.Fill(h.Series(now, span, buckets, metrics.DiskPct(m)), 3)
		extra, warn := "", false
		if last := metrics.Last(vals); !math.IsNaN(last) {
			extra = fmt.Sprintf("%.0f %%", last)
		}
		if days, ok := h.Full(m, now); ok {
			extra += " · " + fmt.Sprintf(i18n.T("voll in ~%d T."), int(math.Round(days)))
			warn = days < fullWarnDays
		}
		line(m, vals, extra, warn)
	}
}
