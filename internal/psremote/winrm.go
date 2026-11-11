// Package psremote: PowerShell auf einem Windows-Server über WinRM
// (wie Enter-PSSession), in reinem Go. Die Sitzung läuft über WinRS
// (ein powershell.exe drüben), die Eingabezeile mit Verlauf und
// Tab-Ergänzung baut sshTermy selbst, weil WinRS keine Konsole hat.
//
// Anmeldung: Negotiate/NTLM (Standard, auch Domänenkonten als DOMÄNE\name
// oder name@domäne) – über HTTP verschlüsselt NTLM jede Nachricht, über
// HTTPS übernimmt das TLS. Basic nur über HTTPS (lokale Konten). Kerberos
// für Server ohne NTLM (kerberos.go).
package psremote

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/masterzen/winrm"
	"github.com/masterzen/winrm/soap"
)

// Auth: Anmeldeverfahren.
const (
	AuthNegotiate = ""      // NTLM (Negotiate), Standard
	AuthBasic     = "basic" // nur über HTTPS
)

// Config: wohin und wie.
type Config struct {
	Host     string
	Port     int // 0 = 5985 bzw. 5986
	HTTPS    bool
	Insecure bool   // Zertifikat nicht prüfen (selbst signiert)
	Auth     string // AuthNegotiate, AuthBasic, AuthKerberos
	User     string
	Password string
	Timeout  time.Duration // Verbindungsaufbau, 0 = 30 s
}

// DefaultPort: 5985 (HTTP) bzw. 5986 (HTTPS).
func DefaultPort(https bool) int {
	if https {
		return 5986
	}
	return 5985
}

// Dial meldet sich an, öffnet eine Sitzung und wartet auf den ersten Prompt.
func Dial(ctx context.Context, cfg Config, cols int, out io.Writer) (*Stream, error) {
	if cfg.Auth == AuthBasic && !cfg.HTTPS {
		return nil, ErrBasicHTTP
	}
	if cfg.Port == 0 {
		cfg.Port = DefaultPort(cfg.HTTPS)
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	type res struct {
		w   *wsman
		err error
	}
	ch := make(chan res, 1)
	go func() {
		w, err := openWSMan(cfg)
		ch <- res{w, err}
	}()
	var w *wsman
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, explain(r.err, cfg)
		}
		w = r.w
	case <-ctx.Done():
		go func() {
			if r := <-ch; r.w != nil {
				r.w.close()
			}
		}()
		return nil, ctx.Err()
	}
	wait := cfg.Timeout
	if dl, ok := ctx.Deadline(); ok {
		wait = time.Until(dl)
	}
	s, err := newStream(w, cols, out, wait)
	if err != nil {
		return nil, explain(err, cfg)
	}
	return s, nil
}

// ErrBasicHTTP: Basic würde das Passwort lesbar übers Netz schicken.
var ErrBasicHTTP = errors.New("Basic-Anmeldung nur über HTTPS (sonst geht das Passwort lesbar übers Netz)")

// wsman: eine WinRS-Shell. Zwei Clients, weil „Receive“ bis zu 20 s auf
// Ausgabe wartet und NTLM pro Verbindung nur eine Anfrage gleichzeitig
// erlaubt; Eingaben gehen über den zweiten Client sofort raus.
type wsman struct {
	url      string
	params   *winrm.Parameters
	tx, rx   *winrm.Client
	txT, rxT winrm.Transporter
	shell    string
	once     sync.Once
}

func newTransport(cfg Config) (winrm.Transporter, error) {
	switch {
	case cfg.Auth == AuthKerberos:
		return kerberosTransport(cfg)
	case cfg.Auth == AuthBasic:
		return winrm.NewClientWithDial(dialer(cfg)), nil
	case cfg.HTTPS:
		return winrm.NewClientNTLMWithDial(dialer(cfg)), nil
	default:
		return winrm.NewEncryption("ntlm") // HTTP: NTLM versiegelt jede Nachricht
	}
}

func dialer(cfg Config) func(network, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: cfg.Timeout, KeepAlive: 30 * time.Second}
	return d.Dial
}

func openWSMan(cfg Config) (*wsman, error) {
	ep := winrm.NewEndpoint(cfg.Host, cfg.Port, cfg.HTTPS, cfg.Insecure, nil, nil, nil, 60*time.Second)
	params := winrm.NewParameters("PT20S", "en-US", 153600)
	w := &wsman{params: params, url: (&url.URL{Scheme: map[bool]string{false: "http", true: "https"}[cfg.HTTPS],
		Host: net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)), Path: "/wsman"}).String()}
	mk := func() (*winrm.Client, winrm.Transporter, error) {
		t, err := newTransport(cfg)
		if err != nil {
			return nil, nil, err
		}
		p := *params
		p.TransportDecorator = func() winrm.Transporter { return t }
		c, err := winrm.NewClientWithParameters(ep, cfg.User, cfg.Password, &p)
		return c, t, err
	}
	var err error
	if w.tx, w.txT, err = mk(); err != nil {
		return nil, err
	}
	resp, err := w.post(w.txT, w.tx, winrm.NewOpenShellRequest(w.url, params))
	if err != nil {
		return nil, err
	}
	if w.shell, err = winrm.ParseOpenShellResponse(resp); err != nil {
		return nil, err
	}
	if w.rx, w.rxT, err = mk(); err != nil {
		w.close()
		return nil, err
	}
	return w, nil
}

func (w *wsman) post(t winrm.Transporter, c *winrm.Client, m *soap.SoapMessage) (string, error) {
	defer m.Free()
	return t.Post(c, m)
}

func (w *wsman) close() {
	w.once.Do(func() {
		if w.shell != "" {
			_, _ = w.post(w.txT, w.tx, winrm.NewDeleteShellRequest(w.url, w.shell, w.params))
		}
	})
}

type wsCmd struct {
	w    *wsman
	id   string
	quit chan struct{}
	once sync.Once
	mu   sync.Mutex
}

func (w *wsman) start(dir string, h handlers) (proc, error) {
	resp, err := w.post(w.txT, w.tx, winrm.NewExecuteCommandRequest(w.url, w.shell, "powershell.exe", psArgs(), w.params))
	if err != nil {
		return nil, err
	}
	id, err := winrm.ParseExecuteCommandResponse(resp)
	if err != nil {
		return nil, err
	}
	c := &wsCmd{w: w, id: id, quit: make(chan struct{})}
	go c.receive(h)
	return c, nil
}

// receive holt Ausgabe, bis PowerShell endet.
func (c *wsCmd) receive(h handlers) {
	fails := 0
	for {
		select {
		case <-c.quit:
			return
		default:
		}
		resp, err := c.w.post(c.w.rxT, c.w.rx, winrm.NewGetOutputRequest(c.w.url, c.w.shell, c.id, "stdout stderr", c.w.params))
		if err != nil {
			if timedOut(err) {
				fails = 0
				continue // keine Ausgabe in 20 s: normal
			}
			select {
			case <-c.quit:
				return
			default:
			}
			if fails++; fails < 3 && transient(err) {
				time.Sleep(time.Duration(fails) * 500 * time.Millisecond)
				continue
			}
			h.end(err)
			return
		}
		fails = 0
		var o, e bytes.Buffer
		done, code, err := winrm.ParseSlurpOutputErrResponse(resp, &o, &e)
		if o.Len() > 0 {
			h.out(o.Bytes())
		}
		if e.Len() > 0 {
			h.err(e.Bytes())
		}
		if err != nil {
			h.end(err)
			return
		}
		if done {
			var ee error
			if code != 0 {
				ee = &ExitError{Code: code}
			}
			h.end(ee)
			return
		}
	}
}

func (c *wsCmd) send(b []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(b) > 0 {
		n := min(len(b), 32*1024)
		if _, err := c.w.post(c.w.txT, c.w.tx, winrm.NewSendInputRequest(c.w.url, c.w.shell, c.id, b[:n], false, c.w.params)); err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}

func (c *wsCmd) stop() {
	c.once.Do(func() {
		close(c.quit)
		_, _ = c.w.post(c.w.txT, c.w.tx, winrm.NewSignalRequest(c.w.url, c.w.shell, c.id, c.w.params))
	})
}

// ExitError: PowerShell drüben endete mit einem Fehlercode.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("PowerShell beendet (Code %d)", e.Code) }

func timedOut(err error) bool {
	s := err.Error()
	return strings.Contains(s, "OperationTimeout") || strings.Contains(s, "2150858793") || strings.Contains(s, "TimedOut")
}

func transient(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) || strings.Contains(err.Error(), "EOF") || strings.Contains(err.Error(), "connection reset")
}

// ---------------------------------------------------------------- Fehlertexte

// Fehler mit Hinweis, was auf dem Server fehlt. Die Texte übersetzt die
// Oberfläche (i18n), hier stehen nur die Arten.
var (
	ErrAuth       = errors.New("Anmeldung abgelehnt")
	ErrRefused    = errors.New("WinRM antwortet nicht")
	ErrCert       = errors.New("Zertifikat nicht vertrauenswürdig")
	ErrUnreadable = errors.New("unerwartete Antwort")
)

// Error: Fehler samt Ursache aus der Bibliothek.
type Error struct {
	Kind  error
	Cause error
}

func (e *Error) Error() string { return e.Kind.Error() + ": " + e.Cause.Error() }
func (e *Error) Unwrap() []error {
	return []error{e.Kind, e.Cause}
}

func explain(err error, cfg Config) error {
	if err == nil || errors.Is(err, ErrStartup) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	s := err.Error()
	var ua x509.UnknownAuthorityError
	var hn x509.HostnameError
	var ci x509.CertificateInvalidError
	switch {
	case errors.As(err, &ua) || errors.As(err, &hn) || errors.As(err, &ci) || strings.Contains(s, "x509:") || strings.Contains(s, "certificate"):
		return &Error{ErrCert, err}
	case strings.Contains(s, "KDC_ERR_S_PRINCIPAL_UNKNOWN"):
		return &Error{ErrKerberosSPN, err}
	case strings.Contains(s, "KDC_ERR_PREAUTH_FAILED") || strings.Contains(s, "KDC_ERR_C_PRINCIPAL_UNKNOWN") || strings.Contains(s, "KDC_ERR_CLIENT_REVOKED"):
		return &Error{ErrAuth, err}
	case cfg.Auth == AuthKerberos && (strings.Contains(s, "KDC") || strings.Contains(s, "kdc") || strings.Contains(s, "_kerberos")):
		return &Error{ErrKerberosKDC, err}
	case httpStatus(s, "401") || strings.Contains(s, "Unauthorized") || strings.Contains(s, "Access is denied") || strings.Contains(s, "logon failure"):
		return &Error{ErrAuth, err}
	case strings.Contains(s, "connection refused") || strings.Contains(s, "i/o timeout") || strings.Contains(s, "no such host") ||
		strings.Contains(s, "actively refused") || strings.Contains(s, "timed out") || strings.Contains(s, "deadline exceeded") ||
		httpStatus(s, "404"):
		return &Error{ErrRefused, err}
	case strings.Contains(s, "malformed HTTP") || strings.Contains(s, "server gave HTTP response to HTTPS client") || strings.Contains(s, "first record does not look like a TLS handshake"):
		return &Error{ErrUnreadable, err}
	}
	return err
}

// httpStatus: steht der Statuscode als eigene Zahl im Text? Nicht als Teil
// einer Portnummer („127.0.0.1:33401“ ist kein 401).
func httpStatus(s, code string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], code)
		if j < 0 {
			return false
		}
		j += i
		end := j + len(code)
		if (j == 0 || !isDigit(s[j-1])) && (end == len(s) || !isDigit(s[end])) && (j == 0 || s[j-1] != ':') {
			return true
		}
		i = j + 1
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// CommandLine: Aufruf von powershell.exe, der script ausführt (für Run).
func CommandLine(script string) string {
	return "powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand " + encodedCommand(script)
}

// Run führt ein kurzes PowerShell-Skript in einer eigenen WinRS-Shell aus,
// neben der offenen Sitzung (deren Eingabezeile bleibt unberührt), und
// liefert die Ausgabe. Für nur lesende Abfragen wie den Log-Blick.
func Run(ctx context.Context, cfg Config, script string) (string, error) {
	if cfg.Auth == AuthBasic && !cfg.HTTPS {
		return "", ErrBasicHTTP
	}
	if cfg.Port == 0 {
		cfg.Port = DefaultPort(cfg.HTTPS)
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	ep := winrm.NewEndpoint(cfg.Host, cfg.Port, cfg.HTTPS, cfg.Insecure, nil, nil, nil, 120*time.Second)
	params := winrm.NewParameters("PT60S", "en-US", 153600)
	t, err := newTransport(cfg)
	if err != nil {
		return "", explain(err, cfg)
	}
	params.TransportDecorator = func() winrm.Transporter { return t }
	c, err := winrm.NewClientWithParameters(ep, cfg.User, cfg.Password, params)
	if err != nil {
		return "", explain(err, cfg)
	}
	out, errOut, code, err := c.RunWithContextWithString(ctx, CommandLine(script), "")
	if err != nil {
		return out, explain(err, cfg)
	}
	if code != 0 && strings.TrimSpace(out) == "" {
		msg := strings.TrimSpace(errOut)
		if i := strings.IndexByte(msg, '\n'); i > 0 {
			msg = msg[:i]
		}
		return "", fmt.Errorf("PowerShell (Code %d): %s", code, msg)
	}
	return out, nil
}
