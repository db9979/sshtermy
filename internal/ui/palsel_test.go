package ui

import (
	"testing"
	"time"
)

// Der Auswahlbalken folgt der Pfeiltaste sofort: kein Einblenden, kein
// Nachleuchten, also auch keine schnellen Zwischenbilder nur dafür.
func TestPaletteSelectionInstant(t *testing.T) {
	a := newPaletteTestApp(t)
	a.intro, a.scr = nil, scrManager
	a.pal.menu = "settings"
	a.View()
	p := a.pal
	p.mo.queryAt = time.Now().Add(-time.Hour) // Öffnen ist längst vorbei
	before := a.View().Content
	p.sel++
	after := a.View().Content
	if before == after {
		t.Fatal("Auswahl hat sich im Bild nicht bewegt")
	}
	if p.animating(time.Now()) {
		t.Fatal("Auswahlwechsel braucht Zwischenbilder, der Balken zieht nach")
	}
	// Bild direkt danach ist schon das Endbild.
	time.Sleep(300 * time.Millisecond)
	if again := a.View().Content; again != after {
		t.Fatal("Balken verändert sich nach dem Wechsel noch")
	}
}

// Der abgedunkelte Hintergrund unter dem Modal wird nur einmal berechnet.
func TestOverlayBackgroundMemo(t *testing.T) {
	a := newPaletteTestApp(t)
	a.intro, a.scr = nil, scrManager
	a.View()
	first := a.ovMemo.out
	if first == "" {
		t.Fatal("kein Hintergrund gemerkt")
	}
	a.pal.sel++
	a.View()
	if a.ovMemo.out != first {
		t.Fatal("Hintergrund wurde neu berechnet, obwohl er gleich blieb")
	}
}
