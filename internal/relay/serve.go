package relay

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// ServeOptions für `sshtermy vermittler start`.
type ServeOptions struct {
	Dir    string
	Listen string // z. B. ":8022"
	URL    string // öffentliche Adresse, z. B. https://vermittler.example.org:8022
	Name   string // Anzeigename ("" = Rechnername)
	// Cert/Key: eigenes Zertifikat (z. B. Let's Encrypt). Leer = selbst
	// signiertes Zertifikat im Datenordner, das sshTermy per Fingerabdruck pinnt.
	Cert, Key string
	// BehindProxy: nur http auf Listen (127.0.0.1 empfohlen), TLS macht ein
	// Reverse-Proxy davor (Caddy, nginx). Dann gilt auch X-Forwarded-For.
	BehindProxy bool
}

// Prepare legt Datenordner, Zertifikat und Zustand an und gibt den Zustand
// zurück (Name, URL und Pin stehen danach darin).
func Prepare(o ServeOptions) (*State, *tls.Certificate, error) {
	if o.URL == "" {
		return nil, nil, i18n.NewError("Öffentliche Adresse fehlt (--url https://…)")
	}
	base, err := NormBase(o.URL)
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return nil, nil, err
	}
	st, err := LoadState(o.Dir)
	if err != nil {
		return nil, nil, err
	}
	var cert *tls.Certificate
	st.FP = ""
	switch {
	case o.BehindProxy:
	case o.Cert != "" || o.Key != "":
		c, err := tls.LoadX509KeyPair(o.Cert, o.Key)
		if err != nil {
			return nil, nil, err
		}
		cert = &c
	default:
		c, err := selfSigned(o.Dir, base)
		if err != nil {
			return nil, nil, err
		}
		cert = &c
		st.FP = CertFP(c.Certificate[0])
	}
	st.URL = base
	if o.Name != "" {
		st.Name = name(o.Name, "")
	}
	if st.Name == "" {
		h, _ := os.Hostname()
		st.Name = name(h, "Vermittler")
	}
	return st, cert, SaveState(o.Dir, st)
}

// Serve startet den Vermittler und läuft, bis ctx endet.
func Serve(ctx context.Context, o ServeOptions) error {
	_, cert, err := Prepare(o)
	if err != nil {
		return err
	}
	s, err := NewServer(o.Dir)
	if err != nil {
		return err
	}
	s.TrustProxy = o.BehindProxy
	hs := &http.Server{Addr: o.Listen, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes: 16 << 10}
	if cert != nil {
		hs.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{*cert}}
	}
	go s.Watch(ctx)
	go func() {
		<-ctx.Done()
		sc, c := context.WithTimeout(context.Background(), 3*time.Second)
		defer c()
		_ = hs.Shutdown(sc)
	}()
	if cert != nil {
		err = hs.ListenAndServeTLS("", "")
	} else {
		err = hs.ListenAndServe()
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Line: Kopplungszeile für einen neuen Code (Gerät oder Anker).
func Line(dir, kind string) (string, error) {
	st, err := LoadState(dir)
	if err != nil {
		return "", err
	}
	if st.URL == "" {
		return "", i18n.NewError("Vermittler noch nie gestartet: erst „sshtermy vermittler start --url https://…“")
	}
	code, err := NewCode(dir, kind)
	if err != nil {
		return "", err
	}
	return PairLine{Base: st.URL, Code: code, FP: st.FP}.String(), nil
}

// selfSigned lädt oder erzeugt ein Zertifikat (ECDSA P-256, 10 Jahre).
// sshTermy pinnt seinen Fingerabdruck, Name und Aussteller spielen keine Rolle.
func selfSigned(dir, base string) (tls.Certificate, error) {
	cp, kp := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if c, err := tls.LoadX509KeyPair(cp, kp); err == nil {
		return c, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	u, _ := url.Parse(base)
	host := u.Hostname()
	sn, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	tpl := &x509.Certificate{
		SerialNumber: sn, Subject: pkix.Name{CommonName: "sshTermy-Vermittler"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(10, 0, 0),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(host); ip != nil {
		tpl.IPAddresses = []net.IP{ip}
	} else if host != "" {
		tpl.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(kp, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(cp, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return tls.Certificate{}, err
	}
	return tls.LoadX509KeyPair(cp, kp)
}
