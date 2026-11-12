package agent

import (
	"reflect"
	"testing"
)

func TestChangedFiles(t *testing.T) {
	cases := []struct {
		cmd  string
		want []string
	}{
		{`echo "x=1" | sudo tee -a /etc/app.conf`, []string{"/etc/app.conf"}},
		{`sudo sed -i 's/a/b/' /etc/nginx/nginx.conf`, []string{"/etc/nginx/nginx.conf"}},
		{`sed -i.bak -e 's/a/b/' ~/x.txt /etc/y`, []string{"~/x.txt", "/etc/y"}},
		{`sed 's/a/b/' /etc/y`, nil},
		{`echo hi > /tmp/a && cat /etc/b >> ~/c`, []string{"/tmp/a", "~/c"}},
		{`echo hi >/tmp/a 2>/dev/null`, []string{"/tmp/a"}},
		{`sudo cp ~/.sshtermy-agent/x/app.ini /etc/gitea/app.ini`, []string{"/etc/gitea/app.ini"}},
		{`sudo mv /etc/a.conf /etc/b.conf`, []string{"/etc/b.conf", "/etc/a.conf"}},
		{`rm -f /etc/nginx/sites-enabled/default`, []string{"/etc/nginx/sites-enabled/default"}},
		{`rm -rf /var/www/old`, nil},
		{`cp a.txt b.txt`, nil},
		{`cp /etc/*.conf /backup/`, nil},
		{`ls -la /etc > /dev/null`, nil},
		{`cat /etc/passwd 2>&1 | grep root`, nil},
		{`DEBIAN_FRONTEND=noninteractive sudo -E truncate -s 0 /var/log/app.log`, []string{"/var/log/app.log"}},
		{`sudo -u www-data tee /var/www/html/index.html < /tmp/x`, []string{"/var/www/html/index.html"}},
	}
	for _, c := range cases {
		var got []string
		for _, f := range ChangedFiles(c.cmd) {
			got = append(got, f.Path)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q, will %q", c.cmd, got, c.want)
		}
	}
	if f := ChangedFiles(`echo x | sudo tee /etc/a`); len(f) != 1 || !f[0].Sudo {
		t.Errorf("sudo tee: %+v", f)
	}
	if f := ChangedFiles(`sudo echo x > /etc/a`); len(f) != 1 || f[0].Sudo {
		t.Errorf("Umleitung macht die Shell: %+v", f)
	}
}

func TestInputWait(t *testing.T) {
	cases := map[string]InputKind{
		"Do you want to continue? [Y/n] ":                 InputYesNo,
		"Proceed ([y]/n)? ":                               InputYesNo,
		"Overwrite existing file? (y/n) ":                 InputYesNo,
		"Möchten Sie fortfahren? [J/n] ":                  InputYesNo,
		"Destination filename [startup-config]? ":         InputNone,
		"Proceed with reload? [confirm]":                  InputConfirm,
		"--More--":                                        InputPager,
		" -- MORE --, next page: Space, next line: Enter": InputPager,
		"(END)":                     InputPager,
		"lines 1-23/120 19%":        InputPager,
		"Press any key to continue": InputAnyKey,
		"user@host:~$ ":             InputNone,
		"":                          InputNone,
	}
	for line, want := range cases {
		if got := InputWait(line, false); got != want {
			t.Errorf("%q: %q, will %q", line, got, want)
		}
	}
	if InputWait(":", true) != InputPager || InputWait(":", false) != InputNone {
		t.Error("less-Doppelpunkt")
	}
}
