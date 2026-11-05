package sshx

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/db9979/sshtermy/internal/domain"
)

// SecretPrompt fragt ein fehlendes Geheimnis interaktiv ab (z. B. Passphrase).
type SecretPrompt func(label string) (string, error)

func clientConfig(c domain.Connection, o Options) (*ssh.ClientConfig, error) {
	methods, err := authMethods(c, o)
	if err != nil {
		return nil, err
	}
	if o.HostKeys == nil {
		return nil, errors.New("keine Host-Key-Prüfung konfiguriert") // nie InsecureIgnoreHostKey
	}
	return &ssh.ClientConfig{
		User:            c.Username,
		Auth:            methods,
		HostKeyCallback: o.HostKeys.Callback(o.Prompt),
		// Bekannte Schlüsseltypen des Hosts bevorzugen, damit kein unnötiger
		// "unbekannter Host"-Dialog bei schon bekanntem Host kommt.
		HostKeyAlgorithms: o.HostKeys.Algorithms(c.Addr()),
		Timeout:           o.Timeout,
		ClientVersion:     "SSH-2.0-sshtermy",
	}, nil
}

func authMethods(c domain.Connection, o Options) ([]ssh.AuthMethod, error) {
	switch c.Auth {
	case domain.AuthPassword:
		pw := c.Password.Reveal()
		m := []ssh.AuthMethod{ssh.PasswordCallback(func() (string, error) {
			if pw == "" && o.AskSecret != nil {
				return o.AskSecret("Passwort für " + c.Username + "@" + c.Host)
			}
			return pw, nil
		})}
		// Viele Server (PAM) verlangen keyboard-interactive statt password.
		m = append(m, ssh.KeyboardInteractive(func(_, _ string, qs []string, echos []bool) ([]string, error) {
			ans := make([]string, len(qs))
			for i, q := range qs {
				switch {
				case pw != "" && strings.Contains(strings.ToLower(q), "pass"):
					ans[i] = pw
				case o.AskSecret != nil:
					a, err := o.AskSecret(strings.TrimSpace(q))
					if err != nil {
						return nil, err
					}
					ans[i] = a
				}
				_ = echos
			}
			return ans, nil
		}))
		return m, nil

	case domain.AuthKeyFile:
		signer, err := loadSigner(c, o)
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil

	case domain.AuthAgent:
		conn, err := dialAgent()
		if err != nil {
			return nil, fmt.Errorf("SSH-Agent nicht erreichbar: %w", err)
		}
		// Der Agent hält die Private Keys; sshtermy sieht sie nie.
		return []ssh.AuthMethod{ssh.PublicKeysCallback(agent.NewClient(conn).Signers)}, nil
	}
	return nil, fmt.Errorf("unbekannte Auth-Methode %q", c.Auth)
}

func loadSigner(c domain.Connection, o Options) (ssh.Signer, error) {
	var pem []byte
	if c.KeyPEM.IsSet() {
		pem = []byte(c.KeyPEM.Reveal())
	} else {
		p := c.KeyPath
		if strings.HasPrefix(p, "~") {
			home, _ := os.UserHomeDir()
			p = filepath.Join(home, p[1:])
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("Keyfile: %w", err)
		}
		pem = b
	}
	defer func() {
		for i := range pem {
			pem[i] = 0
		}
	}()
	signer, err := ssh.ParsePrivateKey(pem)
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) {
		pass := c.KeyPassphrase.Reveal()
		if pass == "" && o.AskSecret != nil {
			if pass, err = o.AskSecret("Passphrase für den Key von " + c.Alias); err != nil {
				return nil, err
			}
		}
		signer, err = ssh.ParsePrivateKeyWithPassphrase(pem, []byte(pass))
	}
	return signer, err
}
