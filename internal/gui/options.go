package gui

import (
	"image"

	tea "charm.land/bubbletea/v2"
)

// Options für Run.
type Options struct {
	Title    string        // Fenstertitel (Standard „sshTermy“)
	FontSize float64       // Schriftgröße in logischen Pixeln (Standard 14)
	Icon     []image.Image // Fenstersymbol in mehreren Größen (Windows/Linux)
	// CloseMsg geht an das Programm, wenn jemand das Fenster schließt, damit
	// es Sitzungen und Protokolle sauber beendet. nil = sofort beenden.
	CloseMsg tea.Msg
	// StateDir: Ordner für window.json (Vollbild/Größe/Schrift merken,
	// internal/winstate). Leer = nichts merken.
	StateDir string

	attach bool // zweites Fenster eines abgetrennten Tabs
}

// scriptEnv: Steuerskript für Bildschirmfotos; das zweite Fenster hat ein
// eigenes, sonst liefe dasselbe Skript in beiden.
func (o Options) scriptEnv() string {
	if o.attach {
		return "SSHTERMY_GUI_SCRIPT_ATTACH"
	}
	return "SSHTERMY_GUI_SCRIPT"
}
