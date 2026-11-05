package ui

import (
	"strings"
	"testing"
)

func TestFindLink(t *testing.T) {
	cases := []struct {
		line, at string // at: Teil der Zeile, auf den geklickt wird
		kind     linkKind
		text     string // URL bzw. Host
		user     string
		port     int
		none     bool
	}{
		{line: "Siehe https://example.com/a?b=1.", at: "example", kind: lkURL, text: "https://example.com/a?b=1"},
		{line: "(Doku: https://de.wikipedia.org/wiki/SSH_(Protokoll))", at: "wiki/", kind: lkURL, text: "https://de.wikipedia.org/wiki/SSH_(Protokoll)"},
		{line: "besuche www.heise.de heute", at: "heise", kind: lkURL, text: "https://www.heise.de"},
		{line: "inet 192.168.1.20/24 brd 192.168.1.255", at: "168.1.20", kind: lkHost, text: "192.168.1.20"},
		{line: "ssh admin@10.0.0.5:2222 now", at: "10.0", kind: lkHost, text: "10.0.0.5", user: "admin", port: 2222},
		{line: "from fe80::1c2b:3ff:fe4d:5e6f%eth0 port", at: "3ff", kind: lkHost, text: "fe80::1c2b:3ff:fe4d:5e6f%eth0"},
		{line: "connect [2001:db8::7]:22 ok", at: "db8", kind: lkHost, text: "2001:db8::7", port: 22},
		{line: "server switch01.lan.example.com up", at: "lan", kind: lkHost, text: "switch01.lan.example.com"},
		{line: "pi@raspberrypi.local", at: "berry", kind: lkHost, text: "raspberrypi.local", user: "pi"},
		{line: "vim /etc/nginx/nginx.conf", at: "nginx.c", none: true},
		{line: "open /var/www/example.com/index.html", at: "example", none: true},
		{line: "Version 1.2.3.4 installiert", at: "2.3", kind: lkHost, text: "1.2.3.4"}, // sieht aus wie IPv4
		{line: "Version 1.2.10 installiert", at: "2.10", none: true},
		{line: "-rw-r--r-- 1 root root 12 syslog.1", at: "syslog", none: true},
		{line: "Uhrzeit 12:30:45", at: "30", none: true},
		{line: "nur text hier", at: "text", none: true},
	}
	for _, c := range cases {
		off := strings.Index(c.line, c.at)
		if off < 0 {
			t.Fatalf("%q: %q nicht gefunden", c.line, c.at)
		}
		l, ok := findLink(c.line, off)
		if c.none {
			if ok {
				t.Errorf("%q: unerwarteter Treffer %+v", c.line, l)
			}
			continue
		}
		if !ok {
			t.Errorf("%q: kein Treffer", c.line)
			continue
		}
		got := l.host
		if l.kind == lkURL {
			got = l.text
		}
		if l.kind != c.kind || got != c.text || l.user != c.user || l.port != c.port {
			t.Errorf("%q: %+v, erwartet %v %q user=%q port=%d", c.line, l, c.kind, c.text, c.user, c.port)
		}
	}
}

func TestCheckPaste(t *testing.T) {
	pc := checkPaste("ls -la\r\nsudo rm -rf /var/tmp/x\necho ok\n", "unix")
	if !pc.multi || len(pc.lines) != 3 || pc.dangerous != 1 || len(pc.lines[1].danger) == 0 {
		t.Fatalf("mehrzeilig: %+v", pc)
	}
	pc = checkPaste("uptime", "unix")
	if pc.multi || pc.dangerous != 0 {
		t.Fatalf("eine Zeile: %+v", pc)
	}
	pc = checkPaste("uptime\n", "unix")
	if !pc.multi || len(pc.lines) != 1 {
		t.Fatalf("Zeile mit Enter am Ende muss geprüft werden: %+v", pc)
	}
	pc = checkPaste("shutdown -r now", "unix")
	if pc.multi || pc.dangerous != 1 {
		t.Fatalf("gefährliche Einzelzeile: %+v", pc)
	}
}
