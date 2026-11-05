package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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
	hcLogs
	hcBroadcast
	hcScan
	hcHost
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
		{"F9", "Spickzettel an/aus"},
		{"F12", "Über & Update"},
		{"F12 w", "Was ist neu"},
		{"Strg+Q", "beenden"},
	}},
	{hcManager, "≡", "Verbindungs-Manager", "F10", false, [][2]string{
		{"⏎", "verbinden"},
		{"n", "neue Verbindung"},
		{"e", "bearbeiten"},
		{"d", "löschen"},
		{"/", "suchen (auch #tag)"},
		{"o", "neuer Ordner (A / B verschachtelt)"},
		{"r", "Ordner umbenennen"},
		{"v", "in Ordner verschieben"},
		{"Shift+↑", "nach oben schieben (↓ runter)"},
		{"← / →", "Ordner zu-/aufklappen"},
		{"- / +", "alle Ordner zu/auf"},
		{"#", "nach Tag filtern"},
		{"t", "Tunnel anlegen"},
		{"q", "Schnellbefehle"},
		{"m", "Broadcast an mehrere Hosts"},
		{"f", "Netzwerk-Scan"},
		{"s", "KI-Einstellungen"},
		{"y", "Sync zwischen Rechnern"},
		{"i", "Verbindungen importieren"},
		{"a", "Aussehen wechseln"},
		{"b", "Tastenkürzel anpassen"},
		{"l", "Sitzungsprotokolle"},
		{"F10", "zurück zum Terminal"},
	}},
	{hcSettings, "⚙", "KI-Einstellungen", "s", true, [][2]string{
		{"Strg+T", "Endpunkt testen"},
		{"Strg+L", "Modell wählen"},
		{"Strg+S", "speichern"},
	}},
	{hcTerminal, "▸", "Terminal", "", false, [][2]string{
		{"Tab / →", "Ghost-Text übernehmen"},
		{"Alt+1…4", "Folgebefehl einfügen"},
		{"Alt+0", "Fehler: Warum? & Fix"},
		{"⇧F1…F8", "Schnellbefehl ausführen"},
		{"F2", "Ziel-Modus (Agent)"},
		{"F3", "KI fragen"},
		{"F4", "Tunnel"},
		{"F5", "Dateien (SFTP)"},
		{"F6", "Host-Übersicht"},
		{"F7 / F8", "Tab wechseln"},
		{"F10", "zum Manager"},
		{"Abbruch", "verbindet neu (⏎ / Esc)"},
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
	}},
	{hcAsk, "✦", "KI fragen", "F3", true, [][2]string{
		{"⏎", "Frage senden"},
		{"⇧⏎ / Strg+J", "neue Zeile"},
		{"1…9", "Befehl einfügen"},
		{"n", "neue Frage"},
	}},
	{hcTunnel, "⇄", "Tunnel", "F4", false, [][2]string{
		{"⏎", "starten / stoppen"},
		{"o", "im Browser öffnen"},
		{"n", "neuer Tunnel"},
		{"e", "bearbeiten"},
		{"d", "löschen"},
	}},
	{hcHost, "▣", "Host-Übersicht", "F6", false, [][2]string{
		{"r / ⏎", "abfragen"},
		{"b", "Switch-Config sichern"},
		{"↑ ↓", "Sicherung wählen"},
		{"d", "Diff zur Vorversion"},
		{"v", "Sicherung ansehen"},
		{"x", "Abfrage abbrechen"},
	}},
	{hcFiles, "▤", "Dateien", "F5", false, [][2]string{
		{"Tab", "Seite wechseln"},
		{"⏎ / →", "Ordner öffnen"},
		{"← / ⌫", "eine Ebene hoch"},
		{"c / F6", "kopieren (hoch/runter)"},
		{"n", "neuer Ordner"},
		{"d", "löschen"},
		{".", "versteckte Dateien"},
		{"x", "Übertragung abbrechen"},
	}},
	{hcLogs, "☰", "Sitzungsprotokolle", "l", false, [][2]string{
		{"/", "suchen (auch Ausgabe)"},
		{"z", "KI-Zusammenfassung"},
		{"Z", "alle Treffer zusammenfassen"},
		{"x", "als Markdown exportieren"},
		{"d", "löschen"},
		{"a", "Mitschreiben an/aus"},
		{"⎵ Befehl", "nicht protokollieren"},
	}},
	{hcBroadcast, "⇉", "Broadcast", "m", false, [][2]string{
		{"Space / g / a", "Host / Ordner / alle"},
		{"Tab", "Hosts · Quelle · Befehl"},
		{"⇧⏎", "weiterer Schritt"},
		{"Strg+S", "prüfen, dann ⏎"},
		{"← / →", "Host-Spalte wählen"},
		{"↑ ↓ Bild↑↓", "Ausgabe blättern"},
		{"z / c", "groß / kopieren"},
		{"r / e / x", "nochmal / ändern / Stopp"},
	}},
	{hcScan, "⌕", "Netzwerk-Scan", "f", false, [][2]string{
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
		{"Esc", "schließen"},
	}},
	{hcNone, "◐", "Aussehen & Tasten", "", false, [][2]string{
		{"a", "Dunkel · Hell · Kontrast"},
		{"b", "Tastenkürzel belegen"},
		{"Strg +/−", "Schrift (Terminalfenster)"},
	}},
	{hcNone, "⌖", "Maus", "", false, [][2]string{
		{"Klick", "Tabs, Listen, Felder"},
		{"Doppelklick", "öffnen / Wort"},
		{"Ziehen", "markieren + kopieren"},
		{"Rechtsklick", "einfügen"},
		{"Rechtsklick Link", "Menü: Verbinden, Ping"},
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
	return &modal{kind: mHelp, title: "Tastenkürzel", ctx: a.helpContext()}
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
	case scrBroadcast:
		return hcBroadcast
	case scrScan:
		return hcScan
	case scrTerminal:
		if s := a.current(); s != nil && s.files.open {
			return hcFiles
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
	title := head.Render(sec.icon + " " + sec.title)
	if sec.key != "" {
		title += " " + s.Muted.Render(relabelDefault(sec.key, sec.ctx))
	}
	if sec.ctx != hcNone && sec.ctx == ctx {
		title += " " + s.Badge.Render("hier")
	}
	lines := []string{title}
	for _, r := range sec.rows {
		k := s.Key.Render(relabelDefault(r[0], sec.ctx))
		k += strings.Repeat(" ", max(0, helpKeyW-lipgloss.Width(k)))
		lines = append(lines, k+s.Text.Render(ansi.Truncate(r[1], helpColW-helpKeyW, "…")))
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
	avail := w - 4 - 6 // Rand und Innenabstand des Modals
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
	cw := lipgloss.Width(body)

	note := s.Muted.Render("Esc führt aus jedem Panel zurück ins Terminal.")
	cw = max(cw, lipgloss.Width(note))
	header := s.Accent.Render("? Tastenkürzel") + s.Muted.Render("  ·  "+version.Short())
	warn := s.ChipDanger.Render("ROT") + " " + s.Text.Render("Gefährliche Befehle brauchen zweimal ⏎ bzw. Alt+N. ") +
		s.AI.Render("KI-Vorschläge") + s.Text.Render(" werden nur eingefügt, nie ausgeführt.")
	warn = lipgloss.NewStyle().Width(cw).Render(warn)

	// Höhe begrenzen: Rand des Fensters (2), Modal-Rahmen und Innenabstand (4),
	// Kopf (2), Fuß (Leerzeile, Warnung, Hinweiszeile).
	bodyLines := strings.Split(body, "\n")
	const closeHint, scrollHint = "beliebige Taste schließt", "▾ mehr · ↑↓ blättern · "
	footH := 1
	if lipgloss.Width(note)+2+lipgloss.Width(scrollHint+closeHint) > cw {
		footH = 2
	}
	maxBody := max(3, h-2-4-2-(1+lipgloss.Height(warn)+footH))
	hint := closeHint
	if total := len(bodyLines); total > maxBody {
		m.scroll = max(0, min(m.scroll, total-maxBody))
		bodyLines = bodyLines[m.scroll : m.scroll+maxBody]
		hint = "↑↓ blättern · " + hint
		if m.scroll+maxBody < total {
			hint = "▾ mehr · " + hint
		}
	} else {
		m.scroll = 0
	}
	footer := s.Muted.Render(hint)
	if gap := cw - lipgloss.Width(note) - lipgloss.Width(footer); footH == 1 {
		footer = note + strings.Repeat(" ", gap) + footer
	} else {
		footer = note + "\n" + footer
	}

	out := header + "\n\n" + strings.Join(bodyLines, "\n") + "\n\n" + warn + "\n" + footer
	return s.Modal.Render(out)
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
