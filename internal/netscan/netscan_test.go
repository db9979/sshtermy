package netscan

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"testing"
	"time"
)

func TestParseTargets(t *testing.T) {
	cases := []struct {
		in    string
		n     int
		first string
		last  string
	}{
		{"192.168.1.0/24", 254, "192.168.1.1", "192.168.1.254"},
		{"10.0.0.5", 1, "10.0.0.5", "10.0.0.5"},
		{"10.0.0.10-20", 11, "10.0.0.10", "10.0.0.20"},
		{"10.0.0.250-10.0.1.2", 9, "10.0.0.250", "10.0.1.2"},
		{"10.0.0.1, 10.0.0.1 10.0.0.2", 2, "10.0.0.1", "10.0.0.2"},
		{"10.0.0.8/31", 2, "10.0.0.8", "10.0.0.9"},
		{"172.16.0.0/20", 4094, "172.16.0.1", "172.16.15.254"},
	}
	for _, c := range cases {
		got, err := ParseTargets(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if len(got) != c.n || got[0].String() != c.first || got[len(got)-1].String() != c.last {
			t.Errorf("%q: %d Adressen %v … %v", c.in, len(got), got[0], got[len(got)-1])
		}
	}
	for _, bad := range []string{"", "10.0.0.0/8", "foo", "10.0.0.20-10", "10.0.0.1-300"} {
		if _, err := ParseTargets(bad); err == nil {
			t.Errorf("%q: Fehler erwartet", bad)
		}
	}
}

func TestDescribe(t *testing.T) {
	for in, want := range map[string]string{
		"SSH-2.0-OpenSSH_9.6p1 Ubuntu-3ubuntu13": "OpenSSH 9.6p1 (Ubuntu)",
		"SSH-2.0-OpenSSH_8.4p1 Debian-5+deb11u3": "OpenSSH 8.4p1 (Debian)",
		"SSH-2.0-Mocana SSH 6.3":                 "Mocana (SSH)",
		"SSH-2.0-Cisco-1.25":                     "Cisco-1.25",
		"SSH-2.0-ROSSSH":                         "ROSSSH",
	} {
		if got := Describe(in); got != want {
			t.Errorf("Describe(%q) = %q, erwartet %q", in, got, want)
		}
	}
}

// listen startet einen Port, der ein Banner schickt (oder schweigt).
func listen(t *testing.T, banner string) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				if banner != "" {
					c.Write([]byte(banner))
				}
				time.Sleep(500 * time.Millisecond)
			}()
		}
	}()
	_, p, _ := net.SplitHostPort(ln.Addr().String())
	n, _ := strconv.Atoi(p)
	return n
}

func TestScanFindsSSH(t *testing.T) {
	lo := []netip.Addr{netip.MustParseAddr("127.0.0.1")}
	o := Options{Timeout: time.Second, Banner: 300 * time.Millisecond, NoDNS: true}

	o.Port = listen(t, "Willkommen\r\nSSH-2.0-ArubaOS 16.10\r\n")
	hs := Scan(context.Background(), lo, o, nil, nil)
	if len(hs) != 1 || !hs[0].Network || hs[0].Banner != "SSH-2.0-ArubaOS 16.10" || hs[0].Kind() != "Netzwerkgerät" {
		t.Fatalf("Aruba nicht erkannt: %+v", hs)
	}

	o.Port = listen(t, "SSH-2.0-OpenSSH_9.6p1 Ubuntu-3ubuntu13\r\n")
	var prog, found int
	hs = Scan(context.Background(), lo, o, func(d, n int) { prog = d }, func(Host) { found++ })
	if len(hs) != 1 || hs[0].Network || prog != 1 || found != 1 {
		t.Fatalf("Server nicht erkannt: %+v (prog %d, found %d)", hs, prog, found)
	}

	o.Port = listen(t, "") // offen, aber kein SSH
	if hs := Scan(context.Background(), lo, o, nil, nil); len(hs) != 0 {
		t.Fatalf("stummer Port gilt als SSH: %+v", hs)
	}

	ln, _ := net.Listen("tcp", "127.0.0.1:0") // geschlossener Port
	_, p, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()
	o.Port, _ = strconv.Atoi(p)
	if hs := Scan(context.Background(), lo, o, nil, nil); len(hs) != 0 {
		t.Fatalf("geschlossener Port gefunden: %+v", hs)
	}
}
