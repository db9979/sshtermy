package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
)

func testDisks(n int) []hostinfo.Disk {
	base := []hostinfo.Disk{
		{Mount: "/", Total: 100 << 30, Used: 40 << 30, Avail: 55 << 30},
		{Mount: "/boot/efi", Total: 512 << 20, Used: 6 << 20, Avail: 506 << 20},
		{Mount: "/data", Total: 4 << 40, Used: 3900 << 30, Avail: 196 << 30},
		{Mount: "/mnt/sicherung-nas", Total: 8 << 40, Used: 2 << 40, Avail: 6 << 40},
	}
	for i := len(base); i < n; i++ {
		base = append(base, hostinfo.Disk{Mount: "/srv/vol" + string(rune('a'+i)), Total: 100, Used: 10, Avail: 90})
	}
	return base[:n]
}

// Mehrere Platten: jede eigene Zeile, nichts läuft über den Rand, volle
// Platten bleiben sichtbar, auch wenn nicht alle passen.
func TestHostPanelDisks(t *testing.T) {
	a := dangerApp(t)
	for _, w := range []int{24, 32, 44, 60, 90} {
		var b strings.Builder
		a.viewDisks(&b, testDisks(4), w, 90)
		out := ansi.Strip(b.String())
		for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			if ansi.StringWidth(l) > w {
				t.Errorf("w=%d: Zeile zu breit (%d): %q", w, ansi.StringWidth(l), l)
			}
		}
		for _, want := range []string{"Platten", "/data", "96%", "/boot/efi"} {
			if !strings.Contains(out, want) {
				t.Errorf("w=%d: %q fehlt in\n%s", w, want, out)
			}
		}
		if w >= 60 && !strings.Contains(out, "TiB") {
			t.Errorf("w=%d: Größe fehlt\n%s", w, out)
		}
		t.Logf("w=%d\n%s", w, out)
	}

	many := testDisks(12)
	many[11].Used, many[11].Avail = 99, 1 // ganz hinten, aber voll
	shown, hidden := pickDisks(many, 90)
	if len(shown) != maxDisks || hidden != 4 {
		t.Fatalf("%d gezeigt, %d versteckt", len(shown), hidden)
	}
	if shown[0].Mount != "/" || shown[len(shown)-1].Mount != many[11].Mount {
		t.Fatalf("Reihenfolge/volle Platte: %+v", shown)
	}
}

// Manager-Details: bis zu vier Platten als Balken, Rest gezählt.
func TestManagerDisks(t *testing.T) {
	a := dangerApp(t)
	s, _ := typedSession(domain.Connection{ID: "c1", Alias: "nas"}, "$ ", "")
	s.host.srv = &hostinfo.Server{CPUPct: 10, Disks: testDisks(6)}
	a.sessions = []*session{s}
	for _, w := range []int{36, 80} {
		var b strings.Builder
		a.mgrLoad(&b, s.conn, w, time.Now().Add(-time.Hour))
		out := ansi.Strip(b.String())
		if !strings.Contains(out, "/data") || !strings.Contains(out, "+2") {
			t.Errorf("w=%d:\n%s", w, out)
		}
		t.Logf("w=%d\n%s", w, out)
	}
}
