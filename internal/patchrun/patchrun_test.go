package patchrun

import (
	"strings"
	"testing"
)

func TestParseAndCompare(t *testing.T) {
	before := ParseState(`root=1
pm=apt
boot=aaa
svc=ssh.service
svc=nginx.service
svc=cron.service
port=0.0.0.0:22
port=[::]:22
port=0.0.0.0:80
port=127.0.0.1:5432
reboot=0
`)
	if before.Manager != MgrApt || !before.Root || len(before.Ports) != 3 || before.Reboot {
		t.Fatalf("vorher falsch gelesen: %+v", before)
	}
	after := ParseState(`pm=apt
boot=bbb
svc=ssh.service
svc=cron.service
fail=nginx.service
port=*:22
port=127.0.0.1:5432
reboot=1
`)
	d := Compare(before, after)
	if !d.Rebooted || d.Problems() != 3 {
		t.Fatalf("Vergleich: %+v", d)
	}
	if d.StoppedServices[0] != "nginx.service" || d.NewFailed[0] != "nginx.service" || d.ClosedPorts[0] != "*:80" {
		t.Fatalf("Vergleich: %+v", d)
	}
	if !after.Reboot {
		t.Fatal("Neustart nötig nicht erkannt")
	}
}

func TestUpdateScript(t *testing.T) {
	if s := UpdateScript(MgrApt, ScopeAll, nil); !strings.Contains(s, "apt-get upgrade -y") || !strings.Contains(s, "DEBIAN_FRONTEND=noninteractive") {
		t.Fatal(s)
	}
	s := UpdateScript(MgrApt, ScopeSecurity, []string{"openssl", "libssl3", "bad name"})
	if !strings.Contains(s, "--only-upgrade") || !strings.Contains(s, "'openssl' 'libssl3'") || strings.Contains(s, "bad") {
		t.Fatal(s)
	}
	if UpdateScript(MgrDnf, ScopeSecurity, nil) != "" {
		t.Fatal("ohne Pakete nichts tun")
	}
	if !strings.Contains(UpdateScript(MgrMicro, ScopeAll, nil), "microdnf -y upgrade") {
		t.Fatal("microdnf")
	}
	if UpdateScript(MgrNone, ScopeAll, nil) != "" {
		t.Fatal("unbekannt")
	}
}

func TestUpgradedCount(t *testing.T) {
	out := "Unpacking openssl (3.0.15-1~deb12u1) over (3.0.11-1~deb12u2) ...\nUnpacking libssl3:amd64 (3.0.15) over (3.0.11) ...\nSetting up openssl\n"
	if n := Upgraded(MgrApt, out); n != 2 {
		t.Fatal(n)
	}
	if !strings.HasPrefix(Tail("a\nb\nc\nd", 2), "… (2") {
		t.Fatal(Tail("a\nb\nc\nd", 2))
	}
}
