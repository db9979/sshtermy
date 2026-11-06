package sshx

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Schlüssel erzeugen und auf einen Server übertragen (wie ssh-keygen und
// ssh-copy-id), ohne dass OpenSSH installiert sein muss.

// KeyType: welche Art Schlüssel erzeugt wird.
type KeyType string

const (
	KeyEd25519 KeyType = "ed25519"  // modern, kurz, Standard
	KeyRSA4096 KeyType = "rsa-4096" // für alte Switches/Server ohne Ed25519
)

// NewKey ist ein frisch erzeugter (oder geladener) Schlüssel.
type NewKey struct {
	PrivatePEM []byte // OpenSSH-Format, mit Passphrase verschlüsselt, falls gesetzt
	PublicLine string // „ssh-ed25519 AAAA… Kommentar“ für authorized_keys
	Signer     ssh.Signer
}

// Wipe überschreibt den privaten Teil im Speicher (best effort).
func (k *NewKey) Wipe() {
	for i := range k.PrivatePEM {
		k.PrivatePEM[i] = 0
	}
}

// KeyComment: „benutzer@rechner-sshtermy“, nur unbedenkliche Zeichen.
func KeyComment() string {
	name := "user"
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = u.Username
		if i := strings.LastIndexAny(name, `\/`); i >= 0 { // Windows: DOMAIN\name
			name = name[i+1:]
		}
	}
	host, _ := os.Hostname()
	if i := strings.IndexByte(host, '.'); i > 0 {
		host = host[:i]
	}
	return safeComment(name + "@" + host + "-sshtermy")
}

func safeComment(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', strings.ContainsRune("@._-", r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// GenerateKey erzeugt einen neuen Schlüssel. Leere Passphrase = unverschlüsselt.
func GenerateKey(t KeyType, comment string, passphrase []byte) (*NewKey, error) {
	var priv any
	switch t {
	case KeyEd25519, "":
		_, k, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		priv = k
	case KeyRSA4096:
		k, err := rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			return nil, err
		}
		priv = k
	default:
		return nil, fmt.Errorf(i18n.T("unbekannter Schlüsseltyp %q"), t)
	}
	comment = safeComment(comment)
	var blk *pem.Block
	var err error
	if len(passphrase) > 0 {
		blk, err = ssh.MarshalPrivateKeyWithPassphrase(priv, comment, passphrase)
	} else {
		blk, err = ssh.MarshalPrivateKey(priv, comment)
	}
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, err
	}
	return &NewKey{PrivatePEM: pem.EncodeToMemory(blk), PublicLine: authorizedLine(signer.PublicKey(), comment), Signer: signer}, nil
}

// LoadKey liest einen vorhandenen privaten Schlüssel (für die Übertragung).
func LoadKey(pemBytes, passphrase []byte) (*NewKey, error) {
	s, err := parseKey(pemBytes, nil)
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) {
		if len(passphrase) == 0 {
			return nil, errors.New(i18n.T("Der Key ist verschlüsselt: Passphrase angeben"))
		}
		s, err = parseKey(pemBytes, passphrase)
	}
	if err != nil {
		return nil, errors.New(i18n.T("Key lässt sich nicht lesen (falsche Passphrase?)"))
	}
	return &NewKey{PrivatePEM: append([]byte(nil), pemBytes...), PublicLine: authorizedLine(s.PublicKey(), KeyComment()), Signer: s}, nil
}

func authorizedLine(pub ssh.PublicKey, comment string) string {
	l := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	if comment != "" {
		l += " " + comment
	}
	return l
}

// WriteKeyFiles legt den privaten Schlüssel (nur für dich lesbar) und die
// .pub-Datei an. Vorhandene Dateien werden nie überschrieben.
func WriteKeyFiles(path string, k *NewKey) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf(i18n.T("%s gibt es schon, bitte anderen Namen wählen"), path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(k.PrivatePEM); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.WriteFile(path+".pub", []byte(k.PublicLine+"\n"), 0o644)
}

// installMarker: daran erkennt sshTermy, dass das Skript durchgelaufen ist.
const (
	markAdded = "SSHTERMY-KEY-ADDED"
	markHave  = "SSHTERMY-KEY-PRESENT"
)

// InstallScript: POSIX-Shell-Skript wie ssh-copy-id. Legt ~/.ssh und
// authorized_keys mit den richtigen Rechten an und hängt den Schlüssel nur
// an, wenn er noch nicht drinsteht.
func InstallScript(pubLine string) (string, error) {
	f := strings.Fields(pubLine)
	if len(f) < 2 || strings.ContainsAny(pubLine, "'\n\r\\") {
		return "", errors.New(i18n.T("ungültiger öffentlicher Schlüssel"))
	}
	if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubLine)); err != nil {
		return "", errors.New(i18n.T("ungültiger öffentlicher Schlüssel"))
	}
	keyPart := f[0] + " " + f[1] // Kommentar zählt beim Vergleich nicht
	return "umask 077; cd || exit 1; mkdir -p .ssh && chmod 700 .ssh && touch .ssh/authorized_keys && chmod 600 .ssh/authorized_keys || exit 1; " +
		"if grep -qF '" + keyPart + "' .ssh/authorized_keys; then echo " + markHave + "; else " +
		"if [ -s .ssh/authorized_keys ] && [ -n \"$(tail -c 1 .ssh/authorized_keys)\" ]; then echo >> .ssh/authorized_keys; fi; " +
		"echo '" + pubLine + "' >> .ssh/authorized_keys && echo " + markAdded + "; fi", nil
}

// InstallKey trägt den öffentlichen Schlüssel auf dem Server ein.
// already = er stand schon in authorized_keys.
func (c *Client) InstallKey(ctx context.Context, pubLine string) (already bool, err error) {
	if c.NetDevice {
		return false, ErrNetDeviceKey
	}
	script, err := InstallScript(pubLine)
	if err != nil {
		return false, err
	}
	res, err := c.Exec(ctx, script)
	out := string(res.Output)
	switch {
	case strings.Contains(out, markHave):
		return true, nil
	case strings.Contains(out, markAdded):
		return false, nil
	case err != nil:
		return false, err
	}
	out = strings.TrimSpace(out)
	if len(out) > 300 {
		out = out[len(out)-300:]
	}
	if out == "" {
		out = fmt.Sprintf("exit %d", res.Exit)
	}
	return false, errors.New(i18n.T("Eintragen hat nicht geklappt (kein Linux/macOS-Server?): ") + out)
}

// ErrNetDeviceKey: Switches/Router haben kein authorized_keys; der Key muss
// dort per Gerätebefehl eingetragen werden.
var ErrNetDeviceKey = i18n.NewError("Netzwerkgerät: Schlüssel bitte am Gerät eintragen")
