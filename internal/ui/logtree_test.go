package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/sessionlog"
)

func tl(id, host, conn string, start time.Time, mins, cmds int, failed bool) *sessionlog.Log {
	l := &sessionlog.Log{ID: id, Host: host, ConnID: conn, Addr: "root@" + host + ":22", Started: start, Ended: start.Add(time.Duration(mins) * time.Minute)}
	for i := 0; i < cmds; i++ {
		l.Entries = append(l.Entries, sessionlog.Entry{At: start.Add(time.Duration(i) * 20 * time.Second), Command: "uptime", Output: "up 3 days", Failed: failed && i == 0})
	}
	return l
}

func at(day, hh, mm int) time.Time { return time.Date(2026, 11, day, hh, mm, 0, 0, time.Local) }

func TestLogRunsMergesFollowUps(t *testing.T) {
	logs := []*sessionlog.Log{ // neueste zuerst
		tl("6", "klebdocker", "", at(5, 13, 36), 1, 1, false),
		tl("5", "klebdocker", "", at(5, 13, 32), 2, 2, false),
		tl("4", "klebdocker", "", at(5, 13, 14), 3, 2, true),
		tl("3", "other", "", at(5, 13, 10), 1, 1, false),
		tl("2", "klebdocker", "", at(5, 13, 8), 1, 1, false),
		tl("1", "klebdocker", "", at(5, 12, 0), 1, 1, false), // > 30 min davor
	}
	runs := logRuns(logs)
	if len(runs) != 4 || len(runs[0]) != 3 || len(runs[2]) != 1 || len(runs[3]) != 1 {
		t.Fatalf("Läufe falsch: %d %v", len(runs), runs)
	}
	if got := timeRange(runs[0]); got != "13:14–13:36" {
		t.Fatalf("timeRange = %q", got)
	}
}

func TestLogBurstsByFolder(t *testing.T) {
	folder := map[string]string{"c1": "Nutanix", "c2": "Nutanix", "c3": "Nutanix", "c4": "Nutanix"}
	f := func(l *sessionlog.Log) string { return folder[l.ConnID] }
	logs := []*sessionlog.Log{
		tl("a", "172.31.5.1", "", at(6, 10, 45), 1, 2, false), // anderer Ordner: nicht in der Gruppe
		tl("b", "Nutanix CVM4", "c4", at(6, 10, 44), 1, 1, false),
		tl("c", "Nutanix CVM3", "c3", at(6, 10, 44), 1, 1, false),
		tl("d", "Nutanix CVM2", "c2", at(6, 10, 43), 1, 1, false),
		tl("e", "Nutanix CVM1", "c1", at(6, 10, 43), 1, 1, false),
		tl("f", "klebnls01", "", at(6, 10, 30), 1, 1, false), // zu weit weg
	}
	b := logBursts(logRuns(logs), f)
	if len(b) != 3 || len(b[1]) != 4 {
		t.Fatalf("Gruppen falsch: %d", len(b))
	}
	if got := groupLabel(flatten(b[1]), f); got != "Nutanix" {
		t.Fatalf("groupLabel = %q", got)
	}
	none := func(*sessionlog.Log) string { return "" }
	if got := groupLabel(flatten(b[1]), none); got != "Nutanix CVM" {
		t.Fatalf("groupLabel ohne Ordner = %q", got)
	}
}

func TestLogTreeDaysAndHosts(t *testing.T) {
	now := at(7, 10, 0)
	logs := []*sessionlog.Log{
		tl("1", "cw", "", at(7, 9, 48), 1, 1, false),
		tl("2", "cw", "", at(7, 9, 40), 1, 1, false),
		tl("3", "syn", "", at(6, 14, 0), 1, 1, false),
		tl("4", "cw", "", at(4, 9, 0), 1, 3, true),
	}
	tr := logTree{logs: logs, now: now, folder: func(*sessionlog.Log) string { return "" }}
	rows := tr.rows()
	// Heute (offen) + 1 Lauf, Gestern (offen) + 1, 04.11. zu
	if len(rows) != 5 || rows[0].kind != lrDay || !rows[0].open || len(rows[1].logs) != 2 || rows[4].open {
		t.Fatalf("Zeilen falsch: %+v", rows)
	}
	tr.open = map[string]bool{rows[1].key: true, rows[4].key: true}
	if rows = tr.rows(); len(rows) != 8 {
		t.Fatalf("aufgeklappt: %d Zeilen", len(rows))
	}
	tr.byHost = true
	tr.open = nil
	rows = tr.rows()
	if len(rows) != 2 || rows[0].label != "cw" || len(rows[0].logs) != 3 {
		t.Fatalf("nach Host falsch: %+v", rows)
	}
	if n, f := rows[0].commands(); n != 5 || f != 1 {
		t.Fatalf("Befehle %d/%d", n, f)
	}
}

// sampleLogs: Verlauf wie in Dominiks Liste (heute, gestern, vorgestern).
func sampleLogs(t *testing.T, g *gold) {
	t.Helper()
	now := time.Now()
	d := func(back, hh, mm int) time.Time {
		x := dayStart(now).AddDate(0, 0, -back)
		return x.Add(time.Duration(hh)*time.Hour + time.Duration(mm)*time.Minute)
	}
	ids := map[string]string{}
	for _, n := range []string{"Nutanix CVM1 RZ1", "Nutanix CVM2 RZ1", "Nutanix CVM3 RZ1", "Nutanix CVM4 RZ2", "Nutanix CVM5 RZ2", "Nutanix CVM6 RZ2", "NuClu3rd", "PrismCentral"} {
		c, err := g.a.st.UpsertConnection(domain.Connection{Alias: n, Host: strings.ToLower(strings.ReplaceAll(n, " ", "-")), Username: "nutanix", Auth: domain.AuthAgent, Group: "Rechenzentrum / Nutanix"})
		if err != nil {
			t.Fatal(err)
		}
		ids[n] = c.ID
	}
	b, err := g.a.logBook()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	add := func(host string, start time.Time, mins int, cmds ...string) {
		n++
		l := &sessionlog.Log{ID: fmt.Sprintf("%s-%04x", start.Format("20060102-150405"), n), Host: host, ConnID: ids[host], Addr: "root@" + host + ":22", Started: start, Ended: start.Add(time.Duration(mins) * time.Minute), System: "Linux"}
		for i, c := range cmds {
			e := sessionlog.Entry{At: start.Add(time.Duration(i*25) * time.Second), Command: c, Output: "ok"}
			if strings.HasPrefix(c, "!") {
				e.Command, e.Failed, e.Reason, e.Output = c[1:], true, "Exit 1", "Error: port 8080 is already allocated"
			}
			if c == "cluster status | grep -c UP" {
				e.Output = "41"
				if host == "NuClu3rd" {
					e.Output = "12"
				}
			}
			l.Entries = append(l.Entries, e)
		}
		if err := b.Save(l); err != nil {
			t.Fatal(err)
		}
	}
	add("clockworkpi", d(0, 9, 48), 4, "uname -r")
	add("clockworkpi", d(0, 9, 40), 1, "sudo reboot")
	add("clockworkpi", d(0, 9, 37), 2, "uname -a", "sudo apt update", "apt list --upgradable", "sudo apt upgrade -y")
	add("Synology", d(0, 9, 36), 1, "df -h")
	add("tars", d(0, 9, 36), 1, "uptime")
	add("clockworkpi", d(0, 7, 45), 1, "htop")
	add("klebdocker", d(1, 14, 10), 1, "docker ps")
	add("meeting.kleiberit.com", d(1, 13, 4), 20, "systemctl status jitsi-videobridge2", "journalctl -u jicofo -n 50", "df -h", "free -m", "top -bn1 | head", "systemctl restart jicofo", "tail -f /var/log/jitsi/jvb.log", "uptime", "ss -tulpn", "exit")
	add("172.31.5.1", d(1, 10, 45), 1, "show version", "show running-config")
	for i, h := range []string{"Nutanix CVM6 RZ2", "Nutanix CVM5 RZ2", "Nutanix CVM4 RZ2", "Nutanix CVM3 RZ1", "Nutanix CVM2 RZ1", "Nutanix CVM1 RZ1", "NuClu3rd", "PrismCentral"} {
		add(h, d(1, 10, 44).Add(-time.Duration(i*20)*time.Second), 1, "cluster status | grep -c UP")
	}
	add("nlscluster", d(1, 10, 30), 1, "pcs status")
	add("klebnls02", d(1, 10, 29), 1, "uptime")
	add("klebnls01", d(1, 10, 29), 1, "uptime")
	add("econ.klebchemie.lan", d(1, 10, 26), 1, "systemctl status econ")
	add("klebdocker", d(1, 9, 55), 1, "docker ps")
	add("172.31.5.29", d(1, 9, 11), 1, "show interfaces brief", "show vlan")
	add("172.31.5.29", d(1, 9, 10), 1, "show lldp neighbors", "show mac-address")
	add("172.31.5.29", d(1, 9, 8), 1, "show version", "!copy running-config tftp", "show log")
	add("klebdocker", d(1, 9, 3), 1, "docker ps")
	add("Kleiberit Spark", d(1, 9, 1), 1, "nvidia-smi")
	add("Kleiberit AI Docker", d(2, 14, 44), 1, "docker compose ps")
	add("klebdocker", d(2, 14, 44), 1, "docker ps")
	add("172.31.5.29", d(2, 13, 40), 2, "show version", "show interfaces brief", "show vlan")
	add("klebdocker", d(2, 13, 36), 1, "docker compose logs -f api")
	add("klebdocker", d(2, 13, 32), 2, "docker ps", "docker compose logs -f api")
	add("klebdocker", d(2, 13, 14), 3, "!docker compose up -d", "docker compose up -d")
	add("klebdocker", d(2, 13, 9), 1, "docker compose pull")
	add("klebdocker", d(2, 13, 8), 1, "df -h /var/lib/docker")
	add("klebdocker", d(2, 13, 6), 1, "docker ps")
}

// TestLogsViewFits: beide Ansichten passen in jede Fenstergröße; mit
// SSHTERMY_SHOTS=<ordner> werden die Bilder als ANSI-Dateien geschrieben.
func TestLogsViewFits(t *testing.T) {
	shots := os.Getenv("SSHTERMY_SHOTS")
	for _, sz := range [][2]int{{64, 18}, {100, 30}, {140, 36}} {
		for _, lang := range []string{"de", "en"} {
			g := newGold(t, lang, sz[0], sz[1], true)
			sampleLogs(t, g)
			g.keys("l")
			check := func(name string) {
				t.Helper()
				v := g.a.View().Content
				if w, h := lipgloss.Width(v), lipgloss.Height(v); w > sz[0] || h != sz[1] {
					t.Errorf("%s %dx%d: Bild ist %dx%d", name, sz[0], sz[1], w, h)
				}
				if sz[0] >= 100 && !strings.Contains(ansi.Strip(v), "klebdocker") {
					t.Errorf("%s: klebdocker fehlt", name)
				}
				if shots != "" {
					p := filepath.Join(shots, fmt.Sprintf("%s-%dx%d-%s.ans", name, sz[0], sz[1], lang))
					if err := os.WriteFile(p, []byte(v), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			check("1-tage")
			g.keys("down", "down", "down", "down", "down", "down", "down", "down", "down", "enter")
			check("2-gruppe")
			g.keys("tab", "down", "down", "down")
			check("3-host")
		}
	}
}
