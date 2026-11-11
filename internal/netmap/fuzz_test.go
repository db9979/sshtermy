package netmap

import (
	"testing"

	"github.com/db9979/sshtermy/internal/fuzzguard"
)

// FuzzParsers: Ausgaben von Switches/Routern (MAC-Tabelle, ARP, LLDP/CDP).
func FuzzParsers(f *testing.F) {
	f.Add("00:11:22:aa:bb:cc  20  1/1/7  dynamic\n", "procurve")
	f.Add("Internet  10.0.20.15  3  0011.22aa.bbcc  ARPA  Vlan20\n", "cisco")
	f.Add("Local Port : 1/1/48\nSystem Name : core-sw\nPort Descr : 1/1/1\n", "aruba-cx")
	f.Add("Device ID: sw2\nInterface: Gi1/0/1,  Port ID (outgoing port): Gi0/1\n", "cisco")
	f.Fuzz(func(t *testing.T, out, vendor string) {
		fuzzguard.Run(t, func() {
			ParseMACTable(out)
			ParseARP(out)
			ParseNeighbors(out)
			NormMAC(out)
			IsIP(out)
			ShortName(out)
			cmds := Commands(vendor, true, true, true)
			Sections(out, cmds)
			d := ParseDevice("a", "h", out, cmds)
			links := Topology([]Device{d, d})
			TreeLines([]Device{d}, links)
			Mermaid(links)
			_, _ = Locate([]Device{d}, out)
		})
	})
}
