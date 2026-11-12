package relay

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/webwatch"
)

// Teilen im lokalen Netz ohne Vermittler
//
// sshTermy startet für die Dauer der Freigabe denselben Teilen-Teil, den
// auch `sshtermy vermittler` hat, auf diesem Rechner: zufälliger Port, frisch
// erzeugtes, selbst signiertes Zertifikat (Fingerabdruck steht im Link und
// wird gepinnt). Nur sshTermy selbst darf dort Freigaben anlegen (LocalKey).
// Zuschauer im selben Netz finden den Rechner über den Platz im Code per
// UDP-Rundruf (LANPort), ohne dass jemand eine Adresse tippt.

// LANPort: UDP-Port für „Wer teilt Platz N?“.
const LANPort = 47831

const lanQuery = "sshtermy-teilen?"

// LAN: eingebauter Teilen-Vermittler.
type LAN struct {
	Base  string // https://192.168.x.y:PORT (für Zuschauer)
	FP    string
	Local *Client // Gastgeber spricht über 127.0.0.1
	srv   *http.Server
	udp   *net.UDPConn
	mu    sync.Mutex
	open  map[int]bool // Plätze, nach denen gefragt werden darf
	once  sync.Once
}

// StartLAN öffnet den eingebauten Vermittler. Fehlt ein Netz, kommt ein Fehler.
func StartLAN() (*LAN, error) {
	ip := lanIP()
	if ip == "" {
		return nil, i18n.NewError("Kein lokales Netz gefunden (WLAN/LAN verbunden?)")
	}
	cert, err := memCert(ip)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	kb := make([]byte, 32)
	_, _ = rand.Read(kb)
	key := base64.RawURLEncoding.EncodeToString(kb)
	s := &Server{LocalKey: key, st: newState(), anchors: map[string]*anchorLink{}, pending: map[string]*pendingChan{},
		active: map[string]*activeChan{}, limit: newLimiter(), Logf: nil}
	m := http.NewServeMux()
	m.HandleFunc("POST "+PathShare, s.shareCreate)
	m.HandleFunc("GET "+PathShareSend, s.shareSend)
	m.HandleFunc("GET "+PathShareWatch, s.shareWatch)
	webwatch.Register(m)
	hs := &http.Server{Handler: m, ReadHeaderTimeout: 10 * time.Second, MaxHeaderBytes: 16 << 10,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}}
	go func() { _ = hs.ServeTLS(ln, "", "") }()
	fp := CertFP(cert.Certificate[0])
	l := &LAN{Base: "https://" + net.JoinHostPort(ip, strconv.Itoa(port)), FP: fp, srv: hs, open: map[int]bool{}}
	l.Local, err = NewClient("https://"+net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), key, fp)
	if err != nil {
		l.Close()
		return nil, err
	}
	if u, err := net.ListenUDP("udp4", &net.UDPAddr{Port: LANPort}); err == nil {
		l.udp = u
		go l.answer()
	}
	return l, nil
}

// Findable: kann man den Rechner per Code finden (UDP-Port frei)?
func (l *LAN) Findable() bool { return l.udp != nil }

// Announce: Platz darf gefunden werden (false = Code verbraucht).
func (l *LAN) Announce(platz int, on bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if on {
		l.open[platz] = true
	} else {
		delete(l.open, platz)
	}
}

type lanAnswer struct {
	Base  string `json:"b"`
	FP    string `json:"fp"`
	Platz int    `json:"p"`
}

func (l *LAN) answer() {
	buf := make([]byte, 512)
	for {
		n, from, err := l.udp.ReadFromUDP(buf)
		if err != nil {
			return
		}
		q := string(buf[:n])
		if !strings.HasPrefix(q, lanQuery) {
			continue
		}
		p, err := strconv.Atoi(strings.TrimPrefix(q, lanQuery))
		l.mu.Lock()
		ok := err == nil && l.open[p]
		l.mu.Unlock()
		if !ok {
			continue
		}
		b, _ := json.Marshal(lanAnswer{Base: l.Base, FP: l.FP, Platz: p})
		_, _ = l.udp.WriteToUDP(b, from)
	}
}

// Close beendet den eingebauten Vermittler.
func (l *LAN) Close() {
	l.once.Do(func() {
		if l.udp != nil {
			_ = l.udp.Close()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = l.srv.Shutdown(ctx)
		_ = l.srv.Close()
	})
}

// Found: ein Rechner im Netz, der den Platz teilt.
type Found struct{ Base, FP string }

// FindLAN fragt im lokalen Netz, wer Platz platz teilt (Rundruf).
func FindLAN(ctx context.Context, platz int, wait time.Duration) ([]Found, error) {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer c.Close()
	q := []byte(lanQuery + strconv.Itoa(platz))
	targets := []string{"255.255.255.255"}
	targets = append(targets, broadcasts()...)
	targets = append(targets, "127.0.0.1") // derselbe Rechner (Test, zweites Fenster)
	for _, t := range targets {
		_, _ = c.WriteToUDP(q, &net.UDPAddr{IP: net.ParseIP(t), Port: LANPort})
	}
	dl := time.Now().Add(wait)
	if d, ok := ctx.Deadline(); ok && d.Before(dl) {
		dl = d
	}
	_ = c.SetReadDeadline(dl)
	seen := map[string]bool{}
	var out []Found
	buf := make([]byte, 1024)
	for {
		n, _, err := c.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return out, nil
			}
			return out, err
		}
		var a lanAnswer
		if json.Unmarshal(buf[:n], &a) != nil || a.Platz != platz || !fpRE.MatchString(a.FP) {
			continue
		}
		if _, err := NormBase(a.Base); err != nil || seen[a.Base] {
			continue
		}
		seen[a.Base] = true
		out = append(out, Found{a.Base, a.FP})
	}
}

var lanIP = LocalIP // Tests: 127.0.0.1

// LocalIP: Adresse dieses Rechners im lokalen Netz (Weg ins Internet, sonst
// die erste private IPv4-Adresse). Es wird nichts gesendet.
func LocalIP() string {
	if c, err := net.Dial("udp4", "192.0.2.1:9"); err == nil {
		ip := c.LocalAddr().(*net.UDPAddr).IP
		c.Close()
		if ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return ip.String()
		}
	}
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		as, _ := i.Addrs()
		for _, a := range as {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && n.IP.IsPrivate() {
				return n.IP.String()
			}
		}
	}
	return ""
}

func broadcasts() []string {
	var out []string
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagBroadcast == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		as, _ := i.Addrs()
		for _, a := range as {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil {
				continue
			}
			ip, m := n.IP.To4(), n.Mask
			if len(m) == 16 {
				m = m[12:]
			}
			b := make(net.IP, 4)
			for k := range b {
				b[k] = ip[k] | ^m[k]
			}
			out = append(out, b.String())
		}
	}
	return out
}

// memCert: selbst signiertes Zertifikat nur im Speicher (gilt einen Tag).
func memCert(ip string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	sn, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	tpl := &x509.Certificate{
		SerialNumber: sn, Subject: pkix.Name{CommonName: "sshTermy (teilen)"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP(ip), net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}
