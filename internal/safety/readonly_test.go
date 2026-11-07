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
