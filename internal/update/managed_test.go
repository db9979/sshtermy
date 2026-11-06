package update

import "testing"

func TestManagedBy(t *testing.T) {
	yes := func(string) bool { return true }
	no := func(string) bool { return false }
	cases := []struct {
		exe, goos string
		exists    func(string) bool
		want      string
	}{
		{"/opt/homebrew/Cellar/sshtermy/1.0.40/bin/sshtermy", "darwin", no, "brew upgrade sshtermy"},
		{"/usr/local/Cellar/sshtermy/1.0.40/bin/sshtermy", "darwin", no, "brew upgrade sshtermy"},
		{"/home/linuxbrew/.linuxbrew/Cellar/sshtermy/1.0.40/bin/sshtermy", "linux", no, "brew upgrade sshtermy"},
		{"/usr/bin/sshtermy", "linux", yes, "sudo apt update && sudo apt install --only-upgrade sshtermy"},
		{"/usr/bin/sshtermy", "linux", no, ""},
		{"/Users/d/Applications/sshTermy.app/Contents/Resources/sshtermy", "darwin", yes, ""},
		{`C:\Users\d\AppData\Local\Microsoft\WinGet\Packages\db9979.sshTermy_x\sshtermy.exe`, "windows", yes, ""},
	}
	for _, c := range cases {
		if got := managedBy(c.exe, c.goos, c.exists); got != c.want {
			t.Errorf("%s: %q, want %q", c.exe, got, c.want)
		}
	}
}
