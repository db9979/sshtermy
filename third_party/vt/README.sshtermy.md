# charmbracelet/x/vt v0.1.0 mit Anpassungen für sshTermy

Kopie von github.com/charmbracelet/x/vt v0.1.0 (MIT, siehe LICENSE), eingebunden
über `replace` in der go.mod von sshTermy. Geändert ist nur, was mit `sshTermy:`
markiert ist:

- `screen.go`: `rotateLines` – Zeilen löschen/einfügen bei voller Breite (jeder
  Zeilenvorschub in der letzten Zeile) verschiebt nur die Zeilen statt jede Zelle
  des Bildschirms einzeln zu kopieren. Durchlaufende Ausgabe wird so rund 3× schneller.
  Test: `rotate_test.go` vergleicht mit dem zellweisen Weg.
- `scrollback.go`, `emulator.go`: `SetScrollbackHook` – Zeilen, die oben aus dem
  Bild laufen, gehen an sshTermy (`internal/termemu/scrollback.go`), das sie kompakt
  speichert (8 statt 112 Byte pro Zeichen).

Beim Wechsel auf eine neuere vt-Version: Dateien ersetzen, die markierten Stellen
wieder einbauen, `go test ./...` hier und im Hauptmodul laufen lassen.
