package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Feste Sitzung (Idee: Dominik Bornhäußer): Beim Verbinden landet man immer
// in derselben tmux- oder screen-Sitzung, auch von einem anderen Rechner aus.
// Zwei Tabs oder zwei Rechner mit demselben Sitzungsnamen sehen denselben
// Bildschirm (Spiegeln); „nur zuschauen“ hängt tmux schreibgeschützt an.
//
// Ablauf: Nach dem Login fragt sshTermy einmal unsichtbar, ob tmux/screen
// installiert sind und welche Sitzungen laufen. Gibt es die benannte
// Sitzung, wird sie angehängt; laufen nur andere, erscheint eine Auswahl;
// sonst wird sie angelegt. Der Befehl wird sichtbar am Prompt getippt.
// Netzwerkgeräte, Telnet und Konsole: nie (keine Exec-Abfrage, kein tmux).

var persistLabels = []string{"aus", "automatisch", "tmux", "screen"}

func persistLabel(p string) string {
	switch p {
	case "auto":
		return persistLabels[1]
	case "tmux", "screen":
		return p
	}
	return persistLabels[0]
}

func persistFromLabel(l string) string {
	switch l {
	case persistLabels[1]:
		return "auto"
	case "tmux", "screen":
		return l
	}
	return ""
}

func addPersistFields(f *form, c domain.Connection, raw func(*form) bool) {
	off := func(f *form) bool { return raw(f) || deviceFromLabel(f.sel("device")) == domain.DeviceNetwork }
	f.choice("persist", i18n.T("Feste Sitzung"), persistLabels, persistLabel(c.Persist),
		i18n.T("beim Verbinden immer in dieselbe tmux-/screen-Sitzung, auch von anderen Rechnern")).hidden = off
	sub := func(f *form) bool { return off(f) || persistFromLabel(f.sel("persist")) == "" }
	f.text("pname", i18n.T("Sitzungsname"), c.PersistName,
		i18n.T("leer = „sshtermy“ · gleicher Name auf zwei Rechnern = derselbe Bildschirm")).hidden = sub
	f.toggle("pview", i18n.T("Nur zuschauen"), c.PersistView,
		i18n.T("nur tmux: mitlesen, was in der Sitzung passiert, ohne mitzutippen")).hidden = func(f *form) bool {
		return sub(f) || f.sel("persist") == "screen"
	}
}

func applyPersistFields(f *form, c *domain.Connection) {
	c.Persist = persistFromLabel(f.sel("persist"))
	c.PersistName = strings.TrimSpace(f.str("pname"))
	c.PersistView = f.flag("pview")
	if c.Persist == "" || c.Protocol.Raw() {
		c.Persist, c.PersistView = "", false
	}
	if c.Persist == "screen" {
		c.PersistView = false
	}
}

// muxSession: eine laufende tmux-/screen-Sitzung auf dem Server.
type muxSession struct {
	kind, name string
	attached   bool
}

// persistProbe: was der Server an tmux/screen hat.
type persistProbe struct {
	tmux, screen bool
	sessions     []muxSession
}

const persistProbeCmd = `command -v tmux >/dev/null 2>&1 && echo 'have tmux'; ` +
	`command -v screen >/dev/null 2>&1 && echo 'have screen'; ` +
	`tmux list-sessions -F 'tmux #{session_attached} #{session_name}' 2>/dev/null; ` +
	`screen -ls 2>/dev/null; true`

func parsePersistProbe(out string) persistProbe {
	var p persistProbe
	for _, l := range strings.Split(out, "\n") {
		t := strings.TrimSpace(strings.TrimRight(l, "\r"))
		switch {
		case t == "have tmux":
			p.tmux = true
		case t == "have screen":
			p.screen = true
		case strings.HasPrefix(t, "tmux "):
			f := strings.SplitN(t, " ", 3)
			if len(f) == 3 && f[2] != "" {
				n, _ := strconv.Atoi(f[1])
				p.sessions = append(p.sessions, muxSession{kind: "tmux", name: f[2], attached: n > 0})
			}
		case strings.HasPrefix(l, "\t"):
			// screen -ls: "\t12345.name\t(06.11.2026 10:00:00)\t(Detached)"
			id := strings.Fields(t)
			if len(id) == 0 {
				continue
			}
			pid, name, ok := strings.Cut(id[0], ".")
			if _, err := strconv.Atoi(pid); !ok || err != nil || name == "" {
				continue
			}
			low := strings.ToLower(t)
			p.sessions = append(p.sessions, muxSession{kind: "screen", name: name,
				attached: strings.Contains(low, "attached)") && !strings.Contains(low, "detached")})
		}
	}
	return p
}

// persistPlan: was nach dem Verbinden passiert.
type persistPlan struct {
	mux  muxState     // kind != "": so anhängen (Vorgabe, auch für die Auswahl)
	pick []muxSession // andere Sitzungen laufen schon: Auswahl zeigen
	hint string       // warum keine feste Sitzung
}

func planPersist(c domain.Connection, p persistProbe) persistPlan {
	kind := c.Persist
	if kind == "auto" {
		kind = ""
		if p.tmux {
			kind = "tmux"
		} else if p.screen {
			kind = "screen"
		}
	}
	if kind == "" {
		return persistPlan{hint: i18n.T("Feste Sitzung: weder tmux noch screen auf dem Server (z. B. sudo apt install tmux)")}
	}
	if (kind == "tmux" && !p.tmux) || (kind == "screen" && !p.screen) {
		return persistPlan{hint: i18n.F("Feste Sitzung: %s fehlt auf dem Server (z. B. sudo apt install %s)", kind, kind)}
	}
	if c.PersistView && kind != "tmux" {
		return persistPlan{hint: i18n.T("Feste Sitzung: nur zuschauen geht nur mit tmux")}
	}
	name := c.PersistSession()
	m := muxState{kind: kind, name: name, shared: true, view: c.PersistView}
	var same []muxSession
	for _, s := range p.sessions {
		if s.kind != kind {
			continue
		}
		if s.name == name {
			return persistPlan{mux: m}
		}
		same = append(same, s)
	}
	if len(same) > 0 {
		m.create = !m.view
		return persistPlan{mux: m, pick: same}
	}
	if m.view {
		return persistPlan{hint: i18n.F("Feste Sitzung: „%s“ läuft noch nicht, zum Zuschauen muss sie schon offen sein", name)}
	}
	m.create = true
	return persistPlan{mux: m}
}

type persistMsg struct {
	sid int
	out string
	err error
}

func probePersist(sid int, c *sshx.Client) tea.Cmd {
	return func() tea.Msg {
		out, err := c.Output(persistProbeCmd, 5*time.Second)
		return persistMsg{sid, out, err}
	}
}

// persistOnConnect: nach dem ersten Verbinden die feste Sitzung vorbereiten.
// Beim Neuverbinden und nach dem Wiederherstellen hängt reconnect.go an.
func (a *App) persistOnConnect(s *session) tea.Cmd {
	if s.conn.Persist == "" || s.client == nil || s.client.NetDevice || s.rc.active || s.mux.kind != "" || s.muxPending {
		return nil
	}
	return probePersist(s.id, s.client)
}

func (a *App) onPersist(m persistMsg) tea.Cmd {
	s := a.byID(m.sid)
	if s == nil || s.state != stConnected || s.mux.kind != "" || s.muxPending {
		return nil
	}
	if m.err != nil && strings.TrimSpace(m.out) == "" {
		a.flash(i18n.T("Feste Sitzung: Server ließ sich nicht abfragen (") + m.err.Error() + ")")
		return nil
	}
	plan := planPersist(s.conn, parsePersistProbe(m.out))
	if plan.hint != "" {
		a.flash(plan.hint)
		return nil
	}
	if len(plan.pick) == 0 || a.pick != nil || a.modal != nil || a.form != nil {
		a.startPersist(s, plan.mux)
		return nil
	}
	return a.openPicker(persistPicker(a, s, plan))
}

func persistPicker(a *App, s *session, plan persistPlan) *picker {
	var items []pickItem
	if !plan.mux.view {
		items = append(items, pickItem{value: "\x00new", label: i18n.F("Neue Sitzung „%s“ anlegen", plan.mux.name)})
	}
	for _, x := range plan.pick {
		hint := i18n.T("frei")
		if x.attached {
			hint = i18n.T("gerade angehängt")
		}
		items = append(items, pickItem{value: x.name, label: x.name, detail: x.kind, hint: hint})
	}
	items = append(items, pickItem{value: "\x00shell", label: i18n.T("Normale Shell (ohne ") + plan.mux.kind + ")"})
	title := s.conn.Alias + i18n.T(": welche Sitzung?")
	p := newPicker(title, i18n.T("Sitzung suchen"), items, "\x00", func(it pickItem) tea.Cmd {
		m := plan.mux
		switch it.value {
		case "\x00shell":
			return nil
		case "\x00new":
		default:
			m.name, m.create = it.value, false
		}
		a.startPersist(s, m)
		return nil
	})
	p.help = i18n.T("Esc = normale Shell")
	return p
}

func (a *App) startPersist(s *session, m muxState) {
	if a.byID(s.id) != s || s.state != stConnected {
		return
	}
	s.mux, s.muxPending = m, true
	a.flash(i18n.T("Feste Sitzung: ") + m.label())
}

// persistPalette: Einträge in der Befehlspalette, solange eine feste
// tmux-Sitzung läuft (Spiegeln: zweiter Tab oder anderer Rechner schaut zu).
func (a *App) persistPalette(s *session) []palItem {
	if s == nil || s.state != stConnected || s.mux.kind != "tmux" || !s.mux.shared || s.netDev() {
		return nil
	}
	m := s.mux
	return []palItem{
		{icon: ic("watch"), label: i18n.T("Feste Sitzung in zweitem Tab zuschauen"), hint: "tmux -r", run: func() tea.Cmd {
			c := s.conn
			c.Persist, c.PersistName, c.PersistView = "tmux", m.name, true
			c.Alias += i18n.T(" · zuschauen")
			return a.connect(c)
		}},
		{icon: ic("copy"), label: i18n.T("Befehl zum Mitschauen kopieren"), hint: i18n.T("für andere Rechner"), run: func() tea.Cmd {
			cmd := watchCommand(s.conn, m.name)
			go func() { _ = clipboard.WriteAll(cmd) }()
			a.flashOK(i18n.T("Kopiert: ") + cmd)
			return nil
		}},
	}
}

// watchCommand: ssh-Befehl, mit dem jemand ohne sshTermy schreibgeschützt zuschaut.
func watchCommand(c domain.Connection, name string) string {
	b := "ssh -t"
	if c.Port != 0 && c.Port != 22 {
		b += fmt.Sprintf(" -p %d", c.Port)
	}
	host := c.Host
	if c.Username != "" {
		host = c.Username + "@" + host
	}
	return b + " " + host + " tmux attach -r -t " + shellQuote(name)
}
