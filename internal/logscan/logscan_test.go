package logscan

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/psremote"
)

var now = time.Date(2026, 11, 10, 10, 15, 0, 0, time.Local)

func unixTS(h, m int) string {
	return fmt.Sprintf("%d.123456", time.Date(2026, 11, 10, h, m, 7, 0, time.Local).Unix())
}

func find(r *Report, part string) *Finding {
	for i := range r.Findings {
		if strings.Contains(r.Findings[i].Title, part) || strings.Contains(r.Findings[i].Key, part) {
			return &r.Findings[i]
		}
	}
	return nil
}

func TestParseUnix(t *testing.T) {
	var b strings.Builder
	b.WriteString("##S meta\nuid=1000\nsudo=1\ngroups=dominik,sudo\n")
	fmt.Fprintf(&b, "now=%d\nup=%d\n", now.Unix(), int(now.Sub(time.Date(2026, 11, 10, 3, 14, 0, 0, time.Local)).Seconds()))
	b.WriteString("##S journal crit\n")
	b.WriteString(unixTS(3, 12) + " web01 kernel: Out of memory: Killed process 2211 (php-fpm8.3) total-vm:2100000kB, anon-rss:1800000kB\n")
	b.WriteString("##S journal err\n")
	for _, m := range []int{57, 58, 59} {
		b.WriteString(unixTS(9, m) + " web01 systemd[1]: nginx.service: Failed with result 'exit-code'.\n")
	}
	b.WriteString("##S journal warn\n")
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&b, "%s web01 sshd[%d]: Failed password for invalid user admin%d from 185.220.101.%d port %d ssh2\n", unixTS(8+i/10, i), 4000+i, i, i, 50000+i)
	}
	for h := 0; h < 10; h++ {
		b.WriteString(unixTS(h, 30) + " web01 systemd-timesyncd[600]: Timeout, no NTP server reachable\n")
	}
	b.WriteString(unixTS(9, 40) + " web01 myapp[77]: warning: cache 42 is slow\n")
	b.WriteString("##S failed\n● php8.3-fpm.service loaded failed failed The PHP 8.3 FastCGI Process Manager\n")
	b.WriteString("##S pkg /var/log/dpkg.log\n2026-11-10 06:25:13 upgrade php8.3-fpm:amd64 8.3.13 8.3.14\n2026-11-10 06:25:20 upgrade php8.3-cli:amd64 8.3.13 8.3.14\n2026-11-08 06:25:20 upgrade old:amd64 1 2\n")
	b.WriteString("##S end\n")

	r := ParseUnix(b.String(), 24, now)
	if r.Lines != 45 {
		t.Errorf("Zeilen = %d", r.Lines)
	}
	oom := find(r, "OOM")
	if oom == nil || oom.Sev != SevCrit || len(oom.Checks) == 0 {
		t.Fatalf("OOM fehlt: %+v", r.Findings)
	}
	if r.Findings[0].Sev != SevCrit {
		t.Errorf("kritisch zuerst: %+v", r.Findings[0])
	}
	if f := find(r, "unit:nginx"); f == nil || f.Count != 3 || f.Checks[0] != "systemctl status nginx" {
		t.Errorf("nginx: %+v", f)
	}
	if f := find(r, "unit:php8.3-fpm"); f == nil || f.Sev != SevErr {
		t.Errorf("fehlgeschlagener Dienst: %+v", f)
	}
	if f := find(r, "authfail"); f == nil || f.Count != 30 || f.Sev != SevWarn {
		t.Errorf("SSH-Fehlversuche nicht zusammengefasst: %+v", f)
	}
	if f := find(r, "Zeitabgleich"); f == nil || f.Count != 10 {
		t.Errorf("NTP: %+v", f)
	}
	if f := find(r, "cache ‹n› is slow"); f == nil {
		t.Errorf("Vorlage ohne Regel fehlt: %+v", r.Findings)
	}
	if len(r.Marks) != 2 || r.Marks[0].Kind != MarkBoot || r.Marks[1].Kind != MarkPkg || !strings.Contains(r.Marks[1].Text, "php8.3-fpm, php8.3-cli") {
		t.Errorf("Marken: %+v", r.Marks)
	}
	if len(r.Notes) != 0 {
		t.Errorf("mit sudo kein Hinweis: %v", r.Notes)
	}
	if !strings.Contains(r.Brief(10), "Speicher voll") {
		t.Errorf("Brief: %s", r.Brief(10))
	}
	if n := len(r.LinesOf(oom.Key)); n != 1 {
		t.Errorf("Zeilen des Musters: %d", n)
	}
}

func TestParseUnixSyslogFile(t *testing.T) {
	out := "##S meta\nuid=1000\nsudo=\ngroups=dominik\n##S file /var/log/system/latest.log\n" +
		"<38>1 2026-11-10T09:12:00+01:00 fw.local sshd 1234 - [meta sequenceId=\"1\"] error: PAM: authentication error for root from 10.0.0.9\n" +
		"<11>1 2026-11-10T09:13:00+01:00 fw.local kernel - - - ada0: Unrecovered read error\n" +
		"Nov 10 09:14:00 fw.local dhcpd: DHCPACK on 10.0.0.5\n"
	r := ParseUnix(out, 24, now)
	if f := find(r, "disk-io"); f == nil || f.Sev != SevCrit {
		t.Errorf("E/A-Fehler: %+v", r.Findings)
	}
	if f := find(r, "authfail"); f == nil {
		t.Errorf("Anmeldung: %+v", r.Findings)
	}
	if len(r.Notes) != 1 {
		t.Errorf("Hinweis auf fehlende Rechte erwartet: %v", r.Notes)
	}
}

func TestParseWindows(t *testing.T) {
	ts := func(h int) int64 { return time.Date(2026, 11, 10, h, 0, 0, 0, time.Local).Unix() }
	var b strings.Builder
	fmt.Fprintf(&b, "E|%d|1|41|Microsoft-Windows-Kernel-Power|Das System wurde neu gestartet, ohne dass es zuvor ordnungsgemäß heruntergefahren wurde.\r\n", ts(2))
	for h := 6; h < 10; h++ {
		fmt.Fprintf(&b, "E|%d|3|153|disk|Der E/A-Vorgang an der logischen Blockadresse 0x%x für Datenträger 1 wurde wiederholt.\n", ts(h), 1000+h)
	}
	fmt.Fprintf(&b, "E|%d|2|7034|Service Control Manager|Der Dienst \"Veeam Backup Service\" wurde unerwartet beendet. Dies ist bereits 3 Mal passiert.\n", ts(9))
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&b, "E|%d|0|4625|Microsoft-Windows-Security-Auditing|4625 administrator @ 10.0.8.%d\n", ts(9), i)
	}
	b.WriteString("S|VeeamBackupSvc|Veeam Backup Service\nS|gupdate|Google Update\n")
	fmt.Fprintf(&b, "B|%d\nU|%d|2026-11 Kumulatives Update für Windows Server 2022 (KB5099999)\nN|%d\n", ts(2)+120, ts(3), now.Unix())
	r := ParseWindows(b.String(), 24, now)
	if f := find(r, "Kernel-Power 41"); f == nil || f.Sev != SevCrit {
		t.Errorf("41: %+v", r.Findings)
	}
	if f := find(r, "windows:disk"); f == nil || f.Count != 4 || f.Sev != SevErr {
		t.Errorf("Disk: %+v", f)
	}
	if f := find(r, "Veeam Backup Service"); f == nil || f.Sev != SevErr {
		t.Errorf("Dienstabsturz: %+v", r.Findings)
	}
	if f := find(r, "4625:administrator"); f == nil || f.Count != 12 {
		t.Errorf("4625: %+v", f)
	}
	if f := find(r, "stopped:VeeamBackupSvc"); f == nil {
		t.Errorf("gestoppter Dienst: %+v", r.Findings)
	}
	if find(r, "gupdate") != nil {
		t.Error("Bedarfsdienst sollte nicht auftauchen")
	}
	if len(r.Marks) != 2 {
		t.Errorf("Marken: %+v", r.Marks)
	}
}

func TestParseProCurve(t *testing.T) {
	var b strings.Builder
	b.WriteString(" ---- Reverse event Log listing: Events Since Boot ----\n")
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&b, " W 11/10/26 09:%02d:12 00077 ports: port 23 is now off-line\n", 10+i*5)
		fmt.Fprintf(&b, " I 11/10/26 09:%02d:15 00076 ports: port 23 is now on-line\n", 10+i*5)
	}
	b.WriteString(" M 11/10/26 08:14:00 00921 loop-protect: port 17 disabled - loop detected on VLAN 20\n")
	b.WriteString(" W 11/10/26 07:55:00 00561 ports: port 31 PoE power denied - insufficient power\n")
	b.WriteString(" I 11/10/26 07:00:00 00422 mgr: SME CONSOLE Session - MANAGER Mode\n")
	b.WriteString(" W 01/01/90 00:00:10 00063 system: Fan failure detected\n")
	ports := " Port  Type      | Intrusion Alert Enabled Status Mode\n ----- --------- + --------- ------- ------ ------\n 1     100/1000T | No        Yes     Up     1000FDx\n 17    100/1000T | No        Yes     Down   1000FDx\n 23    100/1000T | No        Yes     Up     1000FDx\n 40    100/1000T | No        No      Down   1000FDx\n"
	r := ParseNetwork(hostinfo.VendorProCurve, []string{b.String()}, ports, 24, now)
	fl := find(r, "link:23")
	if fl == nil || fl.Sev != SevCrit || !strings.Contains(fl.Title, "flattert") || fl.Count != 16 {
		t.Fatalf("Flattern: %+v", fl)
	}
	if f := find(r, "loop:17"); f == nil || f.Sev != SevCrit {
		t.Errorf("Loop: %+v", r.Findings)
	}
	if f := find(r, "poe"); f == nil {
		t.Errorf("PoE: %+v", r.Findings)
	}
	if f := find(r, "network:hw"); f == nil || !f.Last.IsZero() {
		t.Errorf("Lüfter ohne Uhrzeit: %+v", f)
	}
	if len(r.Ports) != 4 || !r.Ports[0].Up || r.Ports[1].Up {
		t.Errorf("Ports: %+v", r.Ports)
	}
}

func TestParseCiscoForti(t *testing.T) {
	c := "*Nov 10 09:58:01.123: %LINK-3-UPDOWN: Interface GigabitEthernet1/0/5, changed state to down\n" +
		"*Nov 10 09:58:03.456: %SYS-5-CONFIG_I: Configured from console by admin on vty0 (10.0.0.50)\n" +
		"*Nov 10 09:59:00.000: %SEC_LOGIN-4-LOGIN_FAILED: Login failed [user: admin] [Source: 10.0.0.66]\n"
	r := ParseNetwork(hostinfo.VendorCisco, []string{c}, "", 24, now)
	if find(r, "link:GigabitEthernet1/0/5") == nil || len(r.Marks) != 1 || r.Marks[0].Kind != MarkConfig {
		t.Errorf("Cisco: %+v / %+v", r.Findings, r.Marks)
	}
	f := `1: date=2026-11-10 time=07:12:00 eventtime=1 logid="0108037893" type="event" subtype="ha" level="critical" vd="root" logdesc="HA failover" msg="Virtual cluster's member state moved"` + "\n" +
		`2: date=2026-11-10 time=09:47:00 logid="0101037141" type="event" subtype="vpn" level="error" logdesc="IPsec tunnel down" vpntunnel="Filiale-Nord" msg="IPsec phase 2 status change"` + "\n" +
		`3: date=2026-11-10 time=09:48:00 logid="0100032002" type="event" subtype="system" level="alert" logdesc="Admin login failed" user="admin" srcip=203.0.113.9 ui="https(203.0.113.9)"` + "\n"
	r = ParseNetwork(hostinfo.VendorFortiOS, []string{"", "", "", f}, "", 24, now)
	if x := find(r, "network:ha"); x == nil || x.Sev != SevCrit {
		t.Errorf("HA: %+v", r.Findings)
	}
	if x := find(r, "network:vpn"); x == nil {
		t.Errorf("VPN: %+v", r.Findings)
	}
	if x := find(r, "authfail"); x == nil {
		t.Errorf("Admin-Login: %+v", r.Findings)
	}
}

func TestTemplate(t *testing.T) {
	a := Template("Failed password for invalid user admin from 185.220.101.4 port 52144 ssh2")
	b := Template("Failed password for invalid user root from 10.1.1.1 port 1 ssh2")
	if a != b {
		t.Errorf("%q != %q", a, b)
	}
	if Template("worker 0x7f3a2b done, id 3f2504e0-4f89-11d3-9a0c-0305e82c3301") != "worker ‹hex› done, id ‹id›" {
		t.Error(Template("worker 0x7f3a2b done, id 3f2504e0-4f89-11d3-9a0c-0305e82c3301"))
	}
}

func TestCompareAndNoise(t *testing.T) {
	r := &Report{Kind: KindUnix, Hours: 24, At: now, Findings: []Finding{
		{Key: "a", Sev: SevWarn, Title: "A", Count: 60},
		{Key: "b", Sev: SevErr, Title: "B", Count: 2},
		{Key: "c", Sev: SevWarn, Title: "C", Count: 5},
	}}
	prev := &Base{Hours: 24, Items: map[string]BaseItem{
		"a": {N: 6, Sev: SevWarn, Title: "A"},
		"c": {N: 5, Sev: SevWarn, Title: "C"},
		"d": {N: 3, Sev: SevErr, Title: "D"},
		"e": {N: 3, Sev: SevInfo, Title: "E"},
	}}
	gone := Compare(r, prev)
	if len(gone) != 1 || gone[0].Title != "D" {
		t.Errorf("weg: %+v", gone)
	}
	if f := find(r, "A"); f.Status != "more" || f.Factor != 10 {
		t.Errorf("häufiger: %+v", f)
	}
	if f := find(r, "B"); f.Status != "new" {
		t.Errorf("neu: %+v", f)
	}
	ApplyNoise(r, func(k string) bool { return k == "b" })
	if r.Findings[len(r.Findings)-1].Key != "b" || !r.Findings[len(r.Findings)-1].Noise {
		t.Errorf("Rauschen zuletzt: %+v", r.Findings)
	}
	if c, e, w := r.Counts(); c != 0 || e != 0 || w != 2 {
		t.Errorf("Zählung ohne Rauschen: %d %d %d", c, e, w)
	}
	if Compare(r, nil) != nil {
		t.Error("ohne Grundlinie")
	}
}

func TestScriptsHaveHours(t *testing.T) {
	if !strings.Contains(UnixScript(6), "H=6") || !strings.Contains(WindowsScript(6), "AddHours(-6)") {
		t.Error("Zeitraum fehlt im Skript")
	}
	if !strings.Contains(WindowsScript(24), "-split \"`r?`n\"") {
		t.Error("PowerShell-Escape kaputt")
	}
	if len(encodedLen(WindowsScript(168))) > 7000 {
		t.Error("Windows-Skript zu lang für die Kommandozeile")
	}
}

func encodedLen(s string) []byte { return make([]byte, len(s)*2*4/3+4) }

// Das Linux-Skript läuft in einer echten Shell durch (Syntax, Abschnitte).
func TestUnixScriptRuns(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil || runtime.GOOS == "windows" {
		t.Skip("kein sh")
	}
	cmd := exec.Command(sh)
	cmd.Stdin = strings.NewReader(UnixScript(1))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.Contains(string(out), "##S meta") || !strings.Contains(string(out), "##S end") {
		t.Fatalf("Abschnitte fehlen: %s", out)
	}
	r := ParseUnix(string(out), 1, time.Now())
	if r.Kind != KindUnix {
		t.Fatal(r)
	}
}

// Das Windows-Skript läuft in echtem PowerShell (CI: Windows-Runner).
func TestWindowsScriptRuns(t *testing.T) {
	exe, err := exec.LookPath("powershell")
	if err != nil {
		if exe, err = exec.LookPath("pwsh"); err != nil {
			t.Skip("kein PowerShell")
		}
	}
	for _, script := range []string{WindowsScript(1), WindowsSinceScript(time.Now().Add(-time.Hour).Unix(), 3)} {
		line := psremote.CommandLine(script)
		args := strings.Fields(line)[1:]
		out, err := exec.Command(exe, args...).Output()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		if !strings.Contains(string(out), "N|") {
			t.Fatalf("Ausgabe ohne Ende-Zeile: %s", out)
		}
		_ = ParseWindows(string(out), 1, time.Now())
	}
}

func TestWatchLines(t *testing.T) {
	now := time.Unix(1731230000, 0)
	e, ok := UnixWatchLine(`{"__REALTIME_TIMESTAMP":"1731229990123456","PRIORITY":"3","SYSLOG_IDENTIFIER":"kernel","MESSAGE":"Out of memory: Killed process 1234 (java) total-vm:1kB"}`, now)
	if !ok || e.Sev != SevErr || e.Source != "kernel" || e.Time.Unix() != 1731229990 {
		t.Fatalf("journal json: %+v %v", e, ok)
	}
	f, ok := Classify(KindUnix, e)
	if !ok || !strings.Contains(f.Title, "java") || f.Key == "" {
		t.Fatalf("Classify: %+v", f)
	}
	if _, ok := UnixWatchLine(`{"MESSAGE":[1,2,3],"PRIORITY":"3"}`, now); ok {
		t.Fatal("Binärnachricht nicht verworfen")
	}
	// Syslog-Empfang: ProCurve mit BSD-Kopf, Cisco ohne, FortiOS kv
	for _, c := range []struct {
		pkt  string
		want string
		sev  Sev
	}{
		{"<132>Nov 10 08:00:00 sw-buero 00077 ports: port 23 is now off-line", "port 23", SevWarn},
		{"<187>42: *Mar  1 00:01:02: %LINK-3-UPDOWN: Interface GigabitEthernet0/1, changed state to down", "GigabitEthernet0/1", SevErr},
		{`<185>date=2026-11-10 time=08:00:00 logid="0101037131" type="event" subtype="vpn" level="error" logdesc="IPsec tunnel down" msg="tunnel down" vpntunnel="hq"`, "", SevErr},
		{"<11>sshd[99]: error: kex_exchange_identification", "kex", SevErr},
	} {
		e, ok := SyslogPacket(c.pkt, now)
		if !ok || !strings.Contains(e.Line(), c.want) || e.Sev != c.sev || e.Time.IsZero() {
			t.Errorf("%q → %+v (%s)", c.pkt, e, e.Sev.Label())
		}
	}
	if _, ok := SyslogPacket("  ", now); ok {
		t.Fatal("leeres Paket")
	}
}

func TestWatchScriptSyntax(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("kein sh")
	}
	for _, w := range []bool{false, true} {
		if out, err := exec.Command(sh, "-n", "-c", UnixWatchScript(w)).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
}

// Link-Wechsel: Schwere der Zeile zählt (Cisco LINK-3 = Fehler), down
// mindestens Warnung, up bleibt Info.
func TestLinkSeverity(t *testing.T) {
	now := time.Now()
	e, _ := SyslogPacket("<187>42: *Mar  1 00:01:02: %LINK-3-UPDOWN: Interface GigabitEthernet0/1, changed state to down", now)
	if f, ok := Classify(KindNetwork, e); !ok || f.Sev != SevErr || !strings.HasPrefix(f.Key, "network:link:") {
		t.Fatalf("Cisco down: %+v", f)
	}
	down := Entry{Time: now, Sev: SevInfo, Source: "interface", Msg: "ether3 link down"}
	if f, ok := Classify(KindNetwork, down); !ok || f.Sev != SevWarn {
		t.Fatalf("down ohne Schwere: %+v", f)
	}
	up := Entry{Time: now, Sev: SevInfo, Source: "interface", Msg: "ether3 link up (speed 1G, full duplex)"}
	if f, ok := Classify(KindNetwork, up); !ok || f.Sev != SevInfo {
		t.Fatalf("up: %+v", f)
	}
}

func TestMergeSyslog(t *testing.T) {
	now := time.Now()
	r := &Report{Kind: KindNetwork, At: now, Hours: 24}
	r.add(Entry{Time: now.Add(-time.Hour), Sev: SevWarn, Source: "ports", Msg: "port 23 is now off-line"})
	Group(r)
	es := []Entry{
		{Time: now.Add(-time.Hour + 20*time.Second), Sev: SevWarn, Source: "ports", Msg: "port 23 is now off-line"}, // doppelt
		{Time: now.Add(-20 * time.Hour), Sev: SevWarn, Source: "ports", Msg: "port 23 is now off-line"},
		{Time: now.Add(-30 * time.Hour), Sev: SevWarn, Source: "ports", Msg: "port 23 is now off-line"}, // zu alt
	}
	if n := r.MergeSyslog(es); n != 1 || len(r.Entries) != 2 || len(r.Findings) != 1 || r.Findings[0].Count != 2 {
		t.Fatalf("Merge: %d %+v", n, r.Findings)
	}
}
