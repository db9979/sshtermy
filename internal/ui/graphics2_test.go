package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/broadcast"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/netmap"
	"github.com/db9979/sshtermy/internal/sessionlog"
	"github.com/db9979/sshtermy/internal/sshx"
)

func TestBcGridShape(t *testing.T) {
	for _, c := range []struct{ n, w, h, cols, rows int }{
		{1, 130, 30, 1, 1},
		{3, 130, 30, 3, 1},
		{4, 130, 30, 2, 2},
		{9, 130, 30, 3, 3},
		{9, 81, 30, 2, 3}, // mindestens 40 breit
		{9, 80, 30, 1, 3},
	} {
		cols, rows := bcGrid(c.n, c.w, c.h, false)
		if cols != c.cols || rows != c.rows {
			t.Errorf("bcGrid(%d, %d, %d) = %d×%d, erwartet %d×%d", c.n, c.w, c.h, cols, rows, c.cols, c.rows)
		}
	}
	if cols, rows := bcGrid(9, 130, 30, true); cols != 1 || rows != 1 {
		t.Errorf("Zoom: %d×%d", cols, rows)
	}
}

func bcTestRun(a *App) {
	a.openBroadcast()
	var conns []domain.Connection
	var rs []broadcast.Result
	for i, name := range []string{"web-01", "web-02", "db-01", "backup"} {
		conns = append(conns, domain.Connection{ID: name, Alias: name, Host: "10.0.0." + string(rune('1'+i))})
		r := broadcast.Result{ConnID: name, Alias: name, State: broadcast.Done, Output: "Filesystem  Size  Used\n/dev/sda1   40G   21G\n", Start: time.Now().Add(-3 * time.Second), End: time.Now()}
		switch i {
		case 2:
			r.State, r.Exit, r.Err = broadcast.Failed, 1, "permission denied"
		case 3:
			r.State, r.Step, r.End = broadcast.Running, 1, time.Time{}
		}
		rs = append(rs, r)
	}
	a.bc.targets, a.bc.steps, a.bc.results, a.bc.phase, a.bc.running = conns, []string{"df -h"}, rs, bcRun, true
}

func TestBroadcastTiles(t *testing.T) {
	a := dangerApp(t)
	a.w, a.h, a.intro = 130, 36, nil
	bcTestRun(a)
	v := ansi.Strip(a.View().Content)
	for _, want := range []string{"web-01", "web-02", "db-01", "backup", "permission denied", "/dev/sda1"} {
		if !strings.Contains(v, want) {
			t.Errorf("%q fehlt\n%s", want, v)
		}
	}
	if a.animNeed(time.Now()) == 0 {
		t.Error("laufender Broadcast braucht Bilder")
	}
}

func TestXferStatusLine(t *testing.T) {
	a := dangerApp(t)
	x := &xfer{what: "↑ backup.tar", pr: &sshx.Progress{}, start: time.Now().Add(-4 * time.Second)}
	x.pr.Total.Store(100 << 20)
	x.files.Store(3)
	x.pr.Files.Store(1)
	now := time.Now().Add(-2 * time.Second)
	for i := 0; i <= 10; i++ {
		x.speed.sample(now.Add(time.Duration(i)*200*time.Millisecond), int64(i)*2<<20)
	}
	x.pr.Done.Store(20 << 20)
	if r := x.speed.rate(); r < 9<<20 || r > 11<<20 {
		t.Fatalf("Tempo %.0f", r)
	}
	out := ansi.Strip(a.xferStatus(x, 120))
	for _, want := range []string{"20 %", "/s", "noch ", "Datei 2 von 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q fehlt in %q", want, out)
		}
	}
	if n := strings.Count(out, "\n"); n != 1 {
		t.Errorf("%d Zeilenumbrüche", n)
	}
}

func TestGoalTimeline(t *testing.T) {
	a := dangerApp(t)
	s := liveSession(a)
	two := 2
	g := &s.goal
	g.started = time.Now().Add(-10 * time.Second)
	g.steps = []agent.DoneStep{
		{Command: "df -h", Output: "Filesystem\n/dev/sda1 40G 21G 52% /\n"},
		{Command: "rm /var/log/old", Exit: &two, Output: "rm: cannot remove: Permission denied\n"},
		{Command: "logrotate", Skipped: true},
	}
	g.stepAt = []time.Time{g.started.Add(2 * time.Second), g.started.Add(3 * time.Second), g.started.Add(3 * time.Second)}
	g.plan = []string{"journalctl --vacuum-size=200M"}
	g.waiting, g.waitSince = true, time.Now().Add(-5*time.Second)
	var b strings.Builder
	a.viewTimeline(&b, s, 60)
	out := ansi.Strip(b.String())
	for _, want := range []string{"✓ df -h", "→ /dev/sda1", "× rm /var/log/old", "→ Exit 2", "» logrotate", "läuft … 5s", "○ journalctl", "│"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q fehlt\n%s", want, out)
		}
	}
	for _, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w > 60 {
			t.Errorf("zu breit (%d): %q", w, l)
		}
	}
}

func TestActivityCalendar(t *testing.T) {
	a := dangerApp(t)
	now := at(7, 18, 0)
	logs := []*sessionlog.Log{
		tl("1", "cw", "", at(7, 9, 48), 1, 5, false),
		tl("2", "cw", "", at(6, 14, 0), 1, 2, false),
		tl("3", "syn", "", at(2, 22, 0), 1, 9, false),
	}
	var b strings.Builder
	a.activityBlock(&b, logs, 80, at(6, 0, 0), now)
	out := ansi.Strip(b.String())
	for _, want := range []string{"Aktivität", "Tageszeit", "Nov", "Mo", "■"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q fehlt\n%s", want, out)
		}
	}
	perDay, perHour := activityCounts(logs)
	if perDay[dayStart(at(2, 0, 0))] != 9 || perHour[9] != 1 || perHour[22] != 1 {
		t.Fatalf("%v %v", perDay, perHour)
	}
	var none strings.Builder
	a.activityBlock(&none, nil, 80, now, now)
	if none.Len() != 0 {
		t.Fatal("ohne Protokolle kein Kalender")
	}
}

// Bilder fürs Paket 2: SSHTERMY_MOTION_SHOTS=<dir> go test ./internal/ui -run TestGraphicsShots2
func TestGraphicsShots2(t *testing.T) {
	dir := os.Getenv("SSHTERMY_MOTION_SHOTS")
	if dir == "" {
		t.Skip()
	}
	strip := regexp.MustCompile(`\x1b\[\d+z`)
	write := func(name, s string) {
		_ = os.WriteFile(dir+"/"+name+".ansi", []byte(strip.ReplaceAllString(s, "")), 0o644)
	}
	a := dangerApp(t)
	a.w, a.h, a.intro = 130, 36, nil
	t.Setenv("HOME", t.TempDir())
	// Broadcast-Kacheln
	bcTestRun(a)
	write("broadcast-kacheln", a.View().Content)
	// Topologie
	a.scr = scrManager
	nb := func(local, name, remote string) netmap.Neighbor {
		return netmap.Neighbor{LocalPort: local, RemoteName: name, RemotePort: remote}
	}
	devs := []netmap.Device{
		{Alias: "core-sw-01", Host: "10.0.0.1", Neigh: []netmap.Neighbor{nb("1/1/47", "etage1", "49"), nb("1/1/48", "etage2", "49"), nb("1/1/46", "lager", "50")}},
		{Alias: "etage1", Host: "10.0.0.11", Neigh: []netmap.Neighbor{nb("12", "ap-01", ""), nb("50", "lager", "51"), nb("14", "drucker-og", "")}},
		{Alias: "etage2", Host: "10.0.0.12", Neigh: []netmap.Neighbor{nb("3", "ap-02", ""), nb("4", "ap-03", ""), nb("5", "phone-7", ""), nb("6", "ap-04", ""), nb("7", "ap-05", "")}},
		{Alias: "lager", Host: "10.0.0.13", Neigh: []netmap.Neighbor{nb("2", "kamera-tor", "")}},
	}
	a.showTopology(netCache{devs: devs, at: time.Now(), lldp: true})
	write("topologie", a.View().Content)
	a.modal = nil
	// Zeitleiste, Übertragung, Kalender als Ausschnitt
	s := liveSession(a)
	two := 2
	g := &s.goal
	g.started = time.Now().Add(-40 * time.Second)
	g.steps = []agent.DoneStep{
		{Command: "df -h /var", Output: "/dev/sda1 40G 37G 93% /var\n"},
		{Command: "du -sh /var/log/* | sort -h | tail -3", Output: "1.2G /var/log/journal\n"},
		{Command: "rm /var/log/syslog.1", Exit: &two, Output: "rm: cannot remove: Permission denied\n"},
		{Command: "sudo journalctl --vacuum-size=200M", Output: "Freed 1.0G\n"},
	}
	g.stepAt = []time.Time{g.started.Add(2 * time.Second), g.started.Add(5 * time.Second), g.started.Add(6 * time.Second), g.started.Add(18 * time.Second)}
	g.plan = []string{"df -h /var", "systemctl restart rsyslog"}
	g.waiting, g.waitSince = true, time.Now().Add(-3*time.Second)
	var b strings.Builder
	a.viewTimeline(&b, s, 64)
	write("zeitleiste", b.String())
	x := &xfer{what: "↑ backup-2026-11.tar.gz", pr: &sshx.Progress{}, start: time.Now().Add(-9 * time.Second)}
	x.pr.Total.Store(840 << 20)
	x.files.Store(3)
	x.pr.Files.Store(1)
	t0 := time.Now().Add(-48 * time.Second)
	var done int64
	for i := 0; i < 240; i++ {
		done += int64((9 + 3*float64(i%37)/37) * (1 << 20) / 5)
		x.speed.sample(t0.Add(time.Duration(i)*200*time.Millisecond), done)
	}
	x.pr.Done.Store(done % (840 << 20))
	write("uebertragung", a.xferStatus(x, 128))
	var logs []*sessionlog.Log
	now := time.Now()
	for d := 0; d < 150; d++ {
		if (d*7)%5 == 0 || d%3 == 0 {
			continue
		}
		day := now.AddDate(0, 0, -d)
		start := time.Date(day.Year(), day.Month(), day.Day(), 8+(d*5)%12, 0, 0, 0, time.Local)
		logs = append(logs, tl(string(rune('a'+d%26)), "cw", "", start, 5, 1+(d*13)%11, false))
	}
	var cal strings.Builder
	a.activityBlock(&cal, logs, 70, dayStart(now.AddDate(0, 0, -4)), now)
	write("kalender", cal.String())
}
