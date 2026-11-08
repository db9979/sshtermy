package netmap

import (
	"strings"
	"testing"
)

func TestNormMAC(t *testing.T) {
	for _, in := range []string{"00:11:22:aa:bb:cc", "00-11-22-AA-BB-CC", "0011.22aa.bbcc", "001122-aabbcc", "001122aabbcc"} {
		if got := NormMAC(in); got != "001122aabbcc" {
			t.Errorf("NormMAC(%q) = %q", in, got)
		}
	}
	if NormMAC("10.0.0.1") != "" || NormMAC("hallo") != "" {
		t.Error("keine MAC erwartet")
	}
	if FormatMAC("001122aabbcc") != "00:11:22:aa:bb:cc" {
		t.Error(FormatMAC("001122aabbcc"))
	}
}

func TestParseMACTable(t *testing.T) {
	pc := `
 Status and Counters - Port Address Table

  MAC Address   Port  VLAN
  ------------- ----- ----
  001122-aabbcc 17    20
  001122-000001 25    1
`
	cx := `
MAC age-time            : 300 seconds
MAC Address          VLAN     Type                      Port
--------------------------------------------------------------
00:11:22:aa:bb:cc    20       dynamic                   1/1/48
`
	cs := `
          Mac Address Table
-------------------------------------------
Vlan    Mac Address       Type        Ports
----    -----------       --------    -----
  20    0011.22aa.bbcc    DYNAMIC     Gi1/0/24
Total Mac Addresses for this criterion: 1
`
	cases := []struct {
		text, port string
		vlan       int
	}{{pc, "17", 20}, {cx, "1/1/48", 20}, {cs, "Gi1/0/24", 20}}
	for i, c := range cases {
		es := ParseMACTable(c.text)
		if len(es) == 0 || es[0].MAC != "001122aabbcc" || es[0].Port != c.port || es[0].VLAN != c.vlan {
			t.Errorf("Fall %d: %+v", i, es)
		}
	}
}

func TestParseARP(t *testing.T) {
	m := ParseARP(`
  IP Address       MAC Address       Type    Port
  10.0.20.15       001122-aabbcc     dynamic 17
Internet  10.0.20.16   5   0011.22aa.bbcd  ARPA   Vlan20
`)
	if m["10.0.20.15"] != "001122aabbcc" || m["10.0.20.16"] != "001122aabbcd" {
		t.Errorf("%v", m)
	}
}

func TestParseNeighbors(t *testing.T) {
	pc := `
 LLDP Remote Devices Information

  LocalPort | ChassisId                 PortId PortDescr SysName
  --------- + ------------------------- ------ --------- ----------------------
  25        | 94 18 82 00 00 01         1/1/48 1/1/48    core
  3         | 00 11 22 33 44 55         1      eth0      ap-03
`
	cx := `
LLDP Neighbor Information
=========================

Total Neighbor Entries          : 1
LOCAL-PORT  CHASSIS-ID         PORT-ID   PORT-DESC   TTL   SYS-NAME
--------------------------------------------------------------------
1/1/48      94:18:82:00:00:02  25        25          120   sw-etage2
`
	cdp := `
Capability Codes: R - Router, T - Trans Bridge, B - Source Route Bridge
Device ID        Local Intrfce     Holdtme    Capability  Platform  Port ID
access-switch-lang.example.com
                 Gig 1/0/1         150             S I    WS-C2960  Gig 0/1
sw4              Gig 1/0/2         150             S I    WS-C2960  Gig 0/2
`
	ns := ParseNeighbors(pc)
	if len(ns) != 2 || ns[0].LocalPort != "25" || ns[0].RemoteName != "core" || ns[0].RemotePort != "1/1/48" {
		t.Errorf("procurve %+v", ns)
	}
	ns = ParseNeighbors(cx)
	if len(ns) != 1 || ns[0].LocalPort != "1/1/48" || ns[0].RemoteName != "sw-etage2" || ns[0].RemotePort != "25" {
		t.Errorf("cx %+v", ns)
	}
	ns = ParseNeighbors(cdp)
	if len(ns) != 2 || ns[0].RemoteName != "access-switch-lang.example.com" || ns[0].LocalPort != "Gig1/0/1" || ns[1].RemotePort != "Gig0/2" {
		t.Errorf("cdp %+v", ns)
	}
}

func TestSections(t *testing.T) {
	cmds := Commands(ProCurve, true, true, true)
	out := "Welcome\nsw-etage2# no page\nsw-etage2# show mac-address\n  001122-aabbcc 17 20\nsw-etage2# show arp\n  10.0.0.1 001122-aabbcc dynamic 17\nsw-etage2# show lldp info remote-device\n  LocalPort | ChassisId PortId PortDescr SysName\n  25 | 941882000001 1/1/48 1/1/48 core\nsw-etage2# "
	sec, name := Sections(out, cmds)
	if name != "sw-etage2" {
		t.Errorf("name %q", name)
	}
	if !strings.Contains(sec["show mac-address"], "001122-aabbcc") || strings.Contains(sec["show mac-address"], "10.0.0.1") {
		t.Errorf("mac %q", sec["show mac-address"])
	}
	d := ParseDevice("etage2", "10.0.0.2", out, cmds)
	if len(d.MACs) != 1 || len(d.ARP) != 1 || len(d.Neigh) != 1 {
		t.Errorf("%+v", d)
	}
}

func lab() []Device {
	return []Device{
		{Alias: "core", Name: "core", MACs: []MACEntry{
			{MAC: "001122aabbcc", Port: "1/1/48", VLAN: 20},
			{MAC: "000000000001", Port: "1/1/48", VLAN: 20},
			{MAC: "000000000002", Port: "1/1/48", VLAN: 20},
		}, ARP: map[string]string{"10.0.20.15": "001122aabbcc"},
			Neigh: []Neighbor{{"1/1/48", "sw-etage2", "25"}}},
		{Alias: "etage2", Name: "sw-etage2", MACs: []MACEntry{
			{MAC: "001122aabbcc", Port: "17", VLAN: 20},
			{MAC: "000000000001", Port: "25", VLAN: 20},
		}, Neigh: []Neighbor{{"25", "core", "1/1/48"}, {"3", "ap-03", "eth0"}}},
	}
}

func TestLocate(t *testing.T) {
	r, err := Locate(lab(), "10.0.20.15")
	if err != nil {
		t.Fatal(err)
	}
	if r.MAC != "001122aabbcc" || !r.Edge || r.Found[0].Device != "etage2" || r.Found[0].Port != "17" {
		t.Errorf("%+v", r)
	}
	if r.Found[1].Uplink != "etage2" {
		t.Errorf("core-Port sollte Uplink sein: %+v", r.Found[1])
	}
	if _, err := Locate(lab(), "10.9.9.9"); err == nil {
		t.Error("unbekannte IP sollte scheitern")
	}
	if _, err := Locate(lab(), "aa:aa:aa:aa:aa:aa"); err == nil {
		t.Error("unbekannte MAC sollte scheitern")
	}
}

func TestTopology(t *testing.T) {
	ls := Topology(lab())
	if len(ls) != 2 {
		t.Fatalf("%+v", ls)
	}
	tree := strings.Join(TreeLines(lab(), ls), "\n")
	if !strings.Contains(tree, "etage2") || !strings.Contains(tree, "ap-03") {
		t.Error(tree)
	}
	m := Mermaid(ls)
	if !strings.Contains(m, "graph LR") || !strings.Contains(m, "ap-03") {
		t.Error(m)
	}
}
