package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/fullbackup"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/relay"
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
	pgSetSafe
	pgSetLook
	pgSetSetup
	pgHelp
	pgTop   // Übersicht: häufige Aktionen
	pgAreas // Übersicht: Ordner der Bereiche, Einstellungen, Hilfe
)

var palGroupName = map[palGroup]string{pgAI: "KI", pgSuggest: "VORSCHLÄGE", pgQuick: "SCHNELLBEFEHLE", pgRunbook: "RUNBOOKS", pgTunnel: "TUNNEL", pgConnect: "VERBINDEN", pgAction: "AKTIONEN", pgConns: "VERBINDUNGEN",
	pgSetBehave: "ALLGEMEIN", pgSetSafe: "SICHERHEIT", pgSetLook: "AUSSEHEN", pgSetSetup: "EINRICHTEN", pgHelp: "HILFE & INFO",
	pgTop: "HÄUFIG", pgAreas: "ALLE AKTIONEN"}

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
	// cycle: Durchschalt-Eintrag (‹ Wert › rechts). ⏎, → und Klick gehen
	// einen Wert weiter, ← einen zurück.
	cycle func(dir int) tea.Cmd
	// hidden: nur bei getipptem Text sichtbar (Einzelwerte hinter einem
	// Durchschalt-Eintrag, damit die Suche z. B. „Matrix“ direkt findet).
	hidden bool
	// pro: Profi-Funktion, im einfachen Modus nur über die Suche (simple.go)
	pro bool
	// open: Eintrag öffnet ein Untermenü (z. B. "settings/ki" oder "view").
	open string
	// area: Bereich der Übersicht (palAreas), in dessen Ordner der Eintrag
	// steht; "" = direkt in der Übersicht.
	area string
	// top: zusätzlich oben unter „Häufig“.
	top bool
	// pos: Position des Werts (i von n) bei Durchschalt-Einträgen, für die
	// Punkte ●○○ (palstyle.go); n = 0 heißt keine Punkte.
	pos [2]int
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
	menu  string    // offenes Untermenü, "" = Übersicht
	mo    palMotion // Übergänge (motion2.go)
	tab   palGroup  // Reiter der Einstellungen (palstyle.go)
}

func isPaletteKey(k string) bool { return activeKeys.is(actPalette, k) }

func newPaletteInput() aiInput {
	in := newAIInput(i18n.T("Ziel, Frage, Befehl, Tunnel, Verbindung oder user@host:port …"))
	in.MaxHeight = 4
	return in
}

func (a *App) openPalette() tea.Cmd {
	a.pal = &palette{in: newPaletteInput(), armed: -1, tab: pgSetBehave}
	return a.pal.in.Focus()
}

func (a *App) paletteItems() []palItem {
	q := strings.TrimSpace(a.pal.in.Value())
	ql := strings.ToLower(q)
	match := func(x string) bool { return ql == "" || strings.Contains(strings.ToLower(x), ql) }
	if m := a.pal.menu; m != "" && !palIsArea(m) {
		return a.palSubmenu(m, ql, match)
	}
	s := a.current()
	inTerm := a.scr == scrTerminal && s != nil && s.state == stConnected
	inMgr := a.scr == scrManager
	items := a.palDirect(q, s, inTerm, match)
	// Aktionen: jede gehört zu einem Bereich (palAreas). Ohne Text zeigt die
	// Übersicht nur die häufigen (top) und je Bereich einen Ordner; mit Text
	// kommen alle Treffer direkt.
	var actions []palItem
	add := func(area string, its ...palItem) {
		for _, it := range its {
			if it.area == "" {
				it.area = area
			}
			actions = append(actions, it)
		}
	}
	if inTerm {
		a.palActsSession(add, s)
	}
	a.palActsGlobal(add, inMgr)
	a.palActsFind(add, s, inTerm, inMgr)
	a.palActsTerminal(add, s, inTerm)
	if inMgr {
		add("tabs", palItem{icon: ic("split"), label: i18n.T("Arbeitsplätze …"), hint: a.keys.label(actWorkspace), keys: "workspace layout", run: func() tea.Cmd { return a.openWorkspaces() }})
	}
	add("", palItem{icon: ic("sshkey"), label: i18n.T("Jetzt sperren"), hint: i18n.T("Master-Passwort"), run: func() tea.Cmd { return a.lockNow() }})
	actions = a.simpleHide(actions) // einfacher Modus (simple.go)
	for i := range actions {
		actions[i].group = pgAction
	}
	for i := range items {
		if g := items[i].group; g == pgQuick || g == pgRunbook || g == pgTunnel {
			items[i].area = "quick"
		}
	}
	if area := a.pal.menu; area != "" {
		// Ein Bereich ist offen: seine Einträge, gefiltert nach dem Text.
		var out []palItem
		for _, x := range append(items, actions...) {
			if x.area == area && (!x.hidden || ql != "") && x.matches(match) {
				x.top = false
				out = append(out, x)
			}
		}
		return out
	}
	if ql == "" {
		return a.palOverview(items, actions)
	}
	for _, x := range actions {
		if x.matches(match) {
			items = append(items, x)
		}
	}
	// Ordner (Bereiche, Einstellungen, Hilfe) mit passendem Namen, dazu die
	// Treffer aus Einstellungen und Hilfe (mit dem Untermenü grau dahinter).
	for _, f := range a.palFolders(append(items, actions...)) {
		if match(f.label + " " + f.keys) {
			items = append(items, f)
		}
	}
	for _, x := range a.palMenuItems() {
		if x.open == "" && x.matches(match) {
			x.sub = "· " + palMenuPath(x.menu)
			x.group = pgAction
			items = append(items, x)
		}
	}
	return items
}

// palAdd hängt Aktionen eines Bereichs an die Liste der Palette.
type palAdd func(area string, its ...palItem)

// palSubmenu: ein Untermenü (Einstellungen, Hilfe) ist offen.
func (a *App) palSubmenu(m, ql string, match func(string) bool) []palItem {
	// Ohne Text: nur die Einträge dieser Ebene. Mit Text: Treffer auch
	// aus den Unterordnern und die Einzelwerte hinter Durchschalt-Einträgen.
	var items []palItem
	for _, x := range a.palMenuItems() {
		switch {
		case ql == "":
			// Einstellungen: nur der offene Reiter (palstyle.go).
			if x.menu == m && !x.hidden && (m != "settings" || x.group == a.pal.tab) {
				items = append(items, x)
			}
		case x.open == "" && (x.menu == m || strings.HasPrefix(x.menu, m+"/")) && x.matches(match):
			if x.menu != m {
				x.sub = "· " + i18n.T(palMenuLabel(x.menu))
			}
			items = append(items, x)
		}
	}
	return items
}

// palDirect: Einträge, die sich aus dem getippten Text und der aktuellen
// Sitzung ergeben (verbinden, KI, Vorschläge, Schnellbefehle, Runbooks,
// Tunnel, Verbindungen).
func (a *App) palDirect(q string, s *session, inTerm bool, match func(string) bool) []palItem {
	var items []palItem
	quick, quickFirst := a.quickItems(q)
	if quickFirst {
		items = append(items, quick...) // user@host:port: Verbinden ist gemeint
	}
	if relay.IsShareLink(q) || relay.IsInvite(q) {
		items = append(items, palItem{group: pgConnect, icon: ic("team"), label: i18n.T("Geteilte Sitzung ansehen"), hint: i18n.T("Code"),
			run: func() tea.Cmd { return a.watchLink(q) }})
	}
	if looksLikeAddr(q) && len(a.netDevices("")) > 0 {
		items = append(items, palItem{icon: ic("scan"), label: i18n.T("Gerät finden: ") + q, hint: i18n.T("MAC/IP → Switch-Port"),
			run: func() tea.Cmd { return a.netFind(q) }})
	}
	if q != "" && inTerm {
		items = append(items,
			palItem{group: pgAI, icon: ic("goal"), label: i18n.T("Als Ziel starten: „") + trunc(oneLine(q), 50) + i18n.T("“"), hint: "⏎",
				run: func() tea.Cmd { return a.startGoal(s, q) }},
			palItem{group: pgAI, icon: ic("ai"), label: i18n.T("KI fragen: „") + trunc(oneLine(q), 50) + i18n.T("“"), hint: "Tab",
				run: func() tea.Cmd { return a.startAsk(s, q) }},
			palItem{group: pgAI, icon: ic("rewrite"), label: i18n.T("Für dieses System umschreiben: „") + trunc(oneLine(q), 40) + i18n.T("“"), hint: i18n.T("Befehl"),
				run: func() tea.Cmd { return a.startAdapt(s, oneLine(q)) }})
	}
	if inTerm && s.fail != nil && match("fehler warum fix error why "+s.fail.Command) {
		items = append(items, palItem{group: pgAI, icon: ic("fix"), label: i18n.T("Warum ist „") + trunc(oneLine(s.fail.Command), 40) + i18n.T("“ fehlgeschlagen? Fix vorschlagen"), hint: "Alt+0",
			run: func() tea.Cmd { return a.explainFailure(s) }})
	}
	if hasSel := inTerm && s.sel != nil && !s.sel.Empty(); hasSel || (inTerm && q != "" && match(i18n.T("KI: Log/Ausgabe auswerten")+" log diagnose journalctl syslog analyse auswerten")) {
		label := i18n.T("KI: Log/Ausgabe auswerten (letzte 400 Zeilen)")
		if hasSel {
			label = i18n.T("KI: markierte Ausgabe auswerten")
		}
		items = append(items, palItem{group: pgAI, icon: ic("ai"), label: label, hint: i18n.T("Diagnose"),
			run: func() tea.Cmd { return a.startDiagnose(s) }})
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
	return items
}

// palActsSession: Aktionen der verbundenen Sitzung, die oben stehen.
func (a *App) palActsSession(add palAdd, s *session) {
	add("ai",
		palItem{icon: ic("goal"), label: i18n.T("Ziel-Modus öffnen"), hint: a.keys.label(actGoal), top: true, run: func() tea.Cmd {
			withPanel(s, panelGoal)
			a.resize(s)
			return a.panelOpened(s)
		}},
		palItem{icon: ic("goal"), label: i18n.T("Agent-Modus öffnen (läuft selbstständig)"), pro: true, run: func() tea.Cmd {
			withPanel(s, panelGoal)
			a.resize(s)
			cmd := a.panelOpened(s)
			if !s.goal.active {
				s.goal.auto = true
			}
			return cmd
		}},
		palItem{icon: ic("ai"), label: i18n.T("KI fragen"), hint: a.keys.label(actAsk), run: func() tea.Cmd { withPanel(s, panelAsk); a.resize(s); return a.panelOpened(s) }},
		palItem{area: "session", icon: ic("open"), label: i18n.T("Dateien (SFTP)"), hint: a.keys.label(actFiles), run: func() tea.Cmd { return a.toggleFiles(s) }},
		palItem{area: "session", icon: ic("host"), label: i18n.T("Host-Übersicht"), hint: a.keys.label(actHost), top: true, run: func() tea.Cmd {
			withPanel(s, panelHost)
			a.resize(s)
			return a.panelOpened(s)
		}},
		palItem{icon: ic("ai"), label: i18n.T("KI-Begleiter"), hint: companionLevelLabel(a.compLevel()), keep: true,
			keys:  "begleiter companion mitlesen kommentar coach stufe " + a.keys.label(actCompanion),
			cycle: func(d int) tea.Cmd { return a.cycleCompanion(d) }, run: func() tea.Cmd { return a.cycleCompanion(1) }},
		palItem{icon: ic("ai"), label: i18n.T("Begleiter-Leiste"), pro: true, hint: a.keys.label(actCompanionBar),
			keys: "begleiter companion leiste feed", run: func() tea.Cmd { return a.toggleCompanionBar(s) }},
		palItem{icon: ic("ai"), label: i18n.T("Begleiter: Rückblick dieser Sitzung"), pro: true, hint: i18n.T("merken, Runbook"),
			keys: "begleiter companion rückblick recap zusammenfassung runbook", run: func() tea.Cmd { return a.openRecap(s, false, nil) }},
		palItem{icon: ic("ai"), label: i18n.T("Vorgang über mehrere Fenster (KI sieht alle)"), hint: a.keys.label(actVorgang), top: true,
			keys: "vorgang mehrere fenster multi window alle tabs fahrplan bahnen task", run: func() tea.Cmd { return a.toggleVorgang() }})
	add("quick",
		palItem{icon: ic("tunnel"), label: i18n.T("Tunnel-Panel"), hint: a.keys.label(actTunnels), run: func() tea.Cmd {
			withPanel(s, panelTunnels)
			a.resize(s)
			return nil
		}},
		palItem{icon: ic("quick"), label: i18n.T("Schnellbefehle bearbeiten"), hint: i18n.T("Liste"), run: func() tea.Cmd { return a.editQuickFor(s) }})
}

// palActsGlobal: Aktionen, die es überall gibt (Verbindungen, mehrere
// Hosts, Netzwerk, Team).
func (a *App) palActsGlobal(add palAdd, inMgr bool) {
	mk := func(k string) string { // Manager-Tasten nur im Manager anzeigen
		if inMgr {
			return k
		}
		return ""
	}
	if a.restartPending != "" {
		add("", palItem{icon: ic("restart"), label: i18n.T("Neu starten (v") + a.restartPending + i18n.T(" aktivieren)"), hint: "Update", top: true, run: func() tea.Cmd { return a.askRestart() }})
	}
	add("conns",
		palItem{icon: ic("new"), label: i18n.T("Neue Verbindung"), hint: mk("n"), top: inMgr, run: func() tea.Cmd { return a.openConnForm(newConnDefaults()) }},
		palItem{icon: ic("sshkey"), label: i18n.T("SSH-Schlüssel erzeugen / auf den Server übertragen"), hint: mk(a.keys.label(actSSHKey)), run: func() tea.Cmd {
			if c, ok := a.sshKeyTarget(); ok {
				return a.openSSHKeyForm(c)
			}
			a.flash(i18n.T("Erst eine Verbindung im Manager wählen oder verbinden"))
			return nil
		}})
	if !inMgr {
		add("conns", palItem{icon: ic("manager"), label: i18n.T("Verbindungs-Manager"), hint: a.keys.label(actManager), run: func() tea.Cmd { a.scr = scrManager; return nil }})
	}
	add("conns", a.palIOItems(inMgr)...)
	if inMgr {
		if c, ok := a.selectedConn(); ok && !c.Protocol.NoHost() && c.JumpHostID == "" && !a.connected(c.ID) {
			add("conns", palItem{icon: ic("connect"), label: i18n.F("%s aufwecken (Wake-on-LAN)", c.Alias), hint: orDefault(c.MAC, i18n.T("MAC fehlt")), run: func() tea.Cmd { return a.wake(c) }})
		}
		for _, it := range a.localPalette() {
			it.top = true
			it.label = i18n.T("Lokale Shell in neuem Tab")
			add("tabs", it)
		}
	}
	if a.st.SyncConfig().Enabled() {
		add("conns", palItem{icon: ic("sync"), label: i18n.T("Jetzt synchronisieren"), hint: "Sync", run: func() tea.Cmd {
			a.flash(i18n.T("Sync läuft …"))
			return a.runSync()
		}})
	}
	add("net",
		palItem{icon: ic("broadcast"), label: i18n.T("Broadcast: Befehl auf mehreren Hosts"), hint: mk(a.keys.label(actBroadcast)), top: inMgr, run: func() tea.Cmd { return a.openBroadcast() }},
		palItem{icon: ic("runbook"), label: i18n.T("Zeitpläne: Broadcast/Runbook nach Uhrzeit"), hint: mk(a.keys.label(actSchedule)), run: func() tea.Cmd { return a.openSchedules() }},
		palItem{icon: ic("scan"), label: i18n.T("Netzwerk-Scan: SSH-Geräte finden"), hint: mk(a.keys.label(actScan)), top: inMgr, run: func() tea.Cmd { return a.openScan() }})
	add("net",
		palItem{icon: ic("scan"), label: i18n.T("Gerät finden: MAC/IP → Switch-Port …"), hint: i18n.T("Switches"),
			keys: "mac ip port vlan switch finden suchen wo hängt locate find", run: func() tea.Cmd { return a.askNetFind() }},
		palItem{icon: ic("scan"), label: i18n.T("Netzwerk-Topologie (LLDP/CDP)"), hint: i18n.T("Switches"),
			keys: "topologie topology lldp cdp nachbarn neighbors karte map plan", run: func() tea.Cmd { return a.netTopology() }})
	add("team",
		palItem{icon: ic("team"), label: i18n.T("Geteilte Sitzung ansehen …"), hint: i18n.T("Code"),
			keys: "zusehen watch link code teilen share live einladung", run: func() tea.Cmd { return a.askWatch() }},
		palItem{icon: ic("team"), label: i18n.T("Mit dem Team teilen (verschlüsselt, ohne Passwörter)"), hint: "Team", run: func() tea.Cmd { a.scr = scrManager; return a.openTeamShare() }},
		palItem{icon: ic("team"), label: i18n.T("Team-Paket übernehmen"), hint: "Team", run: func() tea.Cmd { return a.openTeamTake() }},
		palItem{icon: ic("team"), label: i18n.T("Team-Ordner (veröffentlicht / abonniert)"), hint: "Team", run: func() tea.Cmd { return a.openTeamLinks() }})
}

// palActsFind: Bereich „Suchen & Logs“: die Suchen im Verlauf, Log-Blick,
// Sitzungsprotokolle, Ordner-Scan und Logwache.
func (a *App) palActsFind(add palAdd, s *session, inTerm, inMgr bool) {
	if a.scr == scrTerminal && s != nil {
		add("find",
			palItem{icon: ic("search"), label: i18n.T("Im Verlauf suchen"), hint: "Strg+F", top: true, run: func() tea.Cmd { return a.openSearch(s) }},
			palItem{icon: ic("search"), label: i18n.T("In allen Tabs suchen"), hint: a.keys.label(actSearchAll), keys: "suche find grep verlauf alle tabs", run: func() tea.Cmd { return a.openSearchAll() }},
			palItem{icon: ic("command"), label: i18n.T("Befehle dieses Tabs (Ausgabe kopieren, an KI)"), hint: a.keys.label(actCmdList), keys: "befehle blöcke blocks ausgabe output kopieren copy commands", run: func() tea.Cmd { return a.openBlocks(s) }},
			palItem{icon: ic("errors"), label: i18n.T("Fehlerzeilen im Verlauf zeigen"), hint: i18n.T("Strg+F Strg+E"), run: func() tea.Cmd {
				cmd := a.openSearch(s)
				s.search.errors, s.search.list = true, true
				a.refreshSearch(s, true)
				return cmd
			}})
	}
	if inTerm {
		add("find", palItem{icon: ic("errors"), label: i18n.T("Log-Blick: Auffälligkeiten in den Logs"), hint: a.keys.label(actLogScan), top: true,
			keys: "logs log journal journalctl syslog eventlog ereignis ereignisanzeige auffälligkeiten fehler show logging", run: func() tea.Cmd { return a.openLogScan(s) }})
	}
	logsKey := ""
	if inMgr {
		logsKey = "l"
	}
	add("find",
		palItem{icon: ic("logs"), label: i18n.T("Sitzungsprotokolle durchsuchen"), hint: logsKey, top: inMgr, run: func() tea.Cmd { return a.openLogs() }},
		palItem{icon: ic("errors"), label: i18n.T("Ordner-Scan: Logs aller Hosts im Ordner …"), hint: i18n.T("Logs"),
			keys: "ordner scan folder logs log blick alle hosts auffälligkeiten journal eventlog", run: func() tea.Cmd { return a.openFolderScan() }})
	if len(a.lfold.hosts) > 0 {
		add("find", palItem{icon: ic("errors"), label: i18n.T("Ordner-Scan: letztes Ergebnis"), hint: a.lfold.folder,
			keys: "ordner scan ergebnis folder logs", run: func() tea.Cmd { a.showFolderScan(); return nil }})
	}
	add("find", palItem{icon: ic("errors"), label: a.watchMenuLabel(), hint: watchLabel(a.watchLevel()),
		keys: "logwache log watch syslog alarm meldungen journal überwachen", run: func() tea.Cmd { return a.openWatchList() }})
}

// palActsTerminal: weitere Aktionen der Sitzung, Ansicht und Tabs.
func (a *App) palActsTerminal(add palAdd, s *session, inTerm bool) {
	if inTerm {
		if s.isSerial() && s.state == stConnected {
			add("session", palItem{icon: ic("break"), label: i18n.T("Break senden (serielle Konsole)"), hint: i18n.T("Konsole"), run: func() tea.Cmd { return a.sendBreak(s) }})
		}
		if o := s.share.Load(); o != nil {
			add("team",
				palItem{icon: ic("team"), label: fmt.Sprintf(i18n.T("Teilen verwalten (%d Zuschauer)"), len(o.guestNames())), hint: i18n.T("live"),
					keys: "teilen share live verwalten zuschauer mitschreiben code einladung stop beenden", run: func() tea.Cmd { return a.openShareManage(s, o) }},
				palItem{icon: ic("team"), label: i18n.T("Teilen beenden"), hint: i18n.T("live"),
					keys: "teilen share live stop beenden", run: func() tea.Cmd { a.stopShare(s); return nil }})
		} else if s.conn.Protocol == domain.ProtoWatch {
			if _, ok := s.shell.(*watch2Stream); ok {
				add("team", palItem{icon: ic("team"), label: i18n.T("Hand heben (um Mitschreiben bitten)"), hint: i18n.T("Zusehen"),
					keys: "hand heben mitschreiben tippen bitten raise", run: func() tea.Cmd { return a.watchHand(s) }})
			}
		} else if s.state == stConnected {
			add("team", palItem{icon: ic("team"), label: i18n.T("Sitzung live teilen (Code für Zuschauer) …"), hint: i18n.T("Netz/Vermittler"),
				keys: "teilen share live zuschauen zeigen kollege screen einladen code", run: func() tea.Cmd { return a.startShare(s) }})
		}
		if a.containersOK(s) {
			add("session", palItem{icon: ic("host"), label: i18n.T("Container (Docker, Podman, Kubernetes)"), hint: i18n.T("F6 c"),
				keys: "docker podman kubernetes k8s kubectl pods container logs", run: func() tea.Cmd { return a.openContainers(s) }})
		}
		if s.client != nil {
			add("session", palItem{icon: ic("open"), label: i18n.T("Server-Datei im eigenen Editor öffnen …"), hint: i18n.T("F5 e"),
				keys: "edit editor bearbeiten vscode notepad datei file", run: func() tea.Cmd { return a.askEditPath(s) }})
		}
		add("session",
			palItem{icon: ic("record"), label: recordLabel(s.recording()), pro: true, hint: "asciinema", run: func() tea.Cmd { return a.toggleRecording(s) }},
			palItem{icon: ic("export"), label: i18n.T("Terminal-Verlauf als Text exportieren"), hint: ".txt", run: func() tea.Cmd { return a.exportScrollback(s) }})
		if s.netDevice() {
			add("net", palItem{icon: ic("errors"), label: i18n.T("Syslog an sshTermy senden (Gerät einrichten) …"), hint: i18n.T("Logwache"),
				keys: "syslog logging remote logwache einrichten switch firewall", run: func() tea.Cmd { return a.askSyslogSetup(s) }})
			add("net", palItem{icon: ic("backup"), label: i18n.T("Switch-Config sichern und vergleichen"), hint: "b", run: func() tea.Cmd {
				withPanel(s, panelHost)
				s.panelFocus = false
				a.resize(s)
				s.host.backups = a.st.Backups(s.conn.ID)
				return a.hostNetCmd(s, false, true)
			}})
		}
	}
	if a.scr == scrTerminal && s != nil {
		add("tabs", palItem{icon: ic("split"), label: i18n.T("Aufteilung wählen …"), hint: a.keys.label(actSplit), keys: "split teilen layout", top: true,
			run: func() tea.Cmd { return a.openLayout() }})
		cur := 0
		for i, m := range layoutOrder {
			if m == a.split {
				cur = i
			}
		}
		add("tabs", palItem{icon: ic("split"), label: i18n.T("Aufteilung"), hint: layoutOrder[cur].label(), keep: true, pos: [2]int{cur, len(layoutOrder)},
			cycle: func(d int) tea.Cmd { a.setSplit(layoutOrder[cycleIndex(cur, len(layoutOrder), d)]); return nil },
			run:   func() tea.Cmd { a.setSplit(layoutOrder[cycleIndex(cur, len(layoutOrder), 1)]); return nil }})
		for _, m := range layoutOrder {
			if m == a.split {
				continue
			}
			m := m
			label := i18n.T("Ansicht teilen: ") + m.label()
			if m == splitOff {
				label = i18n.T("Ansicht nicht mehr teilen")
			}
			add("tabs", palItem{icon: ic("split"), label: label, hidden: true, run: func() tea.Cmd { a.setSplit(m); return nil }})
		}
		if a.split != splitOff {
			zl := i18n.T("Bereich zoomen (ganzes Fenster)")
			if a.zoom {
				zl = i18n.T("Zoom beenden (alle Bereiche zeigen)")
			}
			add("tabs",
				palItem{icon: ic("split"), label: zl, hint: a.keys.label(actZoom), run: func() tea.Cmd { a.toggleZoom(); return nil }},
				palItem{icon: ic("split"), label: i18n.T("Alle Bereiche gleich groß"), hint: "Strg+T =", run: func() tea.Cmd { a.equalize(); return nil }})
		}
		add("tabs",
			palItem{icon: ic("split"), label: i18n.T("Arbeitsplätze …"), keys: "workspace layout", run: func() tea.Cmd { return a.openWorkspaces() }},
			palItem{icon: ic("split"), label: i18n.T("Ansicht als Arbeitsplatz speichern"), keys: "workspace save", run: func() tea.Cmd { return a.saveWorkspaceAsk() }})
		if a.split != splitOff {
			label := i18n.T("Eingabe an alle geteilten Bereiche: einschalten")
			if a.syncIn {
				label = i18n.T("Eingabe an alle geteilten Bereiche: ausschalten")
			}
			add("tabs", palItem{icon: ic("broadcast"), label: label, hint: a.keys.label(actSyncInput), run: func() tea.Cmd { a.toggleSyncInput(); return nil }})
		}
		for _, it := range a.persistPalette(s) {
			it.pro = true
			add("session", it)
		}
		if s.detached() {
			add("tabs", palItem{icon: ic("attach"), label: i18n.T("Tab aus eigenem Fenster zurückholen"), hint: a.keys.label(actDetach),
				run: func() tea.Cmd { return a.toggleDetach(s) }})
		} else if s.state == stConnected {
			add("tabs", palItem{icon: ic("detach"), label: i18n.T("Tab in eigenes Fenster abtrennen"), hint: a.keys.label(actDetach),
				run: func() tea.Cmd { return a.toggleDetach(s) }})
		}
		add("tabs",
			palItem{icon: ic("tabs"), label: i18n.T("Alle Tabs zeigen"), hint: a.keys.label(actTabList), run: func() tea.Cmd { return a.openTabList() }},
			palItem{icon: ic("new"), label: i18n.T("Neuer Tab"), hint: a.keys.label(actNewTab), top: true, run: func() tea.Cmd { return a.openNewTab() }},
			palItem{icon: ic("command"), label: i18n.T("Lokale Shell in neuem Tab"), hint: a.keys.label(actLocal), keys: "local lokal shell terminal powershell pwsh cmd zsh bash", run: func() tea.Cmd { return a.openLocal("") }},
			palItem{icon: ic("rename"), label: i18n.T("Tab umbenennen"), hint: trunc(s.tabName(), 16), run: func() tea.Cmd { return a.renameTab(s, nil) }},
			palItem{icon: ic("tabs"), label: i18n.T("Tab duplizieren"), hint: trunc(s.conn.Alias, 16), run: func() tea.Cmd { return a.duplicateTab(s) }},
			palItem{icon: ic("close"), label: i18n.T("Tab schließen"), hint: a.keys.label(actCloseTab), run: func() tea.Cmd { return a.closeTabAsk(s, nil) }})
	}
}

// ---------------------------------------------------------------- Untermenüs

type palMenu struct {
	id, icon, label, hint, keys string
}

var palMenus = []palMenu{
	{"settings", "settings", "Einstellungen", "Maus, Aussehen, Sprache, KI …", "settings optionen schalter"},
	{"help", "help", "Hilfe & Info", "Tasten, Tour, Neues, Update", "help hilfe info"},
}

// palAreas: Bereiche der Übersicht. Ohne Text steht je Bereich ein Ordner
// in der Palette; die Einträge darin tragen palItem.area.
var palAreas = []palMenu{
	{"quick", "quick", "Schnellbefehle & Tunnel", "Befehle, Runbooks, Tunnel", "schnellbefehle quick runbook tunnel"},
	{"ai", "ai", "KI", "Ziel, Agent, Frage, Vorgang, Begleiter", "ki ai"},
	{"session", "host", "Diese Sitzung", "Host, Dateien, Container, Aufnahme", "sitzung session"},
	{"find", "search", "Suchen & Logs", "Verlauf, alle Tabs, Log-Blick, Protokolle", "suchen logs"},
	{"tabs", "tabs", "Tabs & Ansicht", "neu, teilen, Zoom, Arbeitsplätze", "tabs fenster ansicht split"},
	{"net", "broadcast", "Mehrere Hosts & Netzwerk", "Broadcast, Scan, Zeitpläne, Switches", "mehrere hosts netzwerk"},
	{"team", "team", "Team & Teilen", "live teilen, Team-Ordner", "team teilen share"},
	{"conns", "manager", "Verbindungen & Daten", "neu, SSH-Key, Import/Export, Sicherung", "verbindungen import export"},
}

func palIsArea(id string) bool {
	for _, m := range palAreas {
		if m.id == id {
			return true
		}
	}
	return false
}

// palOverview: Übersicht ohne Text. Oben was gerade passt (KI, Vorschläge),
// dann „Häufig“, die Verbindungen und die Ordner.
func (a *App) palOverview(items, actions []palItem) []palItem {
	var out []palItem
	for _, x := range items {
		if x.area == "" && x.group != pgConns {
			out = append(out, x)
		}
	}
	for _, x := range actions {
		if x.top {
			x.group = pgTop
			out = append(out, x)
		}
	}
	for _, x := range items {
		if x.group == pgConns {
			out = append(out, x)
		}
	}
	out = append(out, a.palFolders(append(items, actions...))...)
	for _, x := range actions {
		if x.area == "" && !x.top && !x.hidden {
			x.group = pgAreas
			out = append(out, x)
		}
	}
	return out
}

// palFolders: je Bereich mit Einträgen ein Ordner, dazu Einstellungen und
// Hilfe & Info; rechts die Zahl der Einträge.
func (a *App) palFolders(all []palItem) []palItem {
	n := map[string]int{}
	for _, x := range all {
		if x.area != "" && !x.hidden {
			n[x.area]++
		}
	}
	for _, x := range a.palMenuItems() {
		if !x.hidden && palParent(x.menu) == "" {
			n[x.menu]++
		}
	}
	var out []palItem
	for _, m := range append(palAreas[:len(palAreas):len(palAreas)], palMenus...) {
		if n[m.id] == 0 {
			continue
		}
		id := m.id
		out = append(out, palItem{group: pgAreas, icon: ic(m.icon), label: i18n.T(m.label) + " ›", sub: i18n.T(m.hint),
			hint: fmt.Sprint(n[id]), keys: m.keys, keep: true, open: id, run: func() tea.Cmd { a.palEnter(id); return nil }})
	}
	return out
}

// Unterordner der Einstellungen: nicht in der Übersicht, sondern als
// Eintrag mit „›“ im Untermenü (palFolder).
var palSubMenus = []palMenu{
	{"settings/ki", "ai", "KI", "Endpunkt, Ersatz-Anbieter, Begleiter, Status", "ai llm ki"},
	{"settings/ki/begleiter", "ai", "KI-Begleiter", "liest mit und kommentiert: Stufe, Grenzen, Datenschutz", "begleiter companion mitlesen kommentar coach"},
	{"settings/relay", "relay", "Vermittler", "Server hinter NAT/Firewall", "relay vermittler"},
	{"settings/logs", "sesslog", "Protokolle & Logs", "Sitzungsprotokoll, Logwache, Syslog, Audit", "protokoll logs syslog audit"},
	{"settings/net", "backup", "Netzwerkgeräte", "Befehlssätze, Regeln für Switch-Configs", "switch netzwerk befehlssatz"},
	{"help/debug", "sesslog", "Fehlersuche", "Debug-Protokoll, Leistungsanzeige", "debug fehler leistung"},
}

// palParent: übergeordnetes Menü ("settings/ki" → "settings", "settings" → "").
func palParent(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return id[:i]
	}
	return ""
}

// palMenuPath: „Einstellungen › KI“ für Titel und graue Herkunft in der Suche.
func palMenuPath(id string) string {
	if id == "" {
		return ""
	}
	if p := palParent(id); p != "" {
		return palMenuPath(p) + " › " + i18n.T(palMenuLabel(id))
	}
	return i18n.T(palMenuLabel(id))
}

func palMenuLabel(id string) string {
	for _, m := range append(append(palAreas[:len(palAreas):len(palAreas)], palMenus...), palSubMenus...) {
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

// palCycle: Durchschalt-Eintrag. value ist der aktuelle Wert, set(dir)
// stellt den nächsten (dir=1) oder vorigen (dir=-1) ein.
func palCycle(group palGroup, icon, label, value, keys string, set func(dir int) tea.Cmd) palItem {
	return palItem{group: group, menu: "settings", icon: icon, label: label, hint: value, keys: keys, keep: true,
		cycle: set, run: func() tea.Cmd { return set(1) }}
}

// palFolder: Eintrag, der einen Unterordner der Einstellungen öffnet.
func (a *App) palFolder(id string) palItem {
	for _, m := range palSubMenus {
		if m.id == id {
			g := pgSetSetup
			if strings.HasPrefix(id, "help") {
				g = pgHelp
			}
			return palItem{group: g, menu: palParent(id), icon: ic(m.icon), label: i18n.T(m.label), sub: i18n.T(m.hint), hint: "›",
				keys: m.keys, keep: true, open: id, run: func() tea.Cmd { a.palEnter(id); return nil }}
		}
	}
	return palItem{}
}

// proItem: Profi-Eintrag (im einfachen Modus nur über die Suche).
func proItem(it palItem) palItem { it.pro = true; return it }

// cycleIndex: Position von cur in n Werten, um dir verschoben (rundherum).
// Unbekannter Wert: vorwärts beim ersten, rückwärts beim letzten beginnen.
func cycleIndex(cur, n, dir int) int {
	if cur < 0 {
		if dir > 0 {
			return 0
		}
		return n - 1
	}
	return ((cur+dir)%n + n) % n
}

// masterHint: wie sich der Store öffnet.
func masterHint(dpapi bool) string {
	if dpapi {
		return i18n.T("+ Windows-Anmeldung")
	}
	return ""
}

// palMenuItems: Inhalt der Untermenüs Einstellungen und Hilfe & Info samt
// Unterordnern, in Anzeigereihenfolge. Einstellungen: Allgemein, Aussehen,
// Sicherheit, Einrichten (Unterordner KI, Protokolle & Logs, Netzwerkgeräte,
// Vermittler). Hilfe & Info hat den Unterordner Fehlersuche.
func (a *App) palMenuItems() []palItem {
	set := a.st.Settings()
	safe := func(it palItem) palItem { it.group = pgSetSafe; return it }
	hide := func(it palItem) palItem { it.hidden = true; return it }
	look := func(it palItem) palItem { it.group = pgSetLook; return it }
	sub := func(menu string, it palItem) palItem { it.menu, it.group = menu, pgSetSetup; return it }
	help := func(menu string, it palItem) palItem { it.menu, it.group = menu, pgHelp; return it }
	lang := "Deutsch"
	if i18n.English() {
		lang = "English"
	}

	// Allgemein
	items := []palItem{
		palToggle(ic("explain"), i18n.T("Einfacher Modus: Profi-Funktionen ausblenden"), a.simple(),
			"einfach simple anfänger beginner profi expert ausblenden weniger", func() tea.Cmd { return a.toggleSimple() }),
		palCycle(pgSetBehave, ic("language"), "Sprache / Language", lang, "english deutsch german", func(int) tea.Cmd { return a.toggleLang() }).at(langIdx(), 2),
		palToggle(ic("mouse"), i18n.T("Maus: klicken, markieren, scrollen"), a.mouseEnabled(), "mouse", func() tea.Cmd { return a.toggleMouse() }),
		palToggle(ic("reconnect"), i18n.T("Auto-Reconnect: nach Abbruch neu verbinden"), a.autoReconnect(), "reconnect", func() tea.Cmd { return a.toggleAutoReconnect() }),
		palCycle(pgSetBehave, ic("reach"), i18n.T("Online-Punkt: Erreichbarkeit der Hosts prüfen"), reachLabel(a.reachMode()),
			"online reach erreichbar ordner zugeklappt", func(dir int) tea.Cmd { return a.cycleReach(dir) }).at(a.reachMode(), 3),
		palCycle(pgSetBehave, ic("news"), i18n.T("Meldung, wenn lange Befehle fertig sind"), notifyLabel(a.notifyLevel()),
			"notify benachrichtigung glocke bell desktop", func(dir int) tea.Cmd { return a.cycleNotify(dir) }).at(a.notifyLevel(), 3),
		hide(palToggle(ic("news"), i18n.T("Meldung, wenn lange Befehle im Hintergrund fertig sind"), a.notifyOn(), "notify benachrichtigung glocke bell", func() tea.Cmd { return a.toggleNotify() })),
		hide(palToggle(ic("news"), i18n.T("Desktop-Benachrichtigung, wenn sshTermy im Hintergrund ist"), a.desktopNotifyOn(), "notify desktop benachrichtigung", func() tea.Cmd { return a.toggleDesktopNotify() })),
		palToggle(ic("explain"), i18n.T("Tastenzeile: Tasten erklären"), a.cheatOn(), "cheat fußzeile footer", func() tea.Cmd { return a.toggleCheat() }),
		palCycle(pgSetBehave, ic("split"), i18n.T("Fenster beim Start"), winStartLabel(a.winStart()), "fenster vollbild fullscreen maximiert start window f11",
			func(dir int) tea.Cmd { return a.cycleWinStart(dir) }).at(winStartIdx(a.winStart()), 3),
	}

	// Aussehen: je ein Durchschalt-Eintrag, die Einzelwerte nur für die Suche.
	cur := set.Theme
	if cur != CustomName {
		cur = ThemeByName(cur).Name
	}
	themeIdx, themeVal := -1, i18n.T("Eigene Farben")
	for i, t := range Themes {
		if t.Name == cur {
			themeIdx, themeVal = i, i18n.T(t.Label)
		}
	}
	colHint := i18n.T("Editor")
	if cur == CustomName {
		colHint = "✓ " + i18n.T("aktiv")
	}
	iconIdx := -1
	for i, st := range iconStyles {
		if iconSet == st.id {
			iconIdx = i
		}
	}
	items = append(items,
		// Sichtbar ist die Auswahl mit Vorschau; Durchschalten nur über die Suche.
		palItem{group: pgSetLook, menu: "settings", icon: ic("theme"), label: i18n.T("Farbschema wählen …"),
			hint: themeVal, keys: "theme farbe color aussehen vorschau preview schema",
			run: func() tea.Cmd { return a.openThemes() }},
		hide(palCycle(pgSetLook, ic("theme"), i18n.T("Farbschema"), themeVal, "theme farbe color aussehen",
			func(dir int) tea.Cmd { return a.setTheme(Themes[cycleIndex(themeIdx, len(Themes), dir)].Name) })),
		palItem{group: pgSetLook, menu: "settings", icon: ic("colors"), label: i18n.T("Farben anpassen (eigene Farben)"), hint: colHint,
			keys: "theme farbe color colors eigene custom", run: func() tea.Cmd { return a.openColors() }},
		palItem{group: pgSetLook, menu: "settings", icon: ic("colors"), label: i18n.T("Farbregeln für die Ausgabe …"), hint: hlHint(a.highlightOn()),
			keys: "highlight farbregeln einfärben fehler ip mac syslog regeln", run: func() tea.Cmd { return a.openHighlightForm() }},
		look(hide(palToggle(ic("split"), i18n.T("Farbiger Rahmen um jeden Tab"), a.tabFramesOn(), "rahmen farbe farbig tab umranden frame border color", func() tea.Cmd { return a.toggleTabFrames() }))),
		look(hide(palToggle(ic("colors"), i18n.T("Farbregeln: Ausgabe einfärben"), a.highlightOn(), "highlight farbregeln einfärben", func() tea.Cmd { return a.toggleHighlight() }))),
		palCycle(pgSetLook, ic("icons"), i18n.T("Symbole"), i18n.T(iconStyleLabel(iconSet)), "icons symbole nerd font unicode minimal tags",
			func(dir int) tea.Cmd { return a.chooseIcons(iconStyles[cycleIndex(iconIdx, len(iconStyles), dir)].id) }).at(iconIdx, len(iconStyles)),
		palCycle(pgSetLook, ic("theme"), i18n.T("Fensterstil"), framesLabel(framesSetting), "fensterstil rahmen ecken rund eckig schatten dialog flach",
			func(dir int) tea.Cmd { return a.cycleFrames(dir) }).at(indexOf(framesValues, set.Frames), len(framesValues)),
		// Bewegung „aus“ schaltet auch die Startanimation ab (motion.go).
		palCycle(pgSetLook, ic("intro"), i18n.T("Bewegung"), motionLabel(motionSetting), "bewegung animation motion animiert spinner puls hover karte ruhig startanimation intro",
			func(dir int) tea.Cmd { return a.cycleMotion(dir) }).at(indexOf(motionValues, set.Motion), len(motionValues)),
		look(hide(palToggle(ic("intro"), i18n.T("Startanimation"), a.introOn(), "intro animation", func() tea.Cmd { return a.toggleIntro() }))),
		look(palToggle(ic("intro"), i18n.T("Barrierearm: keine Animationen, Formen statt nur Farben"), accessible,
			"barrierefrei barrierearm accessibility a11y screenreader farbenblind animation ruhig", func() tea.Cmd { return a.toggleAccessible() })),
		look(palToggle(ic("split"), i18n.T("Geteilte Ansicht: andere Bereiche abdunkeln"), a.dimPanes(), "split dim abdunkeln", func() tea.Cmd { return a.toggleDimPanes() })),
	)
	for _, t := range Themes {
		t := t
		hint := ""
		if cur == t.Name {
			hint = "✓ " + i18n.T("aktiv")
		}
		items = append(items, palItem{group: pgSetLook, menu: "settings", icon: ic("theme"), label: i18n.T("Aussehen: ") + i18n.T(t.Label), hint: hint,
			keys: "theme farbe color", keep: true, hidden: true, run: func() tea.Cmd { return a.setTheme(t.Name) }})
	}
	for _, st := range iconStyles {
		st := st
		hint := i18n.T(st.sample)
		if iconSet == st.id {
			hint = "✓ " + i18n.T("aktiv")
		}
		items = append(items, palItem{group: pgSetLook, menu: "settings", icon: ic("icons"), label: i18n.T("Symbole: ") + i18n.T(st.label), hint: hint,
			keys: "icons symbole nerd font unicode minimal tags", keep: true, hidden: true, run: func() tea.Cmd { return a.chooseIcons(st.id) }})
	}

	items = append(items,
		// Sicherheit
		safe(palToggle(ic("paste"), i18n.T("Einfügeschutz: Vorschau bei mehreren Zeilen"), a.pasteGuardEnabled(), "paste einfügen", func() tea.Cmd { return a.togglePasteGuard() })),
		proItem(safe(palToggle(ic("paste"), i18n.T("Server darf in die Zwischenablage kopieren (OSC 52)"), a.osc52On(), "clipboard zwischenablage osc52 tmux vim", func() tea.Cmd { return a.toggleOSC52() }))),
		palCycle(pgSetSafe, ic("sshkey"), i18n.T("Automatisch sperren bei Leerlauf"), lockLabel(set.LockAfterMin), "lock sperren idle leerlauf",
			func(dir int) tea.Cmd { return a.cycleLockAfter(dir) }).at(indexInt(lockChoices, set.LockAfterMin), len(lockChoices)),
		palItem{group: pgSetSafe, menu: "settings", icon: ic("sshkey"), label: i18n.T("Master-Passwort ändern …"), hint: masterHint(a.st.V.HasDPAPI()),
			keys: "master passwort password ändern change dpapi windows anmeldung", run: func() tea.Cmd { return a.openMasterForm() }},

		// Einrichten: Verwandtes in Unterordnern
		a.palFolder("settings/ki"),
		a.palFolder("settings/logs"),
		a.palFolder("settings/net"),
		palItem{group: pgSetSetup, menu: "settings", icon: ic("keys"), label: i18n.T("Tastenkürzel anpassen"), hint: a.keys.label(actKeys), keys: "keys", run: func() tea.Cmd { return a.openKeys() }},
		palItem{group: pgSetSetup, menu: "settings", icon: ic("open"), label: i18n.T("Editor für Server-Dateien"), hint: trunc(orDefault(a.editorSetting(), i18n.T("Standard")), 18),
			keys: "editor vscode code notepad bearbeiten", run: func() tea.Cmd { return a.askEditor() }},
		palItem{group: pgSetSetup, menu: "settings", icon: ic("sync"), label: i18n.T("Sync zwischen Rechnern einrichten"), hint: "y", keys: "sync", run: func() tea.Cmd { return a.openSync() }},
		a.palFolder("settings/relay"),

		// Einstellungen › KI
		palItem{group: pgSetSetup, menu: "settings/ki", icon: ic("ai"), label: i18n.T("Endpunkt & Modell …"), hint: "s", keys: "ai llm ki-einstellungen endpunkt modell model url", run: func() tea.Cmd { return a.openSettings() }},
		palItem{group: pgSetSetup, menu: "settings/ki", icon: ic("ai"), label: i18n.T("KI-Ersatz-Anbieter (Ollama, OpenAI, Claude)"), keys: "ai llm fallback ollama openai claude", run: func() tea.Cmd { return a.openFallbackForm() }},
		sub("settings/ki", palCycle(pgSetSetup, ic("ai"), i18n.T("KI-Datenschutz"), privacyLabel(a.st.LLM().Privacy),
			"ki ai datenschutz privacy lokal local cloud aus off kunde", func(dir int) tea.Cmd { return a.cyclePrivacy(dir) })),
		palItem{group: pgSetSetup, menu: "settings/ki", icon: ic("ai"), label: i18n.T("Eigene KI-Server (auch extern, per API) …"), hint: ownHostsHint(a.st.LLM().OwnHosts),
			keys: "ki ai eigene own server privat private extern api datenschutz privacy lokal", run: func() tea.Cmd { return a.askOwnHosts() }},
		a.palFolder("settings/ki/begleiter"),
		proItem(sub("settings/ki", palCycle(pgSetSetup, ic("ai"), i18n.T("MCP-Zugriff für KI-Clients (Claude, …)"), mcpLabel(a.mcpLevel()),
			"mcp claude model context protocol ki ai client server zugriff", func(dir int) tea.Cmd { return a.cycleMCP(dir) }))),
		palItem{group: pgSetSetup, menu: "settings/ki", icon: ic("ai"), label: i18n.T("Letzte KI-Anfrage ansehen"), hint: i18n.T("was ging hinaus"),
			keys: "ki ai datenschutz privacy anfrage request prompt gesendet", run: func() tea.Cmd { a.modal = a.lastRequestModal(); return nil }},
		palItem{group: pgSetSetup, menu: "settings/ki", icon: ic("llmcheck"), label: i18n.T("KI-Status: Endpunkt erreichbar? jetzt prüfen"), keys: "ai llm status",
			run: func() tea.Cmd { a.openLLMStatus(); return a.llmProbeCmd() }},

		// Einstellungen › KI › KI-Begleiter
		sub("settings/ki/begleiter", palCycle(pgSetSetup, ic("ai"), i18n.T("KI-Begleiter"), companionLevelLabel(a.compLevel()),
			"begleiter companion stufe aus leise gesprächig coach", func(dir int) tea.Cmd { return a.cycleCompanion(dir) }).at(a.compLevel(), domain.CompanionCoach+1)),
		sub("settings/ki/begleiter", palCycle(pgSetSetup, ic("ai"), i18n.T("Höchstens KI-Anfragen pro Stunde"), fmt.Sprint(a.compCfg().Hour()),
			"begleiter companion grenze limit stunde rate kosten", func(dir int) tea.Cmd { return a.cycleCompanionHour(dir) })),
		sub("settings/ki/begleiter", palCycle(pgSetSetup, ic("ai"), i18n.T("Tagesbudget (geschätzte Tokens)"), kTok(a.compCfg().DayTokens()),
			"begleiter companion budget tokens kosten tag", func(dir int) tea.Cmd { return a.cycleCompanionBudget(dir) })),
		sub("settings/ki/begleiter", palToggle(ic("ai"), i18n.T("Begleiter nur mit eigener KI"), a.compCfg().OnlyLocal || a.compCfg().Raw,
			"begleiter companion lokal eigene datenschutz privacy", func() tea.Cmd { return a.toggleCompanionLocal() })),
		sub("settings/ki/begleiter", palToggle(ic("ai"), i18n.T("Filter aus bei eigener KI (Geheimnisse nicht entfernen)"), a.compCfg().Raw,
			"begleiter companion filter datenschutz privacy roh raw geheimnisse eigene", func() tea.Cmd { return a.toggleCompanionRaw() })),
		sub("settings/ki/begleiter", palToggle(ic("ai"), i18n.T("Coach: Rückblick beim Schließen des Tabs"), !a.compCfg().NoRecap,
			"begleiter companion coach rückblick recap runbook merken", func() tea.Cmd {
				a.setCompanion(func(c *domain.CompanionConfig) { c.NoRecap = !c.NoRecap })
				return nil
			})),

		// Einstellungen › Protokolle & Logs
		sub("settings/logs", palToggle(ic("sesslog"), i18n.T("Sitzungsprotokoll schreiben"), a.logsEnabled(), "log protokoll", func() tea.Cmd { return a.toggleSessionLog() })),
		sub("settings/logs", palCycle(pgSetSetup, ic("errors"), i18n.T("Logwache"), watchLabel(a.watchLevel()),
			"logwache log watch journal überwachen fehler warnungen", func(dir int) tea.Cmd { return a.cycleWatch(dir) }).at(a.watchLevel(), 3)),
		proItem(sub("settings/logs", palCycle(pgSetSetup, ic("errors"), i18n.T("Syslog-Empfang (Switches, Firewalls)"), syslogLabel(set.SyslogPort),
			"syslog udp 514 5514 empfang switch firewall logwache", func(dir int) tea.Cmd { return a.cycleSyslog(dir) }))),
		proItem(sub("settings/logs", palToggle(ic("errors"), i18n.T("Syslog auch von unbekannten Absendern annehmen"), set.SyslogAny,
			"syslog absender unbekannt fremd alle", func() tea.Cmd { return a.toggleSyslogAny() }))),
		proItem(sub("settings/logs", palToggle(ic("sesslog"), i18n.T("Audit-Protokoll (wer, wann, was; fälschungssicher)"), a.auditOn(), "audit protokoll revision compliance nachweis log", func() tea.Cmd { return a.toggleAudit() }))),
		palItem{group: pgSetSetup, menu: "settings/logs", icon: ic("export"), label: i18n.T("Audit-Protokoll prüfen und speichern"), hint: ".txt", pro: true,
			keys: "audit protokoll prüfen verify export revision", run: func() tea.Cmd { return a.exportAudit() }},

		// Einstellungen › Netzwerkgeräte
		palItem{group: pgSetSetup, menu: "settings/net", icon: ic("backup"), label: i18n.T("Befehlssätze (Hersteller) …"), hint: a.cmdsetHint(),
			keys: "befehlssatz befehlssätze hersteller vendor dialekt switch firewall router wlan yaml modell", run: func() tea.Cmd { return a.openCmdSets() }},
		palItem{group: pgSetSetup, menu: "settings/net", icon: ic("backup"), label: i18n.T("Regeln für Switch-Configs …"), hint: a.configRulesHint(),
			keys: "config regeln rules compliance prüfen check ntp telnet snmp switch", run: func() tea.Cmd { return a.openConfigRulesForm() }},

		// Einstellungen › Vermittler
		palItem{group: pgSetSetup, menu: "settings/relay", icon: ic("relay"), label: i18n.T("Systeme über Vermittler (Server hinter NAT/Firewall)"), keys: "relay", run: func() tea.Cmd { return a.openRelaySystems() }},
		palItem{group: pgSetSetup, menu: "settings/relay", icon: ic("relay"), label: i18n.T("Vermittler koppeln / verwalten (eigener oder Speech on DGX Spark)"), keys: "relay", run: func() tea.Cmd { return a.openRelayManage() }},
		palItem{group: pgSetSetup, menu: "settings/relay", icon: ic("relay"), label: i18n.T("Eigenen Vermittler einrichten (Assistent über SSH)"), hint: i18n.T("Server"),
			keys: "relay vermittler einrichten assistent setup installieren server", run: func() tea.Cmd { return a.openRelaySetup(a.current()) }},
		palItem{group: pgSetSetup, menu: "settings/relay", icon: ic("team"), label: i18n.T("Öffentlicher Vermittler fürs Teilen …"), hint: orDefault(trunc(strings.TrimPrefix(a.st.Settings().Share.Public, "https://"), 24), i18n.T("keiner")),
			keys: "relay teilen share öffentlich public offen vermittler internet", run: func() tea.Cmd { return a.askPublicRelay() }},

		// Hilfe & Info
		palItem{group: pgHelp, menu: "help", icon: ic("help"), label: i18n.T("Tastenkürzel anzeigen"), hint: a.keys.label(actHelp), keys: "help keys", run: func() tea.Cmd { a.modal = a.helpModal(); return nil }},
		palItem{group: pgHelp, menu: "help", icon: ic("tour"), label: i18n.T("Einführungstour"), hint: i18n.T("5 Schritte"), keys: "tour", run: func() tea.Cmd { a.modal = a.tourModal(); return nil }},
		palItem{group: pgHelp, menu: "help", icon: ic("help"), label: i18n.T("Handbuch & FAQ (Webseite)"), hint: "web", keys: "manual handbuch faq docs", run: func() tea.Cmd { openManual(); return nil }},
		palItem{group: pgHelp, menu: "help", icon: ic("news"), label: i18n.T("Was ist neu (Changelog)"), hint: a.keys.label(actAbout) + " w", run: func() tea.Cmd { a.modal = a.changelogModal(nil); return nil }},
		palItem{group: pgHelp, menu: "help", icon: ic("about"), label: i18n.T("Über sshTermy / Update"), hint: a.keys.label(actAbout), keys: "about version", run: func() tea.Cmd { a.modal = a.aboutModal(); return nil }},
		help("help", palCycle(pgHelp, ic("news"), i18n.T("Updates anbieten"), updateWaitLabel(set.UpdateWaitH), "update kanal channel stabil stable sofort neue version release",
			func(dir int) tea.Cmd { return a.cycleUpdateWait(dir) })),
		a.palFolder("help/debug"),

		// Hilfe & Info › Fehlersuche
		proItem(help("help/debug", palToggle(ic("sesslog"), i18n.T("Debug-Protokoll für Fehlerberichte"), a.debugOn(), "debug log protokoll fehler bug support", func() tea.Cmd { return a.toggleDebug() }))),
		palItem{group: pgHelp, menu: "help/debug", icon: ic("export"), label: i18n.T("Debug-Protokoll bereinigt speichern (für Fehlerbericht)"), hint: ".txt",
			keys: "debug log protokoll fehler bug support issue", run: func() tea.Cmd { return a.exportDebug() }},
		palItem{group: pgHelp, menu: "help/debug", icon: ic("sesslog"), label: i18n.T("Debug-Protokoll leeren"), hidden: true,
			keys: "debug log protokoll leeren löschen clear", run: func() tea.Cmd { return a.clearDebug() }},
		proItem(help("help/debug", palToggle(ic("sesslog"), i18n.T("Leistungsanzeige (Bildrate, Zeichenzeit)"), a.perf.on, "leistung performance fps bildrate langsam ruckelt debug", func() tea.Cmd { return a.togglePerf() }))),
	)
	return a.simpleHide(items)
}

// palIOItems: Import, Export und Gesamtsicherung. Das sind Aktionen, keine
// Einstellungen; sie stehen im Bereich „Verbindungen & Daten“.
func (a *App) palIOItems(inMgr bool) []palItem {
	imp := ""
	if inMgr {
		imp = a.keys.label(actImport)
	}
	return []palItem{
		{icon: ic("import"), label: i18n.T("Verbindungen importieren (ssh-config, PuTTY, MobaXterm, Ansible, CSV)"), hint: imp, keys: "import netbox", run: func() tea.Cmd { return a.openImport() }},
		{icon: ic("export"), label: i18n.T("Verbindungen als CSV exportieren (ohne Passwörter)"), hint: ".csv", keys: "export csv excel", run: func() tea.Cmd { return a.exportConnsCSV() }},
		{icon: ic("export"), label: i18n.T("Gesamtsicherung speichern (für Umzug/Sicherung) …"), hint: i18n.T("alles, verschlüsselt"), keys: "backup sicherung umzug neuer rechner migrate", run: func() tea.Cmd { return a.openBackupForm(false) }},
		{icon: ic("import"), label: i18n.T("Gesamtsicherung einspielen …"), hint: fullbackup.Ext, keys: "restore backup sicherung umzug wiederherstellen migrate", run: func() tea.Cmd { return a.openBackupForm(true) }},
	}
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
	var cur palItem
	if p.sel < len(items) {
		cur = items[p.sel]
	}
	switch key := k.String(); {
	case (key == "tab" || key == "shift+tab") && a.palTabbed():
		dir := 1
		if key == "shift+tab" {
			dir = -1
		}
		a.palTabMove(dir)
		return nil
	case (key == "left" || key == "right") && cur.cycle != nil:
		dir := 1
		if key == "left" {
			dir = -1
		}
		p.palChanged(cur.label)
		return a.palRun(palItem{menu: cur.menu, keep: true, run: func() tea.Cmd { return cur.cycle(dir) }})
	case key == "right" && cur.open != "" && p.in.Value() == "":
		return a.palRun(cur)
	case key == "esc" && p.menu != "":
		a.palEnter(palParent(p.menu))
		return nil
	case key == "left" && p.menu != "" && p.in.Value() == "":
		a.palEnter(palParent(p.menu))
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
		return a.palRunTrack(it)
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
	dw := a.palDetailWidth(w)
	p.in.SetWidth(w - 8)
	now := time.Now()
	p.track(now)
	query := p.in.Value()
	var glow color.Color
	if since := now.Sub(p.mo.queryAt); motionLevel() > 0 && since < palHitGlow && query != "" {
		glow = mixColor(st.T.Surface, st.T.Accent, 0.4*(1-easeOut(float64(since)/float64(palHitGlow))))
	}
	var b strings.Builder
	b.WriteString(promptBlock(st.Accent.Render("$ "), p.in.View()) + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(st.T.Border).Render(strings.Repeat("─", w-4)) + "\n")
	maxRows := max(5, a.h-12)
	tabbed := a.palTabbed()
	if tabbed {
		b.WriteString(a.palTabsRow() + "\n")
		maxRows -= 2
	}
	// Hilfesatz unter der Auswahl nur ohne Detailspalte (dort steht er rechts).
	var help string
	if dw == 0 && p.sel < len(items) {
		help = palHelp(items[p.sel])
	}
	var helpLines []string
	if help != "" {
		// höchstens zwei Zeilen, umgebrochen
		helpLines = strings.Split(lipgloss.NewStyle().Width(w-10).Render(help), "\n")
		if len(helpLines) > 2 {
			helpLines = helpLines[:2]
			helpLines[1] = trunc(strings.TrimRight(helpLines[1], " ")+" …", w-10)
		}
		maxRows -= len(helpLines)
	}
	// Leerzeilen zwischen den Gruppen nur, wenn sie Platz haben: jede kostet
	// eine Zeile, die sonst ein Eintrag hätte.
	groups := 0
	for i, it := range items {
		if i == 0 || it.group != items[i-1].group {
			groups++
		}
	}
	gaps := a.h >= 30 && len(items)+groups <= maxRows
	// Fenster um die Auswahl
	start := 0
	if p.sel >= maxRows {
		start = p.sel - maxRows + 1
	}
	last := palGroup(-1)
	rows := 0
	for i := start; i < len(items) && rows < maxRows; i++ {
		it := items[i]
		gcol := a.palGroupColor(it.group)
		if it.group != last {
			last = it.group
			if !tabbed {
				head := i18n.T(palGroupName[it.group])
				if it.group == pgConns {
					head = a.palConnHeader(p.in.Value())
				}
				b.WriteString(a.palGroupRule(head, w-4, gcol, gaps && rows > 0) + "\n")
			} else if rows == 0 {
				b.WriteString("\n")
			}
		}
		iconStyle := st.Accent
		if gcol != nil {
			iconStyle = lipgloss.NewStyle().Foreground(gcol)
		}
		if it.group == pgConns && it.more {
			iconStyle = st.Muted
		}
		if it.iconColor != nil {
			iconStyle = lipgloss.NewStyle().Foreground(it.iconColor)
		}
		it.icon = padIcon(it.icon)
		iw := lipgloss.Width(it.icon) - 1
		hint := a.palHint(it, p.armed == i)
		if k := p.glowK(it.label, now); k > 0 {
			// umgeschalteter Wert leuchtet kurz auf (Bewegung)
			hint = lipgloss.NewStyle().Background(mixColor(st.T.Surface, st.T.Warn, 0.55*k)).Render(ansi.Strip(hint))
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
		hits := fuzzyHits(query, label) // getroffene Buchstaben leuchten (motion2.go)
		line := iconStyle.Render(it.icon) + " " + hitLabel(label, hits, labelStyle, st.Accent.Bold(true), glow) + st.Muted.Render(sub) + strings.Repeat(" ", gap) + hint
		if k := p.trailK(i, now); k > 0 {
			// Nachleuchten: die alte Auswahl verblasst, der Balken „gleitet“.
			line = lipgloss.NewStyle().Background(mixColor(st.T.Surface, st.T.Accent, 0.35*k)).Width(w - 4).Render(ansi.Strip(line))
		}
		if i == p.sel {
			// Auswahl als Balken in Akzentfarbe mit ▶ (wie der aktive Tab).
			fill := st.T.Accent
			if motionLevel() > 0 {
				if k := easeOut(float64(now.Sub(p.mo.selAt)) / float64(selFadeDur)); k < 1 {
					fill = mixColor(st.T.Surface, st.T.Accent, k)
				}
			}
			ink := inkOn(st.T, fill)
			mark := "▶" + strings.Repeat(" ", max(0, lipgloss.Width(it.icon)-1))
			bar := lipgloss.NewStyle().Background(fill).Foreground(ink)
			line = bar.Bold(true).Render(mark+" "+label) + bar.Render(ansi.Strip(sub)+strings.Repeat(" ", gap)) + bar.Bold(true).Render(ansi.Strip(hint))
		}
		if k := p.rowK(rows, now); k < 1 {
			line = fadeANSI(line, st.T, k) // Zeilen blenden nacheinander ein
		}
		i, it := i, it
		line = a.z.click(line, func(tea.Mouse, bool) tea.Cmd {
			p.sel = i
			if it.danger != "" && p.armed != i {
				p.armed = i // rot: erst scharf schalten, zweiter Klick führt aus
				return nil
			}
			return a.palRunTrack(it)
		})
		b.WriteString(line + "\n")
		if i == p.sel {
			for k, l := range helpLines {
				pre := "      "
				if k == 0 {
					pre = "    › "
				}
				b.WriteString(st.Muted.Render(pre+strings.TrimRight(l, " ")) + "\n")
			}
		}
		rows++
	}
	if len(items) == 0 {
		b.WriteString(st.Muted.Render(i18n.T("Nichts gefunden. Im Terminal wird Text zum Ziel oder zur Frage.")) + "\n")
	}
	keys := a.palKeys(items)
	title := i18n.T("Befehlspalette")
	sub := fmt.Sprintf(i18n.T("%d Einträge"), len(items))
	if p.menu != "" {
		title += " › " + palMenuPath(p.menu)
	}
	if tabbed {
		sub = palTitle(i18n.T(palGroupName[p.tab])) + " · " + fmt.Sprint(len(items))
	}
	body := b.String()
	width := w - 4
	maxW := 0
	if dw > 0 && p.sel < len(items) {
		// Detailspalte rechts: Erklärung, Vorschau, Inhalt von Ordnern.
		list := strings.TrimSuffix(body, "\n")
		lh := lipgloss.Height(list)
		det := a.palDetail(items[p.sel], dw, max(lh, a.h-14))
		hh := max(lh, lipgloss.Height(det))
		sep := lipgloss.NewStyle().Foreground(st.T.Border).Render(strings.TrimSuffix(strings.Repeat(" │ \n", hh), "\n"))
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(w-4).Render(list), sep, det) + "\n"
		width = w - 4 + 3 + dw
		maxW = width + 6
	}
	box := st.dialog(dialog{icon: "◆", title: title, sub: sub, tone: st.T.AI,
		body: body, keys: keys, width: width, maxW: maxW, compact: true}, a.w)
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

// palRunTrack: wie palRun, merkt sich aber umgeschaltete Werte fürs
// Aufleuchten (Bewegung).
func (a *App) palRunTrack(it palItem) tea.Cmd {
	if a.pal != nil && (it.cycle != nil || it.state != palNoState) {
		a.pal.palChanged(it.label)
	}
	return a.palRun(it)
}

// palGroupRule: Gruppenkopf der Palette als Linie mit Namen
// („── ALLGEMEIN ─────────“), damit sich die Bereiche deutlich trennen. Ab
// der zweiten Gruppe steht eine Leerzeile davor (gap), wenn Platz ist.
func (a *App) palGroupRule(head string, w int, col color.Color, gap bool) string {
	st := a.s
	name := head
	rule := lipgloss.NewStyle().Foreground(st.T.Border)
	if col == nil {
		col = st.T.Accent
	}
	label := lipgloss.NewStyle().Foreground(col).Bold(true).Render(name)
	rest := max(2, w-lipgloss.Width(name)-4)
	out := rule.Render("── ") + label + rule.Render(" "+strings.Repeat("─", rest))
	if gap {
		out = "\n" + out
	}
	return out
}
