package ui

import (
	"image/color"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/winstate"
)

// Aussehen der Menüfenster (Befehlspalette): Farbe je Bereich, Auswahl als
// Balken, Schalter als Pillen, Detailspalte, Reiter in den Einstellungen,
// kleine Übergänge und eine Fußzeile passend zum gewählten Eintrag.

// palGroupColor: Farbe eines Bereichs für Kopf und Symbole; nil = Standard.
func (a *App) palGroupColor(g palGroup) color.Color {
	t := a.s.T
	switch g {
	case pgSetBehave, pgTop:
		return t.Accent
	case pgSetLook, pgAI, pgRunbook:
		return t.AI
	case pgSetSafe:
		return accentColor(t, "orange")
	case pgSetSetup, pgConns:
		return accentColor(t, "cyan")
	case pgSuggest:
		return t.OK
	case pgHelp:
		return t.Warn
	}
	return nil
}

// ---------------------------------------------------------------- Werte

// palHint: rechter Teil einer Zeile. Schalter als Pille (an = grüne Fläche,
// aus = graue), Durchschalt-Werte mit Punkten für die Position.
func (a *App) palHint(it palItem, armed bool) string {
	st := a.s
	t := st.T
	switch {
	case it.danger != "" && armed:
		return st.Danger.Render(i18n.T("nochmal ⏎"))
	case it.danger != "":
		return st.Danger.Render("▲ " + it.danger)
	case it.state == palOn:
		return lipgloss.NewStyle().Background(t.OK).Foreground(inkOn(t, t.OK)).Bold(true).Render(" ● " + i18n.T("an") + " ")
	case it.state == palOff:
		return lipgloss.NewStyle().Background(t.Surface2).Foreground(t.Muted).Render(" ○ " + i18n.T("aus") + " ")
	case it.cycle != nil:
		h := st.Muted.Render("‹ ") + st.Accent.Bold(true).Render(it.hint) + st.Muted.Render(" ›")
		if n := it.pos[1]; n > 1 && n <= 8 {
			var d strings.Builder
			for k := 0; k < n; k++ {
				if k == it.pos[0] {
					d.WriteString("●")
				} else {
					d.WriteString("○")
				}
			}
			h += " " + st.Accent.Render(d.String())
		}
		return h
	}
	return st.Muted.Render(it.hint)
}

// at: Position eines Durchschalt-Werts (i von n) für die Punkte.
func (it palItem) at(i, n int) palItem {
	if i >= 0 && i < n {
		it.pos = [2]int{i, n}
	}
	return it
}

// ---------------------------------------------------------------- Hilfe

// palHelpText: ein Satz zu Einstellungen und Ordnern, steht unter der
// Auswahl bzw. in der Detailspalte. Schlüssel ist das deutsche Label.
var palHelpText = map[string]string{
	"Einfacher Modus: Profi-Funktionen ausblenden":            "Blendet Profi-Einträge aus Palette und Fußzeilen aus. Die Suche findet weiter alles.",
	"Sprache / Language":                                      "Sprache der Oberfläche. Wirkt sofort, auch auf Hilfe und Kurzreferenz.",
	"Maus: klicken, markieren, scrollen":                      "Aus: Das Terminal bekommt die Maus, markieren geht dann wie im normalen Terminal.",
	"Auto-Reconnect: nach Abbruch neu verbinden":              "Bricht eine Verbindung ab, verbindet sshTermy von selbst neu und zeigt den Versuch im Tab.",
	"Online-Punkt: Erreichbarkeit der Hosts prüfen":           "Grüner Punkt im Manager, wenn ein Host antwortet. Nur sichtbare oder alle Hosts.",
	"Meldung, wenn lange Befehle fertig sind":                 "Meldet Befehle, die länger als 10 s liefen, in sshTermy oder zusätzlich auf dem Desktop.",
	"Tastenzeile: Tasten erklären":                            "Zeigt in der Fußzeile zu jeder Taste ein Wort mehr.",
	"Fenster beim Start":                                      "Vollbild, maximiert oder so wie beim letzten Beenden.",
	"Farbschema wählen …":                                     "26 Farbschemas, hell und dunkel, mit Vorschau. ⏎ öffnet die Auswahl.",
	"Farben anpassen (eigene Farben)":                         "Eigenes Farbschema: jede Farbe einzeln, mit Kontrastprüfung.",
	"Farbregeln für die Ausgabe …":                            "Färbt Fehler, Warnungen, IPs und MACs in der Terminalausgabe ein.",
	"Symbole":                                                 "Text-Tags laufen überall, Nerd Font braucht eine passende Schrift.",
	"Fensterstil":                                             "Ecken der Dialoge (rund oder eckig) und ob sie einen Schatten werfen.",
	"Bewegung":                                                "Übergänge, Einblenden und Startanimation. Aus = alles ruhig.",
	"Barrierearm: keine Animationen, Formen statt nur Farben": "Keine Bewegung, Zustände zusätzlich als Form (▲ ● ○) statt nur als Farbe.",
	"Geteilte Ansicht: andere Bereiche abdunkeln":             "In der geteilten Ansicht treten die nicht aktiven Bereiche zurück.",
	"Einfügeschutz: Vorschau bei mehreren Zeilen":             "Mehrzeiliges Einfügen zeigt erst eine Vorschau, bevor etwas beim Server ankommt.",
	"Server darf in die Zwischenablage kopieren (OSC 52)":     "Erlaubt vim, tmux & Co. auf dem Server, in deine Zwischenablage zu schreiben.",
	"Automatisch sperren bei Leerlauf":                        "Sperrt sshTermy nach der gewählten Zeit ohne Eingabe; entsperren mit dem Master-Passwort.",
	"Master-Passwort ändern …":                                "Schützt gespeicherte Passwörter und Schlüssel. Unter Windows auch per Windows-Anmeldung.",
	"KI":                                                      "Endpunkt und Modell, Ersatz-Anbieter, KI-Begleiter, Datenschutz und Status.",
	"Protokolle & Logs":                                       "Sitzungsprotokoll, Logwache, Syslog-Empfang und Audit-Protokoll.",
	"Netzwerkgeräte":                                          "Befehlssätze je Hersteller und Regeln für Switch-Configs.",
	"Tastenkürzel anpassen":                                   "Jede Taste frei belegen; Konflikte zeigt sshTermy gleich an.",
	"Editor für Server-Dateien":                               "Welcher Editor Dateien vom Server öffnet: eingebaut oder dein eigener.",
	"Sync zwischen Rechnern einrichten":                       "Verbindungen, Ordner und Einstellungen verschlüsselt zwischen deinen Rechnern abgleichen.",
	"Vermittler":                                              "Sitzungen teilen, auch wenn der Server hinter NAT oder einer Firewall steht.",
	// Häufige Aktionen
	"Ziel-Modus öffnen":                        "Beschreibe, was erreicht werden soll; die KI schlägt Befehle vor, du bestätigst jeden.",
	"Agent-Modus öffnen (läuft selbstständig)": "Die KI arbeitet ein Ziel selbst ab und hält vor allem Gefährlichen an.",
	"KI fragen":      "Frage zur Sitzung oder zur letzten Ausgabe; die Antwort erscheint im Seitenpanel.",
	"Host-Übersicht": "CPU, Speicher, Platten, Dienste und Netz des Hosts auf einen Blick.",
	"Vorgang über mehrere Fenster (KI sieht alle)": "Ein Ziel über mehrere Tabs: die KI plant je Fenster und schützt das Zielfenster.",
	"Im Verlauf suchen":                            "Sucht im Zurückblättern dieses Tabs; ↑↓ springt zwischen Treffern.",
	"In allen Tabs suchen":                         "Sucht gleichzeitig im Verlauf aller offenen Tabs.",
	"Log-Blick: Auffälligkeiten in den Logs":       "Liest journal, syslog oder Eventlog und zeigt Fehler, Warnungen und neue Muster.",
	"Aufteilung wählen …":                          "Mehrere Tabs nebeneinander: sieben Aufteilungen mit Skizze.",
	"Neuer Tab":                                    "Öffnet den Verbindungs-Manager; ⏎ verbindet in einem zusätzlichen Tab.",
	"Neue Verbindung":                              "Formular für eine neue Verbindung: SSH, Telnet, seriell, lokal oder PowerShell.",
	"Dateien (SFTP)":                               "Dateien von Server und Rechner nebeneinander, mit Maus und Ziehen.",
	"Jetzt sperren":                                "Sperrt sshTermy sofort; entsperren mit dem Master-Passwort.",
}

// palHelp: Hilfesatz zum Eintrag (übersetzt) oder "".
func palHelp(it palItem) string {
	for k, v := range palHelpText {
		if it.label == i18n.T(k) || it.label == k {
			return i18n.T(v)
		}
	}
	return ""
}

// ---------------------------------------------------------------- Reiter

// palTabGroups: Reiter der Einstellungen in Anzeigereihenfolge.
var palTabGroups = []palGroup{pgSetBehave, pgSetLook, pgSetSafe, pgSetSetup}

// palTabbed: zeigt das Menü gerade Reiter (Einstellungen ohne Suchtext)?
func (a *App) palTabbed() bool {
	p := a.pal
	return p != nil && p.menu == "settings" && strings.TrimSpace(p.in.Value()) == ""
}

// palTabMove: nächster/vorheriger Reiter.
func (a *App) palTabMove(dir int) {
	p := a.pal
	i := 0
	for k, g := range palTabGroups {
		if g == p.tab {
			i = k
		}
	}
	p.tab = palTabGroups[cycleIndex(i, len(palTabGroups), dir)]
	p.sel, p.armed = 0, -1
}

// palTabsRow: Reiterleiste, klickbar.
func (a *App) palTabsRow() string {
	st := a.s
	p := a.pal
	var parts []string
	for _, g := range palTabGroups {
		name := palTitle(i18n.T(palGroupName[g]))
		col := a.palGroupColor(g)
		var s string
		if g == p.tab {
			s = lipgloss.NewStyle().Background(col).Foreground(inkOn(st.T, col)).Bold(true).Render(" " + name + " ")
		} else {
			s = lipgloss.NewStyle().Background(st.T.Surface2).Foreground(st.T.Muted).Render(" " + name + " ")
		}
		g := g
		parts = append(parts, a.z.click(s, func(tea.Mouse, bool) tea.Cmd {
			p.tab, p.sel, p.armed = g, 0, -1
			return nil
		}))
	}
	return strings.Join(parts, " ")
}

// palTitle: „ALLGEMEIN“ → „Allgemein“ (nur erster Buchstabe groß).
func palTitle(s string) string {
	r := []rune(strings.ToLower(s))
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// ---------------------------------------------------------------- Bewegung

const (
	palTrailDur = 260 * time.Millisecond // Nachleuchten der alten Auswahl
	palGlowDur  = 650 * time.Millisecond // umgeschalteter Wert leuchtet
)

// palChanged merkt sich, welcher Wert gerade umgeschaltet wurde.
func (p *palette) palChanged(label string) {
	p.mo.changed, p.mo.changedAt = label, time.Now()
}

// glowK: wie stark der umgeschaltete Wert noch leuchtet (1 … 0).
func (p *palette) glowK(label string, now time.Time) float64 {
	if motionLevel() == 0 || label != p.mo.changed {
		return 0
	}
	d := now.Sub(p.mo.changedAt)
	if d >= palGlowDur {
		return 0
	}
	return 1 - easeOut(float64(d)/float64(palGlowDur))
}

// trailK: Nachleuchten der vorher gewählten Zeile (1 … 0).
func (p *palette) trailK(i int, now time.Time) float64 {
	if motionLevel() == 0 || i != p.mo.prevSel || i == p.sel {
		return 0
	}
	d := now.Sub(p.mo.selAt)
	if d >= palTrailDur {
		return 0
	}
	return 1 - easeOut(float64(d)/float64(palTrailDur))
}

// ---------------------------------------------------------------- Fußzeile

// palKeys: Tasten unten, passend zum gewählten Eintrag.
func (a *App) palKeys(items []palItem) [][2]string {
	p := a.pal
	var cur palItem
	have := p.sel < len(items)
	if have {
		cur = items[p.sel]
	}
	keys := [][2]string{{"↑↓", i18n.T("wählen")}}
	if a.palTabbed() {
		keys = append(keys, [2]string{"Tab", i18n.T("Reiter")})
	}
	switch {
	case !have:
	case cur.cycle != nil:
		keys = append(keys, [2]string{"← →", i18n.T("ändern")})
	case cur.state != palNoState:
		keys = append(keys, [2]string{"⏎", i18n.T("an/aus")})
	case cur.open != "":
		keys = append(keys, [2]string{"→ ⏎", i18n.T("öffnen")})
	case cur.danger != "":
		keys = append(keys, [2]string{"⏎ ⏎", i18n.T("zweimal bestätigen")})
	default:
		keys = append(keys, [2]string{"⏎", i18n.T("ausführen")})
	}
	if p.menu == "" {
		if _, ok := palAskItem(items); ok {
			keys = append(keys, [2]string{"Tab", i18n.T("KI fragen")})
		}
		return append(keys, [2]string{"Esc", i18n.T("schließen")})
	}
	return append(keys, [2]string{"Esc", i18n.T("zurück")})
}

// ---------------------------------------------------------------- Detailspalte

// palDetailWidth: Breite der Detailspalte rechts; 0 = Fenster zu schmal.
func (a *App) palDetailWidth(listW int) int {
	free := a.w - 6 - listW - 3
	if free < 30 || a.h < 20 {
		return 0
	}
	return min(46, free)
}

// palDetail: Erklärung und Vorschau zum gewählten Eintrag.
func (a *App) palDetail(it palItem, w, h int) string {
	st := a.s
	t := st.T
	col := a.palGroupColor(it.group)
	if col == nil {
		col = t.Accent
	}
	wrap := lipgloss.NewStyle().Width(w)
	var out []string
	add := func(s string) { out = append(out, strings.Split(s, "\n")...) }
	add(lipgloss.NewStyle().Foreground(col).Bold(true).Width(w).Render(ansi.Strip(it.label)))
	out = append(out, lipgloss.NewStyle().Foreground(t.Border).Render(strings.Repeat("─", w)))
	if h := palHelp(it); h != "" {
		add(wrap.Render(h))
	} else if it.sub != "" {
		add(st.Muted.Width(w).Render(strings.TrimLeft(it.sub, " ·")))
	}
	switch {
	case it.state == palOn || it.state == palOff || it.cycle != nil:
		out = append(out, "", st.Muted.Render(i18n.T("Jetzt: "))+a.palHint(it, false))
	case it.hint != "" && it.open == "" && it.hint != "›":
		out = append(out, "", st.Muted.Render(i18n.T("Taste / Wert: "))+lipgloss.NewStyle().Background(t.Surface2).Foreground(col).Bold(true).Render(" "+it.hint+" "))
	}
	if pv := a.palPreview(it, w); pv != "" {
		out = append(out, "")
		add(pv)
	}
	if it.open != "" {
		if kids := a.palChildren(it.open); len(kids) > 0 {
			out = append(out, "", st.Muted.Render(i18n.T("Darin:")))
			for i, k := range kids {
				if i >= max(3, h-len(out)-2) {
					out = append(out, st.Muted.Render(fmtMore(len(kids)-i)))
					break
				}
				icon := lipgloss.NewStyle().Foreground(col).Render(padIcon(k.icon))
				out = append(out, icon+" "+trunc(k.label, w-lipgloss.Width(padIcon(k.icon))-1))
			}
		}
	}
	if len(out) > h {
		out = out[:h]
	}
	return strings.Join(out, "\n")
}

func fmtMore(n int) string {
	return "… " + i18n.T("und") + " " + strconv.Itoa(n) + " " + i18n.T("weitere")
}

// palChildren: Einträge eines Ordners (für die Detailspalte), ohne die
// Auswahl der Palette anzufassen.
func (a *App) palChildren(menu string) []palItem {
	p := a.pal
	saveMenu, saveTab := p.menu, p.tab
	p.menu = menu
	defer func() { p.menu, p.tab = saveMenu, saveTab }()
	if menu == "settings" {
		var all []palItem
		for _, x := range a.palMenuItems() {
			if x.menu == menu && !x.hidden {
				all = append(all, x)
			}
		}
		return all
	}
	return a.paletteItems()
}

// palPreview: kleine Vorschau für Aussehen-Einstellungen.
func (a *App) palPreview(it palItem, w int) string {
	st := a.s
	t := st.T
	switch it.label {
	case i18n.T("Farbschema wählen …"), i18n.T("Farben anpassen (eigene Farben)"):
		bg := lipgloss.NewStyle().Background(t.Bg)
		line := func(parts ...string) string {
			s := strings.Join(parts, "")
			return bg.Width(w).Render(s)
		}
		fg := func(c color.Color, s string) string {
			return lipgloss.NewStyle().Background(t.Bg).Foreground(c).Render(s)
		}
		sw := func(c color.Color) string { return lipgloss.NewStyle().Background(c).Render("  ") + " " }
		return strings.Join([]string{
			st.Muted.Render(i18n.T("Vorschau")),
			line(fg(t.OK, " deploy@web-01"), fg(t.Text, ":"), fg(t.Accent, "~"), fg(t.Text, "$ systemctl status")),
			line(fg(t.OK, " ● "), fg(t.Text, "nginx.service")),
			line(fg(t.Muted, "   Active: "), fg(t.OK, "active (running)")),
			line(fg(t.Muted, "   Warn:   "), fg(t.Warn, "worker 3 slow")),
			line(fg(t.Muted, "   Error:  "), fg(t.Danger, "upstream timed out")),
			"",
			sw(t.Accent) + sw(t.AI) + sw(t.OK) + sw(t.Warn) + sw(accentColor(t, "orange")) + sw(t.Danger) + sw(accentColor(t, "cyan")),
		}, "\n")
	case i18n.T("Symbole"):
		var b strings.Builder
		for _, id := range []string{"host", "ai", "sshkey", "split", "open", "errors", "team", "theme"} {
			b.WriteString(st.Accent.Render(padIcon(ic(id))) + " ")
		}
		return st.Muted.Render(i18n.T("So sehen Symbole aus:")) + "\n" + b.String()
	case i18n.T("Fensterstil"):
		box := st.dialog(dialog{title: i18n.T("Probe"), body: i18n.T("So sehen Dialoge aus."), width: min(w-6, 26), compact: true, noClose: true}, w)
		return zoneRe.ReplaceAllString(box, "")
	case i18n.T("Bewegung"), i18n.T("Barrierearm: keine Animationen, Formen statt nur Farben"):
		return st.Muted.Width(w).Render(i18n.T("Wirkt sofort: Diese Palette blendet dann ein bzw. bleibt ruhig."))
	}
	return ""
}

func langIdx() int {
	if i18n.Lang() == i18n.EN {
		return 1
	}
	return 0
}

func winStartIdx(m string) int {
	return indexOf([]string{winstate.StartFull, winstate.StartMax, winstate.StartLast}, m)
}
