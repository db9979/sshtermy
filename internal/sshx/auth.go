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
	cfg := &ssh.ClientConfig{
		User:            c.Username,
		Auth:            methods,
		HostKeyCallback: o.HostKeys.Callback(o.Prompt),
		// Bekannte Schlüsseltypen des Hosts bevorzugen, damit kein unnötiger
		// "unbekannter Host"-Dialog bei schon bekanntem Host kommt.
		HostKeyAlgorithms: o.HostKeys.Algorithms(c.Addr()),
		Timeout:           o.Timeout,
		ClientVersion:     "SSH-2.0-sshtermy",
	}
	if o.legacy {
		// Nur als zweiter Versuch, wenn der Server keine moderne Kombination
		// anbietet (alte Switches/Router). Moderne Verfahren stehen vorn.
		sup, old := ssh.SupportedAlgorithms(), ssh.InsecureAlgorithms()
		cfg.KeyExchanges = append(sup.KeyExchanges, old.KeyExchanges...)
		cfg.Ciphers = append(sup.Ciphers, old.Ciphers...)
		cfg.MACs = append(sup.MACs, old.MACs...)
		if len(cfg.HostKeyAlgorithms) == 0 {
			cfg.HostKeyAlgorithms = append(sup.HostKeys, old.HostKeys...)
		}
	}
	return cfg, nil
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

	case domain.AuthAuto:
		return autoMethods(c, o), nil
	}
	return nil, fmt.Errorf("unbekannte Auth-Methode %q", c.Auth)
}

// autoMethods: Agent + unverschlüsselte Standard-Keys, danach Passwort und
// keyboard-interactive mit Rückfrage. Höchstens 5 Keys, sonst bricht OpenSSH
// mit "Too many authentication failures" ab, bevor das Passwort drankommt.
func autoMethods(c domain.Connection, o Options) []ssh.AuthMethod {
	var m []ssh.AuthMethod
	signers := func() ([]ssh.Signer, error) {
		var out []ssh.Signer
		if conn, err := dialAgent(); err == nil {
			if ss, err := agent.NewClient(conn).Signers(); err == nil {
				out = append(out, ss...)
			}
		}
		home, _ := os.UserHomeDir()
		for _, n := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
			b, err := os.ReadFile(filepath.Join(home, ".ssh", n))
			if err != nil {
				continue
			}
			if s, err := ssh.ParsePrivateKey(b); err == nil {
				out = append(out, s)
			}
		}
		if len(out) > 5 {
			out = out[:5]
		}
		return out, nil
	}
	m = append(m, ssh.PublicKeysCallback(signers))
	if o.AskSecret == nil {
		return m
	}
	m = append(m, ssh.PasswordCallback(func() (string, error) {
		return o.AskSecret("Passwort für " + c.Username + "@" + c.Host)
	}))
	m = append(m, ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
		ans := make([]string, len(qs))
		for i, q := range qs {
			label := strings.TrimSpace(q)
			if strings.Contains(strings.ToLower(q), "pass") {
				label = "Passwort für " + c.Username + "@" + c.Host // gleiche Frage wie oben, nur einmal
			}
			a, err := o.AskSecret(label)
			if err != nil {
				return nil, err
			}
			ans[i] = a
		}
		return ans, nil
	}))
	return m
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
