package agent

import (
	"context"
	"strings"
	"testing"
)

func TestSchedChecks(t *testing.T) {
	ag, seen := fakeSeen(t, `{"befehle": ["systemctl is-active nginx", "systemctl restart nginx", "df -h /", "rm -rf /tmp/x", "journalctl -p err --since -1d --no-pager | tail -20", "uptime", "free -m", "ss -ltn"]}`)
	cmds, err := ag.SchedChecks(context.Background(), "läuft nginx? password=geheim", []string{"web1", "web2"}, Context{Host: "web", OS: "Debian 12"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) == 0 || len(cmds) > MaxSchedChecks {
		t.Fatalf("%v", cmds)
	}
	for _, c := range cmds {
		if strings.Contains(c, "restart") || strings.Contains(c, "rm ") {
			t.Errorf("ändernd: %q", c)
		}
	}
	if all := strings.Join(*seen, "\n"); strings.Contains(all, "geheim") || !strings.Contains(all, "web2") {
		t.Errorf("Material:\n%s", all)
	}
}

func TestSchedJudge(t *testing.T) {
	ag, _ := fakeSeen(t, `{"hosts": [{"host": "web1", "ok": true, "befund": "alles gut"}, {"host": "web2", "ok": false, "befund": "Platte 97 % voll"}, {"host": "erfunden", "ok": false, "befund": "x"}], "zusammenfassung": "web2 braucht Platz"}`)
	vs, sum, err := ag.SchedJudge(context.Background(), "Platte", []HostOutput{
		{Host: "web1", Results: []CheckResult{{Command: "df -h /", Output: "40%"}}},
		{Host: "web2", Results: []CheckResult{{Command: "df -h /", Output: "97%"}}},
	}, Context{})
	if err != nil || len(vs) != 2 || vs[1].OK || !strings.Contains(sum, "web2") {
		t.Fatalf("%+v %q %v", vs, sum, err)
	}
}
