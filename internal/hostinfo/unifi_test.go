package hostinfo

import "testing"

func TestUniFi(t *testing.T) {
	info := `
Model:       USW-24-PoE
Version:     6.6.55.15189
MAC Address: 74:ac:b9:00:11:22
IP Address:  192.168.1.20
Hostname:    sw-keller
Uptime:      266460 seconds
NTP:         Synchronized

Status:      Connected (http://192.168.1.2:8080/inform)
`
	if DetectVendor(info) != VendorUniFi {
		t.Fatal("UniFi nicht erkannt")
	}
	sys := `             total       used       free     shared    buffers     cached
Mem:        124488      93484      31004          0       5476      29724
-/+ buffers/cache:      58284      66204
Mem: 93412K used, 31076K free, 0K shrd, 5476K buff, 29724K cached
CPU:   4% usr   9% sys   0% nic  86% idle   0% io   0% irq   0% sirq
Load average: 0.36 0.39 0.36 1/89 12345
`
	ports := `Port  Link  Speed  Duplex  FlowCtrl
----  ----  -----  ------  --------
   1  Up    1000   Full    Off
   2  Down
   3  Up    100    Full    Off
  25  Down
`
	s := ParseSwitch(VendorUniFi, info, sys, ports, "")
	if s.Model != "USW-24-PoE" || s.Firmware != "6.6.55.15189" || s.Name != "sw-keller" || s.Uptime != "3d 2h 1m" {
		t.Errorf("Kopf: %+v", s)
	}
	if s.CPUPct != 14 || s.MemPct != 46 {
		t.Errorf("CPU/RAM: %d %d", s.CPUPct, s.MemPct)
	}
	if s.PortsUp != 2 || s.PortsDown != 2 {
		t.Errorf("Ports: %d/%d", s.PortsUp, s.PortsDown)
	}
	if VendorUniFi.ConfigCommand() != "cat /tmp/system.cfg" || VendorUniFi.VersionCommand() != "info" || VendorUniFi.PagingOff() != "" {
		t.Error("UniFi-Befehle")
	}
	if p := freeMemPct("              total        used        free      shared  buff/cache   available\nMem:         246084       91280       93800         896       61004      141548\n"); p != 42 {
		t.Errorf("free neu: %d", p)
	}
	if !IsPrompt("USW-24-PoE.6.6.55#") {
		t.Error("UniFi-Prompt")
	}
}
