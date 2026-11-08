package safety

import "testing"

func TestReadOnly(t *testing.T) {
	yes := []struct{ cmd, kind string }{
		{"df -h /var", "unix"},
		{"sudo du -sh /var/log/app/* | sort -h | tail -3", "unix"},
		{"ls /etc/logrotate.d/ | grep -c app", "unix"},
		{"systemctl status nginx --no-pager", "unix"},
		{"journalctl -u nginx -n 50 --no-pager 2>&1", "unix"},
		{"find /var/log -name '*.gz' -mtime +30", "unix"},
		{"ip -br a", "unix"},
		{"cat /etc/os-release && uname -a", "unix"},
		{"ping -c 3 10.0.0.1", "unix"},
		{"date +%F", "unix"},
		{"show interfaces brief", Network},
		{"show running-config | include ntp", Network},
		{"Get-Service sshd | Select-Object Status", "windows"},
	}
	for _, c := range yes {
		if !ReadOnly(c.cmd, c.kind) {
			t.Errorf("%q sollte nur lesen", c.cmd)
		}
	}
	no := []struct{ cmd, kind string }{
		{"rm -f /tmp/x", "unix"},
		{"echo hi > /etc/motd", "unix"},
		{"cat a | tee b", "unix"},
		{"find / -name x -delete", "unix"},
		{"find / -name x -exec rm {} +", "unix"},
		{"sed -i s/a/b/ f", "unix"},
		{"ls $(rm -rf /)", "unix"},
		{"systemctl restart nginx", "unix"},
		{"apt-get install -y nginx", "unix"},
		{"ip addr add 10.0.0.5/24 dev eth0", "unix"},
		{"sudo -s", "unix"},
		{"date -s 12:00", "unix"},
		{"hostname neu", "unix"},
		{"env rm -rf /", "unix"},
		{"sort -o /etc/passwd x", "unix"},
		{"ping 10.0.0.1", "unix"},
		{"bash ~/.sshtermy-agent/a1/x.sh", "unix"},
		{"sleep 1 &", "unix"},
		{"configure terminal", Network},
		{"write memory", Network},
		{"show run; reload", Network},
		{"Stop-Service sshd", "windows"},
		{"Get-Service | % { Stop-Service $_ }", "windows"},
		{"", "unix"},
	}
	for _, c := range no {
		if ReadOnly(c.cmd, c.kind) {
			t.Errorf("%q darf nicht als nur lesend gelten", c.cmd)
		}
	}
}

func TestReadOnlyWriters(t *testing.T) {
	for _, c := range []string{"nvidia-smi -r", "nvidia-smi -pl 200", "nvidia-smi -i 0 -pm 1", "nvidia-smi --gpu-reset",
		"sensors -s", "dmesg -n 1", "ss -K dst 1.2.3.4", "git diff --output=/etc/x", "find . -fprint0 /tmp/x"} {
		if ReadOnly(c, "unix") {
			t.Errorf("ReadOnly(%q) = true", c)
		}
	}
	for _, c := range []string{"nvidia-smi", "nvidia-smi -L", "nvidia-smi --query-gpu=name,memory.used --format=csv", "nvidia-smi -i 0 -q -d MEMORY",
		"sensors", "dmesg -T", "ss -tlnp", "git log --oneline"} {
		if !ReadOnly(c, "unix") {
			t.Errorf("ReadOnly(%q) = false", c)
		}
	}
	for _, c := range []string{"Format-Volume -DriveLetter D", "format C:", "format"} {
		if ReadOnly(c, "windows") {
			t.Errorf("ReadOnly(%q, windows) = true", c)
		}
	}
	if !ReadOnly("Get-Process | Format-Table", "windows") || !ReadOnly("Get-ChildItem | Sort-Object Name | Select-Object -First 3", "windows") {
		t.Error("Format-Table/Sort-Object sollten nur lesen")
	}
}
