package ui

import (
	"fmt"
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
)

var palGroupName = map[palGroup]string{pgAI: "KI", pgSuggest: "VORSCHLÄGE", pgQuick: "SCHNELLBEFEHLE", pgRunbook: "RUNBOOKS", pgTunnel: "TUNNEL", pgConnect: "VERBINDEN", pgAction: "AKTIONEN"}

type palItem struct {
	group  palGroup
	icon   string
	label  string
	hint   string
	danger string
	run    func() tea.Cmd
}

type palette struct {
	in    aiInput
	sel   int
	armed int
}

func isPaletteKey(k string) bool { return activeKeys.is(actPalette, k) }

func (a *App) openPalette() tea.Cmd {
	in := newAIInput(i18n.T("Ziel, Frage, Befehl, Tunnel, Verbindung oder user@host:port …"))
	in.MaxHeight = 4
	a.pal = &palette{in: in, armed: -1}
	return a.pal.in.Focus()
}

func (a *App) paletteItems() []palItem {
	q := strings.TrimSpace(a.pal.in.Value())
	ql := strings.ToLower(q)
	s := a.current()
	inTerm := a.scr == scrTerminal && s != nil && s.state == stConnected
	var items []palItem
	quick, quickFirst := a.quickItems(q)
	if quickFirst {
		items = append(items, quick...) // user@host:port: Verbinden ist gemeint
	}
	if q != "" && inTerm {
		items = append(items,
			palItem{group: pgAI, icon: "◎", label: i18n.T("Als Ziel starten: „") + trunc(oneLine(q), 50) + "“", hint: "⏎",
				run: func() tea.Cmd { return a.startGoal(s, q) }},
			palItem{group: pgAI, icon: "✦", label: i18n.T("KI fragen: „") + trunc(oneLine(q), 50) + "“", hint: "Tab",
				run: func() tea.Cmd { return a.startAsk(s, q) }},
			palItem{group: pgAI, icon: "⇆", label: i18n.T("Für dieses System umschreiben: „") + trunc(oneLine(q), 40) + "“", hint: i18n.T("Befehl"),
				run: func() tea.Cmd { return a.startAdapt(s, oneLine(q)) }})
	}
	match := func(x string) bool { return ql == "" || strings.Contains(strings.ToLower(x), ql) }
	if inTerm && s.fail != nil && match("fehler warum fix error why "+s.fail.Command) {
		items = append(items, palItem{group: pgAI, icon: "✗", label: i18n.T("Warum ist „") + trunc(oneLine(s.fail.Command), 40) + i18n.T("“ fehlgeschlagen? Fix vorschlagen"), hint: "Alt+0",
			run: func() tea.Cmd { return a.explainFailure(s) }})
	}
	if inTerm {
		for _, c := range s.chips {
			c := c
			if !match(c.Command) {
				continue
			}
			it := palItem{group: pgSuggest, icon: "$", label: c.Command, hint: i18n.T("einfügen"),
				run: func() tea.Cmd { s.term.SendText(c.Command); s.panelFocus = false; return nil }}
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
			items = append(items, palItem{group: pgRunbook, icon: "▶", label: rb.Name, hint: plural(len(rb.Steps), i18n.T("Schritt"), i18n.T("Schritte")),
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
					items = append(items, palItem{group: pgTunnel, icon: "↗", label: i18n.T("Öffnen: ") + u, hint: "Browser",
						run: func() tea.Cmd { _ = sshx.OpenBrowser(u); return nil }})
				}
			}
			items = append(items, palItem{group: pgTunnel, icon: "⇄", label: label, hint: hint, run: func() tea.Cmd {
				if s.tunnels == nil {
					return nil
				}
				if s.tunnels.Get(t.ID) != nil {
					s.tunnels.Stop(t.ID)
				} else if at, err := s.tunnels.Start(t); err != nil {
					a.flash(i18n.T("Tunnel: ") + err.Error())
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
	for _, c := range a.st.Connections() {
		c := c
		if !match(c.Alias + " " + c.Host + " " + c.Group) {
			continue
		}
		items = append(items, palItem{group: pgConnect, icon: "▸", label: c.Alias, hint: c.Username + "@" + c.Host,
			run: func() tea.Cmd { return a.openOrConnect(c) }})
	}
	actions := []palItem{
		{icon: "◎", label: i18n.T("Ziel-Modus öffnen"), hint: a.keys.label(actGoal), run: func() tea.Cmd {
			if inTerm {
				withPanel(s, panelGoal)
				a.resize(s)
				return a.panelOpened(s)
			}
			return nil
		}},
		{icon: "⇄", label: i18n.T("Tunnel-Panel"), hint: a.keys.label(actTunnels), run: func() tea.Cmd {
			if inTerm {
				withPanel(s, panelTunnels)
				a.resize(s)
			}
			return nil
		}},
		{icon: "↯", label: i18n.T("Schnellbefehle bearbeiten"), hint: i18n.T("Liste"), run: func() tea.Cmd {
			if inTerm {
				return a.editQuickFor(s)
			}
			return nil
		}},
		{icon: "≡", label: i18n.T("Verbindungs-Manager"), hint: a.keys.label(actManager), run: func() tea.Cmd { a.scr = scrManager; return nil }},
		{icon: "+", label: i18n.T("Neue Verbindung"), hint: "n", run: func() tea.Cmd { return a.openConnForm(newConnDefaults()) }},
		{icon: "⚿", label: i18n.T("SSH-Schlüssel erzeugen / auf den Server übertragen"), hint: a.keys.label(actSSHKey), run: func() tea.Cmd {
			if c, ok := a.sshKeyTarget(); ok {
				return a.openSSHKeyForm(c)
			}
			a.flash(i18n.T("Erst eine Verbindung im Manager wählen oder verbinden"))
			return nil
		}},
		{icon: "⇉", label: i18n.T("Broadcast: Befehl auf mehreren Hosts"), hint: a.keys.label(actBroadcast), run: func() tea.Cmd { return a.openBroadcast() }},
		{icon: "⌕", label: i18n.T("Netzwerk-Scan: SSH-Geräte finden"), hint: a.keys.label(actScan), run: func() tea.Cmd { return a.openScan() }},
		{icon: "✦", label: i18n.T("KI-Einstellungen"), hint: "s", run: func() tea.Cmd { return a.openSettings() }},
		{icon: "⇅", label: i18n.T("Sync zwischen Rechnern einrichten"), hint: "y", run: func() tea.Cmd { return a.openSync() }},
		{icon: "⇩", label: i18n.T("Verbindungen importieren (ssh-config, PuTTY, MobaXterm)"), hint: a.keys.label(actImport), run: func() tea.Cmd { return a.openImport() }},
		{icon: "?", label: i18n.T("Tastenkürzel anzeigen"), hint: a.keys.label(actHelp), run: func() tea.Cmd { a.modal = a.helpModal(); return nil }},
		{icon: "⌨", label: i18n.T("Tastenkürzel anpassen"), hint: a.keys.label(actKeys), run: func() tea.Cmd { return a.openKeys() }},
		{icon: "i", label: i18n.T("Über sshTermy / Update"), hint: a.keys.label(actAbout), run: func() tea.Cmd { a.modal = a.aboutModal(); return nil }},
		{icon: "★", label: i18n.T("Was ist neu (Changelog)"), hint: a.keys.label(actAbout) + " w", run: func() tea.Cmd { a.modal = a.changelogModal(nil); return nil }},
		{icon: "↖", label: mouseLabel(a.mouseEnabled()), hint: i18n.T("Maus"), run: func() tea.Cmd { return a.toggleMouse() }},
		{icon: "⟳", label: reconnectLabel(a.autoReconnect()), hint: "Reconnect", run: func() tea.Cmd { return a.toggleAutoReconnect() }},
		{icon: "●", label: reachLabel(a.reachEnabled()), hint: i18n.T("Online"), run: func() tea.Cmd { return a.toggleReach() }},
		{icon: "☰", label: i18n.T("Sitzungsprotokolle durchsuchen"), hint: "l", run: func() tea.Cmd { return a.openLogs() }},
		{icon: "●", label: logLabel(a.logsEnabled()), hint: i18n.T("Protokoll"), run: func() tea.Cmd { return a.toggleSessionLog() }},
		{icon: "▶", label: i18n.T("Einführungstour"), hint: i18n.T("5 Schritte"), run: func() tea.Cmd { a.modal = a.tourModal(); return nil }},
		{icon: "?", label: cheatLabel(a.cheatOn()), hint: a.keys.label(actCheat), run: func() tea.Cmd { return a.toggleCheat() }},
		{icon: "⎘", label: pasteGuardLabel(a.pasteGuardEnabled()), hint: i18n.T("Einfügen"), run: func() tea.Cmd { return a.togglePasteGuard() }},
		{icon: "⚠", label: dangerAllLabel(a.dangerAll()), hint: i18n.T("Gefahr"), run: func() tea.Cmd { return a.toggleDangerAll() }},
		{icon: "✦", label: introLabel(a.introOn()), hint: "Start", run: func() tea.Cmd { return a.toggleIntro() }},
		{icon: "◇", label: langLabel(), hint: "Language", run: func() tea.Cmd { return a.toggleLang() }},
	}
	for _, t := range Themes {
		t := t
		hint := ""
		if ThemeByName(a.st.Settings().Theme).Name == t.Name {
			hint = i18n.T("aktiv")
		}
		actions = append(actions, palItem{icon: "◐", label: i18n.T("Aussehen: ") + i18n.T(t.Label), hint: hint, run: func() tea.Cmd { return a.setTheme(t.Name) }})
	}
	if a.restartPending != "" {
		actions = append(actions, palItem{icon: "⟳", label: i18n.T("Neu starten (v") + a.restartPending + i18n.T(" aktivieren)"), hint: "Update", run: func() tea.Cmd { return a.askRestart() }})
	}
	if a.st.SyncConfig().Enabled() {
		actions = append(actions, palItem{icon: "⇅", label: i18n.T("Jetzt synchronisieren"), hint: "Sync", run: func() tea.Cmd {
			a.flash(i18n.T("Sync läuft …"))
			return a.runSync()
		}})
	}
	if !inTerm {
		actions = actions[3:]
	} else {
		actions = append(actions, palItem{icon: "▤", label: i18n.T("Host-Übersicht"), hint: a.keys.label(actHost), run: func() tea.Cmd {
			withPanel(s, panelHost)
			a.resize(s)
			return a.panelOpened(s)
		}})
		actions = append(actions,
			palItem{icon: "●", label: recordLabel(s.recording()), hint: "asciinema", run: func() tea.Cmd { return a.toggleRecording(s) }},
			palItem{icon: "⇩", label: i18n.T("Terminal-Verlauf als Text exportieren"), hint: ".txt", run: func() tea.Cmd { return a.exportScrollback(s) }})
		if s.client != nil && s.client.NetDevice {
			actions = append(actions, palItem{icon: "▣", label: i18n.T("Switch-Config sichern und vergleichen"), hint: "b", run: func() tea.Cmd {
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
			palItem{icon: "⌕", label: i18n.T("Im Verlauf suchen"), hint: "Strg+F", run: func() tea.Cmd { return a.openSearch(s) }},
			palItem{icon: "⚠", label: i18n.T("Fehlerzeilen im Verlauf zeigen"), hint: i18n.T("Strg+F Strg+E"), run: func() tea.Cmd {
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
			actions = append(actions, palItem{icon: "▦", label: label, hint: a.keys.label(actSplit), run: func() tea.Cmd { a.setSplit(m); return nil }})
		}
	}
	actions = append(actions, palItem{icon: "●", label: i18n.T("KI-Status: Endpunkt erreichbar? jetzt prüfen"), hint: "llm",
		run: func() tea.Cmd { a.openLLMStatus(); return a.llmProbeCmd() }})
	for _, x := range actions {
		x.group = pgAction
		if match(x.label) || (x.hint == "Language" && match("sprache language english deutsch german")) {
			items = append(items, x)
		}
	}
	return items
}

func (a *App) paletteKey(k tea.KeyPressMsg) tea.Cmd {
	p := a.pal
	items := a.paletteItems()
	switch key := k.String(); {
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
		// Tab = „KI fragen“ (zweiter Eintrag, wenn Text da ist)
		for _, it := range items {
			if it.group == pgAI && strings.HasPrefix(it.label, i18n.T("KI fragen")) {
				a.pal = nil
				return it.run()
			}
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
		a.pal = nil
		return it.run()
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
	b.WriteString(promptBlock(st.AI.Bold(true).Render("✦ "), p.in.View()) + "\n")
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
			b.WriteString(st.Muted.Render(i18n.T(palGroupName[it.group])) + "\n")
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
		hint := st.Muted.Render(it.hint)
		if it.danger != "" {
			hint = st.Danger.Render("⚠ " + it.danger)
			if p.armed == i {
				hint = st.Danger.Render(i18n.T("nochmal ⏎"))
			}
		}
		label := trunc(it.label, w-8-lipgloss.Width(hint))
		gap := max(1, w-6-lipgloss.Width(label)-lipgloss.Width(hint))
		line := iconStyle.Render(it.icon) + " " + st.Text.Render(label) + strings.Repeat(" ", gap) + hint
		if i == p.sel {
			line = st.Selected.Width(w - 4).Render(it.icon + " " + label + strings.Repeat(" ", gap) + ansiPlain(hint))
		}
		i, it := i, it
		line = a.z.click(line, func(tea.Mouse, bool) tea.Cmd {
			p.sel = i
			if it.danger != "" && p.armed != i {
				p.armed = i // rot: erst scharf schalten, zweiter Klick führt aus
				return nil
			}
			a.pal = nil
			return it.run()
		})
		b.WriteString(line + "\n")
		rows++
	}
	if len(items) == 0 {
		b.WriteString(st.Muted.Render(i18n.T("Nichts gefunden. Im Terminal wird Text zum Ziel oder zur Frage.")) + "\n")
	}
	b.WriteString("\n" + st.Muted.Render(fmt.Sprintf(i18n.T("↑↓ wählen · ⏎ ausführen · Tab KI fragen · Esc schließen · %d Einträge"), len(items))))
	box := st.Modal.BorderForeground(st.T.AI).Padding(0, 1).Width(w).Render(b.String())
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
