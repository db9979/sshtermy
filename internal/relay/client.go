package relay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Client spricht mit einem Vermittler (Client-Seite des Protokolls).
type Client struct {
	Base string // https://host[:port][/pfad]
	Key  string // Geräteschlüssel aus der Kopplung ("" vor dem Koppeln)
	FP   string // SHA-256 des Zertifikats (hex), "" = Prüfung gegen die System-CAs
	http *http.Client
}

// NewClient: fp gepinnt (selbst signiertes Zertifikat) oder normale Prüfung.
func NewClient(base, key, fp string) (*Client, error) {
	b, err := NormBase(base)
	if err != nil {
		return nil, err
	}
	return &Client{Base: b, Key: key, FP: fp, http: &http.Client{Transport: transport(fp), Timeout: 0}}, nil
}

// transport: mit fp zählt nur das Zertifikat mit genau diesem Fingerabdruck
// (wie beim Pinnen in spark-tui); sonst die Zertifizierungsstellen.
func transport(fp string) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	cfg := &tls.Config{MinVersion: tls.VersionTLS13} // beide Seiten sind sshTermy bzw. der eigene Vermittler
	if fp != "" {
		cfg.InsecureSkipVerify = true // ersetzt durch VerifyPeerCertificate (Pin)
		cfg.VerifyPeerCertificate = func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 {
				return i18n.NewError("Vermittler zeigt kein Zertifikat")
			}
			if CertFP(raw[0]) != fp {
				return i18n.NewError("Zertifikat des Vermittlers passt nicht zum Fingerabdruck aus der Kopplung")
			}
			return nil
		}
	}
	t.TLSClientConfig = cfg
	return t
}

// CertFP: SHA-256 eines Zertifikats (DER) als hex.
func CertFP(der []byte) string {
	h := sha256.Sum256(der)
	return hex.EncodeToString(h[:])
}

// StatusError: Antwort des Vermittlers mit HTTP-Fehler.
type StatusError struct {
	Code int
	Msg  string
}

func (e *StatusError) Error() string {
	switch e.Code {
	case http.StatusUnauthorized, http.StatusForbidden:
		if e.Msg != "" {
			return i18n.T("Vermittler lehnt ab: ") + e.Msg
		}
		return i18n.T("Vermittler kennt diesen Schlüssel nicht (neu koppeln)")
	case http.StatusLocked:
		return i18n.T("Vermittler ist gesperrt: erst entsperren (Code)")
	case http.StatusNotFound:
		return i18n.T("beim Vermittler nicht gefunden")
	case http.StatusConflict:
		return i18n.T("System ist gerade nicht mit dem Vermittler verbunden (offline)")
	case http.StatusTooManyRequests:
		return i18n.T("zu viele Versuche, bitte kurz warten")
	case http.StatusServiceUnavailable:
		return i18n.T("Vermittler nimmt gerade nichts an (abgeschaltet oder Notaus)")
	}
	if e.Msg != "" {
		return fmt.Sprintf(i18n.T("Vermittler: %s (HTTP %d)"), e.Msg, e.Code)
	}
	return fmt.Sprintf(i18n.T("Vermittler antwortet mit HTTP %d"), e.Code)
}

// Locked: der Vermittler will erst einen Code.
func Locked(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == http.StatusLocked
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.auth(req.Header)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return &StatusError{Code: resp.StatusCode, Msg: errMsg(b)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf(i18n.T("Antwort des Vermittlers unlesbar: %w"), err)
	}
	return nil
}

// errMsg: {"detail": "…"} (FastAPI, Spark) oder {"fehler": "…"} (sshtermy vermittler).
func errMsg(b []byte) string {
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	for _, k := range []string{"fehler", "detail", "error"} {
		if s, ok := m[k].(string); ok {
			return clean(s, 200)
		}
	}
	return ""
}

// clean: nur druckbare Zeichen, gekürzt (Texte vom Vermittler sind Daten).
func clean(s string, n int) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			continue
		}
		b.WriteRune(r)
		if b.Len() >= n {
			break
		}
	}
	return b.String()
}

func (c *Client) auth(h http.Header) {
	if c.Key != "" {
		h.Set("Authorization", "Bearer "+c.Key)
	}
}

// Pair tauscht den Einmal-Code gegen einen eigenen Geräteschlüssel.
func (c *Client) Pair(ctx context.Context, code, device string) (PairResult, error) {
	var r PairResult
	err := c.do(ctx, http.MethodPost, PathPair, map[string]string{"code": code, "name": device}, &r)
	if err == nil && r.Key == "" {
		err = i18n.NewError("Vermittler hat keinen Schlüssel geschickt")
	}
	if err == nil {
		c.Key = r.Key
		r.Name = clean(r.Name, 60)
	}
	return r, err
}

// Systems: alle Systeme des Vermittlers (bei gesperrtem Vermittler leer, Locked gesetzt).
func (c *Client) Systems(ctx context.Context) (SystemList, error) {
	var l SystemList
	err := c.do(ctx, http.MethodGet, PathSystems, nil, &l)
	l.Name = clean(l.Name, 60)
	for i := range l.Systems {
		s := &l.Systems[i]
		s.Name, s.OS, s.Folder, s.User = clean(s.Name, 60), clean(s.OS, 60), clean(s.Folder, 120), clean(s.User, 32)
	}
	return l, err
}

// Unlock entsperrt einen Vermittler mit Sperre (Spark: Code aus der App).
func (c *Client) Unlock(ctx context.Context, code string) (time.Time, error) {
	var r struct {
		Until int64 `json:"bis"`
	}
	if err := c.do(ctx, http.MethodPost, PathUnlock, map[string]string{"code": code}, &r); err != nil {
		return time.Time{}, err
	}
	return time.Unix(r.Until, 0), nil
}

// Dial öffnet einen Kanal zum sshd des Systems. Die Verbindung trägt rohe
// Bytes; darüber läuft SSH Ende-zu-Ende. addr ist die Adresse, unter der
// sshTermy den Host-Key merkt (RemoteAddr der Verbindung).
func (c *Client) Dial(ctx context.Context, system, addr string) (net.Conn, error) {
	u := c.Base + PathChannel + "?system=" + url.QueryEscape(system)
	h := http.Header{}
	c.auth(h)
	ws, resp, err := websocket.Dial(ctx, u, &websocket.DialOptions{HTTPClient: c.http, HTTPHeader: h})
	if err != nil {
		if resp != nil && resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusSwitchingProtocols {
			var b []byte
			if resp.Body != nil {
				b, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<16))
			}
			return nil, &StatusError{Code: resp.StatusCode, Msg: errMsg(b)}
		}
		return nil, fmt.Errorf(i18n.T("Kanal zum Vermittler: %w"), err)
	}
	ws.SetReadLimit(-1)
	nc := websocket.NetConn(context.Background(), ws, websocket.MessageBinary)
	return &namedConn{Conn: nc, addr: fakeAddr(addr)}, nil
}

// namedConn meldet als Gegenstelle die Adresse des Systems (für known_hosts),
// nicht die des Vermittlers.
type namedConn struct {
	net.Conn
	addr net.Addr
}

func (n *namedConn) RemoteAddr() net.Addr { return n.addr }

type fakeAddr string

func (fakeAddr) Network() string  { return "vermittler" }
func (a fakeAddr) String() string { return string(a) }

// NewPairLine holt beim Vermittler eine Kopplungszeile für ein weiteres Gerät
// (kind CodeClient/CodeAdmin) oder einen Server (CodeAnchor). Nur für Geräte
// mit Verwaltungsrecht.
func (c *Client) NewPairLine(ctx context.Context, kind string) (string, error) {
	var r struct {
		Line string `json:"zeile"`
	}
	if err := c.do(ctx, http.MethodPost, PathCodes, map[string]string{"art": kind}, &r); err != nil {
		return "", err
	}
	if _, err := ParseLine(r.Line); err != nil {
		return "", err
	}
	return r.Line, nil
}

// ServesBrowser: liefert der Vermittler die Seite zum Zusehen im Browser aus
// (sshtermy vermittler ab Teilen im Browser; Spark und ältere nicht)?
func (c *Client) ServesBrowser(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/zusehen/", nil)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode == http.StatusOK && strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html")
}
