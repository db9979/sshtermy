package gui

import (
	"image"

	tea "charm.land/bubbletea/v2"
)

// Options für Run.
type Options struct {
	Title    string      // Fenstertitel (Standard „sshTermy“)
	FontSize float64     // Schriftgröße in logischen Pixeln (Standard 14)
	Icon     image.Image // Fenstersymbol (Windows/Linux)
	// CloseMsg geht an das Programm, wenn jemand das Fenster schließt, damit
	// es Sitzungen und Protokolle sauber beendet. nil = sofort beenden.
	CloseMsg tea.Msg
}
