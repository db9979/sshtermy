//go:build windows || darwin || linux

package gui

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/db9979/sshtermy/internal/winstate"
)

// Fensterzustand merken: Startart (Vollbild/maximiert/wie zuletzt),
// Größe, Position und Schriftgröße (internal/winstate). F11 schaltet das
// Vollbild um (Alt+⏎ bleibt für Programme im Terminal frei).

func (w *window) applySaved() {
	s := w.saved
	if w.opts.StateDir == "" || w.script != nil { // Bildschirmfotos: feste Größe
		return
	}
	if s.W > 200 && s.H > 120 {
		ebiten.SetWindowSize(s.W, s.H)
	}
	if s.HasXY {
		ebiten.SetWindowPosition(s.X, s.Y)
	}
	switch s.Mode() {
	case winstate.StartFull:
		ebiten.SetFullscreen(true)
	case winstate.StartMax:
		ebiten.MaximizeWindow()
	case winstate.StartLast:
		if s.Full {
			ebiten.SetFullscreen(true)
		} else if s.Max {
			ebiten.MaximizeWindow()
		}
	}
}

func (w *window) toggleFullscreen() {
	ebiten.SetFullscreen(!ebiten.IsFullscreen())
	w.forceDraw = true
}

func (w *window) rememberFont() {
	if w.opts.StateDir == "" {
		return
	}
	w.saved = winstate.Load(w.opts.StateDir) // Startart kann das Programm geändert haben
	w.saved.Font = w.fontSize
	if w.fontSize == defaultFontSize {
		w.saved.Font = 0
	}
	_ = winstate.Save(w.opts.StateDir, w.saved)
}

// rememberGeometry: höchstens einmal pro Sekunde prüfen, nur bei Änderung
// schreiben (force: beim Schließen sofort).
func (w *window) rememberGeometry(force bool) {
	if w.opts.StateDir == "" || (!force && time.Since(w.savedAt) < time.Second) {
		return
	}
	w.savedAt = time.Now()
	cur := w.saved
	cur.Full = ebiten.IsFullscreen()
	cur.Max = !cur.Full && ebiten.IsWindowMaximized()
	if !cur.Full && !cur.Max && !ebiten.IsWindowMinimized() {
		cur.W, cur.H = ebiten.WindowSize()
		cur.X, cur.Y = ebiten.WindowPosition()
		cur.HasXY = true
	}
	if cur == w.saved {
		return
	}
	disk := winstate.Load(w.opts.StateDir)
	cur.Start = disk.Start // Startart stellt das Programm ein
	w.saved = cur
	_ = winstate.Save(w.opts.StateDir, cur)
}
