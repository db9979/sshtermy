package i18n

import (
	"os"
	"path/filepath"
	"strings"
)

// hintFile liegt unverschlüsselt im Datenordner, damit schon der
// Entsperr-Bildschirm (vor dem Entschlüsseln der Einstellungen) die gewählte
// Sprache zeigt. Er enthält nur „de“ oder „en“.
const hintFile = "language"

// Load wählt die Sprache vor dem Entsperren: zuerst die zuletzt gewählte
// (Datei im Datenordner), sonst die Sprache des Systems.
func Load(dir string) string {
	if dir != "" {
		if b, err := os.ReadFile(filepath.Join(dir, hintFile)); err == nil {
			if l := strings.TrimSpace(string(b)); l == DE || l == EN {
				Set(l)
				return l
			}
		}
	}
	Set(Detect())
	return Lang()
}

// Save merkt sich die Sprache für den nächsten Start.
func Save(dir, l string) error {
	if dir == "" {
		return nil
	}
	if b, err := os.ReadFile(filepath.Join(dir, hintFile)); err == nil && strings.TrimSpace(string(b)) == l {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, hintFile), []byte(l+"\n"), 0o600)
}

// Detect leitet die Sprache aus dem System ab: Deutsch bei einer deutschen
// Umgebung (und wenn nichts bekannt ist), sonst Englisch.
func Detect() string { return fromLocale(systemLocale()) }

func fromLocale(loc string) string {
	loc = strings.ToLower(strings.TrimSpace(loc))
	switch {
	case loc == "" || loc == "c" || loc == "posix" || strings.HasPrefix(loc, "c."):
		return DE
	case strings.HasPrefix(loc, "de"):
		return DE
	}
	return EN
}

func envLocale() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
