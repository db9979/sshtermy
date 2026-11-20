package agent

import (
	"context"
	"strings"
	"testing"
)

func TestVerifyChecksOnlyReadOnly(t *testing.T) {
	ag, seen := fakeSeen(t, `{"pruefungen": ["systemctl is-active nginx", "systemctl restart nginx", "curl -sI http://localhost", "cat /etc/nginx/nginx.conf", "ss -ltn"]}`)
	exit := 0
	steps := []DoneStep{{Command: "apt install -y nginx", Exit: &exit, Output: "Setting up nginx\npassword=geheim"}}
	cmds, err := ag.VerifyChecks(context.Background(), "nginx installieren", "läuft", steps, Context{Host: "web", OS: "Debian 12"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cmds {
		if strings.Contains(c, "restart") {
			t.Errorf("ändernder Befehl durchgelassen: %q", c)
		}
	}
	if len(cmds) == 0 || len(cmds) > MaxVerifyChecks {
		t.Errorf("Prüfbefehle: %v", cmds)
	}
	all := strings.Join(*seen, "\n")
	if strings.Contains(all, "geheim") || !strings.Contains(all, "apt install -y nginx") {
		t.Errorf("Material falsch:\n%s", all)
	}
}

func TestVerifyJudge(t *testing.T) {
	ag, _ := fakeSeen(t, `{"erreicht": false, "befund": "nginx läuft, aber Port 443 ist zu."}`)
	v, err := ag.VerifyJudge(context.Background(), "nginx mit TLS", "fertig", nil,
		[]CheckResult{{Command: "ss -ltn", Output: "LISTEN 0 511 *:80", Exit: 0}}, Context{Host: "web"})
	if err != nil || v.OK || !strings.Contains(v.Finding, "443") {
		t.Fatalf("%+v %v", v, err)
	}
	bad, _ := fakeSeen(t, `ich weiß nicht`)
	if _, err := bad.VerifyJudge(context.Background(), "x", "y", nil, nil, Context{}); err != ErrUnreadable {
		t.Errorf("unlesbar: %v", err)
	}
}
