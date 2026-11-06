package hostinfo

import (
	"strings"
	"testing"
)

func TestStatPct(t *testing.T) {
	a := "cpu  1000 0 500 8000 500 0 0 0 0 0"
	b := "cpu  1600 0 700 8100 600 0 0 0 0 0" // +600 user +200 sys +100 idle +100 iowait = 1000
	p, ok := statPct(a, b)
	if !ok || p != 80 {
		t.Fatalf("got %d %v, want 80", p, ok)
	}
	if _, ok := statPct(a, a); ok {
		t.Fatal("keine Differenz darf nichts liefern")
	}
	if _, ok := statPct("cpu0 1 2 3 4", b); ok {
		t.Fatal("nur die Summenzeile zählt")
	}
}

func TestParseServerCPU(t *testing.T) {
	out := "cpus=4\nstat1=cpu  100 0 100 800 0 0 0 0\nstat2=cpu  550 0 100 850 0 0 0 0\nmem_kb=1000 50\ndisk_kb=100 95 5\n"
	s, err := ParseServer(out)
	if err != nil {
		t.Fatal(err)
	}
	if s.CPUPct != 90 {
		t.Fatalf("CPU %d, erwartet 90", s.CPUPct)
	}
	al := s.Alerts(Limits{CPU: 90, RAM: 90, Disk: 96})
	if len(al) != 2 || al[0].Metric != MetricCPU || al[1].Metric != MetricRAM {
		t.Fatalf("Warnungen %+v", al)
	}
	if !strings.HasPrefix(s.Summary(), "CPU 90%") {
		t.Fatalf("Zusammenfassung %q", s.Summary())
	}
	// macOS: top
	m, _ := ParseServer("cpus=8\ncpu_top=CPU usage: 3.33% user, 5.0% sys, 91.66% idle\nmem_b=100\n")
	if m.CPUPct != 8 {
		t.Fatalf("macOS CPU %d", m.CPUPct)
	}
	// ohne Messung: unbekannt, keine Warnung
	u, _ := ParseServer("cpus=2\nmem_kb=1000 900\n")
	if u.CPUPct != -1 || len(u.Alerts(Limits{CPU: 1, RAM: 90, Disk: 1})) != 0 {
		t.Fatalf("unbekannt: %+v", u)
	}
}

func TestParseSwitchLoad(t *testing.T) {
	procurve := "  Up Time            : 32 days             Memory   - Total   : 343,068,672\n" +
		"  CPU Util (%)       : 7                   Free    : 34,306,867\n"
	s := ParseSwitch(VendorProCurve, "", procurve, "", "")
	if s.CPUPct != 7 || s.MemPct != 90 {
		t.Fatalf("ProCurve CPU %d RAM %d", s.CPUPct, s.MemPct)
	}
	if al := s.Alerts(Limits{CPU: 90, RAM: 90, Disk: 1}); len(al) != 1 || al[0].Metric != MetricRAM {
		t.Fatalf("Warnungen %+v", al)
	}
	cx := ParseSwitch(VendorArubaCX, "", "CPU Util (%)          : 12\nMemory Usage (%)      : 41\n", "", "")
	if cx.CPUPct != 12 || cx.MemPct != 41 {
		t.Fatalf("CX %+v", cx)
	}
	ci := ParseSwitch(VendorCisco, "", "CPU utilization for five seconds: 23%/0%; one minute: 5%; five minutes: 4%", "", "")
	if ci.CPUPct != 23 || ci.MemPct != -1 {
		t.Fatalf("Cisco %+v", ci)
	}
	none := ParseSwitch(VendorProCurve, "", "", "", "")
	if none.CPUPct != -1 || none.MemPct != -1 || len(none.Alerts(Limits{1, 1, 1})) != 0 {
		t.Fatalf("leer %+v", none)
	}
}
