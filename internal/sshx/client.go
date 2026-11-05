// Package sshx ist die SSH-Schicht: Verbindungsaufbau (inkl. Jump-Host),
// Authentifizierung, Host-Key-Prüfung, PTY-Sitzungen und Port-Forwarding.
// Basis ist golang.org/x/crypto/ssh (reines Go, kein cgo, daher problemlos für
// windows/arm64, windows/386 und darwin cross-kompilierbar).
package sshx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/sysprof"
)

// Resolver liefert eine gespeicherte Verbindung (für Jump-Hosts).
type Resolver func(id string) (domain.Connection, bool)

// Client ist eine offene SSH-Verbindung samt optionalem Jump-Host.
type Client struct {
	*ssh.Client
	Conn   domain.Connection
	jump   *Client
	Remote RemoteInfo
	// Profile: frisch erkanntes Systemprofil (SSH-Kennung + Abfrage). Die
	// UI legt es mit sysprof.Merge über das gespeicherte Profil.
	Profile domain.SystemProfile
	// NetDevice: Gegenstelle ist ein Switch/Router. Dann gibt es keine
	// Exec-Abfragen, und die Shell wird möglichst schlicht angefordert.
	NetDevice bool
	// Learned: NetDevice wurde erst durch den Rückfall erkannt (Verbindung
	// starb nach der Systemabfrage). Die UI merkt sich das in der Verbindung.
	Learned bool
	cancel  context.CancelFunc
}

// RemoteInfo: was wir über das Zielsystem wissen (Kontext fürs LLM).
type RemoteInfo struct {
	OS     string // z. B. "Ubuntu 24.04.1 LTS"
	Kernel string // uname -srm
	Shell  string // $SHELL
}

func (r RemoteInfo) String() string {
	parts := []string{}
	for _, p := range []string{r.OS, r.Kernel, r.Shell} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return "unbekannt"
	}
	return strings.Join(parts, " · ")
}

// Options für Dial.
type Options struct {
	HostKeys  *HostKeyStore
	Prompt    HostKeyPrompt
	Resolve   Resolver
	AskSecret SecretPrompt // fehlende Passwörter/Passphrasen erfragen (optional)
	Timeout   time.Duration

	forceNet  bool    // intern: zweiter Versuch nach abgebrochener Systemabfrage
	legacy    bool    // intern: alte Algorithmen (nur wenn der Server nichts anderes kann)
	waitScale float64 // intern: Wartezeiten zwischen Versuchen skalieren (Tests)
}

// Dial baut die Verbindung auf. Bei JumpHostID wird zuerst der Jump-Host
// verbunden und die Zielverbindung durch ihn getunnelt (wie ssh -J).
//
// Netzwerkgeräte: Viele Switches (z. B. Aruba/ProCurve) beenden die ganze
// Verbindung, sobald ein Exec-Kanal kommt oder der erste Kanal schließt. Dann
// käme nach dem Login sofort EOF. Deshalb: bei bekanntem Banner oder
// Gerätetyp "network" keine Systemabfrage; bricht die Verbindung nach der
// Abfrage trotzdem ab, wird einmal ohne Abfrage neu verbunden.
func Dial(ctx context.Context, c domain.Connection, o Options) (*Client, error) {
	if o.AskSecret != nil {
		o.AskSecret = rememberSecrets(o.AskSecret) // Neuverbindung fragt nicht doppelt
	}
	cl, err := dialRetry(ctx, c, o, 2*time.Second)
	if err != nil && c.Device != domain.DeviceServer && isNoCommonAlgo(err) {
		// Alte Geräte können oft nur diffie-hellman-group14-sha1, aes-cbc
		// oder ssh-rsa. Die Aushandlung bevorzugt trotzdem moderne Verfahren.
		o.legacy = true
		cl, err = dialRetry(ctx, c, o, 2*time.Second)
	}
	if err != nil {
		return nil, err
	}
	if cl.NetDevice || c.Device == domain.DeviceServer {
		return cl, nil
	}
	if cl.alive() {
		return cl, nil
	}
	// Verbindung hat die Systemabfrage nicht überlebt → Netzwerkgerät. Der
	// Switch gibt die abgebrochene Sitzung erst nach ein paar Sekunden frei
	// (sonst "handshake failed: EOF"), deshalb kurz warten und notfalls
	// mehrmals versuchen.
	cl.Close()
	o.forceNet = true
	if err := sleepCtx(ctx, o.retryWait(1500*time.Millisecond)); err != nil {
		return nil, err
	}
	cl, err = dialRetry(ctx, c, o, 2*time.Second, 4*time.Second)
	if err != nil {
		return nil, err
	}
	cl.Learned = true
	return cl, nil
}

// dialRetry wiederholt den Aufbau, wenn die Gegenstelle mitten im Handshake
// auflegt (EOF/Reset). Typisch für Switches mit wenigen SSH-Plätzen, solange
// eine alte Sitzung noch nicht freigegeben ist. Andere Fehler (falsches
// Passwort, Host-Key abgelehnt, Timeout) werden nicht wiederholt.
func dialRetry(ctx context.Context, c domain.Connection, o Options, waits ...time.Duration) (*Client, error) {
	for i := 0; ; i++ {
		cl, err := dial(ctx, c, o)
		if err == nil || i >= len(waits) || !isHangup(err) {
			return cl, err
		}
		if err := sleepCtx(ctx, o.retryWait(waits[i])); err != nil {
			return nil, err
		}
	}
}

func isHangup(err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	m := err.Error()
	return strings.Contains(m, "connection reset") || strings.HasSuffix(m, ": EOF")
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// retryWait: in Tests verkürzt.
func (o Options) retryWait(d time.Duration) time.Duration {
	if o.waitScale > 0 {
		return time.Duration(float64(d) * o.waitScale)
	}
	return d
}

func dial(ctx context.Context, c domain.Connection, o Options) (*Client, error) {
	if o.Timeout == 0 {
		o.Timeout = 15 * time.Second
	}
	var (
		raw  net.Conn
		jump *Client
	)
	// Die Handshake-Frist darf nicht ablaufen, während der Benutzer im Dialog
	// (Host-Key, Passwort) überlegt. Sonst kam nach „j“ nur noch
	// "handshake failed: i/o timeout".
	pause := func() {
		if raw != nil {
			_ = raw.SetDeadline(time.Time{})
		}
	}
	resume := func() {
		if raw != nil {
			_ = raw.SetDeadline(time.Now().Add(o.Timeout))
		}
	}
	if p := o.Prompt; p != nil {
		o.Prompt = func(host, fp, typ string) bool {
			pause()
			defer resume()
			return p(host, fp, typ)
		}
	}
	if ask := o.AskSecret; ask != nil {
		o.AskSecret = func(label string) (string, error) {
			pause()
			defer resume()
			return ask(label)
		}
	}
	cfg, err := clientConfig(c, o)
	if err != nil {
		return nil, err
	}

	if c.JumpHostID != "" {
		if o.Resolve == nil {
			return nil, errors.New("Jump-Host kann nicht aufgelöst werden")
		}
		jc, ok := o.Resolve(c.JumpHostID)
		if !ok {
			return nil, errors.New("Jump-Host nicht gefunden")
		}
		if jc.JumpHostID == c.ID {
			return nil, errors.New("Jump-Host-Schleife")
		}
		jo := o
		jo.forceNet, jo.legacy = false, false
		jump, err = Dial(ctx, jc, jo)
		if err != nil {
			return nil, fmt.Errorf("Jump-Host %s: %w", jc.Alias, err)
		}
		raw, err = jump.Dial("tcp", c.Addr())
	} else {
		d := net.Dialer{Timeout: o.Timeout, KeepAlive: 30 * time.Second}
		raw, err = d.DialContext(ctx, "tcp", c.Addr())
	}
	if err != nil {
		if jump != nil {
			jump.Close()
		}
		return nil, err
	}
	_ = raw.SetDeadline(time.Now().Add(o.Timeout)) // nur für den Handshake
	sc, chans, reqs, err := ssh.NewClientConn(raw, c.Addr(), cfg)
	if err != nil {
		raw.Close()
		if jump != nil {
			jump.Close()
		}
		return nil, err
	}
	_ = raw.SetDeadline(time.Time{})

	kctx, cancel := context.WithCancel(context.Background())
	cl := &Client{Client: ssh.NewClient(sc, chans, reqs), Conn: c, jump: jump, cancel: cancel}
	banner := string(sc.ServerVersion())
	switch {
	case o.forceNet || c.Device == domain.DeviceNetwork:
		cl.NetDevice = true
	case c.Device == domain.DeviceAuto && IsNetworkBanner(banner):
		cl.NetDevice = true
	case c.Device == domain.DeviceAuto && c.Profile != nil && c.Profile.Family == domain.FamilyNetwork:
		cl.NetDevice = true // früher als Netzwerkgerät erkannt (Profil)
	}
	cl.Profile = sysprof.FromBanner(banner)
	if cl.NetDevice {
		// Keine Abfrage: nur die Kennung, der Rest kommt passiv aus dem Terminal.
		if cl.Profile.Family != domain.FamilyNetwork {
			cl.Profile.Family, cl.Profile.Dialect = domain.FamilyNetwork, "network"
		}
		cl.Remote = RemoteInfo{OS: "Netzwerkgerät (" + strings.TrimPrefix(banner, "SSH-2.0-") + ")"}
		if cl.Profile.OS != "" {
			cl.Remote.OS = cl.Profile.OS
		}
	} else {
		pr := cl.probe()
		if pr.OS != "" || pr.Version != "" {
			pr.Banner = cl.Profile.Banner
			cl.Profile = pr
		}
		cl.Remote = RemoteInfo{OS: cl.Profile.OS, Kernel: pr.Version, Shell: pr.Shell}
		if cl.Remote.OS == "" && cl.Remote.Kernel == "" {
			cl.Remote.OS = "Windows (OpenSSH) oder unbekannt"
		}
	}
	go cl.keepAlive(kctx, c.KeepAlive)
	return cl, nil
}

// alive prüft mit einer Rundreise, ob die Verbindung noch steht. Ein Server,
// der nach der Exec-Abfrage auflegt, hat das bis zur Antwort getan (SSH
// verarbeitet Pakete der Reihe nach). Keine Antwort in 3 s gilt als lebendig.
func (c *Client) alive() bool {
	done := make(chan error, 1)
	go func() {
		_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
		done <- err
	}()
	select {
	case err := <-done:
		return err == nil
	case <-time.After(3 * time.Second):
		return true
	}
}

// networkBanners: Teile der SSH-Kennung typischer Switches/Router/Firewalls.
// Geräte mit OpenSSH (z. B. ArubaOS-CX, Junos) erkennt das nicht; dort greift
// der Rückfall in Dial oder der Gerätetyp in der Verbindung.
var networkBanners = []string{
	"cisco", "rosssh", "mikrotik", "huawei", "comware", "h3c", "romsshell",
	"allegro", "mocana", "procurve", "aruba", "netscreen", "fortissh",
	"zte", "lancom", "ruckus", "brocade", "extreme", "dell networking",
	"ssh-2.0--", // ältere Cisco-IOS-Varianten
}

// IsNetworkBanner sagt, ob die SSH-Kennung des Servers nach Netzwerkgerät aussieht.
func IsNetworkBanner(v string) bool {
	l := strings.ToLower(v)
	for _, b := range networkBanners {
		if strings.Contains(l, b) {
			return true
		}
	}
	return false
}

func isNoCommonAlgo(err error) bool {
	return strings.Contains(err.Error(), "no common algorithm")
}

// rememberSecrets merkt sich Antworten auf Rückfragen, damit ein zweiter
// Verbindungsversuch nicht erneut nach Passwort oder Passphrase fragt.
func rememberSecrets(ask SecretPrompt) SecretPrompt {
	var mu sync.Mutex
	seen := map[string]string{}
	return func(label string) (string, error) {
		mu.Lock()
		v, ok := seen[label]
		mu.Unlock()
		if ok {
			return v, nil
		}
		v, err := ask(label)
		if err == nil {
			mu.Lock()
			seen[label] = v
			mu.Unlock()
		}
		return v, err
	}
}

// Close schließt Ziel und Jump-Host.
func (c *Client) Close() error {
	c.cancel()
	err := c.Client.Close()
	if c.jump != nil {
		c.jump.Close()
	}
	return err
}

func (c *Client) keepAlive(ctx context.Context, sec int) {
	if sec <= 0 {
		sec = 30
	}
	t := time.NewTicker(time.Duration(sec) * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, _, err := c.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				return
			}
		}
	}
}

// probe fragt System, Kernel, Shell, Paketmanager und Init-System ab, in
// einer eigenen Exec-Sitzung, also unsichtbar für das Terminal des Nutzers.
func (c *Client) probe() domain.SystemProfile {
	s, err := c.NewSession()
	if err != nil {
		return domain.SystemProfile{}
	}
	defer s.Close()
	done := make(chan []byte, 1)
	go func() { out, _ := s.Output(sysprof.ProbeCommand); done <- out }()
	select {
	case out := <-done:
		return sysprof.FromProbe(string(out))
	case <-time.After(4 * time.Second):
		return domain.SystemProfile{}
	}
}
