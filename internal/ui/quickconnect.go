package ui

import (
	"net"
	"os"
	"os/user"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Schnellverbinden: In der Palette user@host:port (oder ein ganzes
// „ssh -p 2222 user@host“) tippen und sofort verbinden, ohne die Verbindung
// vorher anzulegen. Angemeldet wird wie beim ssh-Kommando: erst SSH-Agent und
// Standard-Keys, dann Passwort-Abfrage. Gespeichert wird nichts, außer man
// wählt „Als Verbindung speichern“.

// parseQuick erkennt ein SSH-Ziel. strong: eindeutig gemeint (mit @, Port,
// „ssh“ davor oder IP-Adresse), dann steht der Eintrag in der Palette oben.
func parseQuick(q string) (c domain.Connection, strong, ok bool) {
	f := strings.Fields(strings.TrimSpace(q))
	if len(f) == 0 {
		return c, false, false
	}
	if f[0] == "ssh" {
		strong = true
		f = f[1:]
	}
	port, userName, target := 0, "", ""
	for i := 0; i < len(f); i++ {
		switch a := f[i]; {
		case a == "-p" && i+1 < len(f):
			p, err := strconv.Atoi(f[i+1])
			if err != nil {
				return c, false, false
			}
			port, strong = p, true
			i++
		case strings.HasPrefix(a, "-p") && len(a) > 2:
			p, err := strconv.Atoi(a[2:])
			if err != nil {
				return c, false, false
			}
			port, strong = p, true
		case a == "-l" && i+1 < len(f):
			userName = f[i+1]
			i++
		case strings.HasPrefix(a, "-"):
			return c, false, false // andere ssh-Schalter: lieber nicht raten
		default:
			if target != "" {
				return c, false, false // mehrere Wörter: Ziel oder Frage, kein Host
			}
			target = a
		}
	}
	if target == "" {
		return c, false, false
	}
	target = strings.TrimPrefix(target, "ssh://")
	if at := strings.LastIndex(target, "@"); at >= 0 {
		userName, target = target[:at], target[at+1:]
		strong = true
		if userName == "" {
			return c, false, false
		}
	}
	host := target
	switch {
	case strings.HasPrefix(target, "["): // [IPv6]:port
		end := strings.Index(target, "]")
		if end < 0 {
			return c, false, false
		}
		host = target[1:end]
		if rest := target[end+1:]; rest != "" {
			if !strings.HasPrefix(rest, ":") {
				return c, false, false
			}
			p, err := strconv.Atoi(rest[1:])
			if err != nil {
				return c, false, false
			}
			port = p
		}
		strong = true
	case strings.Count(target, ":") == 1:
		h, ps, _ := strings.Cut(target, ":")
		p, err := strconv.Atoi(ps)
		if err != nil {
			return c, false, false
		}
		host, port, strong = h, p, true
	case strings.Count(target, ":") > 1: // nacktes IPv6
		if net.ParseIP(target) == nil {
			return c, false, false
		}
	}
	if host == "" || !validHost(host) || port < 0 || port > 65535 {
		return c, false, false
	}
	if net.ParseIP(host) != nil {
		strong = true
	} else if !strong && !strings.Contains(host, ".") {
		return c, false, false // einzelnes Wort: eher Suche nach Alias
	}
	if port == 0 {
		port = 22
	}
	if userName == "" {
		userName = localUser()
	}
	alias := userName + "@" + host
	if port != 22 {
		alias += ":" + strconv.Itoa(port)
	}
	c = domain.Connection{
		ID: "quick:" + alias, Alias: alias, Host: host, Port: port, Username: userName,
		Auth: domain.AuthAuto,
	}
	return c, strong, true
}

func validHost(h string) bool {
	for _, r := range h {
		ok := r == '.' || r == '-' || r == '_' || r == ':' || r == '%' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return !strings.HasPrefix(h, "-") && !strings.HasPrefix(h, ".") && !strings.HasSuffix(h, ".")
}

// localUser: Anmeldename wie beim ssh-Kommando (ohne Windows-Domäne).
func localUser() string {
	name := ""
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	if name == "" {
		name = os.Getenv("USER")
	}
	if name == "" {
		name = os.Getenv("USERNAME")
	}
	if i := strings.LastIndex(name, `\`); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		name = "root"
	}
	return strings.ToLower(name)
}

// quickItems: Palette-Einträge für ein getipptes SSH-Ziel.
func (a *App) quickItems(q string) (items []palItem, strong bool) {
	if rc, ok := parseQuickRaw(q); ok {
		return a.quickRawItems(rc), true
	}
	c, strong, ok := parseQuick(q)
	if !ok {
		return nil, false
	}
	hint := i18n.T("Key/Passwort")
	items = append(items,
		palItem{group: pgConnect, icon: "⚡", label: i18n.T("Schnell verbinden: ") + c.Alias, hint: hint,
			run: func() tea.Cmd { return a.openOrConnect(c) }},
		palItem{group: pgConnect, icon: "+", label: i18n.T("Als Verbindung speichern: ") + c.Alias, hint: i18n.T("anlegen"),
			run: func() tea.Cmd {
				n := newConnDefaults()
				n.Host, n.Port, n.Username, n.Alias = c.Host, c.Port, c.Username, c.Host
				return a.openConnForm(n)
			}})
	return items, strong
}

// openOrConnect: zu einem offenen Tab derselben Verbindung wechseln, sonst verbinden.
func (a *App) openOrConnect(c domain.Connection) tea.Cmd {
	for i, x := range a.sessions {
		if x.conn.ID == c.ID && x.state != stClosed {
			a.focusSession(i)
			a.scr = scrTerminal
			return nil
		}
	}
	return a.connect(c)
}
