//go:build !windows

package vault

// Auf macOS/Linux gibt es kein DPAPI. Der Store wird dort nur mit dem
// Master-Passwort (Argon2id + AES-256-GCM) geschützt. Eine Erweiterung um den
// macOS-Schlüsselbund (Keychain) ist als eigener Wrap-Typ "keychain" möglich,
// braucht aber cgo oder das security-CLI und ist deshalb nicht Teil des Kerns.

// DPAPIAvailable: außerhalb von Windows nie.
func DPAPIAvailable() bool { return false }

func dpapiProtect(_, _ []byte) ([]byte, error)   { return nil, ErrDPAPI }
func dpapiUnprotect(_, _ []byte) ([]byte, error) { return nil, ErrDPAPI }
