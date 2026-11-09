package ui

// Bilder pixelgenau: Läuft sshTermy in kitty oder Ghostty, schickt der
// Emulator Bilder vom Server (Sixel, kitty, iTerm2) als echtes Bild an
// dieses Terminal und setzt Platzhalter in die Zellen (termemu/kittypass.go).
// Sonst bleibt es bei Halbblöcken in Farbe.

import (
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/termemu"
)

// pixelsOn: Terminal kann es und es ist nicht abgeschaltet.
var pixelsOn = termemu.PixelImagesSupported(os.Getenv)

// EnablePixelImages: Das eigene Fenster (internal/gui) zeichnet kitty-Bilder
// mit Platzhaltern selbst. Aufruf vor New; SSHTERMY_NO_PIXELS schaltet ab.
func EnablePixelImages() { pixelsOn = os.Getenv("SSHTERMY_NO_PIXELS") == "" }

// pixelImages: Emulator der Sitzung aufs Durchreichen einstellen.
func (s *session) pixelImages() {
	if !pixelsOn || s.term == nil {
		return
	}
	s.term.SetPixelImages(func(seq []byte) {
		s.pixMu.Lock()
		s.pixQ = append(s.pixQ, seq...)
		s.pixMu.Unlock()
	})
}

// takePixels: wartende Bilder ans Terminal schicken (vor dem nächsten Bild,
// damit die Platzhalter gleich etwas zeigen).
func (a *App) takePixels(sid int) tea.Cmd {
	s := a.byID(sid)
	if s == nil {
		return nil
	}
	s.pixMu.Lock()
	q := s.pixQ
	s.pixQ = nil
	s.pixMu.Unlock()
	if len(q) == 0 {
		return nil
	}
	return tea.Raw(string(q))
}
