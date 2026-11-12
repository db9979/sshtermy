package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Jeder Hilfe-Abschnitt steht in genau einem Reiter.
func TestHelpTabsCoverAllSections(t *testing.T) {
	seen := map[string]int{}
	for _, tab := range helpTabs {
		for _, title := range tab.secs {
			if _, ok := helpSectionByTitle(title); !ok {
				t.Errorf("Reiter %q: Abschnitt %q gibt es nicht", tab.title, title)
			}
			seen[title]++
		}
	}
	for _, sec := range helpSections {
		if seen[sec.title] != 1 {
			t.Errorf("Abschnitt %q steht in %d Reitern", sec.title, seen[sec.title])
		}
	}
}

// F1 öffnet den Reiter zur Ansicht, ←/→ wechselt, Tippen filtert über alle
// Reiter, Esc leert erst die Suche und schließt dann.
func TestHelpTabsAndFilter(t *testing.T) {
	g := newGold(t, i18n.DE, 140, 40, true)
	g.session("web-01", termOut)
	g.keys("f1")
	if g.a.modal == nil || g.a.modal.kind != mHelp {
		t.Fatal("Hilfe nicht offen")
	}
	if scr := g.screen(); !strings.Contains(scr, "Ghost-Text übernehmen") || strings.Contains(scr, "Broadcast an mehrere Hosts") {
		t.Fatalf("Reiter Terminal erwartet:\n%s", scr)
	}
	g.keys("right")
	if scr := g.screen(); !strings.Contains(scr, "Ziel-Modus") {
		t.Fatalf("Reiter KI erwartet:\n%s", scr)
	}
	g.typ("sync")
	scr := g.screen()
	if !strings.Contains(scr, "Sync zwischen Rechnern") || !strings.Contains(scr, "/ sync") {
		t.Fatalf("Filter über alle Reiter fehlt:\n%s", scr)
	}
	if strings.Contains(scr, "…") && strings.Contains(scr, "Sync zwischen Rech…") {
		t.Fatal("Text abgeschnitten")
	}
	g.keys("esc")
	if g.a.modal == nil || g.a.modal.filter != "" {
		t.Fatal("Esc soll erst die Suche leeren")
	}
	g.keys("esc")
	if g.a.modal != nil {
		t.Fatal("zweites Esc schließt")
	}
}

// Eine Kopfzeile: Logo, Tabs und Sitzungszustand in einer Zeile, kein
// Credit, Produktiv als Marke; unten F-Tasten als Ziffer + Wort.
func TestOneHeaderLineAndFKeyBar(t *testing.T) {
	for _, w := range []int{80, 120, 160} {
		g := newGold(t, i18n.DE, w, 30, true)
		g.session("db-01", strings.ReplaceAll(termOut, "web-01", "db-01"))
		g.session("web-01", termOut)
		lines := strings.Split(g.screen(), "\n")
		head := lines[0]
		if !strings.Contains(head, "sshTermy") || !strings.Contains(head, "web-01") || !strings.Contains(head, "▲ PROD") {
			t.Errorf("w=%d: Kopfzeile unvollständig: %q", w, head)
		}
		if strings.Contains(head, "Dominik") {
			t.Errorf("w=%d: Credit in der Kopfzeile: %q", w, head)
		}
		if !strings.HasPrefix(lines[1], "Linux web-01") {
			t.Errorf("w=%d: Terminal soll direkt unter der Kopfzeile beginnen: %q", w, lines[1])
		}
		foot := lines[len(lines)-2]
		for _, want := range []string{"1Hilfe", "2Ziel", "3KI", "4Tunnel"} {
			if !strings.Contains(foot, want) {
				t.Errorf("w=%d: %q fehlt in der F-Tasten-Leiste: %q", w, want, foot)
			}
		}
		if w >= 120 && !strings.Contains(foot, "10Manager") {
			t.Errorf("w=%d: F10 fehlt: %q", w, foot)
		}
	}
}

func TestFKeyLabel(t *testing.T) {
	for k, want := range map[string]bool{"F2": true, "F10": true, "F7/F8": true, "⇧F9": false, "Strg+F": false, "F": false, "Fx": false} {
		if isFKeyLabel(k) != want {
			t.Errorf("isFKeyLabel(%q) = %v", k, !want)
		}
	}
	a := &App{s: NewStyles(Nacht), keys: newKeymap(nil)}
	got := ansi.Strip(zoneRe.ReplaceAllString(a.fkeyChip("F7/F8", "Tabs"), ""))
	if got != "7/8Tabs " || fkeyChipWidth("F7/F8", "Tabs") != len([]rune(got)) {
		t.Errorf("fkeyChip = %q, Breite %d", got, fkeyChipWidth("F7/F8", "Tabs"))
	}
}

// Manager: Ordner zeigt seinen Inhalt statt einer Tastenliste, KI und Sync
// stehen oben rechts statt bei jeder Verbindung.
func TestManagerDetailsTidy(t *testing.T) {
	g := newGold(t, i18n.DE, 140, 36, true)
	scr := g.screen()
	if strings.Contains(scr, "auflösen, Inhalt bleibt") {
		t.Error("Ordner zeigt noch die Tastenliste")
	}
	if !strings.Contains(strings.Split(scr, "\n")[0], "Sync aus") {
		t.Errorf("Sync gehört in die Kopfzeile: %q", strings.Split(scr, "\n")[0])
	}
	g.keys("down", "down", "down", "down")
	scr = g.screen()
	if strings.Contains(scr, "KI-Copilot") {
		t.Error("KI-Copilot steht noch bei der Verbindung")
	}
	if !strings.Contains(scr, "▲ PRODUKTIV") {
		t.Error("Produktiv-Marke fehlt im Kopf der Details")
	}
}
