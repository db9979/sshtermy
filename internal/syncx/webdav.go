package syncx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// WebDAV spricht mit einem WebDAV-Ordner (Synology „WebDAV Server“, QNAP,
// Nextcloud, Apache/nginx …). URL zeigt auf den Ordner, z. B.
// https://diskstation:5006/home/sshtermy/
type WebDAV struct {
	URL      string
	User     string
	Password string
	// CertPin: SHA-256 (hex) des Server-Zertifikats. Gesetzt = genau dieses
	// (z. B. selbstsignierte) Zertifikat wird akzeptiert, sonst nichts.
	CertPin string

	client *http.Client
}

func (w *WebDAV) Describe() string { return "WebDAV " + w.URL }

// CertError: das Zertifikat ist nicht von einer bekannten Stelle signiert.
// Fingerprint kann der Nutzer mit der NAS-Oberfläche vergleichen und pinnen.
type CertError struct {
	Fingerprint string // SHA-256, hex mit Doppelpunkten
	Pin         string // SHA-256, hex
	Subject     string
	Reason      string
}

func (e *CertError) Error() string { return i18n.T("Zertifikat nicht vertrauenswürdig: ") + e.Reason }

// HTTPError: Server hat mit einem unerwarteten Status geantwortet.
type HTTPError struct {
	Method string
	Status int
	HTML   bool // Antwort war eine Webseite (oft: keine WebDAV-Adresse)
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: HTTP %d %s", e.Method, e.Status, http.StatusText(e.Status))
}

func certPin(raw []byte) (pin, pretty string) {
	h := sha256.Sum256(raw)
	pin = hex.EncodeToString(h[:])
	parts := make([]string, len(h))
	for i, b := range h {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return pin, strings.Join(parts, ":")
}

func (w *WebDAV) http() *http.Client {
	if w.client != nil {
		return w.client
	}
	tc := &tls.Config{MinVersion: tls.VersionTLS12}
	if w.CertPin != "" {
		want := strings.ToLower(strings.ReplaceAll(w.CertPin, ":", ""))
		tc.InsecureSkipVerify = true // ersetzt durch die Pin-Prüfung unten
		tc.VerifyPeerCertificate = func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 {
				return errors.New(i18n.T("kein Zertifikat"))
			}
			if p, _ := certPin(raw[0]); p != want {
				return &CertError{Reason: i18n.T("das Zertifikat hat sich geändert (anderer Fingerprint als gespeichert)")}
			}
			return nil
		}
	}
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 8 * time.Second}).DialContext,
		TLSClientConfig:     tc,
		TLSHandshakeTimeout: 8 * time.Second,
	}
	w.client = &http.Client{Transport: tr, Timeout: 30 * time.Second,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) > 5 {
				return errors.New(i18n.T("zu viele Weiterleitungen"))
			}
			// Zugangsdaten nur an denselben Host weitergeben
			if r.URL.Host != via[0].URL.Host {
				r.Header.Del("Authorization")
			}
			return nil
		}}
	return w.client
}

func (w *WebDAV) fileURL(name string) string {
	return strings.TrimSuffix(w.URL, "/") + "/" + url.PathEscape(name)
}

func (w *WebDAV) do(ctx context.Context, method, u string, body []byte, hdr map[string]string) (*http.Response, []byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, nil, err
	}
	if w.User != "" || w.Password != "" {
		req.SetBasicAuth(w.User, w.Password)
	}
	req.Header.Set("User-Agent", "sshTermy-Sync")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := w.http().Do(req)
	if err != nil {
		return nil, nil, classifyNetErr(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return resp, b, err
}

// classifyNetErr macht aus TLS-Fehlern einen CertError mit Fingerprint.
func classifyNetErr(err error) error {
	var ce *CertError
	if errors.As(err, &ce) {
		return ce
	}
	var ua x509.UnknownAuthorityError
	var hn x509.HostnameError
	var ci x509.CertificateInvalidError
	switch {
	case errors.As(err, &ua) && ua.Cert != nil:
		pin, fp := certPin(ua.Cert.Raw)
		return &CertError{Fingerprint: fp, Pin: pin, Subject: ua.Cert.Subject.CommonName, Reason: i18n.T("selbstsigniert oder unbekannte Zertifizierungsstelle")}
	case errors.As(err, &hn) && hn.Certificate != nil:
		pin, fp := certPin(hn.Certificate.Raw)
		return &CertError{Fingerprint: fp, Pin: pin, Subject: hn.Certificate.Subject.CommonName, Reason: i18n.T("ausgestellt für einen anderen Namen (") + hn.Host + ")"}
	case errors.As(err, &ci) && ci.Cert != nil:
		pin, fp := certPin(ci.Cert.Raw)
		return &CertError{Fingerprint: fp, Pin: pin, Subject: ci.Cert.Subject.CommonName, Reason: i18n.T("abgelaufen oder ungültig")}
	}
	return err
}

func etagOf(resp *http.Response, body []byte) string {
	if e := resp.Header.Get("ETag"); e != "" && !strings.HasPrefix(e, "W/") {
		return "etag:" + e
	}
	return sum(body) // Server ohne (starken) ETag: über den Inhalt vergleichen
}

func (w *WebDAV) Read(ctx context.Context) ([]byte, string, error) {
	resp, b, err := w.do(ctx, http.MethodGet, w.fileURL(FileName), nil, nil)
	if err != nil {
		return nil, "", err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return b, etagOf(resp, b), nil
	case http.StatusNotFound:
		return nil, "", ErrNotFound
	}
	return nil, "", &HTTPError{Method: "GET", Status: resp.StatusCode, HTML: isHTML(resp)}
}

func (w *WebDAV) Write(ctx context.Context, data []byte, ifVersion string) error {
	hdr := map[string]string{"Content-Type": "application/octet-stream"}
	switch {
	case ifVersion == "":
		hdr["If-None-Match"] = "*"
	case strings.HasPrefix(ifVersion, "etag:"):
		hdr["If-Match"] = strings.TrimPrefix(ifVersion, "etag:")
	default: // Prüfsumme: direkt vorher noch einmal lesen
		cur, v, err := w.Read(ctx)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if cur == nil || sum(cur) != ifVersion {
			_ = v
			return ErrConflict
		}
	}
	resp, _, err := w.do(ctx, http.MethodPut, w.fileURL(FileName), data, hdr)
	if err != nil {
		return err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	case http.StatusPreconditionFailed:
		return ErrConflict
	}
	return &HTTPError{Method: "PUT", Status: resp.StatusCode, HTML: isHTML(resp)}
}

func isHTML(r *http.Response) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "text/html")
}

// propfind prüft, ob der Ordner existiert und WebDAV spricht.
func (w *WebDAV) propfind(ctx context.Context, u string) (int, bool, error) {
	body := []byte(`<?xml version="1.0" encoding="utf-8"?><d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/></d:prop></d:propfind>`)
	resp, _, err := w.do(ctx, "PROPFIND", u, body, map[string]string{"Depth": "0", "Content-Type": "application/xml"})
	if err != nil {
		return 0, false, err
	}
	return resp.StatusCode, isHTML(resp), nil
}

func (w *WebDAV) mkcol(ctx context.Context, u string) (int, error) {
	resp, _, err := w.do(ctx, "MKCOL", u, nil, nil)
	if err != nil {
		return 0, err
	}
	return resp.StatusCode, nil
}

func (w *WebDAV) del(ctx context.Context, u string) error {
	_, _, err := w.do(ctx, http.MethodDelete, u, nil, nil)
	return err
}
