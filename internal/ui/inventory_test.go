package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
)

func TestInventoryFolder(t *testing.T) {
	a := vulnTestApp(t)
	for _, n := range []string{"db01", "web01"} {
		if _, err := a.st.UpsertConnection(domain.Connection{Alias: n, Host: n, Username: "root", Group: "Prod"}); err != nil {
			t.Fatal(err)
		}
	}
	base := "os=Debian 12\nkernel=6.1.0-18\npkg=nginx|1.22\nuser=root|0|/bin/bash\nport=tcp|0.0.0.0:22|users:((\"sshd\",pid=1,fd=3))\n"
	out := map[string]string{"db01": base, "web01": base}
	old := invRunHost
	defer func() { invRunHost = old }()
	invRunHost = func(*App) func(context.Context, domain.Connection, string) (string, error) {
		return func(_ context.Context, c domain.Connection, _ string) (string, error) { return out[c.Alias], nil }
	}
	run := func() string {
		a.openInventoryFolder()
		a.pickerChoose(pickItem{value: "Prod"})
		pumpEvents(t, a, func() bool { return !a.inv.busy })
		return strings.Join(a.modal.lines, "\n")
	}
	if txt := run(); strings.Count(txt, "erster Stand gespeichert") != 2 {
		t.Fatalf("erster Lauf:\n%s", txt)
	}
	out["db01"] = base + "user=eve|1001|/bin/bash\nadmin=eve|sudo\nport=tcp|0.0.0.0:6379|users:((\"redis-server\",pid=5,fd=6))\n"
	out["web01"] = strings.Replace(base, "nginx|1.22", "nginx|1.24", 1)
	txt := run()
	if !strings.Contains(txt, "3 Änderungen") || !strings.Contains(txt, "+ Benutzer eve (uid 1001") {
		t.Fatalf("zweiter Lauf:\n%s", txt)
	}
	if i, j := strings.Index(txt, "db01"), strings.Index(txt, "web01"); i < 0 || i > j {
		t.Fatalf("db01 nicht oben:\n%s", txt)
	}
	if !strings.Contains(txt, "web01                  1 Paket-Update") {
		t.Fatalf("Paket-Update fehlt:\n%s", txt)
	}
	// CSV
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if _, ok := a.invKey("e"); !ok {
		t.Fatal("e nicht behandelt")
	}
	files, _ := filepath.Glob(filepath.Join(exportDir(), "sshtermy-inventar-*.csv"))
	if len(files) != 1 {
		t.Fatalf("CSV: %v", files)
	}
	raw, _ := os.ReadFile(files[0])
	if strings.Count(string(raw), "host,kategorie") != 1 || !strings.Contains(string(raw), "web01,package,nginx,1.24") {
		t.Fatalf("%s", raw)
	}
}

func TestTrendRecord(t *testing.T) {
	a := vulnTestApp(t)
	now := time.Now()
	const gb = 1 << 30
	for i := 0; i < 48; i++ { // zwei Tage, alle Stunde 1 GB mehr
		at := now.Add(time.Duration(i-47) * time.Hour)
		a.recordMetrics("c1", "web01", hostinfo.Server{At: at, CPUPct: 10 + i, MemTotal: 100, MemFree: 50,
			Disks: []hostinfo.Disk{{Mount: "/", Total: 200 * gb, Used: uint64(100+i) * gb, Avail: uint64(100-i) * gb}}})
	}
	h := a.metricsHistory("c1")
	if len(h.Samples) != 48 {
		t.Fatalf("%d Punkte", len(h.Samples))
	}
	days, ok := h.Full("/", now)
	if !ok || days < 1.5 || days > 3 {
		t.Fatalf("voll in %v %v", days, ok)
	}
	found := false
	for _, x := range a.toasts {
		if strings.Contains(x.text, "Platte / voll in ~") {
			found = true
		}
	}
	if !found {
		t.Fatalf("keine Meldung: %+v", a.toasts)
	}
	// gespeichert?
	a.trend.hist = nil
	if len(a.metricsHistory("c1").Samples) != 48 {
		t.Fatal("nicht gespeichert")
	}
	s := &session{conn: domain.Connection{ID: "c1"}}
	var b strings.Builder
	a.viewTrend(&b, s, 80)
	if out := b.String(); !strings.Contains(out, "Verlauf 24 h") || !strings.Contains(out, "▅▅▅") || !strings.Contains(out, "voll in ~") {
		t.Fatalf("%s", out)
	}
}
