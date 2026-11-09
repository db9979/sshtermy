package sshx

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// SSH-Zertifikate: In vielen Firmen signiert eine CA die Benutzer-Keys
// (ssh-keygen -s ca -I name -n user id_ed25519.pub → id_ed25519-cert.pub).
// Der Server vertraut dann der CA statt einzelnen Keys. sshTermy sucht das
// Zertifikat wie OpenSSH neben dem Keyfile oder nimmt den Pfad aus der
// Verbindung, und meldet sich mit Zertifikat + Key an.

// ErrCertExpired: Das Zertifikat ist abgelaufen oder noch nicht gültig.
var ErrCertExpired = errors.New("certificate not valid")

// certCandidates: wo das Zertifikat liegen könnte.
func certCandidates(c domain.Connection, home string) []string {
	if p := strings.TrimSpace(c.CertPath); p != "" {
		// wie beim Key: auch derselbe Pfad unter dem eigenen Home und
		// ~/.ssh/<Name>, falls die Verbindung von einem anderen Rechner kommt
		return domain.KeyPathCandidates(p, home)
	}
	if c.KeyPath == "" {
		return nil
	}
	var out []string
	for _, k := range domain.KeyPathCandidates(c.KeyPath, home) {
		out = append(out, k+"-cert.pub")
	}
	return out
}

// withCert hängt das Zertifikat an den Signer, wenn es eins gibt. Ein
// ausdrücklich eingetragenes, aber fehlendes oder kaputtes Zertifikat ist
// ein Fehler; ein fehlendes <key>-cert.pub nicht.
func withCert(c domain.Connection, signer ssh.Signer, now time.Time) (ssh.Signer, error) {
	home, _ := os.UserHomeDir()
	explicit := strings.TrimSpace(c.CertPath) != ""
	var firstErr error
	for _, p := range certCandidates(c, home) {
		b, err := os.ReadFile(p)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		cert, err := ParseCert(b)
		if err != nil {
			if explicit {
				return nil, err
			}
			continue
		}
		if err := CertValid(cert, now); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		cs, err := ssh.NewCertSigner(cert, signer)
		if err != nil {
			return nil, fmt.Errorf(i18n.T("Zertifikat passt nicht zum Key: %w"), err)
		}
		return cs, nil
	}
	if explicit && firstErr != nil {
		return nil, fmt.Errorf(i18n.T("Zertifikat: %w"), firstErr)
	}
	return signer, nil
}

// ParseCert liest ein Benutzer-Zertifikat im OpenSSH-Format (*-cert.pub).
func ParseCert(b []byte) (*ssh.Certificate, error) {
	pub, _, _, _, err := ssh.ParseAuthorizedKey(b)
	if err != nil {
		return nil, errors.New(i18n.T("Das ist kein SSH-Zertifikat (*-cert.pub)"))
	}
	cert, ok := pub.(*ssh.Certificate)
	if !ok || cert.CertType != ssh.UserCert {
		return nil, errors.New(i18n.T("Das ist kein SSH-Zertifikat (*-cert.pub)"))
	}
	return cert, nil
}

// CertValid prüft den Gültigkeitszeitraum (Server würden es sonst mit einem
// nichtssagenden „permission denied“ ablehnen).
func CertValid(cert *ssh.Certificate, now time.Time) error {
	unix := uint64(now.Unix())
	if cert.ValidAfter != 0 && unix < cert.ValidAfter {
		return fmt.Errorf("%w: "+i18n.T("erst gültig ab %s"), ErrCertExpired, time.Unix(int64(cert.ValidAfter), 0).Format("2006-01-02 15:04"))
	}
	if cert.ValidBefore != ssh.CertTimeInfinity && unix >= cert.ValidBefore {
		return fmt.Errorf("%w: "+i18n.T("abgelaufen am %s, bitte neu signieren lassen"), ErrCertExpired, time.Unix(int64(cert.ValidBefore), 0).Format("2006-01-02 15:04"))
	}
	return nil
}

// defaultCert: ~/.ssh/<key>-cert.pub zu den Standard-Keys (automatische Anmeldung).
func defaultCert(home, keyName string, signer ssh.Signer, now time.Time) ssh.Signer {
	b, err := os.ReadFile(filepath.Join(home, ".ssh", keyName+"-cert.pub"))
	if err != nil {
		return nil
	}
	cert, err := ParseCert(b)
	if err != nil || CertValid(cert, now) != nil {
		return nil
	}
	cs, err := ssh.NewCertSigner(cert, signer)
	if err != nil {
		return nil
	}
	return cs
}
