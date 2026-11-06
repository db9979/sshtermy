package hostinfo

import (
	"fmt"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Überwachung light: CPU, RAM und Platte gegen Warnschwellen prüfen.

// Limits: Warnschwellen in Prozent.
type Limits struct{ CPU, RAM, Disk int }

// Metric: welche Kennzahl.
type Metric string

const (
	MetricCPU  Metric = "cpu"
	MetricRAM  Metric = "ram"
	MetricDisk Metric = "disk"
)

// Label für die Anzeige.
func (m Metric) Label() string {
	switch m {
	case MetricCPU:
		return "CPU"
	case MetricRAM:
		return "RAM"
	}
	return i18n.T("Platte /")
}

// Alert: eine Kennzahl liegt auf oder über ihrer Schwelle.
type Alert struct {
	Metric Metric
	Pct    int
	Limit  int
}

// Text: z. B. "CPU 97 % (Grenze 90 %)".
func (a Alert) Text() string {
	return fmt.Sprintf(i18n.T("%s %d %% (Grenze %d %%)"), a.Metric.Label(), a.Pct, a.Limit)
}

// AlertsText: alle Warnungen in einer Zeile.
func AlertsText(as []Alert) string {
	var parts []string
	for _, a := range as {
		parts = append(parts, a.Text())
	}
	return strings.Join(parts, " · ")
}

func check(cpu, ram, disk int, l Limits) []Alert {
	var out []Alert
	add := func(m Metric, v, lim int) {
		if v >= 0 && lim > 0 && v >= lim {
			out = append(out, Alert{m, v, lim})
		}
	}
	add(MetricCPU, cpu, l.CPU)
	add(MetricRAM, ram, l.RAM)
	add(MetricDisk, disk, l.Disk)
	return out
}

// Alerts eines Servers (unbekannte Werte lösen nichts aus).
func (s Server) Alerts(l Limits) []Alert {
	ram, disk := -1, -1
	if s.MemTotal > 0 {
		ram = s.MemPct()
	}
	if s.DiskTotal > 0 {
		disk = s.DiskPct()
	}
	return check(s.CPUPct, ram, disk, l)
}

// Alerts eines Netzwerkgeräts (CPU und Speicher, keine Platte).
func (s Switch) Alerts(l Limits) []Alert {
	return check(s.CPUPct, s.MemPct, -1, l)
}

// Over: liegt die Kennzahl m in der Liste?
func Over(as []Alert, m Metric) bool {
	for _, a := range as {
		if a.Metric == m {
			return true
		}
	}
	return false
}
