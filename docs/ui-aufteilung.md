# internal/ui aufteilen

`internal/ui` ist mit über 300 Dateien und gut 80.000 Zeilen ein einziges Paket. Das macht Änderungen
riskanter (alles sieht alles) und die Tests langsam (jede Änderung baut und testet das ganze Paket).

## Stand

Erster Schritt (2026-11-20): zwei Teile ohne Bezug zu `App`/`session` sind eigene Pakete:

| Paket | Inhalt |
|---|---|
| `internal/ui/clickzone` | klickbare Bereiche im fertigen Bild (`Map`, `Action`, `Rect`) |
| `internal/ui/dropped` | ins Fenster gezogene Dateien erkennen (`Paths`, `Split`) |
| `internal/ui/theme` | Farben der Themes (schon vorher) |

## Vorgehen für weitere Teile

1. Datei probeweise in ein leeres Paket kopieren und bauen: die Meldungen `undefined: …` zeigen, was sie
   noch aus `ui` braucht. Gut geeignet sind Dateien ohne `*App`, `*session`, `modal`, `Styles`.
2. Mit `gopls rename -w datei.go:Zeile:Spalte NeuerName` erst alle Namen in `ui` auf eindeutige
   Zwischennamen bringen (z. B. `CzMap`), Felder und Methoden gleich exportiert benennen.
3. Datei mit `git mv` verschieben, Paketzeile ändern, Zwischennamen im neuen Paket kürzen (`CzMap` → `Map`)
   und in `ui` durch den qualifizierten Namen ersetzen (`clickzone.Map`). Weil die Zwischennamen eindeutig
   sind, ist das ein sicheres Suchen/Ersetzen.
4. `go build ./... && go test ./internal/ui/...`

## Nächste Kandidaten

- `logtree.go` (Baum der Sitzungsprotokolle, braucht nur `plural`) → `ui/logtree`
- `stamp.go` + `fillbg.go` (Overlays zusammensetzen) → `ui/canvas`
- `graph.go` (Kurven, braucht `mixColor`) zusammen mit den Farbhilfen
- danach Bereiche mit eigenem Zustand (Dateiansicht F5, Teilen, KI-Begleiter) hinter kleinen Schnittstellen
