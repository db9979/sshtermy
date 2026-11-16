package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/vuln"
)

func TestFirmwareFolder(t *testing.T) {
	a := vulnTestApp(t)
	add := func(alias, dialect, ver string) domain.Connection {
		c := domain.Connection{Alias: alias, Host: alias, Username: "admin", Group: "Netz", Device: domain.DeviceNetwork}
		if dialect != "" {
			c.Profile = &domain.SystemProfile{Family: domain.FamilyNetwork, Dialect: dialect, Version: ver}
		}
		c, err := a.st.UpsertConnection(c)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	fw := add("fw01", "fortios", "v7.2.5,build1517 (GA)")
	add("fw02", "fortios", "v7.2.8,build1639 (GA)")
	add("sw01", "unifi", "7.0.50")
	add("sw02", "", "")
	oldP, oldK := fwProduct, fwKEV
	defer func() { fwProduct, fwKEV = oldP, oldK }()
	asked := map[string]int{}
	fwProduct = func(*App) func(context.Context, string, func(string)) ([]vuln.NVDCVE, bool, error) {
		return func(_ context.Context, p string, _ func(string)) ([]vuln.NVDCVE, bool, error) {
			asked[p]++
			if p != "cpe:2.3:o:fortinet:fortios" {
				return nil, false, errors.New("falsches Produkt")
			}
			return []vuln.NVDCVE{{ID: "CVE-2024-21762", Score: 9.8, Summary: "out-of-bounds write", Matches: []vuln.NVDMatch{{StartIncl: "7.2.0", EndExcl: "7.2.7"}}}}, false, nil
		}
	}
	fwKEV = func(context.Context) vuln.KEV { return vuln.KEV{"CVE-2024-21762": {}} }

	a.openFirmwareFolder()
	if a.pick == nil {
		t.Fatal("Ordnerwahl fehlt")
	}
	a.pickerChoose(pickItem{value: "Netz"})
	deadline := time.After(5 * time.Second)
	for a.fwchk.busy {
		select {
		case m := <-a.events:
			a.Update(m)
		case <-deadline:
			t.Fatal("keine Antwort")
		}
	}
	if asked["cpe:2.3:o:fortinet:fortios"] != 1 {
		t.Fatalf("Produkt nicht genau einmal gefragt: %v", asked)
	}
	txt := strings.Join(a.modal.lines, "\n")
	for _, w := range []string{"fw01", "7.2.7", "keine NVD-Zuordnung", "Hersteller unbekannt"} {
		if !strings.Contains(txt, w) {
			t.Fatalf("%q fehlt:\n%s", w, txt)
		}
	}
	// fw01 oben (KEV), gespeichert für F6
	if i, j := strings.Index(txt, "fw01"), strings.Index(txt, "fw02"); i > j {
		t.Fatalf("Reihenfolge:\n%s", txt)
	}
	if r := a.storedFirmware(fw.ID); r == nil || r.Counts().KEV != 1 || r.MinFixed != "7.2.7" {
		t.Fatalf("nicht gespeichert: %+v", r)
	}
	// Einzelansicht
	a.fwchk = fwState{single: true, title: "fw01", res: []fwResult{{t: fwTarget{conn: fw, vendor: "fortios", firmware: "7.2.5"}, rep: a.storedFirmware(fw.ID)}}}
	a.showFirmware()
	txt = strings.Join(a.modal.lines, "\n")
	if !strings.Contains(txt, "CVE-2024-21762") || !strings.Contains(txt, "KEV") || !strings.Contains(txt, "fortiguard.com") {
		t.Fatalf("Einzelansicht:\n%s", txt)
	}
	// Sicherheits-Ergebnisse des OSV-Checks enthalten keine Firmware-Einträge
	if len(a.st.VulnReports()) != 0 {
		t.Fatal("Firmware-Bericht taucht im Sicherheitscheck auf")
	}
}
