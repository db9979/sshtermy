// Package wol weckt Rechner im eigenen Netz per Wake-on-LAN auf: ein
// „Magic Packet“ (6× 0xFF, dann 16× die MAC-Adresse) als UDP-Broadcast.
// Dazu findet es die MAC-Adresse eines Nachbarn in der ARP-Tabelle des
// Betriebssystems, damit man sie nicht von Hand eintragen muss.
package wol

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// ParseMAC liest aa:bb:cc:dd:ee:ff, aa-bb-…, aabb.ccdd.eeff und aabbccddeeff.
// Nur 6-Byte-Adressen (Ethernet); die Null- und die Broadcast-Adresse gelten nicht.
func ParseMAC(s string) (net.HardwareAddr, error) {
	s = strings.TrimSpace(s)
	if len(s) == 12 && !strings.ContainsAny(s, ":-.") {
		s = s[0:2] + ":" + s[2:4] + ":" + s[4:6] + ":" + s[6:8] + ":" + s[8:10] + ":" + s[10:12]
	}
	hw, err := net.ParseMAC(s)
	if err != nil {
		return nil, err
	}
	if len(hw) != 6 {
		return nil, errors.New("wol: keine Ethernet-Adresse")
	}
	zero, ones := true, true
	for _, b := range hw {
		zero, ones = zero && b == 0, ones && b == 0xff
	}
	if zero || ones {
		return nil, errors.New("wol: keine Geräteadresse")
	}
	return hw, nil
}

// Packet baut das Magic Packet.
func Packet(hw net.HardwareAddr) []byte {
	p := bytes.Repeat([]byte{0xff}, 6)
	for i := 0; i < 16; i++ {
		p = append(p, hw...)
	}
	return p
}

// Targets: wohin das Paket geht. Der allgemeine Broadcast und der jedes
// eigenen IPv4-Netzes (manche Systeme schicken 255.255.255.255 nur über eine
// Schnittstelle), je auf den üblichen Ports 9 und 7.
func Targets() []string {
	seen := map[string]bool{}
	var out []string
	add := func(ip net.IP) {
		for _, port := range []string{"9", "7"} {
			if a := net.JoinHostPort(ip.String(), port); !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	add(net.IPv4bcast)
	ifs, _ := net.Interfaces()
	for _, in := range ifs {
		if in.Flags&net.FlagUp == 0 || in.Flags&net.FlagLoopback != 0 || in.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, _ := in.Addrs()
		for _, a := range addrs {
			if b := broadcastOf(a); b != nil {
				add(b)
			}
		}
	}
	return out
}

func broadcastOf(a net.Addr) net.IP {
	n, ok := a.(*net.IPNet)
	if !ok {
		return nil
	}
	ip, mask := n.IP.To4(), n.Mask
	if ip == nil || len(mask) != 4 {
		if len(mask) == 16 {
			mask = mask[12:]
		}
		if ip == nil || len(mask) != 4 {
			return nil
		}
	}
	b := make(net.IP, 4)
	for i := range b {
		b[i] = ip[i] | ^mask[i]
	}
	return b
}

// send: Variable für Tests.
var send = func(addr string, pkt []byte) error {
	c, err := net.DialTimeout("udp4", addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	_, err = c.Write(pkt)
	return err
}

// Wake schickt das Magic Packet an alle Ziele. Es gibt keine Antwort: ob der
// Rechner aufwacht, zeigt erst die Erreichbarkeit danach. Fehler nur, wenn
// kein einziges Paket hinausging.
func Wake(mac string) (sent int, err error) {
	hw, err := ParseMAC(mac)
	if err != nil {
		return 0, err
	}
	pkt := Packet(hw)
	var last error
	for _, t := range Targets() {
		if e := send(t, pkt); e != nil {
			last = e
			continue
		}
		sent++
	}
	if sent == 0 {
		if last == nil {
			last = errors.New("wol: kein Netz mit Broadcast gefunden")
		}
		return 0, last
	}
	return sent, nil
}

var reMAC = regexp.MustCompile(`(?i)\b([0-9a-f]{1,2}([:-])[0-9a-f]{1,2}(?:[:-][0-9a-f]{1,2}){4})\b`)

// macIn sucht in einer Zeile der ARP-Tabelle die MAC zur IP.
func macIn(line, ip string) string {
	fs := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' || r == '(' || r == ')' })
	hit := false
	for _, f := range fs {
		if f == ip {
			hit = true
		}
	}
	if !hit {
		return ""
	}
	m := reMAC.FindString(line)
	if m == "" {
		return ""
	}
	// macOS lässt führende Nullen weg (a:b:c:…): auffüllen.
	parts := strings.FieldsFunc(m, func(r rune) bool { return r == ':' || r == '-' })
	for i, p := range parts {
		if len(p) == 1 {
			parts[i] = "0" + p
		}
	}
	hw, err := ParseMAC(strings.Join(parts, ":"))
	if err != nil {
		return ""
	}
	return hw.String()
}

// parseARP: erste passende Zeile.
func parseARP(table []byte, ip string) string {
	sc := bufio.NewScanner(bytes.NewReader(table))
	for sc.Scan() {
		if m := macIn(sc.Text(), ip); m != "" {
			return m
		}
	}
	return ""
}

// Lookup: MAC-Adresse eines Nachbarn im eigenen Netz aus der ARP-Tabelle
// ("" = unbekannt, z. B. hinter einem Router). Sinnvoll direkt nach einer
// Verbindung, dann steht der Eintrag sicher drin.
func Lookup(ctx context.Context, host string) string {
	ip := net.ParseIP(host)
	if ip == nil {
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
		if err != nil || len(ips) == 0 {
			return ""
		}
		ip = ips[0]
	}
	if ip = ip.To4(); ip == nil || !ip.IsPrivate() {
		return ""
	}
	if runtime.GOOS == "linux" {
		if b, err := os.ReadFile("/proc/net/arp"); err == nil {
			return parseARP(b, ip.String())
		}
	}
	args := []string{"-n", ip.String()}
	if runtime.GOOS == "windows" {
		args = []string{"-a", ip.String()}
	}
	cmd := exec.CommandContext(ctx, "arp", args...)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseARP(out, ip.String())
}
