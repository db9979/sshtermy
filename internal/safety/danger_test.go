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
