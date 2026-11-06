package ui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Klickbare Links, IPs und Hostnamen im Terminal
//
//   - Strg+Klick auf eine URL öffnet sie im Browser.
//   - Rechtsklick (oder Strg+Klick) auf eine IP, einen Hostnamen oder
//     user@host zeigt ein kleines Menü: per SSH verbinden (direkt oder über
//     den aktuellen Server als Jump-Host), Ping vom Server, Ping von diesem
//     Rechner, im Browser öffnen, kopieren.
//   - Rechtsklick auf eine URL: öffnen oder kopieren.
//
// Eine bestehende Markierung hat Vorrang (Rechtsklick kopiert sie), und ohne
// Treffer fügt der Rechtsklick wie gewohnt ein.

type linkKind int

const (
	lkURL linkKind = iota
	lkHost
)

type link struct {
	kind linkKind
	text string // wie im Terminal
	user string // lkHost: aus user@host
	host string // lkHost: IP oder Name (IPv6 ohne Klammern); lkURL: Host der URL
	port int    // 0 = keiner angegeben
	ip   bool   // host ist eine IP-Adresse
}

var (
	rxURL  = regexp.MustCompile(`(?i)\b(?:https?|ftp)://[^\s<>"'` + "`" + `]+|\bwww\.[a-z0-9-]+(?:\.[a-z0-9-]+)+(?:[/?#][^\s<>"'` + "`" + `]*)?`)
	rxIPv4 = regexp.MustCompile(`\b(?:[a-z_][a-z0-9_.-]*@)?(?:\d{1,3}\.){3}\d{1,3}(?:/\d{1,2}|:\d{1,5})?\b`)
	rxIPv6 = regexp.MustCompile(`(?i)(?:[a-z_][a-z0-9_.-]*@)?(?:\[[0-9a-f:.%a-z0-9]+\](?::\d{1,5})?|\b[0-9a-f]{0,4}(?::[0-9a-f]{0,4}){2,7}(?:%[a-z0-9]+)?(?:/\d{1,3})?)`)
	rxHost = regexp.MustCompile(`(?i)\b(?:[a-z_][a-z0-9_.-]*@)?[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+(?::\d{1,5})?\b`)
)

// Endungen, die eher Dateien als Hostnamen sind (nginx.conf, syslog.1 …).
var fileExt = map[string]bool{
	"txt": true, "log": true, "conf": true, "cfg": true, "ini": true, "json": true, "yaml": true, "yml": true,
	"xml": true, "csv": true, "md": true, "html": true, "htm": true, "js": true, "ts": true, "go": true, "py": true,
	"sh": true, "ps1": true, "bat": true, "exe": true, "dll": true, "so": true, "c": true, "h": true, "rs": true,
	"java": true, "rb": true, "php": true, "pl": true, "tar": true, "gz": true, "tgz": true, "zip": true, "bz2": true,
	"xz": true, "7z": true, "deb": true, "rpm": true, "iso": true, "img": true, "pem": true, "crt": true, "key": true,
	"pub": true, "csr": true, "lock": true, "pid": true, "sock": true, "tmp": true, "bak": true, "old": true,
	"swp": true, "service": true, "socket": true, "timer": true, "mount": true, "target": true, "d": true,
	"jpg": true, "jpeg": true, "png": true, "gif": true, "svg": true, "pdf": true, "doc": true, "docx": true,
	"xls": true, "xlsx": true, "env": true, "toml": true, "sql": true, "db": true, "mod": true, "sum": true,
}

// findLink sucht einen Link, der das Byte off der Zeile überdeckt.
func findLink(line string, off int) (link, bool) {
	hit := func(rx *regexp.Regexp) (string, int, bool) {
		for _, m := range rx.FindAllStringIndex(line, -1) {
			if off >= m[0] && off < m[1] {
				return line[m[0]:m[1]], m[0], true
			}
		}
		return "", 0, false
	}
	if t, _, ok := hit(rxURL); ok {
		t = trimURL(t)
		u := t
		if !strings.Contains(u, "://") {
			u = "https://" + u
		}
		h := u[strings.Index(u, "://")+3:]
		if i := strings.IndexAny(h, "/?#"); i >= 0 {
			h = h[:i]
		}
		if i := strings.LastIndex(h, "@"); i >= 0 {
			h = h[i+1:]
		}
		if hh, _, err := net.SplitHostPort(h); err == nil {
			h = hh
		}
		return link{kind: lkURL, text: u, host: strings.Trim(h, "[]")}, true
	}
	if t, start, ok := hit(rxIPv4); ok && !afterSlash(line, start) {
		if l, ok := parseHostToken(t); ok {
			return l, true
		}
	}
	if t, _, ok := hit(rxIPv6); ok && strings.Count(t, ":") >= 2 {
		if l, ok := parseHostToken(t); ok && l.ip {
			return l, true
		}
	}
	if t, start, ok := hit(rxHost); ok && !afterSlash(line, start) && !beforeSlash(line, start+len(t)) {
		if l, ok := parseHostToken(t); ok {
			return l, true
		}
	}
	return link{}, false
}

// afterSlash: Treffer gehört zu einem Pfad (/etc/example.com).
func afterSlash(line string, start int) bool {
	return start > 0 && (line[start-1] == '/' || line[start-1] == '\\' || line[start-1] == '.')
}
func beforeSlash(line string, end int) bool {
	return end < len(line) && (line[end] == '/' || line[end] == '\\')
}

// trimURL entfernt Satzzeichen am Ende („siehe https://x.de/a.“) und eine
// schließende Klammer, die nicht zur URL gehört.
func trimURL(u string) string {
	for len(u) > 0 {
		last := u[len(u)-1]
		switch {
		case strings.IndexByte(".,;:!?'\"", last) >= 0:
			u = u[:len(u)-1]
		case last == ')' && strings.Count(u, "(") < strings.Count(u, ")"),
			last == ']' && strings.Count(u, "[") < strings.Count(u, "]"),
			last == '}' && strings.Count(u, "{") < strings.Count(u, "}"):
			u = u[:len(u)-1]
		default:
			return u
		}
	}
	return u
}

// parseHostToken zerlegt user@host:port, [v6]:port, 10.0.0.0/24 usw.
func parseHostToken(t string) (link, bool) {
	l := link{kind: lkHost, text: t}
	if i := strings.LastIndex(t, "@"); i >= 0 {
		l.user, t = t[:i], t[i+1:]
	}
	if i := strings.IndexByte(t, '/'); i >= 0 { // CIDR: nur die Adresse
		t = t[:i]
	}
	switch {
	case strings.HasPrefix(t, "["):
		end := strings.IndexByte(t, ']')
		if end < 0 {
			return link{}, false
		}
		h, rest := t[1:end], t[end+1:]
		if strings.HasPrefix(rest, ":") {
			l.port, _ = strconv.Atoi(rest[1:])
		}
		t = h
	case strings.Count(t, ":") == 1:
		h, p, _ := strings.Cut(t, ":")
		l.port, _ = strconv.Atoi(p)
		t = h
	}
	if l.port < 0 || l.port > 65535 {
		return link{}, false
	}
	l.host = t
	if ip := net.ParseIP(strings.SplitN(t, "%", 2)[0]); ip != nil {
		l.ip = true
		return l, !ip.IsUnspecified()
	}
	if strings.Count(t, ":") > 0 {
		return link{}, false
	}
	// Hostname: braucht einen Punkt, das letzte Glied muss Buchstaben haben
	// und darf keine typische Dateiendung sein.
	labels := strings.Split(strings.ToLower(t), ".")
	if len(labels) < 2 {
		return link{}, false
	}
	tld := labels[len(labels)-1]
	if fileExt[tld] || strings.Trim(tld, "0123456789") == "" || len(tld) < 2 {
		return link{}, false
	}
	allNum := true
	for _, p := range labels {
		if strings.Trim(p, "0123456789") != "" {
			allNum = false
		}
	}
	if allNum { // sieht aus wie eine Versionsnummer
		return link{}, false
	}
	return l, true
}

// linkAt: Link unter der Mausposition (Terminal-Koordinaten).
func (a *App) linkAt(s *session, tx, ty int) (link, bool) {
	if s.term == nil {
		return link{}, false
	}
	cols, rows := a.termSize(s)
	p := s.term.ScreenPos(clamp(tx, 0, cols-1), clamp(ty, 0, rows-1), s.scroll)
	text, off := s.term.LineText(p.Line)
	if p.Col >= len(off) {
		return link{}, false
	}
	if l, ok := findLink(text, off[p.Col]); ok {
		return l, true
	}
	// Umgebrochene URL: über die nächste Zeile hinweg suchen
	next, _ := s.term.LineText(p.Line + 1)
	if strings.TrimRight(text, " ") != text || next == "" {
		return link{}, false
	}
	if l, ok := findLink(text+strings.TrimRight(next, " "), off[p.Col]); ok && l.kind == lkURL {
		return l, true
	}
	return link{}, false
}

// ---------------------------------------------------------------- Menü

type menuItem struct {
	label string
	hint  string
	run   func() tea.Cmd
}

func (a *App) openURL(u string) tea.Cmd {
	if err := sshx.OpenBrowser(u); err != nil {
		a.flashErr(i18n.T("Browser ließ sich nicht öffnen: ") + err.Error())
	} else {
		a.flash(i18n.T("Öffne im Browser: ") + trunc(u, 60))
	}
	return nil
}

// linkMenu baut das Kontextmenü zu einem Link an der Bildschirmposition x, y.
func (a *App) linkMenu(s *session, l link, x, y int) *modal {
	var items []menuItem
	copyIt := func(t string) menuItem {
		return menuItem{label: i18n.T("Kopieren"), hint: trunc(t, 30), run: func() tea.Cmd {
			go func() { _ = clipboard.WriteAll(t) }()
			a.flash(i18n.T("Kopiert: ") + trunc(t, 50))
			return tea.SetClipboard(t)
		}}
	}
	title := trunc(l.text, 40)
	if l.kind == lkURL {
		items = append(items, menuItem{label: i18n.T("Im Browser öffnen"), hint: "Strg+Klick", run: func() tea.Cmd { return a.openURL(l.text) }}, copyIt(l.text))
	} else {
		target := l.host
		if l.port != 0 {
			target = net.JoinHostPort(l.host, strconv.Itoa(l.port))
		}
		items = append(items, menuItem{label: i18n.T("Verbinden (SSH)"), hint: a.connectHint(s, l), run: func() tea.Cmd { return a.connectTo(s, l, false) }})
		if s.conn.ID != "" && !strings.HasPrefix(s.conn.ID, quickPrefix) {
			items = append(items, menuItem{label: i18n.T("Verbinden über ") + trunc(s.conn.Alias, 18), hint: i18n.T("Jump-Host"), run: func() tea.Cmd { return a.connectTo(s, l, true) }})
		}
		items = append(items,
			menuItem{label: i18n.T("Ping vom Server"), hint: trunc(s.conn.Alias, 16), run: func() tea.Cmd { return a.pingRemote(s, l.host) }},
			menuItem{label: i18n.T("Ping von diesem Rechner"), hint: i18n.T("lokal"), run: func() tea.Cmd { return a.pingLocal(l.host) }},
			menuItem{label: i18n.T("Im Browser öffnen"), hint: "https://", run: func() tea.Cmd {
				u := "https://" + target
				if l.ip && strings.Contains(l.host, ":") && l.port == 0 {
					u = "https://[" + l.host + "]"
				}
				return a.openURL(u)
			}},
			copyIt(target))
	}
	items = append(items, menuItem{label: i18n.T("Einfügen"), hint: i18n.T("Zwischenablage"), run: func() tea.Cmd { return a.pasteClipboard(s) }})
	return &modal{kind: mMenu, title: title, items: items, at: [2]int{x, y}, anchored: true}
}

func (a *App) connectHint(s *session, l link) string {
	if c, ok := a.storedConn(l); ok {
		return c.Alias
	}
	return orDefault(l.user, s.conn.Username) + "@" + l.host
}

const quickPrefix = "link:"

// storedConn: gespeicherte Verbindung zu diesem Host (und Port)?
func (a *App) storedConn(l link) (domain.Connection, bool) {
	for _, c := range a.st.Connections() {
		if !strings.EqualFold(c.Host, l.host) || (l.user != "" && c.Username != l.user) {
			continue
		}
		p := c.Port
		if p == 0 {
			p = 22
		}
		if l.port == 0 || l.port == p {
			return c, true
		}
	}
	return domain.Connection{}, false
}

// connectTo öffnet einen neuen Tab. Gibt es den Host schon als gespeicherte
// Verbindung, wird diese genommen. Sonst entsteht eine Verbindung nur für
// diese Sitzung: Benutzer aus user@host oder vom aktuellen Server, Key oder
// Agent werden übernommen, ein Passwort nie (es wird abgefragt).
func (a *App) connectTo(s *session, l link, viaJump bool) tea.Cmd {
	if c, ok := a.storedConn(l); ok && !viaJump {
		for i, x := range a.sessions {
			if x.conn.ID == c.ID && x.state != stClosed {
				a.focusSession(i)
				return nil
			}
		}
		return a.connect(c)
	}
	src := s.conn
	c := domain.Connection{
		ID:       quickPrefix + l.host + ":" + strconv.Itoa(l.port),
		Alias:    l.host,
		Host:     l.host,
		Port:     l.port,
		Username: orDefault(l.user, src.Username),
		Auth:     domain.AuthPassword,
		Device:   src.Device,
	}
	if c.Port == 0 {
		c.Port = 22
	}
	if src.Auth == domain.AuthAgent || src.Auth == domain.AuthKeyFile {
		c.Auth, c.KeyPath, c.KeyPEM, c.KeyPassphrase = src.Auth, src.KeyPath, src.KeyPEM, src.KeyPassphrase
		if src.Keychain && !c.KeyPassphrase.IsSet() {
			c.KeyPassphrase = domain.Secret(sshx.KeychainSecret(src.ID, sshx.SecretPassphrase))
		}
	}
	if viaJump {
		c.JumpHostID = src.ID
		c.Alias = l.host + " ⤳"
	}
	return a.connect(c)
}

// pingRemote tippt einen Ping auf dem Server (ein Netzwerkgerät bekommt die
// schlichte Form ohne Zähler).
func (a *App) pingRemote(s *session, host string) tea.Cmd {
	if s.term == nil || s.state != stConnected {
		return nil
	}
	cmd := "ping -c 4 " + host
	switch {
	case s.netDev(), s.conn.Device == domain.DeviceNetwork:
		cmd = "ping " + host
	case s.client != nil && strings.Contains(strings.ToLower(s.client.Remote.String()), "windows"):
		cmd = "ping -n 4 " + host
	}
	s.sel, s.scroll, s.panelFocus = nil, 0, false
	if s.term.AtIdlePrompt() {
		s.term.SendText(cmd + "\r")
	} else {
		s.term.SendText(cmd)
		a.flash(i18n.T("Ping eingefügt, ⏎ zum Ausführen"))
	}
	return nil
}

type pingDoneMsg struct {
	host, out string
	err       error
}

// pingLocal pingt vom eigenen Rechner aus und zeigt das Ergebnis im Dialog.
func (a *App) pingLocal(host string) tea.Cmd {
	a.modal = &modal{kind: mBusy, title: i18n.T("Ping ") + host, body: i18n.T("Pinge ") + host + i18n.T(" von diesem Rechner …")}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		args := []string{"-c", "4", host}
		if runtime.GOOS == "windows" {
			args = []string{"-n", "4", host}
		}
		out, err := exec.CommandContext(ctx, "ping", args...).CombinedOutput()
		return pingDoneMsg{host, strings.TrimSpace(string(out)), err}
	}
}

func (a *App) onPingDone(m pingDoneMsg) {
	if a.modal == nil || a.modal.kind != mBusy {
		return // inzwischen geschlossen
	}
	body := m.out
	if m.err != nil && body == "" {
		body = m.err.Error()
	}
	head := i18n.T("✓ erreichbar")
	var ee *exec.ExitError
	switch {
	case errors.As(m.err, &ee):
		head = i18n.T("✗ nicht erreichbar")
	case m.err != nil:
		head = i18n.T("✗ Ping nicht möglich")
	}
	a.modal = &modal{kind: mInfo, title: i18n.T("Ping ") + m.host + " · " + head, body: lastLines(body, 14)}
}

func lastLines(s string, n int) string {
	l := strings.Split(strings.ReplaceAll(s, "\r", ""), "\n")
	if len(l) > n {
		l = append([]string{"…"}, l[len(l)-n:]...)
	}
	return strings.Join(l, "\n")
}

// menuView zeichnet ein Kontextmenü.
func (a *App) menuView(m *modal) string {
	st := a.s
	w := lipgloss.Width(m.title) + 2
	for _, it := range m.items {
		w = max(w, lipgloss.Width(it.label)+lipgloss.Width(it.hint)+6)
	}
	w = min(w, max(20, a.w-6))
	var b strings.Builder
	for i, it := range m.items {
		label := fmt.Sprintf("%d %s", i+1, it.label)
		gap := max(1, w-lipgloss.Width(label)-lipgloss.Width(it.hint))
		line := label + strings.Repeat(" ", gap) + st.Muted.Render(it.hint)
		if i == m.sel {
			line = st.Selected.Render(label + strings.Repeat(" ", gap) + it.hint)
		}
		i := i
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(a.z.click(line, func(tea.Mouse, bool) tea.Cmd {
			m.sel = i
			return a.menuRun()
		}))
	}
	return st.dialog(dialog{title: m.title, body: b.String(), width: w, compact: true}, a.w)
}

func (a *App) menuRun() tea.Cmd {
	m := a.modal
	if m == nil || m.kind != mMenu || m.sel >= len(m.items) {
		return nil
	}
	a.modal = nil
	return m.items[m.sel].run()
}

func (a *App) menuKey(key string) tea.Cmd {
	m := a.modal
	switch key {
	case "esc", "q":
		a.modal = nil
	case "up", "k", "shift+tab":
		m.sel = (m.sel - 1 + len(m.items)) % len(m.items)
	case "down", "j", "tab":
		m.sel = (m.sel + 1) % len(m.items)
	case "enter", "space":
		return a.menuRun()
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' && int(key[0]-'1') < len(m.items) {
			m.sel = int(key[0] - '1')
			return a.menuRun()
		}
	}
	return nil
}

// termLinkClick: Strg+Klick (öffnet URL, sonst Menü) und Rechtsklick (Menü).
// ok = false, wenn dort kein Link steht.
func (a *App) termLinkClick(s *session, m tea.Mouse, tx, ty int, direct bool) (tea.Cmd, bool) {
	l, ok := a.linkAt(s, tx, ty)
	if !ok {
		return nil, false
	}
	if direct && l.kind == lkURL {
		return a.openURL(l.text), true
	}
	a.modal = a.linkMenu(s, l, m.X, m.Y)
	return nil, true
}
