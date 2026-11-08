package hostinfo

import "testing"

func TestJunos(t *testing.T) {
	ver := "fpc0:\n--------------------------------------------------------------------------\nHostname: ex-core1\nModel: ex2300-24t\nJunos: 21.4R3-S4.9\nJUNOS OS Kernel 64-bit  [20230428.f3a2b1c_builder_stable_12]\n"
	if DetectVendor(ver) != VendorJunos {
		t.Fatal("Junos nicht erkannt")
	}
	sys := `Routing Engine status:
    Temperature                 42 degrees C / 107 degrees F
    Total memory              2048 MB Max   553 MB used ( 27 percent)
      Control plane memory    1350 MB Max   553 MB used ( 41 percent)
    Memory utilization          27 percent
    CPU utilization:
      User                       3 percent
      Background                 0 percent
      Kernel                     5 percent
      Interrupt                  0 percent
      Idle                      92 percent
    Model                          RE-EX2300-24T
    Start time                     2026-10-01 08:12:44 CEST
    Uptime                         38 days, 2 hours, 3 minutes, 1 second
`
	ports := `Interface               Admin Link Proto    Local                 Remote
ge-0/0/0                up    up
ge-0/0/0.0              up    up   eth-switch
ge-0/0/1                up    down
ge-0/0/2                down  down
xe-0/1/0                up    up
irb                     up    up
vme                     up    down
`
	vlans := `Routing instance        VLAN name             Tag          Interfaces
default-switch          default               1
                                                               ge-0/0/0.0*
default-switch          mgmt                  10
default-switch          voice                 20
`
	s := ParseSwitch(VendorJunos, ver, sys, ports, vlans)
	if s.Name != "ex-core1" || s.Model != "ex2300-24t" || s.Firmware != "21.4R3-S4.9" {
		t.Errorf("Kopf: %+v", s)
	}
	if s.CPUPct != 8 || s.MemPct != 27 || s.Uptime == "" {
		t.Errorf("CPU/RAM/Uptime: %d %d %q", s.CPUPct, s.MemPct, s.Uptime)
	}
	if s.PortsUp != 2 || s.PortsDown != 2 {
		t.Errorf("Ports: %d/%d", s.PortsUp, s.PortsDown)
	}
	if len(s.VLANs) != 3 || s.VLANs[1] != (VLAN{10, "mgmt"}) {
		t.Errorf("VLANs: %+v", s.VLANs)
	}
}

func TestRouterOS(t *testing.T) {
	res := `                   uptime: 3w2d4h12m
                  version: 7.12.1 (stable)
               build-time: 2023-11-17 11:38:45
              free-memory: 768.0MiB
             total-memory: 1024.0MiB
                      cpu: ARM64
                cpu-count: 4
                 cpu-load: 3%
               board-name: RB5009UG+S+
                 platform: MikroTik
`
	if DetectVendor(res) != VendorMikroTik {
		t.Fatal("RouterOS nicht erkannt")
	}
	ports := ` 0 R  name=ether1 default-name=ether1 mtu=1500 mac-address=48:A9:8A:00:00:01
 1    name=ether2 default-name=ether2 mtu=1500
 2 RS name=ether3 default-name=ether3 mtu=1500
 3 X  name=ether4 default-name=ether4 mtu=1500
`
	vlans := ` 0 R  name=vlan10 mtu=1500 arp=enabled vlan-id=10 interface=bridge
 1 R  name="vlan 20" mtu=1500 vlan-id=20 interface=bridge
`
	s := ParseSwitch(VendorMikroTik, res, "  name: rb-keller\n", ports, vlans)
	if s.Name != "rb-keller" || s.Model != "RB5009UG+S+" || s.Firmware != "7.12.1" || s.Uptime != "3w2d4h12m" {
		t.Errorf("Kopf: %+v", s)
	}
	if s.CPUPct != 3 || s.MemPct != 25 {
		t.Errorf("CPU/RAM: %d %d", s.CPUPct, s.MemPct)
	}
	if s.PortsUp != 2 || s.PortsDown != 2 {
		t.Errorf("Ports: %d/%d", s.PortsUp, s.PortsDown)
	}
	if len(s.VLANs) != 2 || s.VLANs[1].Name != "vlan 20" {
		t.Errorf("VLANs: %+v", s.VLANs)
	}
}

func TestFortiOS(t *testing.T) {
	st := `Version: FortiGate-60F v7.2.5,build1517,230606 (GA.F)
Security Level: 1
Serial-Number: FGT60FTK2209XXXX
Hostname: FGT-Buero
Operation Mode: NAT
`
	if DetectVendor(st) != VendorFortiOS {
		t.Fatal("FortiOS nicht erkannt")
	}
	perf := `CPU states: 2% user 1% system 0% nice 96% idle 0% iowait 0% irq 1% softirq
CPU0 states: 2% user 1% system 0% nice 96% idle 0% iowait 0% irq 1% softirq
Memory: 2010676k total, 1063216k used (52.9%), 747736k free (37.2%), 199724k freeable (9.9%)
Average network usage: 1234 / 567 kbps in 1 minute
Uptime: 12 days,  3 hours,  4 minutes
`
	ports := `== [onboard]
        ==[wan1]
                mode: dhcp
                ip: 192.0.2.10 255.255.255.0
                status: up
                speed: 1000Mbps (Duplex: full)
        ==[wan2]
                mode: static
                status: down
        ==[internal1]
                status: up
`
	s := ParseSwitch(VendorFortiOS, st, perf, ports, "")
	if s.Name != "FGT-Buero" || s.Model != "FortiGate-60F" || s.Firmware != "v7.2.5" {
		t.Errorf("Kopf: %+v", s)
	}
	if s.CPUPct != 4 || s.MemPct != 52 || s.Uptime != "12 days, 3 hours, 4 minutes" {
		t.Errorf("CPU/RAM/Uptime: %d %d %q", s.CPUPct, s.MemPct, s.Uptime)
	}
	if s.PortsUp != 2 || s.PortsDown != 1 {
		t.Errorf("Ports: %d/%d", s.PortsUp, s.PortsDown)
	}
}

func TestVendorCommandsAndPrompts(t *testing.T) {
	for v, cfg := range map[Vendor]string{VendorJunos: "show configuration | display set", VendorMikroTik: "/export", VendorFortiOS: "show full-configuration", VendorCisco: "show running-config"} {
		if v.ConfigCommand() != cfg {
			t.Errorf("%s: %q", v, v.ConfigCommand())
		}
	}
	for _, p := range []string{"admin@ex-core1>", "admin@ex-core1#", "[admin@MikroTik] >", "[admin@rb-keller] /interface>", "FGT-Buero #", "FGT-Buero (root) #"} {
		if !IsPrompt(p) {
			t.Errorf("%q ist ein Prompt", p)
		}
	}
	for _, p := range []string{"a b#", "user@host:~$", "Press any key to continue", "[admin@MikroTik] > /export"} {
		if IsPrompt(p) {
			t.Errorf("%q ist kein Prompt", p)
		}
	}
	for _, raw := range []string{"line\n---(more)---", "line\n---(more 45%)---", "x\n-- [Q quit|D dump|down]"} {
		if !HasMore(raw) {
			t.Errorf("Blättern nicht erkannt: %q", raw)
		}
	}
}
