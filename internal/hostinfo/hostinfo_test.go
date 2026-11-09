package hostinfo

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseServer(t *testing.T) {
	out := "cpus=8\nload=0.42 0.30 0.25\nmem_kb=16303412 12500000\ndisk_kb=102400000 44032000 58368000\n" +
		"uptime_s=1054800.12\nnow=1730800000\nupd=apt 5 2\nreboot=1\n"
	s, err := ParseServer(out)
	if err != nil {
		t.Fatal(err)
	}
	if s.CPUs != 8 || s.Load[0] != 0.42 || s.Updates != 5 || s.Security != 2 || !s.RebootNeeded || s.UpdateTool != "apt" {
		t.Fatalf("parse: %+v", s)
	}
	if s.MemPct() != 23 || s.DiskPct() != 43 {
		t.Fatalf("pct mem=%d disk=%d", s.MemPct(), s.DiskPct())
	}
	if got := Duration(s.Uptime); got != "12 T 5 Std" {
		t.Fatalf("uptime %q", got)
	}
	if !strings.Contains(s.Summary(), "↑5") {
		t.Fatalf("summary %q", s.Summary())
	}
}

func TestParseServerMacAndUnknownUpdates(t *testing.T) {
	s, err := ParseServer("cpus=10\nload= 1,52 1,40 1,33 \nmem_b=17179869184\nmemfree_b=4294967296\ndisk_kb=1000 500 500\nboot=1730000000\nnow=1730086400\nupd=dnf ? ?\n")
	if err != nil {
		t.Fatal(err)
	}
	if s.Uptime != 24*time.Hour || s.Updates != -1 || s.MemPct() != 75 || s.Load[0] != 1.52 {
		t.Fatalf("parse: %+v", s)
	}
	if _, err := ParseServer("sh: not found"); err == nil {
		t.Fatal("expected error for empty output")
	}
}

const procurveVersion = `Image stamp:    /ws/swbuildm/rel_ukiah_qaoff/code/build/bom(swbuildm_rel_ukiah_qaoff_rel_ukiah)
                Mar 10 2020 13:56:04
                YA.16.10.0009
                1102
Boot Image:     Primary`

const procurveSystem = ` Status and Counters - General System Information

  System Name        : sw-buero
  System Contact     :
  System Location    :

  MAC Age Time (sec) : 300

  Time Zone          : 0
  Daylight Time Rule : None

  Software revision  : YA.16.10.0009        Base MAC Addr      : 941882-123456
  ROM Version        : YA.15.20            Serial Number      : CN12345678

  Up Time            : 32 days             Memory   - Total   : 343,068,672`

const procurvePorts = ` Status and Counters - Port Status

                          | Intrusion                           Flow  Bcast
  Port         Type       | Alert     Enabled Status Mode       Ctrl  Limit
  ------------ ---------- + --------- ------- ------ ---------- ----- ------
  1            100/1000T  | No        Yes     Up     1000FDx    off   0
  2            100/1000T  | No        Yes     Down   1000FDx    off   0
  3            100/1000T  | No        Yes     Down   1000FDx    off   0
  25           SFP+SR     | No        Yes     Up     10GigFD    off   0
  Trk1         Trunk      | No        Yes     Up     10GigFD    off   0`

const procurveVLANs = ` Status and Counters - VLAN Information

  Maximum VLANs to support : 256
  Primary VLAN : DEFAULT_VLAN
  Management VLAN :

  VLAN ID Name                             | Status     Voice Jumbo
  ------- -------------------------------- + ---------- ----- -----
  1       DEFAULT_VLAN                     | Port-based No    No
  10      Server                           | Port-based No    No
  20      Clients                          | Port-based No    No`

func TestProCurve(t *testing.T) {
	v := DetectVendor(procurveVersion)
	if v != VendorProCurve || v.PagingOff() != "no page" {
		t.Fatalf("vendor %q", v)
	}
	s := ParseSwitch(v, procurveVersion, procurveSystem, procurvePorts, procurveVLANs)
	if s.Firmware != "YA.16.10.0009" || s.Uptime != "32 days" || s.Name != "sw-buero" {
		t.Fatalf("fields: %+v", s)
	}
	if s.PortsUp != 3 || s.PortsDown != 2 {
		t.Fatalf("ports up=%d down=%d", s.PortsUp, s.PortsDown)
	}
	if len(s.VLANs) != 3 || s.VLANs[1].ID != 10 || s.VLANs[1].Name != "Server" {
		t.Fatalf("vlans %+v", s.VLANs)
	}
	if m := ModelFromConfig("Running configuration:\n\n; J9773A Configuration Editor; Created on release #YA.16.10.0009\n"); m != "J9773A" {
		t.Fatalf("model %q", m)
	}
}

func TestArubaCXAndCisco(t *testing.T) {
	cxVer := "-----------------------------------------------------------------------------\nArubaOS-CX\n(c) Copyright 2017-2020 Hewlett Packard Enterprise Development LP\n-----------------------------------------------------------------------------\nVersion      : FL.10.04.3000\nBuild Date   : 2020-01-01 00:00:00 UTC\n"
	cxSys := "Hostname          : core-01\nProduct Name      : JL658A 6300M 24SFP+ 4SFP56 Swch\nUp Time           : 3 weeks, 2 days, 4 hours\n"
	cxPorts := "--------------------------------------------------------------------------------------------------------\nPort      Native  Mode   Type           Enabled Status  Reason                  Speed   Description\n          VLAN                                                                  (Mb/s)\n--------------------------------------------------------------------------------------------------------\n1/1/1     1       access 10G-SFP+SR     yes     up                              10000   --\n1/1/2     1       access --             yes     down    Waiting for link        --      --\n"
	cxVLAN := "--------------------------------------------------------------------------------------------\nVLAN  Name                              Status  Reason          Type      Interfaces\n--------------------------------------------------------------------------------------------\n1     DEFAULT_VLAN_1                    up      ok              default   1/1/1-1/1/28\n100   mgmt                              up      ok              static    1/1/1\n"
	v := DetectVendor(cxVer)
	s := ParseSwitch(v, cxVer, cxSys, cxPorts, cxVLAN)
	if v != VendorArubaCX || s.Firmware != "FL.10.04.3000" || s.Name != "core-01" || !strings.HasPrefix(s.Model, "JL658A") ||
		s.PortsUp != 1 || s.PortsDown != 1 || len(s.VLANs) != 2 {
		t.Fatalf("cx: %q %+v", v, s)
	}

	ciscoVer := "Cisco IOS Software, C2960X Software (C2960X-UNIVERSALK9-M), Version 15.2(7)E3, RELEASE SOFTWARE (fc3)\nsw-lager uptime is 3 weeks, 2 days, 1 hour, 5 minutes\ncisco WS-C2960X-48FPD-L (APM86XXX) processor (revision A0) with 524288K bytes of memory.\n"
	ciscoPorts := "Port      Name               Status       Vlan       Duplex  Speed Type\nGi1/0/1   Drucker            connected    10         a-full a-1000 10/100/1000BaseTX\nGi1/0/2                      notconnect   1            auto   auto 10/100/1000BaseTX\n"
	ciscoVLAN := "VLAN Name                             Status    Ports\n---- -------------------------------- --------- -------------------------------\n1    default                          active    Gi1/0/2\n10   Drucker                          active    Gi1/0/1\n1002 fddi-default                     act/unsup\n"
	v = DetectVendor(ciscoVer)
	s = ParseSwitch(v, ciscoVer, "", ciscoPorts, ciscoVLAN)
	if v != VendorCisco || s.Firmware != "15.2(7)E3" || s.Name != "sw-lager" || s.Model != "WS-C2960X-48FPD-L" ||
		s.PortsUp != 1 || s.PortsDown != 1 || len(s.VLANs) != 3 || !strings.HasPrefix(s.Uptime, "3 weeks") {
		t.Fatalf("cisco: %+v", s)
	}
}

func TestCleanAndOutput(t *testing.T) {
	raw := "show vlans\r\n\x1b[2K\x1b[24;1H VLAN ID Name\r\n ------- ----\r\n 1       DEFAULT_VLAN\r\n" +
		"-- MORE --, next page: Space, next line: Enter, quit: Control-C\x1b[2K\r 10      Server\r\n\x1b[1;24r\x1b[24;1Hsw-buero# "
	if HasMore(raw) {
		t.Fatal("MORE in the middle must not count")
	}
	if !HasMore("line\r\n-- MORE --, next page: Space, next line: Enter, quit: Control-C") || !HasMore(" --More-- ") {
		t.Fatal("MORE at end not detected")
	}
	lines, done := Output(raw, "show vlans", "sw-buero#")
	if !done {
		t.Fatalf("prompt not detected: %q", lines)
	}
	want := []string{" VLAN ID Name", " ------- ----", " 1       DEFAULT_VLAN", " 10      Server"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q", lines)
	}
	if !IsPrompt("sw-buero# ") || !IsPrompt("core(config)#") || !IsPrompt("sw01>") || IsPrompt("Press any key to continue") || IsPrompt("a b#") {
		t.Fatal("IsPrompt")
	}
}

func TestDiffAndSameConfig(t *testing.T) {
	a := "! Last configuration change at 10:00\nhostname sw\nvlan 10\n name Server\nvlan 20\n name Clients\ninterface 1\n untagged 10\n"
	b := "! Last configuration change at 11:00\nhostname sw\nvlan 10\n name Server\nvlan 30\n name Gaeste\ninterface 1\n untagged 10\n"
	if !SameConfig(a, strings.Replace(a, "10:00", "12:00", 1)) {
		t.Fatal("volatile line must be ignored")
	}
	if SameConfig(a, b) {
		t.Fatal("different configs reported equal")
	}
	lines, add, del := Diff(ConfigLines(a), ConfigLines(b), 1)
	if add != 2 || del != 2 {
		t.Fatalf("add=%d del=%d %+v", add, del, lines)
	}
	if lines[0].Op != DiffHunk || lines[0].Text != "@@ Zeile 3 @@" {
		t.Fatalf("hunk %+v", lines[0])
	}
	var ops []string
	for _, l := range lines {
		ops = append(ops, string(l.Op)+l.Text)
	}
	got := strings.Join(ops, "|")
	want := "@@@ Zeile 3 @@|  name Server|-vlan 20|- name Clients|+vlan 30|+ name Gaeste| interface 1"
	if got != want {
		t.Fatalf("diff\n got %s\nwant %s", got, want)
	}
	if l, a2, d2 := Diff([]string{"x"}, []string{"x"}, 3); len(l) != 0 || a2 != 0 || d2 != 0 {
		t.Fatal("identical must be empty")
	}
}

// Das Skript läuft wirklich mit sh und liefert lesbare Werte.
func TestServerScriptRunsLocally(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("das Skript läuft auf Linux/macOS-Servern, nicht unter Git-Bash")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("kein sh")
	}
	cmd := exec.Command(sh, "-s")
	cmd.Stdin = strings.NewReader(ServerScript)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	s, err := ParseServer(string(out))
	if err != nil || s.CPUs == 0 || s.MemTotal == 0 || s.DiskTotal == 0 || s.Uptime == 0 {
		t.Fatalf("%v %+v\n%s", err, s, out)
	}
	t.Logf("%s\n%s", out, s.Summary())
}

func TestConfigRules(t *testing.T) {
	cfg := "hostname \"sw1\"\nsnmp-server community \"public\" unrestricted\ntelnet-server\nntp server 10.0.0.1\n"
	rules := ParseConfigRules([]string{"+^ntp server", "+^logging", "-telnet-server", "-community \"public\"", "", "# Kommentar"})
	if len(rules) != 4 {
		t.Fatalf("%d Regeln", len(rules))
	}
	v := CheckConfig(cfg, rules)
	if len(v) != 3 {
		t.Fatalf("%+v", v)
	}
	if v[0].Text() != "fehlt: ^logging" || v[1].Line != "telnet-server" || !strings.Contains(v[2].Line, "public") {
		t.Errorf("%q %q %q", v[0].Text(), v[1].Line, v[2].Line)
	}
	if _, _, err := ParseConfigRule("ntp"); err == nil {
		t.Error("ohne +/- sollte scheitern")
	}
	if _, _, err := ParseConfigRule("+(("); err == nil {
		t.Error("kaputter Ausdruck sollte scheitern")
	}
}

func TestParseServerDisks(t *testing.T) {
	out := "cpus=4\nmem_kb=1000 500\ndisk_kb=1000 400 600\n" +
		"fs=ext4 1000 400 600 /dev/sda2 /\n" +
		"fs=tmpfs 800 0 800 tmpfs /run\n" +
		"fs=devtmpfs 800 0 800 udev /dev\n" +
		"fs=overlay 1000 400 600 overlay /var/lib/docker/overlay2/abc/merged\n" +
		"fs=squashfs 50 50 0 /dev/loop0 /snap/core/1\n" +
		"fs=xfs 2000 1900 100 /dev/sdb1 /data\n" +
		"fs=ext4 1000 400 600 /dev/sda2 /srv/bind\n" +
		"fs=vfat 500 10 490 /dev/sda1 /boot/efi\n" +
		"fs=cifs 4000 1000 3000 //nas/backup /mnt/nas backup\n"
	s, err := ParseServer(out)
	if err != nil {
		t.Fatal(err)
	}
	var mounts []string
	for _, d := range s.Disks {
		mounts = append(mounts, d.Mount)
	}
	if got := strings.Join(mounts, ","); got != "/,/boot/efi,/data,/mnt/nas backup" {
		t.Fatalf("Platten %q", got)
	}
	if s.Disks[2].Pct() != 95 || s.Disks[2].Type != "xfs" {
		t.Fatalf("/data %+v", s.Disks[2])
	}
	al := s.Alerts(Limits{CPU: 90, RAM: 90, Disk: 90})
	if len(al) != 1 || al[0].Mount != "/data" || !strings.Contains(al[0].Text(), "/data 95") {
		t.Fatalf("Warnungen %+v", al)
	}
	if !Has(al, Alert{Metric: MetricDisk, Mount: "/data"}) || Has(al, Alert{Metric: MetricDisk, Mount: "/"}) {
		t.Fatal("Has unterscheidet Platten nicht")
	}
	if !strings.Contains(s.Summary(), "/data 95%") {
		t.Fatalf("Zusammenfassung %q", s.Summary())
	}
	// macOS (df ohne -T): Systemvolumes weg, Data bleibt
	m, _ := ParseServer("cpus=8\nfs=? 1000 100 900 /dev/disk3s1s1 /\nfs=? 300 1 299 devfs /dev\n" +
		"fs=? 1000 20 980 /dev/disk3s6 /System/Volumes/VM\nfs=? 1000 600 400 /dev/disk3s5 /System/Volumes/Data\n")
	if len(m.Disks) != 2 || m.Disks[1].Mount != "/System/Volumes/Data" || m.DiskTotal != 1000*1024 {
		t.Fatalf("macOS %+v", m.Disks)
	}
	// älteres Skript ohne fs=: "/" aus disk_kb
	o, _ := ParseServer("cpus=1\ndisk_kb=100 95 5\n")
	if l := o.DiskList(); len(l) != 1 || l[0].Mount != "/" || len(o.Alerts(Limits{Disk: 90})) != 1 {
		t.Fatalf("alt %+v", l)
	}
}
