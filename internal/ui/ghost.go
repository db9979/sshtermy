package ui

import (
	"strings"
	"time"
)

// Ghost-Text-Speicher: Wer löscht und dieselbe Zeile neu tippt oder einen
// Befehl wiederholt, bekommt den Vorschlag sofort, ohne neue Anfrage an die
// KI. Gilt nur kurz, weil sich die Lage auf dem Server ändert.

const (
	ghostMemoTTL = 3 * time.Minute
	ghostMemoMax = 64
)

type ghostMemo struct {
	rest string
	at   time.Time
}

// ghostRemember merkt sich die Antwort auf typed (auch eine leere: „nichts
// Passendes“ muss nicht noch einmal gefragt werden).
func (s *session) ghostRemember(typed, rest string, now time.Time) {
	if s.ghostMemo == nil {
		s.ghostMemo = map[string]ghostMemo{}
	}
	if len(s.ghostMemo) >= ghostMemoMax {
		var oldK string
		var oldT time.Time
		for k, v := range s.ghostMemo {
			if oldK == "" || v.at.Before(oldT) {
				oldK, oldT = k, v.at
			}
		}
		delete(s.ghostMemo, oldK)
	}
	s.ghostMemo[typed] = ghostMemo{rest, now}
}

// ghostLookup: Vorschlag aus dem Speicher. Passt eine frühere, kürzere
// Eingabe samt Vorschlag zum jetzt Getippten, gilt deren Rest.
func (s *session) ghostLookup(typed string, now time.Time) (string, bool) {
	if m, ok := s.ghostMemo[typed]; ok && now.Sub(m.at) < ghostMemoTTL {
		return m.rest, true
	}
	for k, m := range s.ghostMemo {
		if m.rest == "" || now.Sub(m.at) >= ghostMemoTTL || !strings.HasPrefix(typed, k) {
			continue
		}
		if full := k + m.rest; strings.HasPrefix(full, typed) && len(full) > len(typed) {
			return full[len(typed):], true
		}
	}
	return "", false
}
