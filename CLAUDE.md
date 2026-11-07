# Hinweise für Claude

## Doku immer mitziehen (Dominik, 2026-11-07)

Bei jeder sichtbaren Änderung (neue Funktion, neue oder geänderte Taste, neuer Modus) im selben PR auch aktualisieren:

- `README.md` (englisch) und `README.de.md` (deutsch): Funktionsliste und Tastentabelle „Usage“/„Bedienung“
- Webseite `site/index.html` (englisch) und `site/de/index.html`: Funktionskarten, Tastentabelle
- Kurzreferenz zum Ausdrucken: Inhalt in `tools/handout/content.mjs` (DE + EN), dann `make handout`
  (schreibt `site/handout/*.pdf` und `tools/handout/keys.sha256`). Layout bleibt druckerfreundlich: weißer Hintergrund.

`TestHandoutKeysCurrent` (internal/ui/handout_test.go) schlägt fehl, sobald sich `actionDefs` oder `helpSections`
ändern und die Kurzreferenz nicht neu erzeugt wurde.
