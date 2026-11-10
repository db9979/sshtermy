package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/version"
)

// Hilfe (F1): Tastenkürzel, nach Bereichen gruppiert. Je nach Fensterbreite
// in ein bis drei Spalten; der Bereich, in dem man gerade ist, wird markiert.
// Passt sie nicht in die Höhe, lässt sie sich mit ↑↓ blättern.

type helpCtx int

const (
	hcNone helpCtx = iota
	hcManager
	hcTerminal
	hcGoal
	hcAsk
	hcTunnel
	hcPalette
	hcFiles
	hcSettings
	hcSearch
	hcLogs
	hcBroadcast
	hcScan
	hcHost
	hcCompanion
	hcLogScan
)

type helpSection struct {
	ctx   helpCtx
	icon  string
	title string
	key   string // Taste, die den Bereich öffnet
	ai    bool   // KI-Bereich: violett statt blau
	rows  [][2]string
}

var helpSections = []helpSection{
	{hcNone, "◆", "Überall", "", false, [][2]string{
		{"Strg+Space", "Befehlspalette"},
		{"F1", "diese Hilfe"},
		{"⇧F9", "Tastenzeile erklären"},
		{"F12", "Über & Update"},
		{"F12 w", "Was ist neu"},
		{"F12 l", "Sprache: Deutsch/English"},
		{"Strg+Q", "beenden (fragt bei offenen Verbindungen)"},
		{"F11", "Vollbild an/aus (eigenes Fenster)"},
	}},
	{hcManager, "≡", "Verbindungs-Manager", "F10", false, [][2]string{
		{"⏎", "verbinden"},
		{"n", "neue Verbindung"},
		{"e", "bearbeiten"},
		{"d", "löschen"},
		{"Strg+Z", "Löschen rückgängig (10 s)"},
		{"Strg+T", "im Formular: Verbindung testen"},
		{"/", "suchen (auch #tag)"},
		{"o", "neuer Ordner (A / B verschachtelt)"},
		{"r", "Ordner bearbeiten"},
		{"v", "in Ordner verschieben"},
		{"Shift+↑", "nach oben schieben (↓ runter)"},
		{"← / →", "Ordner zu-/aufklappen"},
		{"- / +", "alle Ordner zu/auf"},
		{"#", "nach Tag filtern"},
		{"t", "Tunnel anlegen"},
		{"c", "SSH-Schlüssel erzeugen / übertragen"},
		{"q", "Schnellbefehle"},
		{"m", "Broadcast an mehrere Hosts"},
		{"f", "Netzwerk-Scan"},
		{"z", "Zeitpläne"},
		{"s", "KI-Einstellungen"},
		{"y", "Sync zwischen Rechnern"},
		{"i", "Verbindungen importieren"},
		{"a", "Farbschema wählen"},
		{"b", "Tastenkürzel anpassen"},
		{"l", "Sitzungsprotokolle"},
		{"F10", "zurück zum Terminal"},
	}},
	{hcSettings, "◈", "KI-Einstellungen", "s", true, [][2]string{
		{"←/→", "Anbieter: Spark, Ollama, OpenAI, Claude"},
		{"Strg+T", "Endpunkt testen"},
		{"Strg+L", "Modell wählen"},
		{"Ersatz ⏎", "Ersatz-Anbieter einrichten"},
		{"Strg+S", "speichern"},
	}},
	{hcTerminal, "▸", "Terminal", "", false, [][2]string{
		{"Tab / →", "Ghost-Text übernehmen"},
		{"Alt+1…4", "Folgebefehl einfügen"},
		{"Alt+0", "Fehler: Warum? & Fix"},
		{"⇧F1…F8", "Schnellbefehl ausführen"},
		{"F9", "Schnellbefehle (Liste)"},
		{"F9 → a", "häufigen Befehl übernehmen"},
		{"F2", "Ziel-Modus (Agent)"},
		{"F3", "KI fragen"},
		{"Alt+K", "KI-Begleiter: Aus/Leise/Gesprächig/Coach"},
		{"Alt+J", "Begleiter-Leiste"},
		{"F4", "Tunnel"},
		{"F5", "Dateien (SFTP)"},
		{"Strg+T", "Aufteilung wählen (1…7)"},
		{"Strg+Pfeil", "Bereich wechseln (geteilt)"},
		{"Alt+⇧Pfeil", "Trennlinie verschieben (Maus: ziehen)"},
		{"Alt+Z", "Bereich zoomen / zurück"},
		{"Titel ziehen", "Bereiche tauschen"},
		{"⇧F10", "Tab in eigenes Fenster (Sitzung bleibt offen)"},
		{"Strg+F", "im Verlauf suchen"},
		{"F6", "Host-Übersicht"},
		{"Alt+E", "Log-Blick: Auffälligkeiten"},
		{"F7 / F8", "Tab wechseln"},
		{"Alt+T", "alle Tabs (Liste)"},
		{"Alt+N", "neuer Tab"},
		{"Alt+L", "lokale Shell (neuer Tab)"},
		{"Alt+W", "Tab schließen"},
		{"F10", "zum Manager"},
		{"Abbruch", "verbindet neu (⏎ / Esc)"},
		{"getrennt: e / k", "Verbindung bearbeiten / neuen Host-Key prüfen"},
	}},
	{hcGoal, "◎", "Ziel-Modus", "F2", true, [][2]string{
		{"⏎", "Ziel starten / ausführen"},
		{"⇧⏎ / Strg+J", "neue Zeile im Ziel"},
		{"i", "nur einfügen (Dry-Run)"},
		{"e", "bearbeiten"},
		{"s", "überspringen"},
		{"r", "neu vorschlagen"},
		{"x", "Ziel beenden"},
		{"↑ ↓", "Runbook wählen"},
		{"Strg+S", "als Runbook speichern"},
		{"Strg+D", "Runbook löschen"},
		{"Tab", "Schritt für Schritt oder Agent (Stufe)"},
		{"Esc", "Agent anhalten"},
		{"?", "Agent anweisen"},
		{"v", "Skript des Agenten ansehen"},
		{"r", "Skripte des Agenten löschen (am Ende)"},
		{"c / l", "Ergebnis kopieren / ganz lesen"},
		{"Bild↑ Bild↓", "im Panel blättern (auch Mausrad)"},
		{"Leertaste", "Plan: Schritt streichen"},
		{"?", "Plan: neu planen lassen"},
	}},
	{hcAsk, "◆", "KI fragen", "F3", true, [][2]string{
		{"⏎", "Frage senden"},
		{"⇧⏎ / Strg+J", "neue Zeile"},
		{"1…9", "Befehl einfügen"},
		{"↑ ↓", "Befehlskarte wählen"},
		{"a", "Befehl ausführen (rote zweimal)"},
		{"c", "Befehl kopieren"},
		{"z", "Erklärung ein-/ausklappen"},
		{"Esc / Strg+C", "Antwort abbrechen"},
		{"n", "neue Frage (ohne Verlauf)"},
		{"f", "Folgefrage (Verlauf geht mit)"},
	}},
	{hcCompanion, "◇", "KI-Begleiter", "Alt+J", true, [][2]string{
		{"Alt+K", "Stufe: Aus/Leise/Gesprächig/Coach"},
		{"Alt+1…4", "Vorschlag einfügen"},
		{"⏎ / a", "einfügen / ausführen"},
		{"m", "Thema stumm"},
		{"l", "Tatsache merken"},
		{"p / g", "Absicht: Plan / Agent übernimmt"},
		{"r", "Rückblick (merken, Runbook)"},
		{"?", "zuletzt gesendet"},
	}},
	{hcTunnel, "↔", "Tunnel", "F4", false, [][2]string{
		{"⏎ / Klick", "starten / stoppen"},
		{"a", "mit Verbindung starten"},
		{"o", "im Browser öffnen"},
		{"n", "neuer Tunnel"},
		{"e", "bearbeiten"},
		{"d", "löschen"},
	}},
	{hcHost, "▣", "Host-Übersicht", "F6", false, [][2]string{
		{"r / ⏎", "abfragen"},
		{"b", "Switch-Config sichern"},
		{"c", "Container (Docker, Podman, k8s)"},
		{"l", "Log-Blick"},
		{"↑ ↓", "Sicherung wählen"},
		{"d", "Diff zur Vorversion"},
		{"v", "Sicherung ansehen"},
		{"g", "Grenzwerte für Warnungen"},
		{"t", "Graphen: 5 min / 1 h"},
		{"z", "Graphen groß"},
		{"x", "Abfrage abbrechen"},
	}},
	{hcFiles, "▤", "Dateien", "F5", false, [][2]string{
		{"Tab", "Seite wechseln"},
		{"⏎ / →", "Ordner öffnen"},
		{"← / Bksp", "eine Ebene hoch"},
		{"c / F6", "kopieren (hoch/runter)"},
		{"e", "im eigenen Editor öffnen"},
		{"Space / a", "markieren / alle"},
		{"Ziehen", "auf die andere Seite kopieren"},
		{"Ablegen", "Dateien aus Explorer/Finder hochladen"},
		{"n", "neuer Ordner"},
		{"d", "löschen"},
		{".", "versteckte Dateien"},
		{"x", "Übertragung abbrechen"},
	}},
	{hcSearch, "/", "Suche im Verlauf", "Strg+F", false, [][2]string{
		{"tippen", "suchen (Groß = genau)"},
		{"⏎ / ↑", "nächster (älter)"},
		{"⇧⏎ / ↓", "zurück (neuer)"},
		{"Strg+E", "nur Fehlerzeilen"},
		{"Tab", "Fehlerliste / Verlauf"},
		{"Esc", "schließen"},
	}},
	{hcLogScan, "▲", "Log-Blick", "Alt+E", false, [][2]string{
		{"⏎", "Zeilen des Musters"},
		{"1…4", "Prüfbefehl einfügen"},
		{"k", "KI erklärt das Muster"},
		{"b", "KI-Lagebericht"},
		{"x / X", "ausblenden: Host / Ordner"},
		{"n", "Was ist neu? (u zurückholen)"},
		{"t", "Zeitleiste (←→, ⏎, k)"},
		{"z", "Zeitraum 1 h … 7 Tage"},
		{"r / c", "neu lesen / kopieren"},
	}},
	{hcLogs, "≡", "Sitzungsprotokolle", "l", false, [][2]string{
		{"/", "suchen (auch Ausgabe)"},
		{"⏎ / → ←", "auf-/zuklappen"},
		{"Tab", "nach Zeit / nach Host"},
		{"f", "Filter: heute · 7 Tage · Fehler"},
		{"s", "Hosts sortieren"},
		{"z", "KI-Zusammenfassung"},
		{"Z", "alle Treffer zusammenfassen"},
		{"b", "Änderungsbericht"},
		{"x", "Export: md · txt · cast"},
		{"d", "löschen"},
		{"a", "Mitschreiben an/aus"},
		{"␣ Befehl", "nicht protokollieren"},
	}},
	{hcBroadcast, "»", "Broadcast", "m", false, [][2]string{
		{"Space / g / a", "Host / Ordner / alle"},
		{"Tab", "Hosts · Quelle · Befehl"},
		{"⇧⏎", "weiterer Schritt"},
		{"Strg+S", "prüfen, dann ⏎"},
		{"← / →", "Host-Spalte wählen"},
		{"↑ ↓ Bild↑↓", "Ausgabe blättern"},
		{"z / c", "groß / kopieren"},
		{"r / e / x", "nochmal / ändern / Stopp"},
		{"{{name}}", "Variable, wird abgefragt"},
		{"{{host}}", "je Host eingesetzt"},
		{"Strg+R", "als Runbook speichern"},
		{"e / Strg+D", "ändern / löschen"},
		{"k / a", "KI prüft / übernehmen"},
	}},
	{hcScan, "/", "Netzwerk-Scan", "f", false, [][2]string{
		{"Strg+S", "Scan starten"},
		{"Space / a", "Fund / alle wählen"},
		{"⏎", "als Verbindungen anlegen"},
		{"x", "Scan stoppen"},
		{"r", "neu scannen"},
	}},
	{hcPalette, "❯", "Befehlspalette", "Strg+Space", false, [][2]string{
		{"tippen", "filtern / Ziel / Frage"},
		{"user@host", "schnell verbinden"},
		{"⏎", "ausführen"},
		{"Tab", "KI fragen"},
		{"◈ Einstellungen ›", "Schalter, Aussehen, Sprache"},
		{"Esc / ← (Untermenü)", "zurück"},
		{"↓ „umschreiben“", "Befehl fürs System anpassen"},
		{"„log“", "KI wertet Ausgabe aus"},
		{"„farb“", "Farbregeln bearbeiten"},
		{"MAC / IP", "Switch-Port finden"},
		{"„topo“", "Netzwerk-Topologie"},
		{"„teilen“ / Link", "live teilen / zusehen"},
		{"Esc", "schließen"},
	}},
	{hcNone, "i", "Systemprofil", "", false, [][2]string{
		{"automatisch", "System + Befehlssatz erkennen"},
		{"e (Manager)", "System, Befehlssatz, KI-Notizen"},
		{"„% Invalid input“", "Befehl wird gemerkt + gemieden"},
		{"KI-Befehl korrigiert", "Korrektur wird gemerkt"},
	}},
	{hcNone, "▲", "Gefahren & Aufnahme", "", false, [][2]string{
		{"Produktiv", "riskanter Befehl: ⏎ halten / j"},
		{"Palette", "„Gefahren-Check“"},
		{"Palette", "„Aufnahme starten“"},
		{"Palette", "„Verlauf als Text“"},
	}},
	{hcNone, "◐", "Aussehen & Tasten", "", false, [][2]string{
		{"a", "Dunkel · Hell · Kontrast · Kontrast hell"},
		{"b", "Tastenkürzel belegen"},
		{"Strg +/−", "Schrift (Terminalfenster)"},
		{"Palette", "„Startanimation aus“"},
		{"Palette", "„Bewegung“: voll · dezent · aus"},
	}},
	{hcNone, "◦", "Maus", "", false, [][2]string{
		{"Klick", "Tabs, Listen, Felder"},
		{"Doppelklick", "öffnen / Wort"},
		{"Ziehen", "markieren + kopieren"},
		{"Rechtsklick", "einfügen"},
		{"Rechtsklick Link", "Menü: Verbinden, Ping"},
		{"Rechtsklick Tab", "Menü: umbenennen, schließen …"},
		{"Mittelklick Tab", "Tab schließen"},
		{"Maus auf Tab", "Karte: Ziel, Dauer, Latenz"},
		{"Strg+Klick", "URL im Browser"},
		{"Rad", "im Verlauf blättern"},
		{"Shift+Ziehen", "markieren in htop/vim"},
		{"Palette", "„Maus aus“"},
	}},
}

const (
	helpKeyW = 13 // Breite der Tastenspalte
	helpColW = 35 // Breite einer Spalte
	helpGap  = 2
)

func (a *App) helpModal() *modal {
	return &modal{kind: mHelp, title: i18n.T("Tastenkürzel"), ctx: a.helpContext()}
}

func (a *App) helpContext() helpCtx {
	if a.pal != nil {
		return hcPalette
	}
	switch a.scr {
	case scrManager:
		return hcManager
	case scrSettings:
		return hcSettings
	case scrLogs:
		return hcLogs
	case scrLogScan:
		return hcLogScan
	case scrBroadcast:
		return hcBroadcast
	case scrScan:
		return hcScan
	case scrTerminal:
		if s := a.current(); s != nil && s.files.open {
			return hcFiles
		}
		if s := a.current(); s != nil && s.search.open && !s.panelFocus {
			return hcSearch
		}
		if s := a.current(); s != nil && s.panelFocus {
			switch s.panel {
			case panelGoal:
				return hcGoal
			case panelAsk:
				return hcAsk
			case panelTunnels:
				return hcTunnel
			case panelHost:
				return hcHost
			case panelCompanion:
				return hcCompanion
			}
		}
		return hcTerminal
	}
	return hcNone
}

func (s Styles) helpSection(sec helpSection, ctx helpCtx) []string {
	head := s.Accent
	if sec.ai {
		head = lipgloss.NewStyle().Foreground(s.T.AI).Bold(true)
	}
	title := head.Render(sec.icon + " " + i18n.T(sec.title))
	if sec.key != "" {
		title += " " + s.Muted.Render(relabelDefault(sec.key, sec.ctx))
	}
	if sec.ctx != hcNone && sec.ctx == ctx {
		title += " " + s.Badge.Render(i18n.T("hier"))
	}
	lines := []string{title}
	// gleiche Reihenfolge wie in der Fußzeile: Palette, F1 … F12, ⇧F, Rest
	rows := make([][2]string, len(sec.rows))
	for i, r := range sec.rows {
		rows[i] = [2]string{relabelDefault(r[0], sec.ctx), r[1]}
	}
	for _, r := range sortFKeys(rows, activeKeys.label(actPalette)) {
		k := s.Key.Render(r[0])
		k += strings.Repeat(" ", max(0, helpKeyW-lipgloss.Width(k)))
		lines = append(lines, k+s.Text.Render(ansi.Truncate(i18n.T(r[1]), helpColW-helpKeyW, "…")))
	}
	return lines
}

// splitColumns verteilt die Bereiche in Reihenfolge auf n Spalten, so dass
// die höchste Spalte möglichst niedrig ist.
func splitColumns(h []int, n int) [][]int {
	best, bestMax := [][]int(nil), 1<<30
	var rec func(start, col int, acc [][]int, curMax int)
	rec = func(start, col int, acc [][]int, curMax int) {
		if curMax >= bestMax {
			return
		}
		if col == n-1 || start == len(h) {
			if start < len(h) {
				idx, sum := []int{}, 0
				for i := start; i < len(h); i++ {
					idx = append(idx, i)
					sum += h[i] + 1
				}
				acc = append(acc, idx)
				curMax = max(curMax, sum-1)
			}
			if curMax < bestMax {
				bestMax = curMax
				best = append([][]int(nil), acc...)
			}
			return
		}
		sum := 0
		for end := start; end < len(h); end++ {
			sum += h[end] + 1
			idx := make([]int, 0, end-start+1)
			for i := start; i <= end; i++ {
				idx = append(idx, i)
			}
			rec(end+1, col+1, append(acc, idx), max(curMax, sum-1))
		}
	}
	rec(0, 0, nil, 0)
	return best
}

func (m *modal) helpView(s Styles, w, h int) string {
	avail := dialog{maxW: 200}.innerMax(w)
	n := max(1, min(3, (avail+helpGap)/(helpColW+helpGap)))
	blocks := make([][]string, len(helpSections))
	heights := make([]int, len(helpSections))
	for i, sec := range helpSections {
		blocks[i] = s.helpSection(sec, m.ctx)
		heights[i] = len(blocks[i])
	}
	var cols []string
	for _, idx := range splitColumns(heights, n) {
		var lines []string
		for j, i := range idx {
			if j > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, blocks[i]...)
		}
		col := lipgloss.NewStyle().Width(helpColW).Render(strings.Join(lines, "\n"))
		if len(cols) > 0 {
			cols = append(cols, strings.Repeat(" ", helpGap))
		}
		cols = append(cols, col)
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	cw := min(avail, lipgloss.Width(body))

	note := s.Muted.Render(i18n.T("Esc führt aus jedem Panel zurück ins Terminal."))
	warn := s.ChipDanger.Render(i18n.T("ROT")) + " " + s.Text.Render(i18n.T("Gefährliche Befehle brauchen zweimal ⏎ bzw. Alt+N. ")) +
		s.AI.Render(i18n.T("KI-Vorschläge")) + s.Text.Render(i18n.T(" werden nur eingefügt, nie ausgeführt."))
	warn = lipgloss.NewStyle().Width(cw).Render(warn)
	note = lipgloss.NewStyle().Width(cw).Render(note)

	// Höhe begrenzen: Fensterrand (2), Rahmen und Abstand (4), Warnung und
	// Hinweis mit Leerzeile davor, Tastenzeile mit Leerzeile davor.
	bodyLines := strings.Split(body, "\n")
	foot := "\n\n" + warn + "\n" + note
	maxBody := max(3, h-2-4-(1+lipgloss.Height(warn)+lipgloss.Height(note))-2)
	if maxBody < 10 {
		// wenige Zeilen (z. B. uConsole mit großer Schrift): Warnung und
		// Hinweis blättern mit, damit mehr Platz für die Tasten bleibt
		bodyLines = append(bodyLines, strings.Split(foot[1:], "\n")...)
		foot, maxBody = "", max(3, h-2-4-2)
	}
	keys := [][2]string{{"Esc", i18n.T("schließen")}}
	info := ""
	if total := len(bodyLines); total > maxBody {
		m.scroll = max(0, min(m.scroll, total-maxBody))
		bodyLines = bodyLines[m.scroll : m.scroll+maxBody]
		keys = append([][2]string{{"↑↓", i18n.T("blättern")}}, keys...)
		if m.scroll+maxBody < total {
			info = i18n.T("▾ mehr")
		}
	} else {
		m.scroll = 0
	}
	return s.dialog(dialog{icon: "?", title: i18n.T("Tastenkürzel"), sub: version.Short(),
		body: strings.Join(bodyLines, "\n") + foot, info: info, keys: keys, width: cw, maxW: 200}, w)
}

func (m *modal) helpKey(key string) (closeIt bool) {
	switch key {
	case "up", "k":
		m.scroll = max(0, m.scroll-1)
	case "down", "j":
		m.scroll++
	case "pgup":
		m.scroll = max(0, m.scroll-10)
	case "pgdown", "space":
		m.scroll += 10
	default:
		return true
	}
	return false
}
