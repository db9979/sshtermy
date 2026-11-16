package ui

// Zurückblättern im Verlauf mit der Tastatur: ⇧Bild↑/↓ seitenweise,
// ⇧Pos1 an den Anfang, ⇧Ende zurück nach unten. Vollbild-Programme
// (less, vim, top) bekommen die Tasten weiter selbst (siehe terminal.go).

func (a *App) scrollKey(s *session, act action) {
	sb := s.term.ScrollbackLen()
	_, h := s.term.Size()
	page := max(h-1, 1)
	switch act {
	case actScrollUp:
		s.scroll = clamp(s.scroll+page, 0, sb)
	case actScrollDown:
		s.scroll = clamp(s.scroll-page, 0, sb)
	case actScrollTop:
		s.scroll = sb
	case actScrollBottom:
		s.scroll = 0
	}
	s.sbSeen = sb
	if s.scroll == 0 {
		s.sel = nil
	}
}
