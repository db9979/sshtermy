package ui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Befehlspalette (Strg+Leertaste): ein Einstieg für alles. Was getippt wird,
// kann als Ziel oder Frage an die KI gehen; darunter die Folgebefehle der
// Sitzung, Tunnel, Verbindungen und Aktionen, gefiltert nach dem Text.
//
// Auch hier gilt: Vorschläge werden nur eingefügt (ohne Enter), rote
// Befehle brauchen einen zweiten Druck.

type palGroup int

const (
	pgAI palGroup = iota
	pgSuggest
	pgQuick
	pgRunbook
	pgTunnel
	pgConnect
	pgAction
	pgConns // gespeicherte Verbindungen (palconn.go)
	pgSetBehave
	pgSetLook
	pgSetSetup
	pgHelp
)

var palGroupName = map[palGroup]string{pgAI: "KI", pgSuggest: "VORSCHLÄGE", pgQuick: "SCHNELLBEFEHLE", pgRunbook: "RUNBOOKS", pgTunnel: "TUNNEL", pgConnect: "VERBINDEN", pgAction: "AKTIONEN", pgConns: "VERBINDUNGEN",
	pgSetBehave: "VERHALTEN", pgSetLook: "AUSSEHEN & SPRACHE", pgSetSetup: "EINRICHTEN", pgHelp: "HILFE & INFO"}

type palItem struct {
	group     palGroup
	icon      string
	iconColor color.Color // nil = Farbe der Gruppe
	label     string
	sub       string // gedimmt hinter dem Namen (Ordner)
	hint      string
	danger    string
	more      bool   // „weitere im Manager“: gedimmt
	menu      string // Untermenü ("" = Übersicht), siehe palMenus
	keys      string // weitere Suchwörter (z. B. englisch)
	state     palState
	keep      bool // Palette bleibt nach dem Ausführen offen (Schalter)
	run       func() tea.Cmd
}

// palState: Schalter zeigen rechts „an“ oder „aus“ statt eines Hinweises.
type palState int8

const (
	palNoState palState = iota
	palOn
	palOff
)

type palette struct {
	in    aiInput
	sel   int
	armed int
	menu  string // offenes Untermenü, "" = Übersicht
}

func isPaletteKey(k string) bool { return activeKeys.is(actPalette, k) }

func newPaletteInput() aiInput {
	in := newAIInput(i18n.T("Ziel, Frage, Befehl, Tunnel, Verbindung oder user@host:port …"))
	in.MaxHeight = 4
	return in
}

func (a *App) openPalette() tea.Cmd {
	a.pal = &palette{in: newPaletteInput(), armed: -1}
	return a.pal.in.Focus()
}

func (a *App) paletteItems() []palItem {
	q := strings.TrimSpace(a.pal.in.Value())
	ql := strings.ToLower(q)
	match := func(x string) bool { return ql == "" || strings.Contains(strings.ToLower(x), ql) }
	if a.pal.menu != "" {
		var items []palItem
		for _, x := range a.palMenuItems() {
			if x.menu == a.pal.menu && x.matches(match) {
				items = append(items, x)
			}
		}
		return items
	}
	s := a.current()
	inTerm := a.scr == scrTerminal && s != nil && s.state == stConnected
	var items []palItem
	quick, quickFirst := a.quickItems(q)
	if quickFirst {
		items = append(items, quick...) // user@host:port: Verbinden ist gemeint
	}
	if q != "" && inTerm {
		items = append(items,
			palItem{group: pgAI, icon: ic("goal"), label: i18n.T("Als Ziel starten: „") + trunc(oneLine(q), 50) + "“", hint: "⏎",
				run: func() tea.Cmd { return a.startGoal(s, q) }},
			palItem{group: pgAI, icon: ic("ai"), label: i18n.T("KI fragen: „") + trunc(oneLine(q), 50) + "“", hint: "Tab",
				run: func() tea.Cmd { return a.startAsk(s, q) }},
			palItem{group: pgAI, icon: ic("rewrite"), label: i18n.T("Für dieses System umschreiben: „") + trunc(oneLine(q), 40) + "“", hint: i18n.T("Befehl"),
				run: func() tea.Cmd { return a.startAdapt(s, oneLine(q)) }})
	}
	if inTerm && s.fail != nil && match("fehler warum fix error why "+s.fail.Command) {
		items = append(items, palItem{group: pgAI, icon: ic("fix"), label: i18n.T("Warum ist „") + trunc(oneLine(s.fail.Command), 40) + i18n.T("“ fehlgeschlagen? Fix vorschlagen"), hint: "Alt+0",
			run: func() tea.Cmd { return a.explainFailure(s) }})
	}
	if inTerm {
		for _, c := range s.chips {
			c := c
			if !match(c.Command) {
				continue
			}
			it := palItem{group: pgSuggest, icon: ic("command"), label: c.Command, hint: i18n.T("einfügen"),
				run: func() tea.Cmd {
					s.term.SendText(c.Command)
					s.noteAI(c.Command, true)
					s.panelFocus = false
					return nil
				}}
			if len(c.Danger) > 0 {
				it.danger = c.Danger[0].Reason
			} else if c.Elevated {
				it.hint = "sudo"
			}
			items = append(items, it)
		}
		items = append(items, a.quickPaletteItems(s, match)...)
		for _, rb := range a.st.Runbooks(s.conn.ID) {
			rb := rb
			if !match(rb.Name + " " + rb.Goal + " " + strings.Join(rb.Steps, " ")) {
				continue
			}
			items = append(items, palItem{group: pgRunbook, icon: ic("runbook"), label: rb.Name, hint: plural(len(rb.Steps), i18n.T("Schritt"), i18n.T("Schritte")),
				run: func() tea.Cmd { return a.startRunbook(s, rb) }})
		}
		for _, t := range a.st.Tunnels(s.conn.ID) {
			t := t
			if !match(t.Name + " " + t.Spec()) {
				continue
			}
			on := s.tunnels != nil && s.tunnels.Get(t.ID) != nil
			label, hint := "„"+t.Name+i18n.T("“ starten · ")+t.Spec(), i18n.T("an")
			if on {
				label, hint = "„"+t.Name+i18n.T("“ stoppen"), i18n.T("aus")
				if at := s.tunnels.Get(t.ID); at.URL() != "" {
					u := at.URL()
					items = append(items, palItem{group: pgTunnel, icon: ic("open"), label: i18n.T("Öffnen: ") + u, hint: "Browser",
						run: func() tea.Cmd { _ = sshx.OpenBrowser(u); return nil }})
				}
			}
			items = append(items, palItem{group: pgTunnel, icon: ic("tunnel"), label: label, hint: hint, run: func() tea.Cmd {
				if s.tunnels == nil {
					return nil
				}
				if s.tunnels.Get(t.ID) != nil {
					s.tunnels.Stop(t.ID)
				} else if at, err := s.tunnels.Start(t); err != nil {
					a.flashErr(i18n.T("Tunnel: ") + err.Error())
				} else if at.URL() != "" {
					a.flash(i18n.T("Web-Service bereit: ") + at.URL())
				}
				return nil
			}})
		}
	}
	if !quickFirst {
		items = append(items, quick...)
	}
	items = append(items, a.palConnItems(q)...)
	actions := []palItem{
		{icon: ic("goal"), label: i18n.T("Ziel-Modus öffnen"), hint: a.keys.label(actGoal), run: func() tea.Cmd {
			if inTerm {
				withPanel(s, panelGoal)
				a.resize(s)
				return a.panelOpened(s)
			}
			return nil
		}},
		{icon: ic("tunnel"), label: i18n.T("Tunnel-Panel"), hint: a.keys.label(actTunnels), run: func() tea.Cmd {
			if inTerm {
				withPanel(s, panelTunnels)
				a.resize(s)
			}
			return nil
		}},
		{icon: ic("quick"), label: i18n.T("Schnellbefehle bearbeiten"), hint: i18n.T("Liste"), run: func() tea.Cmd {
			if inTerm {
				return a.editQuickFor(s)
			}
			return nil
		}},
		{icon: ic("manager"), label: i18n.T("Verbindungs-Manager"), hint: a.keys.label(actManager), run: func() tea.Cmd { a.scr = scrManager; return nil }},
		{icon: ic("new"), label: i18n.T("Neue Verbindung"), hint: "n", run: func() tea.Cmd { return a.openConnForm(newConnDefaults()) }},
		{icon: ic("sshkey"), label: i18n.T("SSH-Schlüssel erzeugen / auf den Server übertragen"), hint: a.keys.label(actSSHKey), run: func() tea.Cmd {
			if c, ok := a.sshKeyTarget(); ok {
				return a.openSSHKeyForm(c)
			}
			a.flash(i18n.T("Erst eine Verbindung im Manager wählen oder verbinden"))
			return nil
		}},
		{icon: ic("broadcast"), label: i18n.T("Broadcast: Befehl auf mehreren Hosts"), hint: a.keys.label(actBroadcast), run: func() tea.Cmd { return a.openBroadcast() }},
		{icon: ic("scan"), label: i18n.T("Netzwerk-Scan: SSH-Geräte finden"), hint: a.keys.label(actScan), run: func() tea.Cmd { return a.openScan() }},
		{icon: ic("team"), label: i18n.T("Mit dem Team teilen (verschlüsselt, ohne Passwörter)"), hint: "Team", run: func() tea.Cmd { a.scr = scrManager; return a.openTeamShare() }},
		{icon: ic("team"), label: i18n.T("Team-Paket übernehmen"), hint: "Team", run: func() tea.Cmd { return a.openTeamTake() }},
		{icon: ic("team"), label: i18n.T("Team-Ordner (veröffentlicht / abonniert)"), hint: "Team", run: func() tea.Cmd { return a.openTeamLinks() }},
		{icon: ic("logs"), label: i18n.T("Sitzungsprotokolle durchsuchen"), hint: "l", run: func() tea.Cmd { return a.openLogs() }},
	}
	if a.restartPending != "" {
		actions = append(actions, palItem{icon: ic("restart"), label: i18n.T("Neu starten (v") + a.restartPending + i18n.T(" aktivieren)"), hint: "Update", run: func() tea.Cmd { return a.askRestart() }})
	}
	if a.st.SyncConfig().Enabled() {
		actions = append(actions, palItem{icon: ic("sync"), label: i18n.T("Jetzt synchronisieren"), hint: "Sync", run: func() tea.Cmd {
			a.flash(i18n.T("Sync läuft …"))
			return a.runSync()
		}})
	}
	if !inTerm {
		actions = actions[3:]
	} else {
		actions = append(actions, palItem{icon: ic("host"), label: i18n.T("Host-Übersicht"), hint: a.keys.label(actHost), run: func() tea.Cmd {
			withPanel(s, panelHost)
			a.resize(s)
			return a.panelOpened(s)
		}})
		if s.isSerial() && s.state == stConnected {
			actions = append(actions, palItem{icon: ic("break"), label: i18n.T("Break senden (serielle Konsole)"), hint: i18n.T("Konsole"), run: func() tea.Cmd { return a.sendBreak(s) }})
		}
		actions = append(actions,
			palItem{icon: ic("record"), label: recordLabel(s.recording()), hint: "asciinema", run: func() tea.Cmd { return a.toggleRecording(s) }},
			palItem{icon: ic("export"), label: i18n.T("Terminal-Verlauf als Text exportieren"), hint: ".txt", run: func() tea.Cmd { return a.exportScrollback(s) }})
		if s.netDev() {
			actions = append(actions, palItem{icon: ic("backup"), label: i18n.T("Switch-Config sichern und vergleichen"), hint: "b", run: func() tea.Cmd {
				withPanel(s, panelHost)
				s.panelFocus = false
				a.resize(s)
				s.host.backups = a.st.Backups(s.conn.ID)
				return a.hostNetCmd(s, false, true)
			}})
		}
	}
	if a.scr == scrTerminal && s != nil {
		actions = append(actions,
			palItem{icon: ic("search"), label: i18n.T("Im Verlauf suchen"), hint: "Strg+F", run: func() tea.Cmd { return a.openSearch(s) }},
			palItem{icon: ic("errors"), label: i18n.T("Fehlerzeilen im Verlauf zeigen"), hint: i18n.T("Strg+F Strg+E"), run: func() tea.Cmd {
				cmd := a.openSearch(s)
				s.search.errors, s.search.list = true, true
				a.refreshSearch(s, true)
				return cmd
			}})
		for _, m := range []splitMode{splitCols, splitRows, splitGrid, splitOff} {
			if m == a.split || (m == splitOff && a.split == splitOff) {
				continue
			}
			m := m
			label := i18n.T("Ansicht teilen: ") + m.label()
			if m == splitOff {
				label = i18n.T("Ansicht nicht mehr teilen")
			}
			actions = append(actions, palItem{icon: ic("split"), label: label, hint: a.keys.label(actSplit), run: func() tea.Cmd { a.setSplit(m); return nil }})
		}
		actions = append(actions, a.persistPalette(s)...)
		if s.detached() {
			actions = append(actions, palItem{icon: ic("attach"), label: i18n.T("Tab aus eigenem Fenster zurückholen"), hint: a.keys.label(actDetach),
				run: func() tea.Cmd { return a.toggleDetach(s) }})
		} else if s.state == stConnected {
			actions = append(actions, palItem{icon: ic("detach"), label: i18n.T("Tab in eigenes Fenster abtrennen"), hint: a.keys.label(actDetach),
				run: func() tea.Cmd { return a.toggleDetach(s) }})
		}
		actions = append(actions,
			palItem{icon: ic("tabs"), label: i18n.T("Alle Tabs zeigen"), hint: a.keys.label(actTabList), run: func() tea.Cmd { return a.openTabList() }},
			palItem{icon: ic("new"), label: i18n.T("Neuer Tab"), hint: a.keys.label(actNewTab), run: func() tea.Cmd { return a.openNewTab() }},
			palItem{icon: ic("rename"), label: i18n.T("Tab umbenennen"), hint: trunc(s.tabName(), 16), run: func() tea.Cmd { return a.renameTab(s, nil) }},
			palItem{icon: ic("tabs"), label: i18n.T("Tab duplizieren"), hint: trunc(s.conn.Alias, 16), run: func() tea.Cmd { return a.duplicateTab(s) }},
			palItem{icon: ic("close"), label: i18n.T("Tab schließen"), hint: a.keys.label(actCloseTab), run: func() tea.Cmd { return a.closeTabAsk(s, nil) }})
	}
	actions = append(actions, palItem{icon: ic("sshkey"), label: i18n.T("Jetzt sperren"), hint: i18n.T("Master-Passwort"), run: func() tea.Cmd { return a.lockNow() }})
	for _, x := range actions {
		x.group = pgAction
		if match(x.label) {
			items = append(items, x)
		}
	}
	// Untermenüs: ohne Text als je ein Eintrag, mit Text direkt die Treffer
	// daraus (mit dem Untermenü grau dahinter).
	for _, m := range palMenus {
		m := m
		if ql == "" || match(i18n.T(m.label)+" "+m.keys) {
			items = append(items, palItem{group: pgAction, icon: ic(m.icon), label: i18n.T(m.label) + " ›", hint: i18n.T(m.hint), keep: true,
				run: func() tea.Cmd { a.palEnter(m.id); return nil }})
		}
	}
	if ql != "" {
		for _, x := range a.palMenuItems() {
			if x.matches(match) {
				x.sub = "· " + i18n.T(palMenuLabel(x.menu))
				x.group = pgAction
				items = append(items, x)
			}
		}
	}
	return items
}

// ---------------------------------------------------------------- Untermenüs

type palMenu struct {
	id, icon, label, hint, keys string
}

var palMenus = []palMenu{
	{"settings", "settings", "Einstellungen", "Maus, Aussehen, Sprache, KI …", "settings optionen schalter"},
	{"help", "help", "Hilfe & Info", "Tasten, Tour, Neues, Update", "help hilfe info"},
}

func palMenuLabel(id string) string {
	for _, m := range palMenus {
		if m.id == id {
			return m.label
		}
	}
	return ""
}

func (it palItem) matches(match func(string) bool) bool {
	return match(it.label) || (it.keys != "" && match(it.keys))
}

// palEnter öffnet ein Untermenü ("" = zurück zur Übersicht).
func (a *App) palEnter(menu string) {
	p := a.pal
	p.menu, p.sel, p.armed = menu, 0, -1
	p.in.SetValue("")
}

// palToggle: Schalter mit Zustand rechts; die Palette bleibt offen.
func palToggle(icon, label string, on bool, keys string, run func() tea.Cmd) palItem {
	st := palOff
	if on {
		st = palOn
	}
	return palItem{group: pgSetBehave, menu: "settings", icon: icon, label: label, state: st, keys: keys, keep: true, run: run}
}

// palMenuItems: Inhalt beider Untermenüs, in Anzeigereihenfolge.
func (a *App) palMenuItems() []palItem {
	items := []palItem{
		palToggle(ic("mouse"), i18n.T("Maus: klicken, markieren, scrollen"), a.mouseEnabled(), "mouse", func() tea.Cmd { return a.toggleMouse() }),
		palToggle(ic("reconnect"), i18n.T("Auto-Reconnect: nach Abbruch neu verbinden"), a.autoReconnect(), "reconnect", func() tea.Cmd { return a.toggleAutoReconnect() }),
		palToggle(ic("reach"), i18n.T("Online-Punkt: Erreichbarkeit der Hosts prüfen"), a.reachEnabled(), "online reach", func() tea.Cmd { return a.toggleReach() }),
		palToggle(ic("sesslog"), i18n.T("Sitzungsprotokoll schreiben"), a.logsEnabled(), "log protokoll", func() tea.Cmd { return a.toggleSessionLog() }),
		palToggle(ic("paste"), i18n.T("Einfügeschutz: Vorschau bei mehreren Zeilen"), a.pasteGuardEnabled(), "paste einfügen", func() tea.Cmd { return a.togglePasteGuard() }),
		palToggle(ic("danger"), i18n.T("Gefahren-Check auf allen Hosts (sonst nur Produktiv)"), a.dangerAll(), "danger gefahr", func() tea.Cmd { return a.toggleDangerAll() }),
		palToggle(ic("news"), i18n.T("Meldung, wenn lange Befehle im Hintergrund fertig sind"), a.notifyOn(), "notify benachrichtigung glocke bell", func() tea.Cmd { return a.toggleNotify() }),
		palToggle(ic("news"), i18n.T("Desktop-Benachrichtigung, wenn sshTermy im Hintergrund ist"), a.desktopNotifyOn(), "notify desktop benachrichtigung", func() tea.Cmd { return a.toggleDesktopNotify() }),
		palToggle(ic("paste"), i18n.T("Server darf in die Zwischenablage kopieren (OSC 52)"), a.osc52On(), "clipboard zwischenablage osc52 tmux vim", func() tea.Cmd { return a.toggleOSC52() }),
		palItem{group: pgSetBehave, menu: "settings", icon: ic("sshkey"), label: i18n.T("Automatisch sperren bei Leerlauf"), hint: lockLabel(a.st.Settings().LockAfterMin),
			keys: "lock sperren idle leerlauf", keep: true, run: func() tea.Cmd { return a.cycleLockAfter() }},
		palToggle(ic("intro"), i18n.T("Startanimation"), a.introOn(), "intro animation", func() tea.Cmd { return a.toggleIntro() }),
		palToggle(ic("explain"), i18n.T("Tastenzeile: Tasten erklären"), a.cheatOn(), "cheat fußzeile footer", func() tea.Cmd { return a.toggleCheat() }),
	}
	cur := a.st.Settings().Theme
	if cur != CustomName {
		cur = ThemeByName(cur).Name
	}
	for _, t := range Themes {
		t := t
		hint := ""
		if cur == t.Name {
			hint = "✓ " + i18n.T("aktiv")
		}
		items = append(items, palItem{group: pgSetLook, menu: "settings", icon: ic("theme"), label: i18n.T("Aussehen: ") + i18n.T(t.Label), hint: hint,
			keys: "theme farbe color", keep: true, run: func() tea.Cmd { return a.setTheme(t.Name) }})
	}
	colHint := i18n.T("Editor")
	if cur == CustomName {
		colHint = "✓ " + i18n.T("aktiv")
	}
	lang := "Deutsch"
	if i18n.English() {
		lang = "English"
	}
	items = append(items,
		palItem{group: pgSetLook, menu: "settings", icon: ic("colors"), label: i18n.T("Farben anpassen (eigene Farben)"), hint: colHint,
			keys: "theme farbe color colors eigene custom", run: func() tea.Cmd { return a.openColors() }},
	)
	for _, st := range iconStyles {
		st := st
		hint := i18n.T(st.sample)
		if iconSet == st.id {
			hint = "✓ " + i18n.T("aktiv")
		}
		items = append(items, palItem{group: pgSetLook, menu: "settings", icon: ic("icons"), label: i18n.T("Symbole: ") + i18n.T(st.label), hint: hint,
			keys: "icons symbole nerd font unicode minimal tags", keep: true, run: func() tea.Cmd { return a.chooseIcons(st.id) }})
	}
	items = append(items,
		palItem{group: pgSetLook, menu: "settings", icon: ic("language"), label: "Sprache / Language", hint: lang, keys: "english deutsch german",
			keep: true, run: func() tea.Cmd { return a.toggleLang() }},
		palItem{group: pgSetSetup, menu: "settings", icon: ic("ai"), label: i18n.T("KI-Einstellungen"), hint: "s", keys: "ai llm", run: func() tea.Cmd { return a.openSettings() }},
		palItem{group: pgSetSetup, menu: "settings", icon: ic("ai"), label: i18n.T("KI-Ersatz-Anbieter (Ollama, OpenAI, Claude)"), hint: "KI", keys: "ai llm fallback ollama openai claude", run: func() tea.Cmd { return a.openFallbackForm() }},
		palItem{group: pgSetSetup, menu: "settings", icon: ic("llmcheck"), label: i18n.T("KI-Status: Endpunkt erreichbar? jetzt prüfen"), hint: "llm",
			run: func() tea.Cmd { a.openLLMStatus(); return a.llmProbeCmd() }},
		palItem{group: pgSetSetup, menu: "settings", icon: ic("keys"), label: i18n.T("Tastenkürzel anpassen"), hint: a.keys.label(actKeys), keys: "keys", run: func() tea.Cmd { return a.openKeys() }},
		palItem{group: pgSetSetup, menu: "settings", icon: ic("sync"), label: i18n.T("Sync zwischen Rechnern einrichten"), hint: "y", keys: "sync", run: func() tea.Cmd { return a.openSync() }},
		palItem{group: pgSetSetup, menu: "settings", icon: ic("relay"), label: i18n.T("Systeme über Vermittler (Server hinter NAT/Firewall)"), hint: i18n.T("Vermittler"), keys: "relay", run: func() tea.Cmd { return a.openRelaySystems() }},
		palItem{group: pgSetSetup, menu: "settings", icon: ic("relay"), label: i18n.T("Vermittler koppeln / verwalten (eigener oder Speech on DGX Spark)"), hint: i18n.T("Vermittler"), keys: "relay", run: func() tea.Cmd { return a.openRelayManage() }},
		palItem{group: pgSetSetup, menu: "settings", icon: ic("import"), label: i18n.T("Verbindungen importieren (ssh-config, PuTTY, MobaXterm)"), hint: a.keys.label(actImport), keys: "import", run: func() tea.Cmd { return a.openImport() }},

		palItem{group: pgHelp, menu: "help", icon: ic("help"), label: i18n.T("Tastenkürzel anzeigen"), hint: a.keys.label(actHelp), keys: "help keys", run: func() tea.Cmd { a.modal = a.helpModal(); return nil }},
		palItem{group: pgHelp, menu: "help", icon: ic("tour"), label: i18n.T("Einführungstour"), hint: i18n.T("5 Schritte"), keys: "tour", run: func() tea.Cmd { a.modal = a.tourModal(); return nil }},
		palItem{group: pgHelp, menu: "help", icon: ic("news"), label: i18n.T("Was ist neu (Changelog)"), hint: a.keys.label(actAbout) + " w", run: func() tea.Cmd { a.modal = a.changelogModal(nil); return nil }},
		palItem{group: pgHelp, menu: "help", icon: ic("about"), label: i18n.T("Über sshTermy / Update"), hint: a.keys.label(actAbout), keys: "about version", run: func() tea.Cmd { a.modal = a.aboutModal(); return nil }},
	)
	return items
}

// palRun führt einen Eintrag aus. Schalter (keep) lassen die Palette offen,
// damit man mehrere nacheinander umlegen kann.
func (a *App) palRun(it palItem) tea.Cmd {
	if !it.keep {
		a.pal = nil
		return it.run()
	}
	p := a.pal
	cmd := it.run()
	if a.pal == p && it.menu != "" {
		// Theme oder Sprache kann gewechselt haben: Feld neu aufbauen.
		v := p.in.Value()
		p.in = newPaletteInput()
		p.in.SetValue(v)
		return tea.Batch(cmd, p.in.Focus())
	}
	return cmd
}

// palAskItem: Eintrag „KI fragen“, den Tab auslöst. Den gibt es nur mit
// Text in einer verbundenen Sitzung; sonst hat Tab keine Wirkung und wird
// auch nicht angezeigt.
func palAskItem(items []palItem) (palItem, bool) {
	for _, it := range items {
		if it.group == pgAI && strings.HasPrefix(it.label, i18n.T("KI fragen")) {
			return it, true
		}
	}
	return palItem{}, false
}

func (a *App) paletteKey(k tea.KeyPressMsg) tea.Cmd {
	p := a.pal
	items := a.paletteItems()
	switch key := k.String(); {
	case key == "esc" && p.menu != "":
		a.palEnter("")
		return nil
	case key == "left" && p.menu != "" && p.in.Value() == "":
		a.palEnter("")
		return nil
	case key == "esc" || isPaletteKey(key):
		a.pal = nil
		return nil
	case key == "up" || key == "shift+tab":
		p.sel, p.armed = max(0, p.sel-1), -1
		return nil
	case key == "down":
		p.sel, p.armed = min(len(items)-1, p.sel+1), -1
		return nil
	case key == "tab":
		if it, ok := palAskItem(items); ok {
			a.pal = nil
			return it.run()
		}
		return nil
	case key == "enter":
		if p.sel >= len(items) {
			return nil
		}
		it := items[p.sel]
		if it.danger != "" && p.armed != p.sel {
			p.armed = p.sel
			return nil
		}
		return a.palRun(it)
	}
	var cmd tea.Cmd
	before := p.in.Value()
	p.in, cmd = p.in.Update(k)
	if p.in.Value() != before {
		p.sel, p.armed = 0, -1
	}
	return cmd
}

func (a *App) viewPalette() string {
	st := a.s
	p := a.pal
	w := min(76, a.w-6)
	items := a.paletteItems()
	if p.sel >= len(items) {
		p.sel = max(0, len(items)-1)
	}
	p.in.SetWidth(w - 8)
	var b strings.Builder
	b.WriteString(promptBlock(st.Accent.Render("$ "), p.in.View()) + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(st.T.Border).Render(strings.Repeat("─", w-4)) + "\n")
	maxRows := max(5, a.h-12)
	// Fenster um die Auswahl
	start := 0
	if p.sel >= maxRows {
		start = p.sel - maxRows + 1
	}
	last := palGroup(-1)
	rows := 0
	for i := start; i < len(items) && rows < maxRows; i++ {
		it := items[i]
		if it.group != last {
			last = it.group
			head := i18n.T(palGroupName[it.group])
			if it.group == pgConns {
				head = a.palConnHeader(p.in.Value())
			}
			b.WriteString(st.Muted.Render(groupHead(head)) + "\n")
		}
		iconStyle := st.Accent
		switch it.group {
		case pgAI:
			iconStyle = st.AI
		case pgSuggest:
			iconStyle = st.OK
		case pgRunbook:
			iconStyle = st.AI
		}
		if it.group == pgConns && it.more {
			iconStyle = st.Muted
		}
		if it.iconColor != nil {
			iconStyle = lipgloss.NewStyle().Foreground(it.iconColor)
		}
		it.icon = padIcon(it.icon)
		iw := lipgloss.Width(it.icon) - 1
		hint := st.Muted.Render(it.hint)
		switch it.state {
		case palOn:
			hint = st.OK.Render("● " + i18n.T("an"))
		case palOff:
			hint = st.Muted.Render("○ " + i18n.T("aus"))
		}
		if it.danger != "" {
			hint = st.Danger.Render("▲ " + it.danger)
			if p.armed == i {
				hint = st.Danger.Render(i18n.T("nochmal ⏎"))
			}
		}
		label := trunc(it.label, w-8-iw-lipgloss.Width(hint))
		sub := ""
		if room := w - 11 - iw - lipgloss.Width(label) - lipgloss.Width(hint); it.sub != "" && room >= 6 {
			sub = "  " + trunc(it.sub, room)
		}
		gap := max(1, w-6-iw-lipgloss.Width(label)-lipgloss.Width(sub)-lipgloss.Width(hint))
		labelStyle := st.Text
		if it.more {
			labelStyle = st.Muted
		}
		line := iconStyle.Render(it.icon) + " " + labelStyle.Render(label) + st.Muted.Render(sub) + strings.Repeat(" ", gap) + hint
		if i == p.sel {
			line = st.Selected.Width(w - 4).Render(it.icon + " " + label + sub + strings.Repeat(" ", gap) + ansiPlain(hint))
		}
		i, it := i, it
		line = a.z.click(line, func(tea.Mouse, bool) tea.Cmd {
			p.sel = i
			if it.danger != "" && p.armed != i {
				p.armed = i // rot: erst scharf schalten, zweiter Klick führt aus
				return nil
			}
			return a.palRun(it)
		})
		b.WriteString(line + "\n")
		rows++
	}
	if len(items) == 0 {
		b.WriteString(st.Muted.Render(i18n.T("Nichts gefunden. Im Terminal wird Text zum Ziel oder zur Frage.")) + "\n")
	}
	keys := [][2]string{{"↑↓", i18n.T("wählen")}, {"⏎", i18n.T("ausführen")}}
	if _, ok := palAskItem(items); ok {
		keys = append(keys, [2]string{"Tab", i18n.T("KI fragen")})
	}
	keys = append(keys, [2]string{"Esc", i18n.T("schließen")})
	title := i18n.T("Befehlspalette")
	if p.menu != "" {
		keys = [][2]string{{"↑↓", i18n.T("wählen")}, {"⏎", i18n.T("ausführen / umschalten")}, {"Esc", i18n.T("zurück")}}
		title += " › " + i18n.T(palMenuLabel(p.menu))
	}
	box := st.dialog(dialog{icon: "◆", title: title, sub: fmt.Sprintf(i18n.T("%d Einträge"), len(items)), tone: st.T.AI,
		body: b.String(), keys: keys, width: w - 4, compact: true}, a.w)
	return a.z.mark(box, zoneAction{block: true, wheel: func(up bool) tea.Cmd {
		if up {
			p.sel = max(0, p.sel-1)
		} else {
			p.sel = min(len(items)-1, p.sel+1)
		}
		p.armed = -1
		return nil
	}})
}

func ansiPlain(s string) string { return lipgloss.NewStyle().Render(ansi.Strip(s)) }
