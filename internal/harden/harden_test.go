package harden

import (
	"strings"
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	out := `root=1
os=Debian GNU/Linux 12 (bookworm)
sshd=permitrootlogin yes
sshd=passwordauthentication yes
sshd=permitemptypasswords no
sshd=maxauthtries 10
nopasswd=deploy ALL=(ALL) NOPASSWD: ALL
nopasswd=backup ALL=(root) NOPASSWD: /usr/bin/rsync
fw=iptables 0
lis=0.0.0.0:22|users:(("sshd",pid=1,fd=3))
lis=[::]:22|users:(("sshd",pid=1,fd=4))
lis=0.0.0.0:6379|users:(("redis-server",pid=2,fd=6))
lis=127.0.0.1:5432|users:(("postgres",pid=3,fd=6))
lis=0.0.0.0:443|users:(("nginx",pid=4,fd=6))
auto=unit apt-daily-upgrade.timer
ntp=no
uid0=root
uid0=toor
cert=/etc/letsencrypt/live/web/cert.pem|Nov 20 10:00:00 2026 GMT
cert=/etc/ssl/old.pem|Jan  2 10:00:00 2026 GMT
`
	r := Check(Parse(out), now)
	ids := map[string]Level{}
	for _, f := range r.Findings {
		if _, ok := ids[f.ID]; !ok {
			ids[f.ID] = f.Level
		}
	}
	want := map[string]Level{"ssh-root": Bad, "ssh-pw": Warn, "ssh-tries": Info, "sudo-nopw": Warn, "fw": Warn, "port-6379": Bad,
		"auto": Warn, "uid0": Bad, "cert": Bad, "ntp": Info}
	for id, lv := range want {
		if ids[id] != lv {
			t.Errorf("%s: %v, will %v", id, ids[id], lv)
		}
	}
	if _, ok := ids["port-5432"]; ok {
		t.Error("Port an 127.0.0.1 gemeldet")
	}
	if r.Findings[0].Level != Bad {
		t.Error("nicht nach Schwere sortiert")
	}
	b, w, _ := r.Counts()
	if b < 4 || w < 3 {
		t.Fatalf("%d %d", b, w)
	}
	for _, f := range r.Findings {
		if f.ID == "sudo-nopw" && strings.Contains(f.Detail, "rsync") {
			t.Error("NOPASSWD für einen Befehl gemeldet")
		}
	}
	// sauberer Host
	good := Check(Parse("root=1\nsshd=permitrootlogin no\nsshd=passwordauthentication no\nfw=ufw Status: active\nauto=apt APT::Periodic::Unattended-Upgrade \"1\";\nlis=0.0.0.0:22|\n"), now)
	if b, w, _ := good.Counts(); b+w != 0 {
		t.Fatalf("%+v", good.Findings)
	}
}
