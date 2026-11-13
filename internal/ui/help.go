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
	hcVorgang
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
		{"Leertaste", "markieren (dann e, v, d für alle)"},
		{"Rechtsklick", "Menü zu Verbindung oder Ordner"},
		{"*", "alle sichtbaren markieren"},
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
		{"Alt+V", "Vorgang über mehrere Fenster (KI sieht alle)"},
		{"F4", "Tunnel"},
		{"F5", "Dateien (SFTP)"},
		{"Strg+T", "Aufteilung wählen (1…7)"},
		{"Strg+Pfeil", "Bereich wechseln (geteilt)"},
		{"Alt+⇧Pfeil", "Trennlinie verschieben (Maus: ziehen)"},
		{"Alt+Z", "Bereich zoomen / zurück"},
		{"Titel ziehen", "Bereiche tauschen"},
		{"⇧F10", "Tab in eigenes Fenster (Sitzung bleibt offen)"},
		{"Mausrad / ⇧Bild↑ Bild↓", "im Verlauf zurückblättern (Esc: unten)"},
		{"⇧Pos1 / ⇧Ende", "Anfang / Ende des Verlaufs"},
		{"Strg+F", "im Verlauf suchen"},
		{"Alt+S", "in allen Tabs suchen"},
		{"Alt+O", "Befehle des Tabs: Ausgabe kopieren, an KI"},
		{"Alt+Bild↑ Bild↓", "von Befehl zu Befehl springen"},
		{"F6", "Host-Übersicht"},
		{"Alt+E", "Log-Blick: Auffälligkeiten"},
		{"F7 / F8", "Tab wechseln"},
		{"Alt+T", "alle Tabs (Liste)"},
		{"Alt+N", "neuer Tab: Verbindung im Manager wählen"},
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
		{"s / ⇧S", "überspringen / mit Grund"},
		{"?", "KI anweisen (gilt ab dem nächsten Schritt)"},
		{"r", "neu vorschlagen"},
		{"x", "Ziel beenden"},
		{"j n q Leertaste", "Befehl fragt nach: Antwort-Knöpfe"},
		{"f", "Folgeziel (kennt die Schritte davor)"},
		{"u", "Änderungen des Ziels zurückholen"},
		{"⏎ nach Abbruch", "Ziel fortsetzen"},
		{"↑ ↓", "Runbook oder früheres Ziel wählen"},
		{"Strg+S", "als Runbook speichern"},
		{"Strg+D", "Runbook / Verlaufseintrag löschen"},
		{"Tab", "Schritt für Schritt oder Agent (Stufe)"},
		{"Esc", "Agent anhalten"},
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
		{"u", "nach Fehler: Fix in die Karte übernehmen"},
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
	{hcVorgang, "◆", "Vorgang über mehrere Fenster", "Alt+V", true, [][2]string{
		{"Leertaste / ← →", "Fenster wählen / was die KI sieht"},
		{"⏎", "Vorgang starten / Frage senden"},
		{"1…9", "Schritt im Fenster vortippen"},
		{"z", "Fahrplan (Schritt für Schritt)"},
		{"p", "Plan in Bahnen (je Fenster eine Spalte)"},
		{"Tab", "Agent: Lesendes läuft allein"},
		{"⏎ / s / w", "weiter / überspringen / nochmal"},
		{"o", "fehlendes System öffnen"},
		{"r", "Rückblick (Change-Protokoll, Runbook)"},
		{"Strg+E / Strg+D", "Fenster ändern / Vorgang beenden"},
		{"Esc", "zum Terminal"},
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
		{"⏎ / 2×Klick", "öffnen, Datei ansehen"},
		{"← / Bksp", "eine Ebene hoch"},
		{"c / F6", "kopieren (hoch/runter)"},
		{"e", "im eigenen Editor öffnen"},
		{"o", "mit Standardprogramm"},
		{"Space / a", "markieren / alle"},
		{"Strg+Klick", "einzeln markieren"},
		{"⇧+Klick", "Bereich markieren"},
		{"Ziehen", "auf die andere Seite kopieren"},
		{"Ablegen", "Dateien aus Explorer/Finder hochladen"},
		{"Rechtsklick", "Menü (Rechte, cd …)"},
		{"F2", "umbenennen"},
		{"y", "Pfad kopieren"},
		{"n / d", "neuer Ordner / löschen"},
		{"s / v", "sortieren / Vorschau"},
		{".", "versteckte Dateien"},
		{"/ · f", "suchen / mitlesen"},
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
		{"w", "Logwache: Meldungen"},
		{"s", "Switch: Syslog an sshTermy senden"},
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
		{"p", "Probelauf an/aus"},
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
		{"Gefahr", "riskanter Befehl: ⏎ halten / j"},
		{"e (Manager)", "„Schutz“ aus: nur Test-/Laborsysteme"},
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
		{"Ziehen", "markieren + kopieren, auch im Panel"},
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
		if a.vg != nil && a.vg.focus {
			return hcVorgang
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

// Reiter der Hilfe: jeder Abschnitt steht in genau einem Reiter. Vorn ist
// der Reiter mit dem Bereich, aus dem F1 gedrückt wurde.
type helpTabDef struct {
	title string
	secs  []string // Titel der Abschnitte (helpSections)
}

var helpTabs = []helpTabDef{
	{"Terminal", []string{"Terminal", "Suche im Verlauf", "Tunnel", "Host-Übersicht", "Dateien", "Gefahren & Aufnahme", "Systemprofil"}},
	{"KI", []string{"Ziel-Modus", "KI fragen", "KI-Begleiter", "Vorgang über mehrere Fenster", "KI-Einstellungen"}},
	{"Manager", []string{"Verbindungs-Manager", "Broadcast", "Netzwerk-Scan"}},
	{"Logs", []string{"Log-Blick", "Sitzungsprotokolle"}},
	{"Überall", []string{"Überall", "Befehlspalette", "Aussehen & Tasten", "Maus"}},
}

// helpTabFor: Reiter, in dem der Bereich ctx steht.
func helpTabFor(ctx helpCtx) int {
	for i, t := range helpTabs {
		for _, title := range t.secs {
			for _, sec := range helpSections {
				if sec.title == title && sec.ctx == ctx && ctx != hcNone {
					return i
				}
			}
		}
	}
	return len(helpTabs) - 1 // Überall
}

func helpSectionByTitle(title string) (helpSection, bool) {
	for _, sec := range helpSections {
		if sec.title == title {
			return sec, true
		}
	}
	return helpSection{}, false
}

// helpBlock: ein Abschnitt in Spaltenbreite cw. Beschreibungen brechen um
// statt mit „…“ abgeschnitten zu werden.
func (s Styles) helpBlock(sec helpSection, ctx helpCtx, cw int, filter string) []string {
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
	rows := make([][2]string, 0, len(sec.rows))
	f := strings.ToLower(filter)
	for _, r := range sec.rows {
		k, d := relabelDefault(r[0], sec.ctx), i18n.T(r[1])
		if f != "" && !strings.Contains(strings.ToLower(k+" "+d+" "+i18n.T(sec.title)), f) {
			continue
		}
		rows = append(rows, [2]string{k, d})
	}
	if len(rows) == 0 {
		return nil
	}
	lines := []string{title}
	for _, r := range sortFKeys(rows, activeKeys.label(actPalette)) {
		k := s.Key.Render(r[0])
		kw := max(helpKeyW, lipgloss.Width(k)+1)
		dw := max(10, cw-kw)
		for i, l := range strings.Split(ansi.Wrap(r[1], dw, " "), "\n") {
			if i == 0 {
				lines = append(lines, k+strings.Repeat(" ", kw-lipgloss.Width(k))+s.Text.Render(l))
			} else {
				lines = append(lines, strings.Repeat(" ", kw)+s.Text.Render(l))
			}
		}
	}
	return lines
}

func (m *modal) helpView(s Styles, w, h int) string {
	avail := dialog{maxW: 160}.innerMax(w)
	if !m.helpTabSet() {
		m.helpTab = helpTabFor(m.ctx) + 1
	}
	tab := m.helpTab - 1

	// Reiterzeile
	var tb strings.Builder
	for i, t := range helpTabs {
		lbl := " " + i18n.T(t.title) + " "
		if i == tab && m.filter == "" {
			tb.WriteString(lipgloss.NewStyle().Background(s.T.Accent).Foreground(s.T.Bg).Bold(true).Render(lbl))
		} else {
			tb.WriteString(lipgloss.NewStyle().Background(s.T.Surface2).Foreground(s.T.Muted).Render(lbl))
		}
		tb.WriteString(" ")
	}
	if m.filter != "" {
		tb.WriteString(s.Accent.Render(" / ") + s.Text.Render(m.filter) + s.Muted.Render(i18n.T("  (alle Reiter)")))
	} else if lipgloss.Width(tb.String())+24 < avail {
		tb.WriteString(s.Muted.Render(i18n.T("  tippen filtert")))
	}
	tabRow := ansi.Truncate(tb.String(), avail, "")

	// Abschnitte: gewählter Reiter, beim Filtern alle
	var secs []helpSection
	if m.filter != "" {
		for _, t := range helpTabs {
			for _, title := range t.secs {
				if sec, ok := helpSectionByTitle(title); ok {
					secs = append(secs, sec)
				}
			}
		}
	} else {
		for _, title := range helpTabs[tab].secs {
			if sec, ok := helpSectionByTitle(title); ok {
				secs = append(secs, sec)
			}
		}
	}
	n := 1
	if avail >= 2*44+helpGap {
		n = 2
	}
	cw := (avail - (n-1)*helpGap) / n
	if n == 1 {
		cw = min(avail, 72)
	}
	var blocks [][]string
	var heights []int
	for _, sec := range secs {
		if b := s.helpBlock(sec, m.ctx, cw, m.filter); b != nil {
			blocks = append(blocks, b)
			heights = append(heights, len(b))
		}
	}
	var body string
	if len(blocks) == 0 {
		body = s.Muted.Render(i18n.T("Keine Taste passt. Backspace löscht, Esc leert die Suche."))
	} else {
		var cols []string
		for _, idx := range splitColumns(heights, min(n, len(blocks))) {
			var lines []string
			for j, i := range idx {
				if j > 0 {
					lines = append(lines, "")
				}
				lines = append(lines, blocks[i]...)
			}
			col := lipgloss.NewStyle().Width(cw).Render(strings.Join(lines, "\n"))
			if len(cols) > 0 {
				cols = append(cols, strings.Repeat(" ", helpGap))
			}
			cols = append(cols, col)
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	}
	cw = avail
	if n == 1 {
		cw = min(avail, 72)
	}

	note := s.Muted.Render(i18n.T("Esc führt aus jedem Panel zurück ins Terminal."))
	warn := s.ChipDanger.Render(i18n.T("ROT")) + " " + s.Text.Render(i18n.T("Gefährliche Befehle brauchen zweimal ⏎ bzw. Alt+N. ")) +
		s.AI.Render(i18n.T("KI-Vorschläge")) + s.Text.Render(i18n.T(" werden nur eingefügt, nie ausgeführt."))
	warn = lipgloss.NewStyle().Width(cw).Render(warn)
	note = lipgloss.NewStyle().Width(cw).Render(note)

	// Höhe begrenzen: Fensterrand (2), Rahmen und Abstand (4), Reiterzeile
	// mit Leerzeile (2), Warnung und Hinweis mit Leerzeile davor, Tasten.
	bodyLines := strings.Split(body, "\n")
	foot := "\n\n" + warn + "\n" + note
	maxBody := max(3, h-2-4-2-(1+lipgloss.Height(warn)+lipgloss.Height(note))-2)
	if maxBody < 10 {
		bodyLines = append(bodyLines, strings.Split(foot[1:], "\n")...)
		foot, maxBody = "", max(3, h-2-4-2-2)
	}
	keys := [][2]string{{"←/→", i18n.T("Reiter")}, {"Esc", i18n.T("schließen")}}
	if m.filter != "" {
		keys = [][2]string{{"Backspace", i18n.T("Suche kürzen")}, {"Esc", i18n.T("Suche leeren")}}
	}
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
		body: tabRow + "\n\n" + strings.Join(bodyLines, "\n") + foot, info: info, keys: keys, width: cw, maxW: 160}, w)
}

// helpTabSet: Reiter schon gewählt? (helpTab zählt ab 1, 0 = noch nicht)
func (m *modal) helpTabSet() bool { return m.helpTab > 0 }

func (m *modal) helpKey(key string) (closeIt bool) {
	if !m.helpTabSet() {
		m.helpTab = helpTabFor(m.ctx) + 1
	}
	switch key {
	case "up":
		m.scroll = max(0, m.scroll-1)
		return false
	case "down":
		m.scroll++
		return false
	case "pgup":
		m.scroll = max(0, m.scroll-10)
		return false
	case "pgdown":
		m.scroll += 10
		return false
	case "right", "tab":
		m.helpTab = m.helpTab%len(helpTabs) + 1
		m.filter, m.scroll = "", 0
		return false
	case "left", "shift+tab":
		m.helpTab = (m.helpTab+len(helpTabs)-2)%len(helpTabs) + 1
		m.filter, m.scroll = "", 0
		return false
	case "backspace":
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
			m.scroll = 0
		}
		return false
	case "esc":
		if m.filter != "" {
			m.filter, m.scroll = "", 0
			return false
		}
		return true
	case "space":
		if m.filter != "" {
			m.filter += " "
			return false
		}
		m.scroll += 10
		return false
	}
	// tippen filtert über alle Reiter
	if r := []rune(key); len(r) == 1 && r[0] >= ' ' {
		m.filter += key
		m.scroll = 0
		return false
	}
	return true
}
