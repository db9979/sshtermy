package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/audit"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Mehrfachauswahl im Manager: Leertaste markiert eine Verbindung, * alle
// sichtbaren. Solange etwas markiert ist, gelten e (Sammeländerung),
// Verschieben und d (löschen) für alle markierten; Esc hebt die Auswahl auf.

const bulkKeep = "unverändert"

// mgrMarked: IDs der markierten Verbindungen, die es noch gibt (in der
// Reihenfolge der Liste). Verschwundene fallen dabei aus der Auswahl.
func (a *App) mgrMarked() []string {
	m := &a.mgr
	if len(m.marked) == 0 {
		return nil
	}
	var ids []string
	for _, c := range a.st.Connections() {
		if m.marked[c.ID] {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) != len(m.marked) {
		m.marked = map[string]bool{}
		for _, id := range ids {
			m.marked[id] = true
		}
	}
	return ids
}

func (a *App) mgrToggleMark(id string) {
	m := &a.mgr
	if m.marked == nil {
		m.marked = map[string]bool{}
	}
	if m.marked[id] {
		delete(m.marked, id)
	} else {
		m.marked[id] = true
	}
}

// mgrMarkAll markiert alle sichtbaren Verbindungen (mit Suche oder
// Tag-Filter: alle Treffer). Sind schon alle markiert, hebt es sie auf.
func (a *App) mgrMarkAll(rows []mgrRow) {
	m := &a.mgr
	if m.marked == nil {
		m.marked = map[string]bool{}
	}
	all := true
	for _, r := range rows {
		if !r.isFolder() && !m.marked[r.conn.ID] {
			all = false
		}
	}
	for _, r := range rows {
		if r.isFolder() {
			continue
		}
		if all {
			delete(m.marked, r.conn.ID)
		} else {
			m.marked[r.conn.ID] = true
		}
	}
}

// bulkKeys: Fußzeile, solange Verbindungen markiert sind.
func (a *App) bulkKeys(n int) [][2]string {
	km := a.keys
	return [][2]string{
		{i18n.T("Leertaste"), i18n.T("markieren")},
		{"*", i18n.T("alle")},
		{"e", fmt.Sprintf(i18n.T("%d ändern"), n)},
		{km.label(actMoveTo), i18n.T("verschieben")},
		{"d", i18n.T("löschen")},
		{"Esc", i18n.T("Auswahl aufheben")},
		{km.label(actHelp), i18n.T("Hilfe")},
	}
}

// bulkKey: Tasten, die bei einer Auswahl für alle markierten gelten.
// handled = false lässt die Taste normal weiterlaufen (Pfeile, Suche …).
func (a *App) bulkKey(key string, ids []string) (handled bool, cmd tea.Cmd) {
	switch {
	case key == "esc":
		a.mgr.marked = nil
		return true, nil
	case key == "e":
		return true, a.openBulkForm(ids)
	case key == "d":
		a.bulkDelete(ids)
		return true, nil
	case a.keys.is(actMoveTo, key):
		return true, a.bulkMoveDialog(ids)
	}
	return false, nil
}

func (a *App) bulkMoveDialog(ids []string) tea.Cmd {
	tree := a.st.Tree()
	title := fmt.Sprintf(i18n.T("%d Verbindungen verschieben nach …"), len(ids))
	p := newPicker(title, i18n.T("Ordner suchen oder neuen Namen tippen"), folderItems(tree, ""), "", func(it pickItem) tea.Cmd {
		if err := a.st.MoveConns(it.value, ids...); err != nil {
			a.flashErr(err.Error())
			return nil
		}
		a.expandTo(it.value)
		where := i18n.T("oberste Ebene")
		if it.value != "" {
			where = i18n.T("„") + it.value + i18n.T("“")
		}
		a.auditAdd(audit.EvSettings, "", fmt.Sprintf("%d Verbindungen verschoben nach %s", len(ids), it.value))
		a.flash(fmt.Sprintf(i18n.T("%d Verbindungen liegen jetzt in %s"), len(ids), where))
		a.mgr.marked = nil
		return nil
	})
	p.extra = newFolderExtra(tree)
	return a.openPicker(p)
}

// bulkDelete löscht alle markierten ohne Rückfrage, der Hinweis bietet wie
// beim einzelnen Löschen „Rückgängig“ an.
func (a *App) bulkDelete(ids []string) {
	open := 0
	for _, id := range ids {
		if a.connected(id) {
			open++
		}
	}
	if open > 0 {
		a.flashErr(fmt.Sprintf(i18n.T("%d der markierten Verbindungen sind noch offen. Erst die Tabs schließen."), open))
		return
	}
	tr, err := a.st.DeleteConnectionsTrash(ids)
	gone := tr.Conns()
	if err != nil {
		a.flashErr(err.Error())
		if len(gone) == 0 {
			return
		}
	}
	a.auditAdd(audit.EvSettings, "", fmt.Sprintf("%d Verbindungen gelöscht", len(gone)))
	a.mgr.marked = nil
	a.mgr.sel = max(0, min(a.mgr.sel, len(a.mgrRows())-1))
	a.offerUndo(fmt.Sprintf(i18n.T("%d Verbindungen gelöscht"), len(gone)), tr, func() {
		for _, c := range gone {
			if c.Keychain {
				sshx.ForgetKeychain(c.ID)
			}
		}
	}, func() {
		a.auditAdd(audit.EvSettings, "", fmt.Sprintf("%d Verbindungen wiederhergestellt", len(gone)))
		for _, c := range gone {
			a.expandTo(c.Group)
		}
	})
}

// openBulkForm: Sammeländerung. Leere Felder und „unverändert“ lassen den
// Wert jeder Verbindung, wie er ist.
func (a *App) openBulkForm(ids []string) tea.Cmd {
	if len(ids) == 0 {
		return nil
	}
	f := &form{title: fmt.Sprintf(i18n.T("%d Verbindungen ändern"), len(ids))}
	f.text("addtags", i18n.T("Tags hinzufügen"), "", i18n.T("durch Komma getrennt · leer = keine Änderung"))
	f.text("deltags", i18n.T("Tags entfernen"), "", i18n.T("durch Komma getrennt · leer = keine Änderung")).in.Placeholder = strings.Join(a.bulkTags(ids), ", ")
	f.text("user", i18n.T("Benutzer"), "", i18n.T("leer = keine Änderung · gilt nicht für lokale Shells"))
	f.choice("guard", i18n.T("Schutz"), []string{bulkKeep, "an", "aus"}, bulkKeep,
		i18n.T("an: gilt als produktiv, Gefährliches fragt nach · aus: nur für Test- und Laborrechner"))
	f.choice("color", i18n.T("Farbe"), append([]string{bulkKeep}, accentLabels()...), bulkKeep, i18n.T("Streifen in Tab, Statuszeile und Liste"))
	a.form, a.formBulk, a.scr = f, ids, scrBulkForm
	return f.setFocus(0)
}

// bulkTags: alle Tags der markierten Verbindungen (Vorschlag im Feld
// „Tags entfernen“).
func (a *App) bulkTags(ids []string) []string {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	seen := map[string]bool{}
	var tags []string
	for _, c := range a.st.Connections() {
		if !want[c.ID] {
			continue
		}
		for _, t := range c.Tags {
			if !seen[t] {
				seen[t] = true
				tags = append(tags, t)
			}
		}
	}
	sort.Strings(tags)
	return tags
}

func splitTags(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#")); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// bulkChange: die Änderung aus dem Formular als Funktion je Verbindung.
type bulkChange struct {
	add, del []string
	user     string
	guard    string // bulkKeep, "an", "aus"
	color    *string
}

func (b bulkChange) empty() bool {
	return len(b.add) == 0 && len(b.del) == 0 && b.user == "" && b.guard == bulkKeep && b.color == nil
}

func (b bulkChange) apply(c *domain.Connection) bool {
	changed := false
	if len(b.del) > 0 {
		keep := c.Tags[:0:0]
		for _, t := range c.Tags {
			drop := false
			for _, d := range b.del {
				drop = drop || strings.EqualFold(t, d)
			}
			if !drop {
				keep = append(keep, t)
			}
		}
		if len(keep) != len(c.Tags) {
			c.Tags, changed = keep, true
		}
	}
	for _, t := range b.add {
		has := false
		for _, x := range c.Tags {
			has = has || strings.EqualFold(x, t)
		}
		if !has {
			c.Tags, changed = append(c.Tags, t), true
		}
	}
	if b.user != "" && c.Protocol != domain.ProtoLocal && c.Username != b.user {
		c.Username, changed = b.user, true
	}
	if off := b.guard == "aus"; b.guard != bulkKeep && c.GuardOff != off {
		c.GuardOff, changed = off, true
	}
	if b.color != nil && c.Color != *b.color {
		c.Color, changed = *b.color, true
	}
	return changed
}

func (a *App) saveBulkForm() error {
	f := a.form
	b := bulkChange{add: splitTags(f.str("addtags")), del: splitTags(f.str("deltags")), user: f.str("user"), guard: f.sel("guard")}
	if col := f.sel("color"); col != bulkKeep {
		id := accentFromLabel(col)
		b.color = &id
	}
	if b.empty() {
		return fmt.Errorf("%s", i18n.T("Nichts geändert. Mindestens ein Feld ausfüllen oder mit Esc abbrechen."))
	}
	n, err := a.st.ChangeConns(a.formBulk, b.apply)
	if err != nil {
		return err
	}
	a.auditAdd(audit.EvSettings, "", fmt.Sprintf("Sammeländerung an %d Verbindungen", n))
	a.pushToast(toastOK, fmt.Sprintf(i18n.T("%d von %d Verbindungen geändert"), n, len(a.formBulk)))
	a.mgr.marked = nil
	return nil
}
