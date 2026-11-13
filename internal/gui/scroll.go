//go:build windows || darwin || linux

package gui

import (
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/db9979/sshtermy/internal/termemu"
)

// Zurückblättern im zweiten eigenen Fenster (abgetrennter Tab, ⇧F10).
// Im Hauptfenster blättert sshTermy selbst (ui/scrollkeys.go); hier hängt
// das Fenster direkt am Server, also blättert es in seinem eigenen
// Verlauf: Mausrad (⇧ + Rad, wenn das Programm die Maus will),
// ⇧Bild↑/↓ seitenweise, ⇧Pos1 an den Anfang, ⇧Ende zurück nach unten.
// Jede andere Taste führt wieder nach unten.

// canScroll: Rückblick ist nur im normalen Bildschirm sinnvoll.
func (w *window) canScroll() bool { return w.opts.attach && !w.term.AltScreen() }

func (w *window) setScroll(n int) {
	n = max(0, min(n, w.term.ScrollbackLen()))
	if n != w.scroll {
		w.scroll, w.forceDraw = n, true
	}
	w.sbSeen = w.term.ScrollbackLen()
}

// scrollKey: ⇧Bild↑/↓, ⇧Pos1, ⇧Ende. true = verbraucht.
func (w *window) scrollKey(code rune, mod uv.KeyMod) bool {
	if !w.canScroll() || mod != uv.ModShift {
		return false
	}
	_, h := w.term.Size()
	page := max(h-1, 1)
	switch code {
	case uv.KeyPgUp:
		w.setScroll(w.scroll + page)
	case uv.KeyPgDown:
		w.setScroll(w.scroll - page)
	case uv.KeyHome:
		w.setScroll(w.term.ScrollbackLen())
	case uv.KeyEnd:
		w.setScroll(0)
	default:
		return false
	}
	return true
}

// localWheel: Das Rad blättert im Fenster selbst, statt an das Programm
// zu gehen (das will die Maus nicht, oder ⇧ ist gedrückt).
func (w *window) localWheel(shift bool) bool {
	return w.opts.attach && (w.term.MouseTracking() == termemu.MouseOff || shift)
}

// scrollWheel: eine Raste Mausrad.
func (w *window) scrollWheel(up bool) {
	if w.term.AltScreen() {
		// less, man ohne Maus: Rad = Pfeiltasten
		code := uv.KeyDown
		if up {
			code = uv.KeyUp
		}
		for i := 0; i < 3; i++ {
			w.sendOrdered(encodeKey(uv.Key{Code: code}))
		}
		return
	}
	d := -3
	if up {
		d = 3
	}
	w.setScroll(w.scroll + d)
}

// followScroll: Neue Ausgabe schiebt die Ansicht beim Lesen nicht weg.
func (w *window) followScroll() {
	sb := w.term.ScrollbackLen()
	if w.scroll > 0 && sb > w.sbSeen {
		w.scroll = min(sb, w.scroll+sb-w.sbSeen)
	}
	if w.term.AltScreen() {
		w.scroll = 0
	}
	w.scroll = min(w.scroll, sb)
	w.sbSeen = sb
}
