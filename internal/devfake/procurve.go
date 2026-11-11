// Package devfake ist ein nachgebauter ArubaOS-Switch (ProCurve) für Tests
// und Vorführungen: Exec-Anfragen beenden die Verbindung (wie beim echten
// Gerät), die Shell blättert mit "-- MORE --", bis "no page" kommt, und
// positioniert den Cursor mit ANSI-Sequenzen. "vlan <n>" ändert die
// Konfiguration, damit sich ein Diff vorführen lässt.
package devfake

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

// Switch ist ein laufender Fake-Switch.
type Switch struct {
	Addr  string
	Execs atomic.Int32 // Zahl der (verbotenen) Exec-Anfragen

	ln    net.Listener
	mu    sync.Mutex
	vlans []int
	// PressAnyKey: nach dem Login erst "Press any key to continue" zeigen.
	PressAnyKey bool
	// Positioned: wie echte ArubaOS-Switches neue Zeilen per Cursor-
	// Position (ESC[24;1H) beginnen statt mit "\r\n".
	Positioned bool
}

// StartSwitch lauscht auf addr ("127.0.0.1:0" für einen freien Port).
func StartSwitch(addr string) (*Switch, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil },
		ServerVersion:    "SSH-2.0-Mocana SSH 6.3",
	}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	sw := &Switch{Addr: ln.Addr().String(), ln: ln, vlans: []int{1, 10, 20}}
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go sw.serve(nc, cfg)
		}
	}()
	return sw, nil
}

// Close beendet den Server.
func (sw *Switch) Close() error {
	if sw.ln == nil {
		return nil
	}
	return sw.ln.Close()
}

// AddVLAN ändert die Konfiguration (wie "vlan <n>" in der Shell).
func (sw *Switch) AddVLAN(id int) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	for _, v := range sw.vlans {
		if v == id {
			return
		}
	}
	sw.vlans = append(sw.vlans, id)
}

func (sw *Switch) serve(nc net.Conn, cfg *ssh.ServerConfig) {
	sc, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		if nch.ChannelType() != "session" {
			nch.Reject(ssh.UnknownChannelType, "nein")
			continue
		}
		ch, creqs, err := nch.Accept()
		if err != nil {
			return
		}
		go func() {
			for r := range creqs {
				switch r.Type {
				case "exec":
					sw.Execs.Add(1)
					sc.Close() // wie Aruba/ProCurve: ganze Verbindung weg
					return
				case "pty-req", "window-change":
					r.Reply(r.WantReply, nil)
				case "shell":
					r.Reply(true, nil)
					go sw.shell(ch)
				default:
					r.Reply(false, nil)
				}
			}
		}()
	}
}

const prompt = "\r\x1b[2Ksw-buero# "

func (sw *Switch) shell(ch io.ReadWriteCloser) {
	defer ch.Close()
	w := func(s string) { _, _ = io.WriteString(ch, s) }
	key := make([]byte, 1)
	if sw.PressAnyKey {
		w("\x1b[2J\x1b[1;1HHP J9773A 2530-24G-PoEP Switch\r\nSoftware revision YA.16.10.0009\r\n\r\nPress any key to continue")
		if _, err := ch.Read(key); err != nil {
			return
		}
	}
	page := true
	pr, nl := prompt, "\r\n"
	if sw.Positioned {
		pr, nl = "\x1b[24;1H\x1b[2Ksw-buero# ", "\x1b[24;1H"
	}
	w(pr)
	var line []byte
	conf := false // Konfigurationsmodus: Prompt sw-buero(config)#
	cur := func() string {
		if conf {
			return strings.Replace(pr, "sw-buero# ", "sw-buero(config)# ", 1)
		}
		return pr
	}
	for {
		if _, err := ch.Read(key); err != nil {
			return
		}
		switch c := key[0]; c {
		case '\r', '\n':
			cmd := strings.TrimSpace(string(line))
			line = line[:0]
			w(nl)
			var out string
			switch {
			case conf && (cmd == "end" || cmd == "exit"):
				conf = false
			case cmd == "exit" || cmd == "logout":
				return
			case cmd == "configure terminal" || cmd == "configure" || cmd == "conf t":
				conf = true
			case conf && strings.HasPrefix(cmd, "logging "):
			default:
				if cmd == "no page" {
					page = false
				}
				out = sw.run(cmd)
			}
			if !sw.pageOut(ch, out, page) {
				return
			}
			w(cur())
		case 0x7f, 0x08:
			if len(line) > 0 {
				line = line[:len(line)-1]
				w("\b \b")
			}
		case 0x03:
			line = line[:0]
			w(nl + cur())
		default:
			if c >= 0x20 {
				line = append(line, c)
				w(string(c))
			}
		}
	}
}

// pageOut gibt Zeilen aus, bei page mit "-- MORE --" alle 22 Zeilen.
func (sw *Switch) pageOut(ch io.ReadWriter, out string, page bool) bool {
	if out == "" {
		return true
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	key := make([]byte, 1)
	for i, l := range lines {
		if page && i > 0 && i%22 == 0 {
			_, _ = io.WriteString(ch, "\x1b[7m-- MORE --, next page: Space, next line: Enter, quit: Control-C\x1b[0m")
			if _, err := ch.Read(key); err != nil {
				return false
			}
			_, _ = io.WriteString(ch, "\r\x1b[2K")
			if key[0] == 0x03 || key[0] == 'q' {
				return true
			}
		}
		_, _ = io.WriteString(ch, l+"\r\n")
	}
	return true
}

func (sw *Switch) run(cmd string) string {
	f := strings.Fields(cmd)
	if len(f) == 0 {
		return ""
	}
	if f[0] == "vlan" && len(f) == 2 {
		var id int
		if _, err := fmt.Sscanf(f[1], "%d", &id); err == nil && id > 0 && id < 4095 {
			sw.AddVLAN(id)
			return ""
		}
	}
	switch cmd {
	case "no page":
		return ""
	case "show version":
		return "Image stamp:    /ws/swbuildm/rel_ukiah_qaoff/code/build/bom(swbuildm_rel_ukiah_qaoff_rel_ukiah)\n                Mar 10 2020 13:56:04\n                YA.16.10.0009\n                1102\nBoot Image:     Primary\n"
	case "show system":
		return " Status and Counters - General System Information\n\n  System Name        : sw-buero\n  System Contact     :\n  System Location    : Keller\n\n  Software revision  : YA.16.10.0009        Base MAC Addr      : 941882-123456\n  ROM Version        : YA.15.20            Serial Number      : CN12345678\n\n  Up Time            : 32 days             Memory   - Total   : 343,068,672\n  CPU Util (%)       : 7                   Free    : 201,326,592\n"
	case "show interfaces brief":
		var b strings.Builder
		b.WriteString(" Status and Counters - Port Status\n\n                          | Intrusion                           Flow  Bcast\n  Port         Type       | Alert     Enabled Status Mode       Ctrl  Limit\n  ------------ ---------- + --------- ------- ------ ---------- ----- ------\n")
		for p := 1; p <= 28; p++ {
			st, typ := "Down", "100/1000T"
			if p%3 == 1 {
				st = "Up"
			}
			if p > 24 {
				typ = "SFP+SR"
			}
			fmt.Fprintf(&b, "  %-12d %-10s | No        Yes     %-6s 1000FDx    off   0\n", p, typ, st)
		}
		return b.String()
	case "show vlans":
		var b strings.Builder
		b.WriteString(" Status and Counters - VLAN Information\n\n  Maximum VLANs to support : 256\n  Primary VLAN : DEFAULT_VLAN\n\n  VLAN ID Name                             | Status     Voice Jumbo\n  ------- -------------------------------- + ---------- ----- -----\n")
		for _, v := range sw.vlanList() {
			fmt.Fprintf(&b, "  %-7d %-32s | Port-based No    No\n", v, vlanName(v))
		}
		return b.String()
	case "show logging -r":
		var b strings.Builder
		b.WriteString(" Keys:   W=Warning   I=Information\n         M=Major     D=Debug E=Error\n ----  Reverse event Log listing: Events Since Boot  ----\n")
		now := time.Now()
		for i := 0; i < 8; i++ {
			t := now.Add(-time.Duration(i*7+1) * time.Minute)
			fmt.Fprintf(&b, " I %s 00076 ports: port 23 is now on-line\n", t.Format("01/02/06 15:04:05"))
			fmt.Fprintf(&b, " W %s 00077 ports: port 23 is now off-line\n", t.Add(-3*time.Second).Format("01/02/06 15:04:05"))
		}
		fmt.Fprintf(&b, " M %s 00921 loop-protect: port 17 disabled - loop detected on VLAN 20\n", now.Add(-2*time.Hour).Format("01/02/06 15:04:05"))
		fmt.Fprintf(&b, " I %s 00422 mgr: SME SSH from 10.0.0.50 - MANAGER Mode\n", now.Add(-3*time.Hour).Format("01/02/06 15:04:05"))
		return b.String()
	case "show mac-address":
		return " Status and Counters - Port Address Table\n\n  MAC Address   Port  VLAN\n  ------------- ----- ----\n  001122-aabbcc 4     1\n  941882-000001 25    1\n  941882-000002 25    1\n"
	case "show arp":
		return " IP ARP table\n\n  IP Address       MAC Address       Type    Port\n  ---------------  ----------------- ------- ----\n  192.168.1.20     001122-aabbcc     dynamic 4\n"
	case "show lldp info remote-device":
		return " LLDP Remote Devices Information\n\n  LocalPort | ChassisId                 PortId PortDescr SysName\n  --------- + ------------------------- ------ --------- ----------\n  25        | 94 18 82 00 00 01         1/1/48 1/1/48    core\n"
	case "show running-config":
		var b strings.Builder
		b.WriteString("\nRunning configuration:\n\n; J9773A Configuration Editor; Created on release #YA.16.10.0009\n; Ver #14:01.4f.f8.1d.9b.3f.bf.bb.ef.7c.59.fc.6b.fb.9f.fc.ff.ff.37.ef:05\nhostname \"sw-buero\"\nmodule 1 type j9773a\nsnmp-server community \"public\" unrestricted\noobm\n   ip address dhcp-bootp\n   exit\n")
		for _, v := range sw.vlanList() {
			fmt.Fprintf(&b, "vlan %d\n   name \"%s\"\n", v, vlanName(v))
			if v == 1 {
				b.WriteString("   untagged 1-28\n   ip address dhcp-bootp\n")
			} else {
				fmt.Fprintf(&b, "   tagged 25-28\n   no ip address\n")
			}
			b.WriteString("   exit\n")
		}
		b.WriteString("spanning-tree\npassword manager\n")
		return b.String()
	}
	return "Invalid input: " + f[0] + "\n"
}

func (sw *Switch) vlanList() []int {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return append([]int(nil), sw.vlans...)
}

func vlanName(v int) string {
	switch v {
	case 1:
		return "DEFAULT_VLAN"
	case 10:
		return "Server"
	case 20:
		return "Clients"
	}
	return fmt.Sprintf("VLAN%d", v)
}
