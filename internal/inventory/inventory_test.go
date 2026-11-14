package inventory

import (
	"strings"
	"testing"
	"time"
)

const sample = `os=Debian GNU/Linux 12 (bookworm)
kernel=6.1.0-18-amd64
pkg=openssl|3.0.11-1~deb12u2
pkg=nginx|1.22.1-9
user=root|0|/bin/bash
user=dominik|1000|/bin/bash
admin=dominik|sudo
admin=dominik|sudo
cron=/etc/crontab|17 *	* * *	root    cd / && run-parts --report /etc/cron.hourly
port=tcp|0.0.0.0:22|users:(("sshd",pid=1,fd=3))
port=tcp|[::]:80|users:(("nginx",pid=2,fd=6))
port=udp|127.0.0.1:53|
svc=ssh.service
svc=nginx.service
`

func TestParse(t *testing.T) {
	s := Parse(sample, time.Unix(100, 0))
	if s.OS != "Debian GNU/Linux 12 (bookworm)" || s.Kernel != "6.1.0-18-amd64" {
		t.Fatalf("os/kernel: %+v", s)
	}
	if s.Packages["nginx"] != "1.22.1-9" || len(s.Packages) != 2 {
		t.Fatalf("pkgs: %v", s.Packages)
	}
	if s.Users["dominik"] != "uid 1000 /bin/bash" {
		t.Fatalf("users: %v", s.Users)
	}
	if len(s.Admins) != 1 || s.Admins[0] != "dominik (sudo)" {
		t.Fatalf("admins: %v", s.Admins)
	}
	want := []string{"tcp *:22 sshd", "tcp *:80 nginx", "udp 127.0.0.1:53"}
	if strings.Join(s.Ports, ";") != strings.Join(want, ";") {
		t.Fatalf("ports: %q", s.Ports)
	}
	if len(s.Cron) != 1 || !strings.HasPrefix(s.Cron[0], "/etc/crontab: 17 * * * * root") {
		t.Fatalf("cron: %q", s.Cron)
	}
	if len(s.Services) != 2 {
		t.Fatalf("svcs: %v", s.Services)
	}
}

func TestDiffUpdate(t *testing.T) {
	var r Record
	a := Parse(sample, time.Unix(100, 0))
	if ch := r.Update(a); ch != nil {
		t.Fatalf("erster Stand: %v", ch)
	}
	b := Parse(strings.NewReplacer(
		"6.1.0-18", "6.1.0-20",
		"nginx|1.22.1-9", "nginx|1.22.1-10",
		"admin=dominik|sudo\nadmin=dominik|sudo\n", "admin=dominik|sudo\nadmin=eve|sudo\n",
		"svc=nginx.service\n", "",
	).Replace(sample)+"pkg=netcat|1.0\n", time.Unix(200, 0))
	ch := r.Update(b)
	got := map[string]bool{}
	for _, c := range ch {
		got[c.Kind+c.Op+c.Item] = true
	}
	for _, w := range []string{"kernel~kernel", "admin+eve (sudo)", "svc-nginx.service", "pkg~nginx", "pkg+netcat"} {
		if !got[w] {
			t.Errorf("fehlt %s in %+v", w, ch)
		}
	}
	if len(r.History) != 1 || !r.History[0].Since.Equal(time.Unix(100, 0)) {
		t.Fatalf("history: %+v", r.History)
	}
	imp := Important(ch)
	for _, c := range imp {
		if c.Kind == "pkg" && c.Op == "~" {
			t.Fatal("Versionssprung in Important")
		}
	}
	if len(imp) != len(ch)-1 {
		t.Fatalf("important %d von %d", len(imp), len(ch))
	}
	// gleicher Stand: keine neue Historie
	b2 := b
	b2.At = time.Unix(300, 0)
	if ch := r.Update(b2); len(ch) != 0 || len(r.History) != 1 {
		t.Fatalf("unverändert: %v, %d", ch, len(r.History))
	}
}

func TestCSV(t *testing.T) {
	s := Parse(sample, time.Unix(100, 0))
	c := s.CSV("web1")
	if !strings.HasPrefix(c, "host,kategorie,name,wert\n") || !strings.Contains(c, "web1,package,openssl,3.0.11-1~deb12u2") {
		t.Fatal(c)
	}
}
