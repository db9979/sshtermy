# charmbracelet/x/ansi v0.11.9 mit Anpassungen für sshTermy

Kopie von github.com/charmbracelet/x/ansi v0.11.9 (MIT, siehe LICENSE), eingebunden
über `replace` in der go.mod von sshTermy. Geändert ist nur, was mit `sshTermy:`
markiert ist:

- `parser_decode.go`: `FirstGraphemeCluster` – druckbares ASCII, dem kein
  Nicht-ASCII-Byte folgt, ist sofort ein Cluster der Breite 1. lipgloss misst mit
  `StringWidth` jede Zeile jedes Bildes; die Unicode-Segmentierung je Zeichen war
  der größte Einzelposten beim Zeichnen.

Beim Wechsel auf eine neuere Version: Dateien ersetzen, die markierte Stelle wieder
einbauen, `go test ./...` hier und im Hauptmodul laufen lassen.
