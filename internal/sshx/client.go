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
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/debuglog"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
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

	// Zustand der Leitung (Statuszeile, Auto-Reconnect)
	latency  atomic.Int64  // letzte Rundreise in ns, 0 = noch keine
	answered atomic.Bool   // Gegenstelle beantwortet Keepalives
	byUs     atomic.Bool   // Close() aufgerufen
	dead     chan struct{} // geschlossen, sobald die Verbindung weg ist

	// Agent-Weiterleitung (agentfwd.go)
	fwdOnce  sync.Once
	fwdErr   error
	fwdAsked error // Server hat die Weiterleitung abgelehnt

	// X11-Weiterleitung (x11.go)
	x11Once sync.Once
	x11     *x11Fwd
	x11Err  error
}

// RemoteInfo: was wir über das Zielsystem wissen (Kontext fürs LLM).
type RemoteInfo struct {
	OS     string // z. B. "Ubuntu 24.04.1 LTS"
	Kernel string // uname -srm
	Shell  string // $SHELL
	Locale string // LC_ALL/LC_CTYPE/LANG, z. B. "de_DE.UTF-8"
}

// Encoding: Zeichensatz aus der Locale ("UTF-8", "ISO-8859-1" …), leer wenn unbekannt.
func (r RemoteInfo) Encoding() string {
	l := r.Locale
	if l == "" {
		return ""
	}
	if i := strings.IndexByte(l, '.'); i >= 0 {
		l = l[i+1:]
		if j := strings.IndexByte(l, '@'); j >= 0 {
			l = l[:j]
		}
	} else if l == "C" || l == "POSIX" {
		return "ASCII"
	}
	switch strings.ToLower(strings.ReplaceAll(l, "-", "")) {
	case "utf8":
		return "UTF-8"
	case "iso88591":
		return "ISO-8859-1"
	case "iso885915":
		return "ISO-8859-15"
	}
	return l
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

// RelayDialer öffnet einen Kanal über einen Vermittler zum sshd des Systems
// (Connection.RelayID/RelaySystem). hostKey ist der vom Anker gemeldete
// Host-Key im authorized_keys-Format ("" = unbekannt, dann wie sonst fragen).
type RelayDialer func(ctx context.Context, c domain.Connection) (conn net.Conn, hostKey string, err error)

// ErrRelayHostKey: der Server zeigt einen anderen Host-Key, als sein Anker
// dem Vermittler gemeldet hat.
var ErrRelayHostKey = i18n.NewError("ACHTUNG: Host-Key passt nicht zu dem, den der Anker beim Vermittler gemeldet hat. Verbindung abgebrochen")

// Options für Dial.
type Options struct {
	HostKeys  *HostKeyStore
	Prompt    HostKeyPrompt
	Resolve   Resolver
	Relay     RelayDialer  // für Verbindungen über einen Vermittler (optional)
	AskSecret SecretPrompt // fehlende Passwörter/Passphrasen erfragen (optional)
	Notice    func(string) // Hinweis während des Verbindens, z. B. „Schlüssel berühren“ (optional)
	Step      func(int)    // Fortschritt: StepConnect, StepHostKey, StepAuth (optional, nur fürs Ziel, nicht für Jump-Hosts)
	Timeout   time.Duration

	forceNet  bool     // intern: zweiter Versuch nach abgebrochener Systemabfrage
	legacy    bool     // intern: alte Algorithmen (nur wenn der Server nichts anderes kann)
	waitScale float64  // intern: Wartezeiten zwischen Versuchen skalieren (Tests)
	hops      []string // intern: bisherige Stationen der Jump-Kette (Schleifenschutz)
}

// Schritte des Verbindungsaufbaus (Options.Step).
const (
	StepConnect = iota // TCP, Vermittler oder Jump-Host
	StepHostKey        // Handshake und Host-Schlüssel prüfen
	StepAuth           // Anmelden
	StepShell          // Shell öffnen (meldet die Oberfläche selbst)
)

// maxJumpHops: mehr Stationen ergeben praktisch nie Sinn, schützt vor Schleifen.
const maxJumpHops = 8

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
	t0 := time.Now()
	debuglog.Printf("ssh", "%s: verbinde (Auth=%s Gerät=%s Jump=%t Vermittler=%t)", c.Alias, c.Auth, c.Device, c.JumpHostID != "", c.ViaRelay())
	cl, err := dialRetry(ctx, c, o, 2*time.Second)
	if err != nil && c.Device != domain.DeviceServer && isNoCommonAlgo(err) {
		debuglog.Printf("ssh", "%s: keine gemeinsamen Verfahren, versuche alte Verfahren (%v)", c.Alias, err)
		// Alte Geräte können oft nur diffie-hellman-group14-sha1, aes-cbc
		// oder ssh-rsa. Die Aushandlung bevorzugt trotzdem moderne Verfahren.
		o.legacy = true
		cl, err = dialRetry(ctx, c, o, 2*time.Second)
	}
	if err != nil {
		debuglog.Printf("ssh", "%s: fehlgeschlagen nach %s: %v", c.Alias, time.Since(t0).Round(time.Millisecond), err)
		return nil, err
	}
	debuglog.Printf("ssh", "%s: verbunden in %s (%s, Netzgerät=%t)", c.Alias, time.Since(t0).Round(time.Millisecond), cl.Profile.Banner, cl.NetDevice)
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
	debuglog.Printf("ssh", "%s: Verbindung hat die Systemabfrage nicht überlebt → Netzwerkgerät, neuer Versuch", c.Alias)
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
		debuglog.Printf("ssh", "%s: Gegenstelle legt im Handshake auf (%v), Versuch %d", c.Alias, err, i+2)
		if err := sleepCtx(ctx, o.retryWait(waits[i])); err != nil {
			return nil, err
		}
	}
}

func isHangup(err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	// Windows meldet einen Reset als WSAECONNRESET mit Text in der
	// Systemsprache („Eine vorhandene Verbindung wurde … geschlossen“).
	var en syscall.Errno
	if errors.As(err, &en) && (en == syscall.ECONNRESET || en == wsaECONNRESET || en == wsaECONNABORTED) {
		return true
	}
	m := err.Error()
	return strings.Contains(m, "connection reset") || strings.HasSuffix(m, ": EOF")
}

// Windows-Socket-Fehler (winerror.h), auf anderen Systemen nie gesetzt.
const (
	wsaECONNABORTED syscall.Errno = 10053
	wsaECONNRESET   syscall.Errno = 10054
)

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
	var (
		relayKey ssh.PublicKey
		err      error
	)
	relayMismatch := false
	if c.ViaRelay() {
		if o.Relay == nil {
			return nil, errors.New(i18n.T("Vermittler auf diesem Rechner nicht gekoppelt"))
		}
		var hk string
		raw, hk, err = o.Relay(ctx, c)
		if err != nil {
			return nil, err
		}
		if hk != "" {
			if relayKey, _, _, _, err = ssh.ParseAuthorizedKey([]byte(hk)); err != nil {
				raw.Close()
				return nil, errors.New(i18n.T("Vermittler meldet einen ungültigen Host-Key"))
			}
		}
		// Unbekannter Host: der Schlüssel, den der Anker gemeldet hat, gilt
		// ohne Rückfrage; ein anderer wird abgelehnt (nie „trotzdem“).
		if relayKey != nil {
			want := ssh.FingerprintSHA256(relayKey)
			o.Prompt = func(host, fp, typ string) bool {
				if fp == want {
					return true
				}
				relayMismatch = true
				return false
			}
		}
	}
	cfg, err := clientConfig(c, o)
	if err != nil {
		if raw != nil {
			raw.Close()
		}
		return nil, err
	}
	if relayKey != nil && len(cfg.HostKeyAlgorithms) == 0 {
		// Unbekannter Host: genau den Schlüsseltyp verlangen, den der Anker
		// gemeldet hat. Sonst bietet ein Server mit mehreren Host-Keys
		// (ed25519 + ecdsa) zuerst ecdsa an, der Anker meldet aber ed25519,
		// und die Verbindung wird fälschlich als Fälschung abgelehnt.
		cfg.HostKeyAlgorithms = algosFor(relayKey.Type())
	}

	step := func(n int) {
		if o.Step != nil {
			o.Step(n)
		}
	}
	if cb := cfg.HostKeyCallback; cb != nil && o.Step != nil {
		cfg.HostKeyCallback = func(host string, remote net.Addr, key ssh.PublicKey) error {
			step(StepHostKey)
			err := cb(host, remote, key)
			if err == nil {
				step(StepAuth)
			}
			return err
		}
	}
	step(StepConnect)
	if c.ViaRelay() {
		// raw kommt schon vom Vermittler
	} else if c.JumpHostID != "" {
		if o.Resolve == nil {
			return nil, errors.New(i18n.T("Jump-Host kann nicht aufgelöst werden"))
		}
		jc, ok := o.Resolve(c.JumpHostID)
		if !ok {
			return nil, errors.New(i18n.T("Jump-Host nicht gefunden"))
		}
		hops := append(append([]string(nil), o.hops...), c.ID)
		for _, h := range hops {
			if h == jc.ID {
				return nil, errors.New(i18n.T("Jump-Host-Schleife: ") + jc.Alias + i18n.T(" kommt in der Kette zweimal vor"))
			}
		}
		if len(hops) >= maxJumpHops {
			return nil, fmt.Errorf(i18n.T("Jump-Host-Kette zu lang (mehr als %d Stationen)"), maxJumpHops)
		}
		jo := o
		jo.forceNet, jo.legacy, jo.hops, jo.Step = false, false, hops, nil
		jump, err = Dial(ctx, jc, jo)
		if err != nil {
			return nil, fmt.Errorf(i18n.T("Jump-Host %s: %w"), jc.Alias, err)
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
		if relayMismatch {
			return nil, ErrRelayHostKey
		}
		return nil, err
	}
	_ = raw.SetDeadline(time.Time{})

	kctx, cancel := context.WithCancel(context.Background())
	cl := &Client{Client: ssh.NewClient(sc, chans, reqs), Conn: c, jump: jump, cancel: cancel, dead: make(chan struct{})}
	go func() { _ = cl.Client.Wait(); close(cl.dead) }()
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
		cl.Remote = RemoteInfo{OS: i18n.T("Netzwerkgerät (") + strings.TrimPrefix(banner, "SSH-2.0-") + ")"}
		if cl.Profile.OS != "" {
			cl.Remote.OS = cl.Profile.OS
		}
	} else if pr, locale, refused := cl.probe(); refused != "" {
		// Das Gerät hat die Abfrage abgelehnt statt sie auszuführen (z. B.
		// ArubaOS-CX: „Command not supported.“, Kennung OpenSSH): also ein
		// Netzwerkgerät. Ab jetzt keine Exec-Befehle mehr; das Profil merkt
		// sich die Familie, damit beim nächsten Mal gar nicht erst gefragt wird.
		cl.NetDevice = true
		cl.Profile.Family, cl.Profile.Dialect = domain.FamilyNetwork, "network"
		cl.Remote = RemoteInfo{OS: i18n.T("Netzwerkgerät (") + strings.TrimPrefix(banner, "SSH-2.0-") + ")"}
	} else {
		if pr.OS != "" || pr.Version != "" {
			pr.Banner = cl.Profile.Banner
			cl.Profile = pr
		}
		cl.Remote = RemoteInfo{OS: cl.Profile.OS, Kernel: pr.Version, Shell: pr.Shell, Locale: locale}
		if cl.Remote.OS == "" && cl.Remote.Kernel == "" {
			cl.Remote.OS = i18n.T("Windows (OpenSSH) oder unbekannt")
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

// Check prüft nach Ruhezustand oder Netzwechsel sofort, ob die Leitung
// noch steht: bleibt die Keepalive-Antwort länger als d aus, wird die
// Verbindung als abgerissen geschlossen (→ Auto-Reconnect). Geräte, die nie
// auf Keepalives antworten, bleiben unangetastet. Rückgabe: noch da?
func (c *Client) Check(d time.Duration) bool {
	if !c.answered.Load() {
		return true
	}
	done := make(chan error, 1)
	go func() {
		_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
		done <- err
	}()
	select {
	case err := <-done:
		return err == nil
	case <-c.dead:
		return false
	case <-time.After(d):
		_ = c.Client.Close() // nicht byUs: zählt als Abbruch
		return false
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
	c.byUs.Store(true)
	c.cancel()
	err := c.Client.Close()
	if c.jump != nil {
		c.jump.Close()
	}
	return err
}

// Latency: Dauer der letzten Keepalive-Rundreise (0 = noch nicht gemessen
// oder die Gegenstelle antwortet nicht auf Keepalives).
func (c *Client) Latency() time.Duration { return time.Duration(c.latency.Load()) }

// Dropped: Die Verbindung ist weg, ohne dass sshTermy sie geschlossen hat
// (Netz weg, VPN getrennt, Server neu gestartet, Keepalive ohne Antwort).
func (c *Client) Dropped() bool {
	select {
	case <-c.dead:
		return !c.byUs.Load()
	default:
		return false
	}
}

// WaitDropped wartet höchstens d darauf, dass die Verbindung wegbricht. Die
// Shell meldet ihr Ende oft einen Moment bevor die Leitung als tot gilt.
func (c *Client) WaitDropped(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-c.dead:
	case <-t.C:
	}
	return c.Dropped()
}

// keepAlive hält die Verbindung offen, misst die Latenz und erkennt eine
// tote Leitung. Ohne Netz würde TCP sonst minutenlang still hängen. Als tot
// gilt die Verbindung nur, wenn die Gegenstelle schon einmal geantwortet hat
// (manche Switches beantworten Keepalives nie, die würden sonst getrennt).
func (c *Client) keepAlive(ctx context.Context, sec int) {
	every := time.Duration(sec) * time.Second
	if sec <= 0 {
		every = 30 * time.Second
	}
	ping := min(every, 10*time.Second)      // Latenz in der Statuszeile aktuell halten
	deadAfter := max(every, 30*time.Second) // so lange ohne Antwort = Leitung tot
	c.keepAliveLoop(ctx, ping, deadAfter)
}

func (c *Client) keepAliveLoop(ctx context.Context, ping, deadAfter time.Duration) {
	res := make(chan error, 1)
	inflight := false
	var sent time.Time
	send := func() {
		inflight, sent = true, time.Now()
		go func() {
			_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
			res <- err
		}()
	}
	send()
	t := time.NewTicker(ping)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.dead:
			return
		case err := <-res:
			inflight = false
			if err != nil {
				return // Leitung schon zu, Wait() meldet das
			}
			// mindestens 1 ns: 0 heißt „noch nicht gemessen“, und unter
			// Windows ist eine Rundreise über localhost schneller als die Uhr
			c.latency.Store(max(int64(time.Since(sent)), 1))
			c.answered.Store(true)
		case <-t.C:
			if !inflight {
				send()
				continue
			}
			if c.answered.Load() && time.Since(sent) >= deadAfter {
				_ = c.Client.Close() // nicht byUs: zählt als Abbruch → Auto-Reconnect
				return
			}
		}
	}
}

// probe fragt System, Kernel, Shell, Paketmanager und Init-System ab, in
// einer eigenen Exec-Sitzung, also unsichtbar für das Terminal des Nutzers.
// refused ist die Fehlermeldung, wenn das Gerät die Abfrage abgelehnt hat.
func (c *Client) probe() (prof domain.SystemProfile, locale, refused string) {
	s, err := c.NewSession()
	if err != nil {
		return domain.SystemProfile{}, "", ""
	}
	defer s.Close()
	done := make(chan []byte, 1)
	go func() { out, _ := s.Output(sysprof.ProbeCommand); done <- out }()
	select {
	case out := <-done:
		if msg := sysprof.ProbeRejected(string(out)); msg != "" {
			return domain.SystemProfile{}, "", msg
		}
		lines := strings.Split(strings.ReplaceAll(string(out), "\r", ""), "\n")
		if len(lines) > 5 {
			locale = strings.TrimSpace(lines[5])
		}
		return sysprof.FromProbe(string(out)), locale, ""
	case <-time.After(4 * time.Second):
		return domain.SystemProfile{}, "", ""
	}
}

// Output führt einen Befehl in einer eigenen Exec-Sitzung aus (unsichtbar
// fürs Terminal). Nicht für Netzwerkgeräte: dort beendet Exec die Verbindung.
func (c *Client) Output(cmd string, d time.Duration) (string, error) {
	if c.NetDevice {
		return "", errors.New(i18n.T("keine Exec-Befehle auf Netzwerkgeräten"))
	}
	s, err := c.NewSession()
	if err != nil {
		return "", err
	}
	defer s.Close()
	type res struct {
		out []byte
		err error
	}
	done := make(chan res, 1)
	go func() { out, err := s.Output(cmd); done <- res{out, err} }()
	select {
	case r := <-done:
		return string(r.out), r.err
	case <-time.After(d):
		return "", errors.New(i18n.T("Zeitüberschreitung"))
	}
}
