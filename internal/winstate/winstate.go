// Package winstate merkt sich, wie das eigene Fenster (internal/gui) startet:
// Vollbild, maximiert oder wie zuletzt, dazu Größe, Position und
// Schriftgröße. Die Datei window.json liegt unverschlüsselt im
// Konfigurationsordner, weil das Fenster vor dem Entsperren aufgeht.
package winstate

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Startarten.
const (
	StartFull = "full" // Vollbild (Standard)
	StartMax  = "max"  // maximiert
	StartLast = "last" // wie beim letzten Schließen
)

const fileName = "window.json"

// State: gespeicherter Zustand des Fensters.
type State struct {
	Start string  `json:"start,omitempty"` // "" = StartFull
	Font  float64 `json:"font,omitempty"`  // Schriftgröße (logische Pixel), 0 = Standard
	W     int     `json:"w,omitempty"`     // Größe im normalen Zustand
	H     int     `json:"h,omitempty"`
	X     int     `json:"x,omitempty"`
	Y     int     `json:"y,omitempty"`
	HasXY bool    `json:"has_xy,omitempty"`
	Full  bool    `json:"full,omitempty"` // zuletzt im Vollbild
	Max   bool    `json:"max,omitempty"`  // zuletzt maximiert
}

// Mode: Startart mit Standardwert.
func (s State) Mode() string {
	switch s.Start {
	case StartMax, StartLast:
		return s.Start
	}
	return StartFull
}

// Load liest den Zustand; fehlt die Datei, gilt der Standard (Vollbild).
func Load(dir string) State {
	var s State
	if dir == "" {
		return s
	}
	b, err := os.ReadFile(filepath.Join(dir, fileName))
	if err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

// Save schreibt den Zustand.
func Save(dir string, s State) error {
	if dir == "" {
		return nil
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, fileName+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, fileName))
}

// SetStart ändert nur die Startart.
func SetStart(dir, mode string) error {
	s := Load(dir)
	s.Start = mode
	return Save(dir, s)
}
