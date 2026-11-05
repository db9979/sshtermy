// Package i18n übersetzt die Oberfläche. Die deutschen Texte im Code sind
// zugleich die Schlüssel: T("Verbindung") liefert auf Deutsch den Text selbst,
// auf Englisch den Eintrag aus en.go (fehlt er, bleibt der deutsche Text
// stehen). Neue Texte im Code brauchen einen Eintrag in en.go; das prüft
// TestCatalogComplete.
package i18n

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// Sprachen, die die Einstellungen kennen.
const (
	DE = "de"
	EN = "en"
)

var lang atomic.Value // string

func init() { lang.Store(DE) }

// Set wählt die Sprache ("de" oder "en"); alles andere gilt als Deutsch.
func Set(l string) {
	if l != EN {
		l = DE
	}
	lang.Store(l)
}

// Lang ist die aktive Sprache.
func Lang() string { return lang.Load().(string) }

// English meldet, ob die Oberfläche englisch ist.
func English() bool { return Lang() == EN }

// T übersetzt einen deutschen Text in die aktive Sprache.
func T(de string) string {
	if Lang() == EN {
		if s, ok := en[de]; ok {
			return s
		}
		// Reine Tastenbeschriftungen („Strg+S“) brauchen keinen Eintrag.
		return strings.ReplaceAll(de, "Strg", "Ctrl")
	}
	return de
}

// F übersetzt ein Format und füllt es wie fmt.Sprintf.
func F(format string, a ...any) string { return fmt.Sprintf(T(format), a...) }

// Has meldet, ob es für den Text eine englische Fassung gibt.
func Has(de string) bool { _, ok := en[de]; return ok }

// keyWords: deutsche Tastennamen und ihre englische Beschriftung.
var keyWords = []string{
	"Strg", "Ctrl",
	"Entf", "Del",
	"Leertaste", "Space",
	"Bild↑", "PgUp",
	"Bild↓", "PgDn",
	"Pos1", "Home",
	"Ende", "End",
	"Pfeil", "Arrow",
}

// Keys übersetzt Tastennamen in einer Tastenbeschriftung („Strg+S“ → „Ctrl+S“).
func Keys(s string) string {
	if Lang() != EN {
		return s
	}
	for i := 0; i < len(keyWords); i += 2 {
		s = strings.ReplaceAll(s, keyWords[i], keyWords[i+1])
	}
	return s
}

// Error ist ein Fehler mit festem deutschem Text, der beim Anzeigen
// übersetzt wird. Für Fehlervariablen auf Paketebene (errors.Is bleibt
// möglich, weil der Zeiger gleich bleibt).
type Error struct{ de string }

// NewError legt einen übersetzbaren Fehler an.
func NewError(de string) *Error { return &Error{de} }

func (e *Error) Error() string { return T(e.de) }

// Pick wählt zwischen zwei Fassungen, wenn ein deutsches Wort keinen
// eindeutigen Schlüssel ergibt (z. B. „Tunnel“ in Ein- und Mehrzahl).
func Pick(de, en string) string {
	if English() {
		return en
	}
	return de
}
