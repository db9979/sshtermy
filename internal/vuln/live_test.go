package vuln

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestLiveLocal: Skript auf diesem Rechner, Abgleich offline gegen die echte
// OSV-Datenbank. Nur mit SSHTERMY_OSV_LIVE=<Cache-Ordner>.
func TestLiveLocal(t *testing.T) {
	cache := os.Getenv("SSHTERMY_OSV_LIVE")
	if cache == "" {
		t.Skip("SSHTERMY_OSV_LIVE nicht gesetzt")
	}
	cmd := exec.Command("sh", "-s")
	cmd.Stdin = strings.NewReader(Script(true))
	out, _ := cmd.Output()
	inv := ParseInventory(string(out))
	t.Logf("System %s %s, %d Pakete, %d Container, root=%v, laufend=%d, Notizen=%v", inv.OSID, inv.OSVersion, len(inv.Packages), len(inv.Containers), inv.Root, len(inv.Running), inv.Notes)
	for k, v := range inv.Running {
		t.Logf("  läuft %s: %v", k, v)
	}
	for _, c := range inv.Containers {
		t.Logf("  Container %s %s: %s %s %d Pakete %s", c.Name, c.Image, c.Inv.OSID, c.Inv.OSVersion, len(c.Inv.Packages), c.Inv.Ecosystems())
	}
	if os.Getenv("SSHTERMY_OSV_SCAN") == "" {
		return
	}
	// Host ist Ubuntu (DB zu groß für den Test): nur die Container prüfen
	inv.Packages = nil
	src := &Offline{HTTP: HTTP{Cache: cache}}
	kev, err := LoadKEV(context.Background(), &src.HTTP)
	t.Logf("KEV %d %v", len(kev), err)
	start := time.Now()
	r, err := Scan(context.Background(), inv, src, kev, Options{Mode: "offline", Enrich: true, Step: func(s string) { fmt.Println(s) }})
	if err != nil {
		t.Fatal(err)
	}
	c := r.Count(nil)
	t.Logf("%v: %+v Notizen %v", time.Since(start), c, r.Notes)
	for i, f := range r.Findings {
		if i > 25 {
			break
		}
		t.Logf("%-40s %-22s %-28s %-28s %4.1f kev=%v minor=%v cves=%d nofix=%d run=%v", f.Container, f.Package, f.Installed, f.Fixed, f.Score, f.KEV, f.Minor, len(f.CVEs), f.NoFix, f.Running)
	}
}
