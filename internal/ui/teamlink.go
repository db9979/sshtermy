package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/team"
)

// Team-Ordner (Abo): Ein Team-Paket an einem festen Ort, den alle erreichen.
// Wer es veröffentlicht, schreibt es bei jeder Änderung neu (Prüfung jede
// Minute, ohne Änderung kein Schreiben). Wer es abonniert, liest es alle
// fünf Minuten und übernimmt Änderungen, auch Löschungen. Logik in
// internal/team/link.go, gespeichert in domain.Data.TeamLinks (nur lokal).

const (
	teamPubEvery = time.Minute
	teamSubEvery = 5 * time.Minute
)

type teamLinkState struct {
	next    time.Time
	running map[string]bool
	checked map[string]time.Time // Abo: letzte Prüfung in dieser Sitzung
}

// teamLinkMsg: Ergebnis im UI-Thread anwenden.
type teamLinkMsg struct{ apply func(a *App) tea.Cmd }

func (a *App) teamLinkTick() tea.Cmd {
	if a.scr == scrUnlock || time.Now().Before(a.tl.next) {
		return nil
	}
	a.tl.next = time.Now().Add(teamPubEvery)
	var cmds []tea.Cmd
	for _, l := range a.st.TeamLinks() {
		switch l.Role {
		case domain.TeamPublish:
			cmds = append(cmds, a.publishLink(l, false))
		case domain.TeamSubscribe:
			if time.Since(a.tl.checked[l.ID]) >= teamSubEvery {
				cmds = append(cmds, a.pullLink(l, false))
			}
		}
	}
	return tea.Batch(cmds...)
}

func (a *App) tlBusy(id string, on bool) bool {
	if a.tl.running == nil {
		a.tl.running, a.tl.checked = map[string]bool{}, map[string]time.Time{}
	}
	if on && a.tl.running[id] {
		return true
	}
	a.tl.running[id] = on
	return false
}

// saveLink: Stand speichern, ohne die Liste neu zu laden (Link kann
// inzwischen entfernt worden sein).
func (a *App) saveLink(l domain.TeamLink) {
	for _, cur := range a.st.TeamLinks() {
		if cur.ID == l.ID {
			_, _ = a.st.UpsertTeamLink(l)
			return
		}
	}
}

// publishLink schreibt das Paket neu, wenn sich der Inhalt geändert hat
// (force: auch ohne Änderung, z. B. „Jetzt veröffentlichen“).
func (a *App) publishLink(l domain.TeamLink, force bool) tea.Cmd {
	d, _, err := a.st.Snapshot()
	if err != nil {
		return nil
	}
	p, sum := team.BuildLink(d, l, time.Now())
	if sum == l.LastSum && l.LastErr == "" && !force {
		return nil
	}
	if a.tlBusy(l.ID, true) {
		return nil
	}
	pass := []byte(l.TeamPass.Reveal())
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		raw, err := team.Seal(p, pass)
		if err == nil {
			err = team.WriteLoc(ctx, l.Path, raw)
		}
		return teamLinkMsg{func(a *App) tea.Cmd {
			a.tlBusy(l.ID, false)
			if err != nil {
				if e := err.Error(); e != l.LastErr {
					a.flashErr(fmt.Sprintf(i18n.T("Team-Ordner „%s“: %s"), l.Name, e))
				}
				l.LastErr = err.Error()
			} else {
				if force {
					a.flashOK(fmt.Sprintf(i18n.T("Team-Ordner „%s“ veröffentlicht: %s"), l.Name, plural(len(p.Connections), i18n.T("Verbindung"), i18n.T("Verbindungen"))))
				}
				l.LastSum, l.LastErr, l.LastRun = sum, "", time.Now().UTC()
			}
			a.saveLink(l)
			return nil
		}}
	}
}

// pullLink liest das Paket und übernimmt Änderungen.
func (a *App) pullLink(l domain.TeamLink, force bool) tea.Cmd {
	if a.tlBusy(l.ID, true) {
		return nil
	}
	a.tl.checked[l.ID] = time.Now()
	pass := []byte(l.TeamPass.Reveal())
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		raw, err := team.ReadLoc(ctx, l.Path)
		sum := ""
		var p *team.Package
		if err == nil {
			sum = team.Sum(raw)
			if sum != l.LastSum || force {
				p, err = team.Open(raw, pass)
			}
		}
		return teamLinkMsg{func(a *App) tea.Cmd {
			a.tlBusy(l.ID, false)
			if err != nil {
				if e := err.Error(); e != l.LastErr || force {
					a.flashErr(fmt.Sprintf(i18n.T("Team-Ordner „%s“: %s"), l.Name, e))
				}
				l.LastErr = err.Error()
				a.saveLink(l)
				return nil
			}
			if p == nil { // unverändert
				if l.LastErr != "" {
					l.LastErr = ""
					a.saveLink(l)
				}
				if force {
					a.flashOK(fmt.Sprintf(i18n.T("Team-Ordner „%s“: keine Änderungen"), l.Name))
				}
				return nil
			}
			var st team.Stats
			var have []string
			if err := a.st.Update(func(d *domain.Data) { st, have = team.Pull(d, p, l, time.Now()) }); err != nil {
				return nil
			}
			if len(st.AddrChanges) > 0 && a.modal != nil {
				sum = l.LastSum // gerade ein anderer Dialog offen: beim nächsten Abgleich fragen
			}
			a.storeTeamCmdSets(p.From, p.CmdSets) // wirken erst nach Freigabe
			l.Have, l.LastSum, l.LastErr, l.LastRun = have, sum, "", time.Now().UTC()
			a.saveLink(l)
			note := fmt.Sprintf(i18n.T("Team-Ordner „%s“: %d neu, %d aktualisiert, %d entfernt"), l.Name, st.Added, st.Updated, st.Removed) + teamGuardNote(st)
			if st.KeptGuard > 0 || st.Review > 0 {
				a.pushToast(toastWarn, note)
			} else {
				a.flash(note)
			}
			if a.modal == nil {
				a.askAddrChanges(st.AddrChanges)
			}
			return nil
		}}
	}
}

// ---------------------------------------------------------------- Anlegen

// addPublishLink wird nach „Mit dem Team teilen“ mit „automatisch aktuell
// halten“ aufgerufen.
func (a *App) addPublishLink(path, pass, from, folder string, ids []string, o team.Options, sum string) error {
	l := domain.TeamLink{Role: domain.TeamPublish, Name: linkName(path), Path: path, TeamPass: domain.Secret(pass),
		Folder: folder, IDs: ids, From: from, Users: o.Users, QuickCmds: o.QuickCmds, Runbooks: o.Runbooks, Tunnels: o.Tunnels,
		LastSum: sum, LastRun: time.Now().UTC()}
	for _, cur := range a.st.TeamLinks() {
		if cur.Role == domain.TeamPublish && cur.Path == path {
			l.ID = cur.ID
		}
	}
	_, err := a.st.UpsertTeamLink(l)
	return err
}

// addSubscribeLink wird nach „Team-Paket übernehmen“ mit „abonnieren“
// aufgerufen.
func (a *App) addSubscribeLink(path, pass, into, user string, have, skip []string, sum string) error {
	l := domain.TeamLink{Role: domain.TeamSubscribe, Name: linkName(path), Path: path, TeamPass: domain.Secret(pass),
		Into: into, DefaultUser: user, Have: have, Skip: skip, LastSum: sum, LastRun: time.Now().UTC()}
	for _, cur := range a.st.TeamLinks() {
		if cur.Role == domain.TeamSubscribe && cur.Path == path {
			l.ID = cur.ID
		}
	}
	l, err := a.st.UpsertTeamLink(l)
	if err == nil && a.tl.checked != nil {
		a.tl.checked[l.ID] = time.Now()
	}
	return err
}

func linkName(path string) string {
	p := team.ShowLoc(path)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		p = p[i+1:]
	}
	return strings.TrimSuffix(p, filepath.Ext(p))
}

// ---------------------------------------------------------------- Verwalten

func linkStatus(l domain.TeamLink) string {
	switch {
	case l.LastErr != "":
		return "× " + trunc(l.LastErr, 40)
	case !l.LastRun.IsZero():
		return "✓ " + ago(l.LastRun)
	}
	return ""
}

func (a *App) openTeamLinks() tea.Cmd {
	links := a.st.TeamLinks()
	if len(links) == 0 {
		a.modal = &modal{kind: mInfo, title: i18n.T("Team-Ordner"), body: i18n.T("Noch keine Team-Ordner.\n\n" +
			"Veröffentlichen: Strg+Leertaste → „Mit dem Team teilen“, als Datei einen gemeinsamen Ordner wählen und „Aktuell halten“ einschalten.\n\n" +
			"Abonnieren: Strg+Leertaste → „Team-Paket übernehmen“ mit „Abonnieren“. sshTermy liest die Datei dann alle 5 Minuten und übernimmt Änderungen.")}
		return nil
	}
	var items []menuItem
	for _, l := range links {
		l := l
		icon, what := "↓", i18n.T("Abo")
		if l.Role == domain.TeamPublish {
			icon, what = "↑", i18n.T("Veröffentlicht")
		}
		items = append(items, menuItem{label: icon + " " + what + ": " + l.Name, hint: linkStatus(l), run: func() tea.Cmd { return a.openTeamLink(l) }})
	}
	a.modal = &modal{kind: mMenu, title: i18n.T("Team-Ordner"), items: items}
	return nil
}

func (a *App) openTeamLink(l domain.TeamLink) tea.Cmd {
	var items []menuItem
	if l.Role == domain.TeamPublish {
		n := 0
		if d, _, err := a.st.Snapshot(); err == nil {
			n = len(team.LinkIDs(d, l))
		}
		src := l.Folder
		if src == "" {
			src = i18n.T("einzeln gewählt")
		}
		items = append(items,
			menuItem{label: i18n.T("Jetzt veröffentlichen"), hint: plural(n, i18n.T("Verbindung"), i18n.T("Verbindungen")) + " · " + src,
				run: func() tea.Cmd { return a.publishLink(l, true) }},
			menuItem{label: i18n.T("Nicht mehr veröffentlichen"), hint: i18n.T("Datei bleibt liegen"), run: func() tea.Cmd {
				a.modal = confirmModal(i18n.T("Team-Ordner beenden?"),
					fmt.Sprintf(i18n.T("„%s“ wird nicht mehr aktualisiert. Die Datei %s bleibt liegen; lösche sie dort, wenn das Team sie nicht mehr bekommen soll."), l.Name, team.ShowLoc(l.Path)),
					func() tea.Cmd { return a.dropLink(l) })
				return nil
			}})
	} else {
		items = append(items,
			menuItem{label: i18n.T("Jetzt abgleichen"), hint: fmt.Sprintf(i18n.T("%d Verbindungen · in „%s“"), len(l.Have), l.Into),
				run: func() tea.Cmd { return a.pullLink(l, true) }},
			menuItem{label: i18n.T("Abo beenden"), hint: i18n.T("Verbindungen bleiben"), run: func() tea.Cmd {
				a.modal = confirmModal(i18n.T("Abo beenden?"),
					fmt.Sprintf(i18n.T("Änderungen aus „%s“ kommen nicht mehr an. Die übernommenen Verbindungen bleiben und gehören ab jetzt dir."), l.Name),
					func() tea.Cmd { return a.dropLink(l) })
				return nil
			}})
	}
	a.modal = &modal{kind: mMenu, title: l.Name + " · " + team.ShowLoc(l.Path), items: items}
	return nil
}

func (a *App) dropLink(l domain.TeamLink) tea.Cmd {
	if err := a.st.DeleteTeamLink(l.ID); err != nil {
		a.flashErr(err.Error())
	} else {
		a.flash(fmt.Sprintf(i18n.T("Team-Ordner „%s“ beendet"), l.Name))
	}
	return nil
}
