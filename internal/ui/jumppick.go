package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Auswahl im Verbindungsformular: Ordner und Jump-Host über eine
// durchsuchbare Liste statt Durchschalten. Beim Jump-Host zeigt jede Zeile
// Ordner, Ziel, Status und die eigene Kette; Einträge, die einen Kreis
// bilden würden, sind gesperrt.

func (a *App) pickFolderField() tea.Cmd {
	f := a.form
	if f == nil {
		return nil
	}
	fl := f.get("group")
	tree := a.st.Tree()
	p := newPicker(i18n.T("Ordner wählen"), i18n.T("Ordner suchen oder neuen Namen tippen"), folderItems(tree, ""), fl.value, func(it pickItem) tea.Cmd {
		fl.value = it.value
		return nil
	})
	p.extra = newFolderExtra(tree)
	return a.openPicker(p)
}

// jumpGet: Verbindungen nachschlagen, mit dem gerade bearbeiteten Stand.
func (a *App) jumpGet(self domain.Connection, jump string) func(string) (domain.Connection, bool) {
	return func(id string) (domain.Connection, bool) {
		if self.ID != "" && id == self.ID {
			c := self
			c.JumpHostID = jump
			return c, true
		}
		return a.st.Connection(id)
	}
}

// jumpFieldText: Anzeige im Formular, z. B. „jump-lan · ops@10.0.0.1  Kette: Dieser PC → bastion → jump-lan → web1“.
func (a *App) jumpFieldText(self domain.Connection, jump string) string {
	s := a.s
	if jump == "" {
		return s.Muted.Render(i18n.T("— direkt (ohne Jump-Host)"))
	}
	j, ok := a.st.Connection(jump)
	if !ok {
		return s.Danger.Render(i18n.T("⚠ Jump-Host gelöscht"))
	}
	chain, cycle := domain.JumpChain(jump, a.jumpGet(self, jump))
	names := []string{i18n.T("Dieser PC")}
	for _, c := range chain {
		names = append(names, c.Alias)
	}
	me := self.Alias
	if me == "" {
		me = i18n.T("diese Verbindung")
	}
	names = append(names, me)
	out := j.Alias + s.Muted.Render(" · "+j.Username+"@"+j.Host)
	if cycle {
		return out + "  " + s.Danger.Render(i18n.T("⚠ Kreis"))
	}
	return out + "  " + s.Muted.Render(strings.Join(names, " → "))
}

func (a *App) pickJumpField(self domain.Connection) tea.Cmd {
	f := a.form
	if f == nil {
		return nil
	}
	fl := f.get("jump")
	items := []pickItem{{value: "", label: i18n.T("— direkt (ohne Jump-Host)")}}
	all := a.st.Tree().AllConns()
	aw := 8
	for _, c := range all {
		aw = max(aw, min(20, len([]rune(c.Alias))))
	}
	for _, c := range all {
		if (self.ID != "" && c.ID == self.ID) || c.Protocol.Raw() {
			continue // Telnet und Konsolen können nicht weiterleiten
		}
		it := pickItem{value: c.ID, label: fmt.Sprintf("%-*s", aw, trunc(c.Alias, aw)), search: c.Alias + " " + c.Host + " " + c.Username + " " + c.Group + " " + strings.Join(c.Tags, " ")}
		folder := c.Group
		if folder == "" {
			folder = i18n.T("oberste Ebene")
		}
		it.detail = fmt.Sprintf("%s@%s · %s", c.Username, c.ShowAddr(), trunc(folder, 24))
		var hint []string
		if c.JumpHostID != "" {
			chain, _ := domain.JumpChain(c.JumpHostID, a.st.Connection)
			var ns []string
			for _, x := range chain {
				ns = append(ns, x.Alias)
			}
			hint = append(hint, i18n.T("über ")+trunc(strings.Join(ns, " → "), 24))
		}
		if a.connected(c.ID) {
			hint = append(hint, i18n.T("● verbunden"))
		}
		if c.Device == domain.DeviceNetwork {
			hint = append(hint, "Switch")
		}
		it.hint = strings.Join(hint, " · ")
		if self.ID != "" && domain.JumpCreatesCycle(self.ID, c.ID, a.st.Connection) {
			it.disabled = i18n.T("geht selbst über „") + orDefault(self.Alias, i18n.T("diese Verbindung")) + i18n.T("“ (Kreis)")
		}
		items = append(items, it)
	}
	title := i18n.T("Jump-Host wählen")
	if self.Alias != "" {
		title += i18n.T(" für „") + self.Alias + "“"
	}
	p := newPicker(title, i18n.T("Name, Host, Ordner oder Tag"), items, fl.value, func(it pickItem) tea.Cmd {
		fl.value = it.value
		return nil
	})
	p.help = i18n.T("✓ aktuell · ● verbunden")
	return a.openPicker(p)
}
