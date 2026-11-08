package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/team"
)

// Team-Freigabe: Verbindungen (samt Tunneln, Runbooks und Schnellbefehlen)
// als verschlüsselte Datei an Kollegen weitergeben. Passwörter und Keys
// gehen nie mit (internal/team). Die Datei kann per Mail, Teams oder einen
// gemeinsamen Ordner wandern; das Team-Passwort sagt man getrennt davon.
//
// Ablauf Teilen:      Liste (Auswahl) → Formular (Passwort, Optionen) → Datei
// Ablauf Übernehmen:  Formular (Datei, Passwort) → Liste (Vorschau) → Übernahme

type teamMode int

const (
	teamShare teamMode = iota
	teamTake
)

type teamItem struct {
	c     domain.Connection
	on    bool
	known bool // Übernehmen: gibt es schon (wird aktualisiert)
}

type teamState struct {
	mode  teamMode
	items []teamItem
	sel   int
	err   string
	pkg   *team.Package // Übernehmen: entschlüsselter Inhalt
	path  string
	ids   []string // Teilen: gewählte Verbindungen

	folder, user string // Übernehmen: Zielordner und Benutzer für Einträge ohne

	srcFolder string // Teilen: aus diesem Ordner geöffnet (Team-Ordner nimmt dann den ganzen Ordner)
	pass, sum string // Übernehmen: für das Abo
	abo       bool   // Übernehmen: abonnieren
}

// ---------------------------------------------------------------- Teilen

func (a *App) openTeamShare() tea.Cmd {
	ctx := a.mgrContext()
	a.team = teamState{mode: teamShare}
	cur, _ := a.mgrCur()
	for _, c := range a.st.Connections() {
		on := false
		switch {
		case cur.node != nil:
			on = domain.InFolder(c.Group, ctx) // Ordner gewählt: sein Inhalt
			a.team.srcFolder = ctx
		case cur.conn.ID != "":
			on = c.ID == cur.conn.ID
		}
		a.team.items = append(a.team.items, teamItem{c: c, on: on})
		if on && a.team.sel == 0 {
			a.team.sel = len(a.team.items) - 1
		}
	}
	if len(a.team.items) == 0 {
		a.flash(i18n.T("Noch keine Verbindungen zum Teilen"))
		return nil
	}
	a.scr = scrTeam
	return nil
}

func defaultTeamFile() string {
	return filepath.Join(exportDir(), "sshtermy-team-"+time.Now().Format("2006-01-02")+team.Ext)
}

func (a *App) openTeamShareForm() tea.Cmd {
	t := &a.team
	t.ids = nil
	for _, it := range t.items {
		if it.on {
			t.ids = append(t.ids, it.c.ID)
		}
	}
	if len(t.ids) == 0 {
		t.err = i18n.T("nichts ausgewählt (Space wählt aus)")
		return nil
	}
	f := &form{title: fmt.Sprintf(i18n.T("Team-Paket mit %s erstellen"), plural(len(t.ids), i18n.T("Verbindung"), i18n.T("Verbindungen")))}
	f.text("from", i18n.T("Von"), localUser(), i18n.T("steht beim Empfänger in der Vorschau"))
	f.secret("pass", i18n.T("Team-Passwort"), "", fmt.Sprintf(i18n.T("mind. %d Zeichen · leer = sshTermy erzeugt eines"), team.MinPassLen))
	f.toggle("users", i18n.T("Benutzernamen"), false, i18n.T("aus = jeder meldet sich mit seinem eigenen Namen an"))
	f.toggle("quick", i18n.T("Schnellbefehle"), true, i18n.T("eigene Schnellbefehle (⇧F1 …) der Verbindungen"))
	f.toggle("runbooks", "Runbooks", true, i18n.T("Runbooks dieser Verbindungen und allgemeine"))
	f.toggle("tunnels", "Tunnel", true, i18n.T("Port-Weiterleitungen der Verbindungen"))
	f.text("path", i18n.T("Datei"), defaultTeamFile(), i18n.T("z. B. in einen gemeinsamen Ordner (Teams, OneDrive, NAS) oder https://…/x.sshteam (WebDAV)"))
	f.toggle("abo", i18n.T("Aktuell halten"), false, i18n.T("Team-Ordner: sshTermy schreibt die Datei bei jeder Änderung neu"))
	a.form, a.scr = f, scrTeamForm
	return f.setFocus(1)
}

func (a *App) saveTeamShare() error {
	f := a.form
	d, _, err := a.st.Snapshot()
	if err != nil {
		return err
	}
	pass, generated := f.raw("pass"), false
	if pass == "" {
		pass, generated = team.Passphrase(), true
	}
	o := team.Options{Users: f.flag("users"), QuickCmds: f.flag("quick"), Runbooks: f.flag("runbooks"), Tunnels: f.flag("tunnels")}
	p, jumps := team.Build(d, a.team.ids, f.str("from"), o, time.Now())
	raw, err := team.Seal(p, []byte(pass))
	if err != nil {
		return err
	}
	path := expandHome(f.str("path"))
	if path == "" {
		path = defaultTeamFile()
	}
	if !strings.HasSuffix(strings.ToLower(path), team.Ext) {
		path += team.Ext
	}
	if !team.IsURL(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := team.WriteLoc(ctx, path, raw); err != nil {
		return err
	}
	abo := f.flag("abo")
	if abo {
		folder, ids := a.team.srcFolder, a.team.ids
		if folder != "" && sameIDs(team.LinkIDs(d, domain.TeamLink{Folder: folder}), ids) {
			ids = nil // ganzer Ordner: neue Verbindungen darin gehen automatisch mit
		} else {
			folder = ""
		}
		l := domain.TeamLink{Folder: folder, IDs: ids, From: f.str("from"), Users: o.Users, QuickCmds: o.QuickCmds, Runbooks: o.Runbooks, Tunnels: o.Tunnels}
		_, sum := team.BuildLink(d, l, time.Now())
		if folder != "" { // Paket so schreiben, wie es der Team-Ordner später auch tut (Ordner relativ)
			p2, _ := team.BuildLink(d, l, time.Now())
			if raw2, err := team.Seal(p2, []byte(pass)); err == nil {
				_ = team.WriteLoc(ctx, path, raw2)
			}
		}
		if err := a.addPublishLink(path, pass, l.From, folder, ids, o, sum); err != nil {
			return err
		}
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(i18n.T("%s · %d Runbooks · %d Tunnel\n"), plural(len(p.Connections), i18n.T("Verbindung"), i18n.T("Verbindungen")), len(p.Runbooks), len(p.Tunnels)))
	if len(jumps) > 0 {
		b.WriteString(fmt.Sprintf(i18n.T("Dazu %d Jump-Host(s), ohne die die Verbindungen nicht gingen.\n"), len(jumps)))
	}
	b.WriteString(i18n.T("Passwörter und Keys sind nicht enthalten.\n\nDatei: ") + team.ShowLoc(path) + "\n")
	if abo {
		b.WriteString(i18n.T("Automatisch aktuell: Änderungen an diesen Verbindungen schreibt sshTermy in die Datei (Strg+Leertaste → „Team-Ordner“).\n"))
	}
	if generated {
		b.WriteString(i18n.T("\nTeam-Passwort (bitte notieren): ") + pass + "\n")
	}
	if abo {
		b.WriteString(i18n.T("\nSag den Kollegen den Ort der Datei und das Passwort (getrennt).\nSie übernehmen sie mit Strg+Leertaste → „Team-Paket übernehmen“ und „Abonnieren“."))
	} else {
		b.WriteString(i18n.T("\nSchick die Datei und sag das Passwort auf einem anderen Weg (Anruf, Chat).\nDer Kollege übernimmt sie mit Strg+Leertaste → „Team-Paket übernehmen“."))
	}
	a.form = nil
	a.scr = scrManager
	a.modal = &modal{kind: mInfo, title: i18n.T("✓ Team-Paket erstellt"), body: b.String()}
	return nil
}

func expandHome(p string) string {
	p = strings.Trim(strings.TrimSpace(p), `"`)
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[1:])
		}
	}
	return p
}

// ---------------------------------------------------------------- Übernehmen

// newestTeamFile: das jüngste .sshteam in Downloads (Vorbelegung).
func newestTeamFile() string {
	m, _ := filepath.Glob(filepath.Join(exportDir(), "*"+team.Ext))
	sort.Slice(m, func(i, j int) bool {
		a, _ := os.Stat(m[i])
		b, _ := os.Stat(m[j])
		return a != nil && b != nil && a.ModTime().After(b.ModTime())
	})
	if len(m) > 0 {
		return m[0]
	}
	return ""
}

func (a *App) openTeamTake() tea.Cmd {
	a.team = teamState{mode: teamTake}
	f := &form{title: i18n.T("Team-Paket übernehmen")}
	f.text("path", i18n.T("Datei"), newestTeamFile(), i18n.T("Pfad zur .sshteam-Datei oder https://…/x.sshteam (Einfügen mit Strg+V)"))
	f.secret("pass", i18n.T("Team-Passwort"), "", i18n.T("bekommst du vom Absender getrennt von der Datei"))
	f.text("folder", i18n.T("In Ordner"), "Team", i18n.T("wird den Ordnern des Absenders vorangestellt; leer = wie beim Absender"))
	f.text("user", i18n.T("Benutzer"), localUser(), i18n.T("für Verbindungen ohne Benutzernamen"))
	f.toggle("abo", i18n.T("Abonnieren"), true, i18n.T("Datei alle 5 min neu lesen und Änderungen des Teams automatisch übernehmen"))
	a.form, a.scr = f, scrTeamForm
	if f.str("path") != "" {
		return f.setFocus(1)
	}
	return f.setFocus(0)
}

func (a *App) openTeamFile() error {
	f := a.form
	path := expandHome(f.str("path"))
	if path == "" {
		return errors.New(i18n.T("Datei fehlt"))
	}
	if !team.IsURL(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, err := team.ReadLoc(ctx, path)
	if err != nil {
		return err
	}
	if !team.IsTeamFile(raw) {
		return team.ErrNotTeamFile
	}
	p, err := team.Open(raw, []byte(f.raw("pass")))
	if err != nil {
		return err
	}
	d, _, err := a.st.Snapshot()
	if err != nil {
		return err
	}
	known := team.Known(d, p)
	t := &a.team
	t.pkg, t.path, t.items, t.sel, t.err = p, path, nil, 0, ""
	t.folder, t.user = domain.NormPath(f.str("folder")), strings.TrimSpace(f.str("user"))
	t.pass, t.sum, t.abo = f.raw("pass"), team.Sum(raw), f.flag("abo")
	for _, c := range p.Connections {
		t.items = append(t.items, teamItem{c: c, on: true, known: known[c.ID]})
	}
	if len(t.items) == 0 {
		return errors.New(i18n.T("das Team-Paket enthält keine Verbindungen"))
	}
	a.form = nil
	a.scr = scrTeam
	return nil
}

func (a *App) commitTeamTake() {
	t := &a.team
	ids := map[string]bool{}
	for _, it := range t.items {
		if it.on {
			ids[it.c.ID] = true
		}
	}
	if len(ids) == 0 {
		t.err = i18n.T("nichts ausgewählt (Space wählt aus)")
		return
	}
	var st team.Stats
	err := a.st.Update(func(d *domain.Data) {
		st = team.Apply(d, t.pkg, ids, t.folder, t.user, time.Now())
	})
	if err != nil {
		t.err = err.Error()
		return
	}
	msg := fmt.Sprintf(i18n.T("Team-Paket übernommen: %d neu, %d aktualisiert"), st.Added, st.Updated)
	if t.abo {
		var have, skip []string
		for _, it := range t.items {
			if it.on {
				have = append(have, it.c.ID)
			} else {
				skip = append(skip, it.c.ID)
			}
		}
		if err := a.addSubscribeLink(t.path, t.pass, t.folder, t.user, have, skip, t.sum); err != nil {
			t.err = err.Error()
			return
		}
		msg += i18n.T(" · abonniert")
	}
	if st.Runbooks > 0 {
		msg += fmt.Sprintf(i18n.T(" · %d Runbooks"), st.Runbooks)
	}
	if st.NeedPw > 0 {
		msg += i18n.T(" · Passwort wird beim Verbinden abgefragt")
	}
	a.scr = scrManager
	a.flash(msg)
	a.askAddrChanges(st.AddrChanges)
}

// askAddrChanges: das Team-Paket will die Adresse von Verbindungen ändern,
// für die hier Zugangsdaten gespeichert sind. Erst nach Bestätigung.
func (a *App) askAddrChanges(ch []team.AddrChange) {
	if len(ch) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString(i18n.T("Das Team-Paket ändert die Adresse von Verbindungen, für die hier Passwort, Key oder Agent-Weiterleitung gespeichert sind:") + "\n\n")
	for i, c := range ch {
		if i == 8 {
			fmt.Fprintf(&b, i18n.T("  … und %d weitere")+"\n", len(ch)-i)
			break
		}
		fmt.Fprintf(&b, "  %s: %s → %s\n", trunc(c.Alias, 24), c.Old, c.New)
	}
	b.WriteString("\n" + i18n.T("Übernehmen? Ohne Bestätigung bleibt die bisherige Adresse."))
	a.modal = confirmModal(i18n.T("Neue Adresse vom Team"), b.String(), func() tea.Cmd {
		if err := a.st.Update(func(d *domain.Data) { team.AcceptAddr(d, ch, time.Now()) }); err != nil {
			a.flashErr(err.Error())
			return nil
		}
		a.flashOK(fmt.Sprintf(i18n.T("Neue Adresse übernommen (%d)"), len(ch)))
		return nil
	})
}

// ---------------------------------------------------------------- Formular

func (a *App) updateTeamForm(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "esc" {
		a.form = nil
		if a.team.mode == teamShare {
			a.scr = scrTeam
		} else {
			a.scr = scrManager
		}
		return nil
	}
	submit, cmd := a.form.update(msg)
	if !submit {
		return cmd
	}
	var err error
	if a.team.mode == teamShare {
		err = a.saveTeamShare()
	} else {
		err = a.openTeamFile()
	}
	if err != nil && a.form != nil {
		a.form.err = err.Error()
	}
	return nil
}

// ---------------------------------------------------------------- Liste

func (a *App) teamKeys() [][2]string {
	n := a.teamCount()
	if a.team.mode == teamShare {
		return [][2]string{{"⏎", fmt.Sprintf(i18n.T("%d weiter"), n)}, {"Space", i18n.T("an/aus")}, {"g", i18n.T("Ordner")}, {"a", i18n.T("alle/keine")}, {"Esc", i18n.T("zurück")}}
	}
	return [][2]string{{"⏎", fmt.Sprintf(i18n.T("%d übernehmen"), n)}, {"Space", i18n.T("an/aus")}, {"a", i18n.T("alle/keine")}, {"Esc", i18n.T("abbrechen")}}
}

func (a *App) teamCount() int {
	n := 0
	for _, it := range a.team.items {
		if it.on {
			n++
		}
	}
	return n
}

func (a *App) updateTeam(msg tea.Msg) tea.Cmd {
	t := &a.team
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	t.err = ""
	switch k.String() {
	case "esc", "q":
		a.scr = scrManager
	case "up", "k":
		t.sel = max(0, t.sel-1)
	case "down", "j":
		t.sel = min(len(t.items)-1, t.sel+1)
	case "pgup":
		t.sel = max(0, t.sel-10)
	case "pgdown":
		t.sel = min(len(t.items)-1, t.sel+10)
	case "space", " ", "x":
		if t.sel < len(t.items) {
			t.items[t.sel].on = !t.items[t.sel].on
		}
	case "a":
		on := a.teamCount() < len(t.items)
		for i := range t.items {
			t.items[i].on = on
		}
	case "g":
		if t.sel < len(t.items) {
			g := t.items[t.sel].c.Group
			on := !t.items[t.sel].on
			for i := range t.items {
				if t.items[i].c.Group == g {
					t.items[i].on = on
				}
			}
		}
	case "enter":
		if t.mode == teamShare {
			return a.openTeamShareForm()
		}
		a.commitTeamTake()
	}
	return nil
}

func (a *App) viewTeam() string {
	s := a.s
	t := &a.team
	w := min(110, a.w-2)
	var b strings.Builder
	if t.mode == teamShare {
		b.WriteString(s.Accent.Render(i18n.T(" ↑ Mit dem Team teilen")) + s.Muted.Render(i18n.T(" · Verbindungen wählen")) + "\n\n")
		b.WriteString(s.Muted.Render(i18n.T(" Passwörter, Keys und eigene Einstellungen bleiben bei dir. Jump-Hosts kommen automatisch mit.")) + "\n\n")
	} else {
		p := t.pkg
		from := p.From
		if from == "" {
			from = "?"
		}
		b.WriteString(s.Accent.Render(i18n.T(" ↓ Team-Paket übernehmen")) + s.Muted.Render(fmt.Sprintf(i18n.T(" · von %s, %s"), from, p.Created.Local().Format("02.01.2006 15:04"))) + "\n\n")
		var extra []string
		if n := len(p.Runbooks); n > 0 {
			extra = append(extra, fmt.Sprintf(i18n.T("%d Runbooks"), n))
		}
		if n := len(p.Tunnels); n > 0 {
			extra = append(extra, fmt.Sprintf(i18n.T("%d Tunnel"), n))
		}
		nq := 0
		for _, c := range p.Connections {
			if c.QuickCmds != nil {
				nq++
			}
		}
		if nq > 0 {
			extra = append(extra, fmt.Sprintf(i18n.T("Schnellbefehle für %d Verbindungen"), nq))
		}
		if len(extra) > 0 {
			b.WriteString(" " + s.Text.Render(i18n.T("Außerdem: ")+strings.Join(extra, " · ")) + "\n")
			b.WriteString(" " + s.Warn.Render(i18n.T("! Befehle aus fremden Runbooks und Schnellbefehlen vor dem Ausführen lesen; sshTermy fragt trotzdem bei jedem Schritt.")) + "\n")
		}
		b.WriteString("\n")
	}
	if t.err != "" {
		b.WriteString(" " + s.Danger.Render(t.err) + "\n\n")
	}
	head := lipgloss.Height(b.String())
	rows := max(3, a.h-2-head-3)
	start := 0
	if t.sel >= rows {
		start = t.sel - rows + 1
	}
	for i := start; i < len(t.items) && i < start+rows; i++ {
		it := t.items[i]
		c := it.c
		box := "[ ]"
		if it.on {
			box = "[x]"
		}
		target := c.Host
		if c.Username != "" {
			target = c.Username + "@" + target
		}
		if c.Port != 0 && c.Port != 22 {
			target += fmt.Sprintf(":%d", c.Port)
		}
		group := c.Group
		if t.mode == teamTake && t.folder != "" {
			group = domain.PathJoin(t.folder, c.Group)
		}
		note := ""
		if t.mode == teamTake {
			note = i18n.T("neu")
			if it.known {
				note = i18n.T("aktualisiert")
			}
		}
		line := trunc(fmt.Sprintf(" %s %-22s %-32s %s", box, trunc(c.Alias, 22), trunc(target, 32), group), w-14)
		var out string
		if i == t.sel {
			out = s.Selected.Width(w - 1).Render(line + "  " + note)
		} else {
			out = s.Text.Render(line)
			if note != "" {
				st := s.OK
				if it.known {
					st = s.Muted
				}
				out += "  " + st.Render(note)
			}
		}
		i := i
		out = a.z.click(out, func(tea.Mouse, bool) tea.Cmd {
			t.sel = i
			t.items[i].on = !t.items[i].on
			return nil
		})
		b.WriteString(out + "\n")
	}
	if t.mode == teamTake {
		b.WriteString("\n" + s.Muted.Render(i18n.T(" Eigene Passwörter, Keys und Benutzernamen bleiben bei schon vorhandenen Verbindungen erhalten.")))
	}
	return b.String()
}

func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]bool{}
	for _, x := range a {
		m[x] = true
	}
	for _, x := range b {
		if !m[x] {
			return false
		}
	}
	return true
}
