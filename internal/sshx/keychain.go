package sshx

import (
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/keychain"
)

// Geheimnisse im Schlüsselbund des Betriebssystems (Connection.Keychain).
// Ein Eintrag pro Verbindung und Art, Konto „<Verbindungs-ID>/password“
// bzw. „…/passphrase“; die ID ist auf allen Rechnern gleich.

const (
	SecretPassword   = "password"
	SecretPassphrase = "passphrase"
)

// KeychainAccount: Kontoname im Schlüsselbund.
func KeychainAccount(connID, kind string) string { return connID + "/" + kind }

// KeychainLabel: lesbarer Name im Schlüsselbund.
func KeychainLabel(c domain.Connection, kind string) string {
	what := c.Username + "@" + c.Host
	if kind == SecretPassphrase {
		what = "Key-Passphrase " + c.Alias
	}
	return keychain.Service + ": " + what
}

// KeychainSecret liest ein Geheimnis der Verbindung aus dem Schlüsselbund
// ("" wenn es keins gibt oder kein Schlüsselbund da ist).
func KeychainSecret(connID, kind string) string {
	v, err := keychain.Get(KeychainAccount(connID, kind))
	if err != nil {
		return ""
	}
	return v
}

// SetKeychainSecret schreibt das Geheimnis, leer = Eintrag löschen.
func SetKeychainSecret(c domain.Connection, kind, secret string) error {
	if secret == "" {
		return keychain.Delete(KeychainAccount(c.ID, kind))
	}
	return keychain.Set(KeychainAccount(c.ID, kind), KeychainLabel(c, kind), secret)
}

// ForgetKeychain löscht beide Einträge der Verbindung (auf diesem Rechner).
func ForgetKeychain(connID string) {
	_ = keychain.Delete(KeychainAccount(connID, SecretPassword))
	_ = keychain.Delete(KeychainAccount(connID, SecretPassphrase))
}

// withKeychain füllt fehlende Geheimnisse aus dem Schlüsselbund, nur für
// den Verbindungsaufbau (die Kopie landet nie im Store).
func withKeychain(c domain.Connection) domain.Connection {
	if !c.Keychain {
		return c
	}
	switch c.Auth {
	case domain.AuthPassword:
		if !c.Password.IsSet() {
			c.Password = domain.Secret(KeychainSecret(c.ID, SecretPassword))
		}
	case domain.AuthKeyFile:
		if !c.KeyPassphrase.IsSet() {
			c.KeyPassphrase = domain.Secret(KeychainSecret(c.ID, SecretPassphrase))
		}
	}
	return c
}

// PasswordLabel / PassphraseLabel: Text der Rückfrage, wenn das Geheimnis
// fehlt. Die Oberfläche erkennt daran, was sie im Schlüsselbund ablegen kann.
func PasswordLabel(c domain.Connection) string {
	return i18n.T("Passwort für ") + c.Username + "@" + c.Host
}

func PassphraseLabel(c domain.Connection) string {
	return i18n.T("Passphrase für den Key von ") + c.Alias
}
