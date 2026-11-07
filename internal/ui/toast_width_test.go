package ui

import (
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

// TestToastSlideInStaysInWindow: Beim Hereingleiten ragte der Toast rechts
// über das Fenster hinaus, das Bild wurde breiter als das Terminal.
func TestToastSlideInStaysInWindow(t *testing.T) {
	a := &App{w: 100, h: 30, s: NewStyles(Nacht)}
	a.toasts = []toastItem{{text: "Ansicht: zwei nebeneinander", born: time.Now(), until: time.Now().Add(time.Minute)}}
	base := lipgloss.NewStyle().Width(100).Height(30).Render("")
	if w := lipgloss.Width(a.withToasts(base)); w > 100 {
		t.Fatalf("Bild %d Spalten breit, Fenster 100", w)
	}
}
