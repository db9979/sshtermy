package ui

// Befehls-Favoriten aus dem Verlauf: sshTermy zählt pro Verbindung, welche
// Befehle ohne Fehler liefen (store/cmdstats.go, nur lokal und
// verschlüsselt). Was oft läuft und noch kein Schnellbefehl ist, erscheint in
// der Schnellbefehl-Liste (F9) unter „Vorschläge aus deinem Verlauf“; dort
// legt a ihn auf eine freie ⇧F-Taste (oder auf die am wenigsten benutzte).
// Erreicht ein Befehl zum ersten Mal die Schwelle, sagt ein Toast Bescheid.

import (
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/store"
)

const (
	favMin  = 3 // ab so vielen erfolgreichen Läufen ein Vorschlag
	favShow = 4 // so viele Vorschläge in der Liste
)

// trivialCmds: zu kurz oder zu allgemein für eine F-Taste.
var trivialCmds = map[string]bool{
	"ls": true, "ll": true, "la": true, "l": true, "cd": true, "pwd": true, "clear": true, "cls": true,
	"exit": true, "logout": true, "quit": true, "q": true, "history": true, "top": true, "htop": true,
	"y": true, "n": true, "yes": true, "no": true, "?": true, "end": true, "configure": true,
	"conf t": true, "configure terminal": true, "enable": true, "en": true, "sudo -i": true, "sudo su": true,
	"su": true, "su -": true, "bash": true, "sh": true, "zsh": true, "vi": true, "vim": true, "nano": true,
	"whoami": true, "date": true, "id": true, "w": true, "who": true, "no page": true, "terminal length 0": true,
}

// secretish: Zeilen, die nach Passwort oder Token aussehen, nie als
// Schnellbefehl vorschlagen (z. B. mysql -pGeheim).
var secretish = regexp.MustCompile(`(?i)passw|token|secret|api[_-]?key|(^|\s)-p\S`)

// favWorthy: taugt der Befehl als Schnellbefehl?
func favWorthy(cmd string) bool {
	c := strings.Join(strings.Fields(cmd), " ")
	switch {
	case len([]rune(c)) < 3 || len(c) > 120 || strings.ContainsAny(cmd, "\n\r"):
		return false
	case secretish.MatchString(c):
		return false
	case trivialCmds[strings.ToLower(c)]:
		return false
	case strings.HasPrefix(c, "cd ") || strings.HasPrefix(c, "#") || strings.HasPrefix(c, "!"):
		return false
	case agent.Redact(c) != c: // Passwort/Token in der Zeile
		return false
	}
	return true
}

// noteFavorite: ein Befehl lief ohne Fehler.
func (a *App) noteFavorite(s *session, cmd string) {
	if s.conn.ID == "" || !a.logsEnabled() || !favWorthy(cmd) {
		return
	}
	u, err := a.st.NoteCommand(s.conn.ID, cmd)
	if err != nil || u.N < favMin || u.Offered || u.Hidden {
		return
	}
	cmds, _ := a.quickCmds(s)
	if inList(cmds, u.Cmd) {
		return
	}
	_ = a.st.MarkCommand(s.conn.ID, u.Cmd, true, false)
	a.flash(fmt.Sprintf(i18n.T("„%s“ lief schon %d× · %s, dann a: als Schnellbefehl"), trunc(u.Cmd, 40), u.N, a.keys.label(actQuickMenu)))
}

func inList(list []string, cmd string) bool {
	cmd = strings.Join(strings.Fields(cmd), " ")
	for _, c := range list {
		if strings.Join(strings.Fields(c), " ") == cmd {
			return true
		}
	}
	return false
}

// favSuggestions: häufige Befehle, die noch keine Schnellbefehle sind.
func (a *App) favSuggestions(s *session) []store.CmdUse {
	if s.conn.ID == "" {
		return nil
	}
	cmds, _ := a.quickCmds(s)
	var out []store.CmdUse
	for _, u := range a.st.TopCommands(s.conn.ID, 0) {
		if u.N < favMin || !favWorthy(u.Cmd) || inList(cmds, u.Cmd) {
			continue
		}
		out = append(out, u)
		if len(out) == favShow {
			break
		}
	}
	return out
}

// adoptSlot: wohin ein neuer Schnellbefehl käme. Zuerst eine freie Taste,
// sonst die, deren Befehl hier am seltensten lief.
func (a *App) adoptSlot(s *session) (slot int, replaced string) {
	cmds, _ := a.quickCmds(s)
	for i := 0; i < maxQuick; i++ {
		if i >= len(cmds) || strings.TrimSpace(cmds[i]) == "" {
			return i, ""
		}
	}
	slot, best := maxQuick-1, -1
	for i := maxQuick - 1; i >= 0; i-- { // bei Gleichstand die hintere Taste
		n := a.st.CommandUse(s.conn.ID, cmds[i]).N
		if best < 0 || n < best {
			slot, best = i, n
		}
	}
	return slot, cmds[slot]
}

// adoptFavorite legt cmd als Schnellbefehl ab.
func (a *App) adoptFavorite(s *session, cmd string) error {
	c, ok := a.st.Connection(s.conn.ID)
	if !ok {
		return fmt.Errorf("%s", i18n.T("Verbindung nicht gefunden"))
	}
	slot, replaced := a.adoptSlot(s)
	cur, _ := a.quickCmds(s)
	list := make([]string, maxQuick)
	copy(list, cur)
	list[slot] = cmd
	for len(list) > 0 && strings.TrimSpace(list[len(list)-1]) == "" {
		list = list[:len(list)-1]
	}
	c.QuickCmds = list
	if _, err := a.st.UpsertConnection(c); err != nil {
		return err
	}
	s.conn.QuickCmds = list
	msg := fmt.Sprintf(i18n.T("„%s“ liegt jetzt auf %s"), trunc(cmd, 40), a.quickLabel(slot))
	if replaced != "" {
		msg += fmt.Sprintf(i18n.T(" (statt „%s“)"), trunc(replaced, 30))
	}
	a.flashOK(msg)
	return nil
}

// sendCommand führt einen Befehl aus, als hätte ihn der Mensch getippt.
func (a *App) sendCommand(s *session, cmd string) tea.Cmd {
	if s.state != stConnected || s.term == nil {
		return nil
	}
	a.clearToasts()
	s.sel, s.scroll, s.ghost, s.chips = nil, 0, "", nil
	a.beginCommand(s, cmd)
	a.sendLine(s, cmd)
	s.history = append(s.history, cmd)
	a.profileCommand(s, cmd)
	return nil
}
