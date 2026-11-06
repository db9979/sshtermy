package keychain

import (
	"encoding/hex"
	"strings"
)

// parseSecurityPassword liest die Zeile „password: …“ aus der Ausgabe von
// macOS „security find-generic-password -g“. Liegt hier (ohne Build-Tag),
// damit der Test überall läuft.
func parseSecurityPassword(out string) (string, error) {
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimRight(l, "\r")
		if !strings.HasPrefix(l, "password:") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(l, "password:"))
		if v == "" {
			return "", nil
		}
		if strings.HasPrefix(v, "0x") {
			h := v[2:]
			if i := strings.IndexByte(h, ' '); i >= 0 {
				h = h[:i]
			}
			b, err := hex.DecodeString(h)
			if err != nil {
				return "", err
			}
			return string(b), nil
		}
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			return v[1 : len(v)-1], nil
		}
		return v, nil
	}
	return "", ErrNotFound
}
