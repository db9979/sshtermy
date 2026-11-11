package wol

import (
	"bytes"
	"net"
	"testing"
)

func TestParseMAC(t *testing.T) {
	for _, s := range []string{"aa:bb:cc:dd:ee:ff", "AA-BB-CC-DD-EE-FF", "aabb.ccdd.eeff", "aabbccddeeff", " aa:bb:cc:dd:ee:ff "} {
		hw, err := ParseMAC(s)
		if err != nil || hw.String() != "aa:bb:cc:dd:ee:ff" {
			t.Errorf("%q: %v %v", s, hw, err)
		}
	}
	for _, s := range []string{"", "aa:bb:cc", "zz:bb:cc:dd:ee:ff", "00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff", "aa:bb:cc:dd:ee:ff:00:11", "10.0.0.1"} {
		if _, err := ParseMAC(s); err == nil {
			t.Errorf("%q angenommen", s)
		}
	}
}

func TestPacket(t *testing.T) {
	hw, _ := ParseMAC("01:02:03:04:05:06")
	p := Packet(hw)
	if len(p) != 102 || !bytes.Equal(p[:6], bytes.Repeat([]byte{0xff}, 6)) {
		t.Fatalf("Länge %d", len(p))
	}
	for i := 0; i < 16; i++ {
		if !bytes.Equal(p[6+i*6:12+i*6], hw) {
			t.Fatalf("Wiederholung %d falsch", i)
		}
	}
}

func TestBroadcastOf(t *testing.T) {
	_, n, _ := net.ParseCIDR("192.168.7.33/24")
	n.IP = net.ParseIP("192.168.7.33")
	if b := broadcastOf(n); b.String() != "192.168.7.255" {
		t.Fatalf("%v", b)
	}
	_, n6, _ := net.ParseCIDR("fe80::1/64")
	if broadcastOf(n6) != nil {
		t.Fatal("IPv6 hat keinen Broadcast")
	}
}

func TestWakeSendsToAllTargets(t *testing.T) {
	old := send
	defer func() { send = old }()
	var got []string
	send = func(addr string, pkt []byte) error {
		if len(pkt) != 102 {
			t.Errorf("Paketlänge %d", len(pkt))
		}
		got = append(got, addr)
		return nil
	}
	n, err := Wake("aa:bb:cc:dd:ee:ff")
	if err != nil || n != len(got) || n < 2 {
		t.Fatalf("%d %v %v", n, err, got)
	}
	if got[0] != "255.255.255.255:9" || got[1] != "255.255.255.255:7" {
		t.Fatalf("%v", got)
	}
	if _, err := Wake("kaputt"); err == nil {
		t.Fatal("ungültige MAC angenommen")
	}
}

func TestParseARP(t *testing.T) {
	linux := "IP address       HW type     Flags       HW address            Mask     Device\n" +
		"192.168.1.1      0x1         0x2         3c:a6:2f:11:22:33     *        eth0\n" +
		"192.168.1.10     0x1         0x0         00:00:00:00:00:00     *        eth0\n" +
		"192.168.1.100    0x1         0x2         b8:27:eb:aa:bb:cc     *        eth0\n"
	mac := "? (192.168.1.100) at b8:27:eb:a:b:c on en0 ifscope [ethernet]\n"
	win := "Interface: 192.168.1.5 --- 0x7\r\n  Internet Address      Physical Address      Type\r\n  192.168.1.100         b8-27-eb-aa-bb-cc     dynamic\r\n"
	for name, c := range map[string][2]string{
		"linux":         {linux, "b8:27:eb:aa:bb:cc"},
		"macos":         {mac, "b8:27:eb:0a:0b:0c"},
		"windows":       {win, "b8:27:eb:aa:bb:cc"},
		"unvollständig": {"192.168.1.100 (incomplete)\n", ""},
	} {
		if got := parseARP([]byte(c[0]), "192.168.1.100"); got != c[1] {
			t.Errorf("%s: %q", name, got)
		}
	}
	// 192.168.1.10 darf nicht auf die Zeile von 192.168.1.100 passen, und
	// ein Eintrag ohne Antwort (lauter Nullen) zählt nicht
	if got := parseARP([]byte(linux), "192.168.1.10"); got != "" {
		t.Errorf("unbeantworteter Eintrag: %q", got)
	}
}
