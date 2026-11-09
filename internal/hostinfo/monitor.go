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
	Mount  string // nur bei Platten: welches Dateisystem
}

// Text: z. B. "CPU 97 % (Grenze 90 %)" oder "Platte /data 95 % (Grenze 90 %)".
func (a Alert) Text() string {
	label := a.Metric.Label()
	if a.Metric == MetricDisk && a.Mount != "" {
		label = i18n.T("Platte") + " " + a.Mount
	}
	return fmt.Sprintf(i18n.T("%s %d %% (Grenze %d %%)"), label, a.Pct, a.Limit)
}

// Same: dieselbe Kennzahl (bei Platten dasselbe Dateisystem).
func (a Alert) Same(b Alert) bool { return a.Metric == b.Metric && a.Mount == b.Mount }

// Has: steht dieselbe Warnung schon in der Liste?
func Has(as []Alert, x Alert) bool {
	for _, a := range as {
		if a.Same(x) {
			return true
		}
	}
	return false
}

// AlertsText: alle Warnungen in einer Zeile.
func AlertsText(as []Alert) string {
	var parts []string
	for _, a := range as {
		parts = append(parts, a.Text())
	}
	return strings.Join(parts, " · ")
}

func check(cpu, ram int, disks []Disk, l Limits) []Alert {
	var out []Alert
	add := func(m Metric, v, lim int, mount string) {
		if v >= 0 && lim > 0 && v >= lim {
			out = append(out, Alert{m, v, lim, mount})
		}
	}
	add(MetricCPU, cpu, l.CPU, "")
	add(MetricRAM, ram, l.RAM, "")
	for _, d := range disks {
		add(MetricDisk, d.Pct(), l.Disk, d.Mount)
	}
	return out
}

// Alerts eines Servers (unbekannte Werte lösen nichts aus). Jede Platte wird
// einzeln gegen die Grenze geprüft.
func (s Server) Alerts(l Limits) []Alert {
	ram := -1
	if s.MemTotal > 0 {
		ram = s.MemPct()
	}
	return check(s.CPUPct, ram, s.DiskList(), l)
}

// Alerts eines Netzwerkgeräts (CPU und Speicher, keine Platte).
func (s Switch) Alerts(l Limits) []Alert {
	return check(s.CPUPct, s.MemPct, nil, l)
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
