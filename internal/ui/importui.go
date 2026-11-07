package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/importer"
	"github.com/db9979/sshtermy/internal/store"
)

// Import (i im Manager): sucht beim Öffnen selbst nach ~/.ssh/config,
// PuTTY-Sitzungen und MobaXterm. Neue Funde sind vorausgewählt, ⏎ übernimmt
// alle auf einmal. Schon vorhandene Ziele sind abgewählt und markiert.

type importItem struct {
	e     importer.Entry
	on    bool
	known bool
}

type importState struct {
	results []importer.Result
	items   []importItem
	sel     int
	asking  bool
	path    textinput.Model
	err     string
}

func (a *App) openImport() tea.Cmd {
	a.imp = importState{path: newInput("", false)}
	a.imp.path.Placeholder = i18n.T(`Pfad: ~/.ssh/config, putty.reg, MobaXterm.ini, Ansible-Inventar, *.csv`)
	for _, r := range importer.Scan() {
		a.addImport(r)
	}
	a.scr = scrImport
	return nil
}

func (a *App) addImport(r importer.Result) {
	a.imp.results = append(a.imp.results, r)
	existing := a.st.Connections()
	for _, e := range r.Entries {
		known := importer.IsKnown(e.Conn, existing)
		for _, it := range a.imp.items { // doppelt in zwei Quellen
			if importer.IsKnown(e.Conn, []domain.Connection{it.e.Conn}) {
				known = true
			}
		}
		a.imp.items = append(a.imp.items, importItem{e: e, on: !known, known: known})
	}
}

func (a *App) importKeys() [][2]string {
	if a.imp.asking {
		return [][2]string{{"⏎", i18n.T("Datei lesen")}, {"Esc", i18n.T("abbrechen")}}
	}
	return [][2]string{{"⏎", fmt.Sprintf(i18n.T("%d übernehmen"), a.importCount())}, {"Space", i18n.T("an/aus")}, {"a", i18n.T("alle/keine")}, {"f", i18n.T("Datei laden")}, {"e", i18n.T("als CSV exportieren")}, {"Esc", i18n.T("zurück")}}
}

func (a *App) importCount() int {
	n := 0
	for _, it := range a.imp.items {
		if it.on {
			n++
		}
	}
	return n
}

func (a *App) updateImport(msg tea.Msg) tea.Cmd {
	m := &a.imp
	if p, ok := msg.(tea.PasteMsg); ok && m.asking {
		var cmd tea.Cmd
		m.path, cmd = m.path.Update(p)
		return cmd
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	if m.asking {
		switch k.String() {
		case "esc":
			m.asking = false
			m.path.Blur()
		case "enter":
			m.asking = false
			m.path.Blur()
			p := strings.TrimSpace(m.path.Value())
			if p == "" {
				return nil
			}
			r := importer.File(p)
			if r.Err != nil {
				m.err = r.Path + ": " + r.Err.Error()
				return nil
			}
			before := len(m.items)
			a.addImport(r)
			m.err = ""
			a.flash(fmt.Sprintf(i18n.T("%s: %d Verbindungen gefunden"), r.Source.Label(), len(m.items)-before))
		default:
			var cmd tea.Cmd
			m.path, cmd = m.path.Update(k)
			return cmd
		}
		return nil
	}
	switch k.String() {
	case "esc", "q":
		a.scr = scrManager
	case "up", "k":
		m.sel = max(0, m.sel-1)
	case "down", "j":
		m.sel = min(len(m.items)-1, m.sel+1)
	case "space", " ", "x":
		if m.sel < len(m.items) {
			m.items[m.sel].on = !m.items[m.sel].on
		}
	case "a":
		on := a.importCount() < len(m.items)
		for i := range m.items {
			m.items[i].on = on
		}
	case "e":
		return a.exportConnsCSV()
	case "f", "/":
		m.asking = true
		return m.path.Focus()
	case "enter":
		n, unresolved, err := a.commitImport()
		if err != nil {
			m.err = err.Error()
			return nil
		}
		a.scr = scrManager
		msg := fmt.Sprintf(i18n.T("%s importiert"), plural(n, i18n.T("Verbindung"), i18n.T("Verbindungen")))
		if unresolved > 0 {
			msg += fmt.Sprintf(i18n.T(" · %d Sprung-Host(s) nicht gefunden"), unresolved)
		}
		a.flash(msg)
	}
	return nil
}

// commitImport legt die gewählten Funde an und verknüpft Sprung-Hosts.
func (a *App) commitImport() (n, unresolved int, err error) {
	var cs []domain.Connection
	var jumps []string
	for _, it := range a.imp.items {
		if !it.on {
			continue
		}
		c := it.e.Conn
		c.ID = store.NewID()
		cs = append(cs, c)
		jumps = append(jumps, it.e.Jump)
	}
	if len(cs) == 0 {
		return 0, 0, errors.New(i18n.T("nichts ausgewählt (Space wählt aus)"))
	}
	pool := append(append([]domain.Connection(nil), cs...), a.st.Connections()...)
	for i, j := range jumps {
		if j == "" {
			continue
		}
		if id := findJump(j, pool); id != "" && id != cs[i].ID {
			cs[i].JumpHostID = id
		} else {
			unresolved++
		}
	}
	if _, err := a.st.AddConnections(cs); err != nil {
		return 0, 0, err
	}
	return len(cs), unresolved, nil
}

// findJump: „alias“, „host“, „user@host“ oder „user@host:port“.
func findJump(j string, pool []domain.Connection) string {
	user, host, port := "", j, 0
	if i := strings.LastIndex(host, "@"); i >= 0 {
		user, host = host[:i], host[i+1:]
	}
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host[:i], ":") {
		port, _ = strconv.Atoi(host[i+1:])
		host = host[:i]
	}
	for _, c := range pool {
		if strings.EqualFold(c.Alias, j) {
			return c.ID
		}
	}
	for _, c := range pool {
		if (strings.EqualFold(c.Host, host) || strings.EqualFold(c.Alias, host)) &&
			(user == "" || c.Username == user) && (port == 0 || c.Port == port) {
			return c.ID
		}
	}
	return ""
}

func (a *App) viewImport() string {
	s := a.s
	m := &a.imp
	w := min(110, a.w-2)
	var b strings.Builder
	b.WriteString(s.Accent.Render(i18n.T(" ↓ Verbindungen importieren")) + "\n\n")
	for _, r := range m.results {
		state := s.OK.Render(plural(len(r.Entries), i18n.T("Verbindung"), i18n.T("Verbindungen")))
		switch {
		case errors.Is(r.Err, importer.ErrNotFound):
			state = s.Muted.Render(i18n.T("nicht gefunden"))
		case r.Err != nil:
			state = s.Danger.Render(r.Err.Error())
		}
		b.WriteString(" " + s.Label.Render(r.Source.Label()) + state + s.Muted.Render("  "+trunc(r.Path, w-50)) + "\n")
	}
	b.WriteString("\n")
	if m.asking {
		m.path.SetWidth(w - 12)
		b.WriteString(" " + s.Accent.Render(i18n.T("Datei: ")) + m.path.View() + "\n\n")
	}
	if m.err != "" {
		b.WriteString(" " + s.Danger.Render(m.err) + "\n\n")
	}
	if len(m.items) == 0 {
		b.WriteString(s.Muted.Render(i18n.T(" Nichts gefunden. f = Datei laden (PuTTY-.reg-Export, MobaXterm.ini, .mxtsessions, ssh_config, Ansible-Inventar oder CSV, auch NetBox-Export).\n") +
			i18n.T(" PuTTY exportieren: reg export HKCU\\Software\\SimonTatham\\PuTTY putty.reg")))
		return b.String()
	}
	// Liste mit Fenster um die Auswahl
	head := lipgloss.Height(b.String())
	rows := max(3, a.h-2-head-3)
	start := 0
	if m.sel >= rows {
		start = m.sel - rows + 1
	}
	for i := start; i < len(m.items) && i < start+rows; i++ {
		it := m.items[i]
		c := it.e.Conn
		box := "[ ]"
		if it.on {
			box = "[x]"
		}
		target := c.Username + "@" + c.Host
		if c.Port != 22 {
			target += ":" + strconv.Itoa(c.Port)
		}
		extra := it.e.Source.Label()
		if c.Group != "" {
			extra += " · " + c.Group
		}
		if it.e.Jump != "" {
			extra += i18n.T(" · über ") + it.e.Jump
		}
		note := ""
		if it.known {
			note = i18n.T("schon vorhanden")
		} else if len(it.e.Notes) > 0 {
			note = it.e.Notes[0]
		}
		line := fmt.Sprintf(" %s %-22s %-32s %s", box, trunc(c.Alias, 22), trunc(target, 32), extra)
		line = trunc(line, w-2)
		var out string
		if i == m.sel {
			full := line
			if note != "" {
				full = trunc(line+"  ! "+note, w-2)
			}
			out = s.Selected.Width(w - 1).Render(full)
		} else {
			st := s.Text
			if it.known {
				st = s.Muted
			}
			out = st.Render(line)
			if note != "" && lipgloss.Width(line)+4 < w-2 {
				ns := s.Warn
				if it.known {
					ns = s.Muted
				}
				out += ns.Render("  ! " + trunc(note, w-6-lipgloss.Width(line)))
			}
		}
		i := i
		out = a.z.click(out, func(_ tea.Mouse, double bool) tea.Cmd {
			m.sel = i
			m.items[i].on = !m.items[i].on
			return nil
		})
		b.WriteString(out + "\n")
	}
	b.WriteString("\n" + s.Muted.Render(i18n.T(" Passwörter werden nicht übernommen; sshTermy fragt beim ersten Verbinden danach.")))
	return b.String()
}

// exportConnsCSV: alle Verbindungen als CSV in den Downloads-Ordner
// (ohne Passwörter und Keys; mit f wieder einlesbar).
func (a *App) exportConnsCSV() tea.Cmd {
	conns := a.st.Connections()
	if len(conns) == 0 {
		a.flash(i18n.T("Keine Verbindungen zum Exportieren"))
		return nil
	}
	data := importer.ExportCSV(conns, func(id string) string {
		if c, ok := a.st.Connection(id); ok {
			return c.Alias
		}
		return ""
	})
	p := filepath.Join(exportDir(), "sshtermy-verbindungen-"+time.Now().Format("2006-01-02")+".csv")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		a.flashErr(i18n.T("Export fehlgeschlagen: ") + err.Error())
		return nil
	}
	a.flashOK(fmt.Sprintf(i18n.T("%s exportiert (ohne Passwörter): "), plural(len(conns), i18n.T("Verbindung"), i18n.T("Verbindungen"))) + p)
	return nil
}
