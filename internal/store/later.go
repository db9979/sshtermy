package store

import (
	"sync"
	"time"
)

// Gebündeltes Speichern
//
// Jedes Speichern verschlüsselt den ganzen Tresor und wartet auf die Platte
// (bei 1000 Verbindungen rund 10 ms, mit fsync mehr). Kleine Änderungen, die
// nebenbei anfallen (zuletzt benutzt, Ordner auf/zu, Runbook-Zähler), gelten
// deshalb sofort im Speicher und gehen kurz darauf gemeinsam in einem
// Schreibvorgang auf die Platte. Alles andere speichert wie bisher sofort
// und nimmt dabei mit, was noch aussteht. Lock und Flush schreiben
// Ausstehendes vor dem Sperren.

// saveDelay: so lange sammelt saveSoon Änderungen. Variable für Tests.
var saveDelay = 400 * time.Millisecond

type lateSave struct {
	mu    sync.Mutex
	dirty bool
	timer *time.Timer
}

// saveSoon merkt eine kleine Änderung zum Speichern vor.
func (s *Store) saveSoon() {
	s.changed()
	s.late.mu.Lock()
	s.late.dirty = true
	if s.late.timer == nil {
		s.late.timer = time.AfterFunc(saveDelay, s.saveLate)
	}
	s.late.mu.Unlock()
}

// takeLate: steht etwas aus? Setzt den Merker zurück und hält die Uhr an.
func (s *Store) takeLate() bool {
	s.late.mu.Lock()
	defer s.late.mu.Unlock()
	if s.late.timer != nil {
		s.late.timer.Stop()
		s.late.timer = nil
	}
	d := s.late.dirty
	s.late.dirty = false
	return d
}

// saveLate läuft nach saveDelay im Hintergrund. Hat inzwischen ein anderes
// sshTermy gespeichert, lädt Fresh dessen Stand, und die kleine Änderung
// entfällt (sie darf dessen Daten nicht überschreiben).
func (s *Store) saveLate() {
	s.late.mu.Lock()
	s.late.timer = nil
	s.late.mu.Unlock()
	if s.Fresh() {
		return
	}
	if s.takeLate() {
		_ = s.save()
	}
}

// Flush schreibt vorgemerkte Änderungen sofort (vor dem Sperren, in Tests).
func (s *Store) Flush() error {
	if !s.takeLate() {
		return nil
	}
	return s.save()
}
