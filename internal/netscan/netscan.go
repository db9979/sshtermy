// Package netscan sucht in einem Subnetz nach SSH-Geräten: TCP-Verbindung auf
// den Port, SSH-Kennung (Banner) lesen, wieder schließen. Es gibt keine
// Anmeldung und keinen Schlüsselaustausch. Am Banner wird erkannt, ob es ein
// Server oder ein Netzwerkgerät (Switch, Router, Firewall) ist.
package netscan

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// MaxHosts: mehr Adressen auf einmal werden nicht gescannt (/20).
const MaxHosts = 4096

// Host ist ein Fund.
type Host struct {
	IP      string
	Port    int
	Name    string // Reverse-DNS, ohne Punkt am Ende (leer, wenn unbekannt)
	Banner  string // z. B. "SSH-2.0-OpenSSH_9.6p1 Ubuntu-3ubuntu13"
	Network bool   // Banner sieht nach Switch/Router aus
}

// Label: kurzer Name des Geräts aus dem Banner, z. B. "OpenSSH 9.6p1 (Ubuntu)".
func (h Host) Label() string { return Describe(h.Banner) }

// Kind: "Netzwerkgerät" oder "Server".
func (h Host) Kind() string {
	if h.Network {
		return i18n.T("Netzwerkgerät")
	}
	return "Server"
}

// ParseTargets versteht CIDR ("192.168.1.0/24"), einzelne Adressen,
// Bereiche ("10.0.0.10-50" oder "10.0.0.10-10.0.0.50") und Listen mit Komma
// oder Leerzeichen. Bei IPv4-Netzen größer /31 fallen Netz- und
// Broadcast-Adresse weg.
func ParseTargets(spec string) ([]netip.Addr, error) {
	var out []netip.Addr
	seen := map[netip.Addr]bool{}
	add := func(a netip.Addr) error {
		if seen[a] {
			return nil
		}
		if len(out) >= MaxHosts {
			return fmt.Errorf(i18n.T("zu viele Adressen (höchstens %d, z. B. ein /20)"), MaxHosts)
		}
		seen[a] = true
		out = append(out, a)
		return nil
	}
	parts := strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '\t' })
	if len(parts) == 0 {
		return nil, errors.New(i18n.T("Subnetz fehlt, z. B. 192.168.1.0/24"))
	}
	for _, p := range parts {
		switch {
		case strings.Contains(p, "/"):
			pf, err := netip.ParsePrefix(p)
			if err != nil {
				return nil, fmt.Errorf(i18n.T("„%s“ ist kein Subnetz"), p)
			}
			pf = pf.Masked()
			bits := pf.Addr().BitLen() - pf.Bits()
			if bits > 12 {
				return nil, fmt.Errorf(i18n.T("„%s“ ist zu groß (höchstens %d Adressen)"), p, MaxHosts)
			}
			n := 1 << bits
			a := pf.Addr()
			for i := 0; i < n; i++ {
				skip := pf.Addr().Is4() && bits >= 2 && (i == 0 || i == n-1)
				if !skip {
					if err := add(a); err != nil {
						return nil, err
					}
				}
				a = a.Next()
			}
		case strings.Contains(p, "-"):
			from, to, _ := strings.Cut(p, "-")
			a, err := netip.ParseAddr(from)
			if err != nil {
				return nil, fmt.Errorf(i18n.T("„%s“: Anfang ist keine Adresse"), p)
			}
			var b netip.Addr
			if n, err := strconv.Atoi(to); err == nil && a.Is4() {
				if n < 0 || n > 255 {
					return nil, fmt.Errorf(i18n.T("„%s“: Ende muss 0–255 sein"), p)
				}
				x := a.As4()
				x[3] = byte(n)
				b = netip.AddrFrom4(x)
			} else if b, err = netip.ParseAddr(to); err != nil {
				return nil, fmt.Errorf(i18n.T("„%s“: Ende ist keine Adresse"), p)
			}
			if b.Less(a) {
				return nil, fmt.Errorf(i18n.T("„%s“: Ende liegt vor dem Anfang"), p)
			}
			for x := a; ; x = x.Next() {
				if err := add(x); err != nil {
					return nil, err
				}
				if x == b {
					break
				}
			}
		default:
			a, err := netip.ParseAddr(p)
			if err != nil {
				return nil, fmt.Errorf(i18n.T("„%s“ ist keine IP-Adresse"), p)
			}
			if err := add(a); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// LocalSubnets: IPv4-Netze der eigenen Netzwerkkarten (ohne Loopback und
// Link-Local), größere Netze auf das eigene /24 verkleinert. Variable, damit
// die Bildschirm-Tests feste Netze einsetzen können.
var LocalSubnets = localSubnets

func localSubnets() []string {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil || ipn.IP.IsLinkLocalUnicast() || ipn.IP.IsLoopback() {
				continue
			}
			ones, _ := ipn.Mask.Size()
			if ones < 24 {
				ones = 24
			}
			ip, _ := netip.AddrFromSlice(ipn.IP.To4())
			pf, err := ip.Prefix(ones)
			if err != nil {
				continue
			}
			s := pf.String()
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// Options für Scan.
type Options struct {
	Port     int           // Standard 22
	Timeout  time.Duration // Verbindungsaufbau pro Adresse, Standard 800 ms
	Banner   time.Duration // Warten auf die SSH-Kennung, Standard 2 s
	Parallel int           // gleichzeitige Versuche, Standard 128
	NoDNS    bool          // keine Namensauflösung
}

// Scan probiert alle Adressen. progress meldet (fertig, gesamt), found jeden
// Fund (beides aus verschiedenen Goroutinen). Ergebnis: Funde nach Adresse.
func Scan(ctx context.Context, addrs []netip.Addr, o Options, progress func(done, total int), found func(Host)) []Host {
	if o.Port <= 0 {
		o.Port = 22
	}
	if o.Timeout <= 0 {
		o.Timeout = 800 * time.Millisecond
	}
	if o.Banner <= 0 {
		o.Banner = 2 * time.Second
	}
	if o.Parallel <= 0 {
		o.Parallel = 128
	}
	var (
		mu    sync.Mutex
		hosts []Host
		done  int
		wg    sync.WaitGroup
	)
	sem := make(chan struct{}, o.Parallel)
	for _, a := range addrs {
		select {
		case <-ctx.Done():
		case sem <- struct{}{}:
			wg.Add(1)
			go func(a netip.Addr) {
				defer wg.Done()
				defer func() { <-sem }()
				h, ok := probe(ctx, a, o)
				mu.Lock()
				done++
				d := done
				if ok {
					hosts = append(hosts, h)
				}
				mu.Unlock()
				if ok && found != nil {
					found(h)
				}
				if progress != nil {
					progress(d, len(addrs))
				}
			}(a)
			continue
		}
		break
	}
	wg.Wait()
	sort.Slice(hosts, func(i, j int) bool {
		a, _ := netip.ParseAddr(hosts[i].IP)
		b, _ := netip.ParseAddr(hosts[j].IP)
		return a.Less(b)
	})
	return hosts
}

func probe(ctx context.Context, a netip.Addr, o Options) (Host, bool) {
	addr := netip.AddrPortFrom(a, uint16(o.Port)).String()
	d := net.Dialer{Timeout: o.Timeout}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return Host{}, false
	}
	_ = c.SetDeadline(time.Now().Add(o.Banner))
	banner := readBanner(c)
	c.Close()
	if banner == "" {
		return Host{}, false // offen, aber kein SSH
	}
	h := Host{IP: a.String(), Port: o.Port, Banner: banner, Network: sshx.IsNetworkBanner(banner)}
	if !o.NoDNS {
		rctx, cancel := context.WithTimeout(ctx, time.Second)
		names, _ := net.DefaultResolver.LookupAddr(rctx, a.String())
		cancel()
		if len(names) > 0 {
			h.Name = strings.TrimSuffix(names[0], ".")
		}
	}
	return h, true
}

// readBanner liest die SSH-Kennung. Server dürfen davor andere Zeilen
// schicken (RFC 4253, 4.2); gesucht wird die erste Zeile mit "SSH-".
func readBanner(c net.Conn) string {
	r := bufio.NewReaderSize(c, 512)
	for i := 0; i < 10; i++ {
		line, err := r.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "SSH-") {
			if len(line) > 255 {
				line = line[:255]
			}
			return line
		}
		if err != nil {
			return ""
		}
	}
	return ""
}

// Describe macht aus der SSH-Kennung etwas Lesbares:
// "SSH-2.0-OpenSSH_9.6p1 Ubuntu-3ubuntu13" → "OpenSSH 9.6p1 (Ubuntu)".
func Describe(banner string) string {
	v := banner
	if i := strings.Index(v, "-"); i >= 0 { // "SSH-"
		v = v[i+1:]
		if j := strings.Index(v, "-"); j >= 0 { // "2.0-"
			v = v[j+1:]
		}
	}
	soft, comment, _ := strings.Cut(v, " ")
	soft = strings.Replace(soft, "_", " ", 1)
	if soft == "" {
		soft = "SSH"
	}
	if comment != "" {
		os := comment
		if i := strings.IndexAny(os, "-_ "); i > 0 {
			os = os[:i]
		}
		return soft + " (" + os + ")"
	}
	return soft
}
