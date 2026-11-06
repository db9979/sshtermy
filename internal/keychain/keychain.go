// Package keychain legt Geheimnisse im Schlüsselbund des Betriebssystems ab:
//
//   - macOS: Schlüsselbund (Programm /usr/bin/security, kein cgo nötig)
//   - Windows: Anmeldeinformationsverwaltung (advapi32 CredWriteW/CredReadW)
//   - Linux: Secret Service (GNOME Schlüsselbund, KWallet) über secret-tool
//
// sshTermy nutzt das für Passwörter und Key-Passphrasen, die ein Nutzer
// lieber dort als im eigenen (verschlüsselten) Store haben will. Einträge
// gehören immer zu genau diesem Rechner; der Sync überträgt sie nicht.
package keychain

import (
	"errors"
	"sync"
)

// Service ist der Dienstname aller Einträge von sshTermy.
const Service = "sshTermy"

var (
	// ErrNotFound: Eintrag gibt es (auf diesem Rechner) nicht.
	ErrNotFound = errors.New("keychain: not found")
	// ErrUnavailable: kein Schlüsselbund nutzbar (z. B. Linux ohne secret-tool).
	ErrUnavailable = errors.New("keychain: not available")
)

// Backend ist ein Schlüsselbund. Tests setzen mit Use einen eigenen ein.
type Backend interface {
	Available() bool
	Get(account string) (string, error)
	Set(account, label, secret string) error
	Delete(account string) error
}

var (
	mu      sync.RWMutex
	current Backend = system{}
)

// Use ersetzt den Schlüsselbund (Tests) und liefert den vorherigen zurück.
func Use(b Backend) Backend {
	mu.Lock()
	defer mu.Unlock()
	old := current
	current = b
	return old
}

func be() Backend {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Available: gibt es auf diesem Rechner einen nutzbaren Schlüsselbund?
func Available() bool { return be().Available() }

// Get liest ein Geheimnis. ErrNotFound, wenn es keinen Eintrag gibt.
func Get(account string) (string, error) {
	b := be()
	if !b.Available() {
		return "", ErrUnavailable
	}
	return b.Get(account)
}

// Set schreibt (oder ersetzt) ein Geheimnis. label ist der für Menschen
// lesbare Name im Schlüsselbund, z. B. „sshTermy: DGX Spark“.
func Set(account, label, secret string) error {
	b := be()
	if !b.Available() {
		return ErrUnavailable
	}
	return b.Set(account, label, secret)
}

// Delete entfernt einen Eintrag; fehlt er, ist das kein Fehler.
func Delete(account string) error {
	b := be()
	if !b.Available() {
		return nil
	}
	if err := b.Delete(account); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

// Memory ist ein Schlüsselbund nur im Arbeitsspeicher (Tests).
type Memory struct {
	mu sync.Mutex
	M  map[string]string
}

func (m *Memory) Available() bool { return true }
func (m *Memory) Get(a string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.M[a]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}
func (m *Memory) Set(a, _, s string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.M == nil {
		m.M = map[string]string{}
	}
	m.M[a] = s
	return nil
}
func (m *Memory) Delete(a string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.M[a]; !ok {
		return ErrNotFound
	}
	delete(m.M, a)
	return nil
}
