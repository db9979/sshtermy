package agent

import "testing"

func TestLooksFailed(t *testing.T) {
	yes := []string{
		"bash: dockr: command not found",
		"cat: /etc/foo: No such file or directory",
		"E: Unable to locate package nginxx",
		"Job for nginx.service failed because the control process exited with error code.",
		"     ^\n% Invalid input detected at '^' marker.",
		"Invalid input: vlam",
		"open /root/x: permission denied",
		"fatal: not a git repository (or any of the parent directories): .git",
		"curl: (7) Failed to connect to localhost port 80: Connection refused",
	}
	for _, s := range yes {
		if ok, _ := LooksFailed(s); !ok {
			t.Errorf("nicht erkannt: %q", s)
		}
	}
	no := []string{
		"total 0\ndrwxr-xr-x 2 root root 40 Nov  5 10:00 .",
		"Reading package lists... Done\n0 upgraded, 0 newly installed, 0 to remove",
		"active (running) since Tue",
		"PASS: 12 tests, 0 errors",
	}
	for _, s := range no {
		if ok, why := LooksFailed(s); ok {
			t.Errorf("Fehlalarm (%s): %q", why, s)
		}
	}
}

func TestVisibleAnswer(t *testing.T) {
	if got := VisibleAnswer("<think>hm"); got != "" {
		t.Fatalf("offener Denkblock sichtbar: %q", got)
	}
	if got := VisibleAnswer("<think>hm</think>\n**Warum?** x"); got != "**Warum?** x" {
		t.Fatalf("got %q", got)
	}
	if got := VisibleAnswer("normal"); got != "normal" {
		t.Fatalf("got %q", got)
	}
}
