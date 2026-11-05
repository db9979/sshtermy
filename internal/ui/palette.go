package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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
	pgRunbook
	pgTunnel
	pgConnect
	pgAction
)

var palGroupName = map[palGroup]string{pgAI: "KI", pgSuggest: "VORSCHLÄGE", pgRunbook: "RUNBOOKS", pgTunnel: "TUNNEL", pgConnect: "VERBINDEN", pgAction: "AKTIONEN"}

type palItem struct {
	group  palGroup
	icon   string
	label  string
	hint   string
	danger string
	run    func() tea.Cmd
}

type palette struct {
	in    textinput.Model
	sel   int
	armed int
}

func isPaletteKey(k string) bool { return k == "ctrl+space" || k == "ctrl+@" }

func (a *App) openPalette() tea.Cmd {
	in := newInput("", false)
	in.Placeholder = "Ziel, Frage, Befehl, Tunnel oder Verbindung …"
	a.pal = &palette{in: in, armed: -1}
	return a.pal.in.Focus()
}

func (a *App) paletteItems() []palItem {
	q := strings.TrimSpace(a.pal.in.Value())
	ql := strings.ToLower(q)
	s := a.current()
	inTerm := a.scr == scrTerminal && s != nil && s.state == stConnected
	var items []palItem
	if q != "" && inTerm {
		items = append(items,
			palItem{group: pgAI, icon: "◎", label: "Als Ziel starten: „" + trunc(q, 50) + "“", hint: "⏎",
				run: func() tea.Cmd { return a.startGoal(s, q) }},
			palItem{group: pgAI, icon: "✦", label: "KI fragen: „" + trunc(q, 50) + "“", hint: "Tab",
				run: func() tea.Cmd { return a.startAsk(s, q) }})
	}
	match := func(x string) bool { return ql == "" || strings.Contains(strings.ToLower(x), ql) }
	if inTerm {
		for _, c := range s.chips {
			c := c
			if !match(c.Command) {
				continue
			}
			it := palItem{group: pgSuggest, icon: "$", label: c.Command, hint: "einfügen",
				run: func() tea.Cmd { s.term.SendText(c.Command); s.panelFocus = false; return nil }}
			if len(c.Danger) > 0 {
				it.danger = c.Danger[0].Reason
			} else if c.Elevated {
				it.hint = "sudo"
			}
			items = append(items, it)
		}
		for _, rb := range a.st.Runbooks(s.conn.ID) {
			rb := rb
			if !match(rb.Name + " " + rb.Goal + " " + strings.Join(rb.Steps, " ")) {
				continue
			}
			items = append(items, palItem{group: pgRunbook, icon: "▶", label: rb.Name, hint: plural(len(rb.Steps), "Schritt", "Schritte"),
				run: func() tea.Cmd { return a.startRunbook(s, rb) }})
		}
		for _, t := range a.st.Tunnels(s.conn.ID) {
			t := t
			if !match(t.Name + " " + t.Spec()) {
				continue
			}
			on := s.tunnels != nil && s.tunnels.Get(t.ID) != nil
			label, hint := "„"+t.Name+"“ starten · "+t.Spec(), "an"
			if on {
				label, hint = "„"+t.Name+"“ stoppen", "aus"
				if at := s.tunnels.Get(t.ID); at.URL() != "" {
					u := at.URL()
					items = append(items, palItem{group: pgTunnel, icon: "↗", label: "Öffnen: " + u, hint: "Browser",
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
					a.flash("Tunnel: " + err.Error())
				} else if at.URL() != "" {
					a.flash("Web-Service bereit: " + at.URL())
				}
				return nil
			}})
		}
	}
	for _, c := range a.st.Connections() {
		c := c
		if !match(c.Alias + " " + c.Host + " " + c.Group) {
			continue
		}
		items = append(items, palItem{group: pgConnect, icon: "▸", label: c.Alias, hint: c.Username + "@" + c.Host,
			run: func() tea.Cmd {
				for i, x := range a.sessions {
					if x.conn.ID == c.ID && x.state != stClosed {
						a.cur, a.scr = i, scrTerminal
						return nil
					}
				}
				return a.connect(c)
			}})
	}
	actions := []palItem{
		{icon: "◎", label: "Ziel-Modus öffnen", hint: "F2", run: func() tea.Cmd {
			if inTerm {
				withPanel(s, panelGoal)
				a.resize(s)
				return a.panelOpened(s)
			}
			return nil
		}},
		{icon: "⇄", label: "Tunnel-Panel", hint: "F4", run: func() tea.Cmd {
			if inTerm {
				withPanel(s, panelTunnels)
				a.resize(s)
			}
			return nil
		}},
		{icon: "≡", label: "Verbindungs-Manager", hint: "F10", run: func() tea.Cmd { a.scr = scrManager; return nil }},
		{icon: "+", label: "Neue Verbindung", hint: "n", run: func() tea.Cmd { return a.openConnForm(newConnDefaults()) }},
		{icon: "✦", label: "KI-Einstellungen", hint: "s", run: func() tea.Cmd { return a.openSettings() }},
		{icon: "?", label: "Tastenkürzel", hint: "F1", run: func() tea.Cmd { a.modal = a.helpModal(); return nil }},
		{icon: "i", label: "Über sshTermy / Update", hint: "F12", run: func() tea.Cmd { a.modal = a.aboutModal(); return nil }},
	}
	if !inTerm {
		actions = actions[2:]
	}
	for _, x := range actions {
		x.group = pgAction
		if match(x.label) {
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
			if it.group == pgAI && strings.HasPrefix(it.label, "KI fragen") {
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
	b.WriteString(st.AI.Bold(true).Render("✦ ") + p.in.View() + "\n")
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
			b.WriteString(st.Muted.Render(palGroupName[it.group]) + "\n")
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
				hint = st.Danger.Render("nochmal ⏎")
			}
		}
		label := trunc(it.label, w-8-lipgloss.Width(hint))
		gap := max(1, w-6-lipgloss.Width(label)-lipgloss.Width(hint))
		line := iconStyle.Render(it.icon) + " " + st.Text.Render(label) + strings.Repeat(" ", gap) + hint
		if i == p.sel {
			line = st.Selected.Width(w - 4).Render(it.icon + " " + label + strings.Repeat(" ", gap) + ansiPlain(hint))
		}
		b.WriteString(line + "\n")
		rows++
	}
	if len(items) == 0 {
		b.WriteString(st.Muted.Render("Nichts gefunden. Im Terminal wird Text zum Ziel oder zur Frage.") + "\n")
	}
	b.WriteString("\n" + st.Muted.Render(fmt.Sprintf("↑↓ wählen · ⏎ ausführen · Tab KI fragen · Esc schließen · %d Einträge", len(items))))
	return st.Modal.BorderForeground(st.T.AI).Padding(0, 1).Width(w).Render(b.String())
}

func ansiPlain(s string) string { return lipgloss.NewStyle().Render(ansi.Strip(s)) }
