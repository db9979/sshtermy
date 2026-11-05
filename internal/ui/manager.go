package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/store"
)

type managerState struct {
	sel       int
	search    textinput.Model
	searching bool
}

func newManager() managerState {
	in := newInput("", false)
	in.Placeholder = "Suchen (Alias, Host, Gruppe, Tag)"
	return managerState{search: in}
}

func (a *App) filtered() []domain.Connection {
	all := a.st.Connections()
	q := strings.ToLower(strings.TrimSpace(a.mgr.search.Value()))
	if q == "" {
		return all
	}
	var out []domain.Connection
	for _, c := range all {
		hay := strings.ToLower(c.Alias + " " + c.Host + " " + c.Group + " " + c.Username + " " + strings.Join(c.Tags, " "))
		if strings.Contains(hay, q) {
			out = append(out, c)
		}
	}
	return out
}

func (a *App) managerKeys() [][2]string {
	k := [][2]string{{"Strg+Space", "Palette"}, {"⏎", "verbinden"}, {"n", "neu"}, {"e", "bearbeiten"}, {"d", "löschen"}, {"/", "suchen"},
		{"t", "Tunnel"}, {"s", "KI"}, {"y", "Sync"}, {"l", "Protokolle"}, {"F1", "Hilfe"}, {"F12", "Über"}, {"Strg+Q", "beenden"}}
	if len(a.sessions) > 0 {
		k = append([][2]string{{"F10", "zum Terminal"}}, k...)
	}
	return k
}

func (a *App) updateManager(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	m := &a.mgr
	list := a.filtered()
	if m.searching {
		switch k.String() {
		case "esc":
			m.searching = false
			m.search.SetValue("")
			m.search.Blur()
		case "enter":
			m.searching = false
			m.search.Blur()
		default:
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(k)
			m.sel = 0
			return cmd
		}
		return nil
	}
	switch k.String() {
	case "up", "k":
		m.sel = max(0, m.sel-1)
	case "down", "j":
		m.sel = min(len(list)-1, m.sel+1)
	case "/":
		m.searching = true
		return m.search.Focus()
	case "n":
		return a.openConnForm(newConnDefaults())
	case "e":
		if m.sel < len(list) {
			return a.openConnForm(list[m.sel])
		}
	case "d":
		if m.sel < len(list) {
			c := list[m.sel]
			a.modal = confirmModal("Verbindung löschen", "„"+c.Alias+"“ und ihre Tunnel wirklich löschen?", func() tea.Cmd {
				if err := a.st.DeleteConnection(c.ID); err != nil {
					a.flash(err.Error())
				}
				m.sel = max(0, m.sel-1)
				return nil
			})
		}
	case "t":
		if m.sel < len(list) {
			return a.openTunnelForm(domain.Tunnel{ConnectionID: list[m.sel].ID, Kind: domain.TunnelLocal, TargetHost: "127.0.0.1"})
		}
	case "s":
		return a.openSettings()
	case "y":
		return a.openSync()
	case "l":
		return a.openLogs()
	case "enter":
		if m.sel < len(list) {
			return a.connect(list[m.sel])
		}
	case "f10":
		if len(a.sessions) > 0 {
			a.scr = scrTerminal
		}
	case "f1":
		a.modal = a.helpModal()
	case "f12":
		a.modal = a.aboutModal()
	}
	return nil
}

func (a *App) connect(c domain.Connection) tea.Cmd {
	a.nextSID++
	s := &session{id: a.nextSID, conn: c, state: stConnecting, started: time.Now(), chipArmed: -1}
	a.sessions = append(a.sessions, s)
	a.cur = len(a.sessions) - 1
	a.scr = scrTerminal
	return a.connectCmd(s)
}

func (a *App) viewManager() string {
	s := a.s
	list := a.filtered()
	h := a.h - 2
	leftW := min(46, max(30, a.w/3))
	rightW := a.w - leftW - 1

	// linke Spalte: Liste, nach Gruppe
	var lb strings.Builder
	if a.mgr.searching || a.mgr.search.Value() != "" {
		lb.WriteString(" " + s.Accent.Render("/") + " " + a.mgr.search.View() + "\n\n")
	} else {
		lb.WriteString(s.PanelTitle.Foreground(s.T.Accent).Render(" Verbindungen") + s.Muted.Render(fmt.Sprintf("  %d", len(list))) +
			"  " + a.keyZone("/", s.Muted.Render("/ suchen")) + "\n\n")
	}
	group := "\x00"
	for i, c := range list {
		if c.Group != group {
			group = c.Group
			g := group
			if g == "" {
				g = "Allgemein"
			}
			lb.WriteString(s.Muted.Render(" "+strings.ToUpper(g)) + "\n")
		}
		dot := s.Muted.Render("○")
		for _, ss := range a.sessions {
			if ss.conn.ID == c.ID && ss.state == stConnected {
				dot = s.OK.Render("●")
			}
		}
		line := fmt.Sprintf(" %s %-18s %s", dot, trunc(c.Alias, 18), s.Muted.Render(trunc(c.Username+"@"+c.Host, leftW-26)))
		if i == a.mgr.sel {
			line = s.Selected.Width(leftW - 1).Render(fmt.Sprintf(" %s %-18s %s", "▶", trunc(c.Alias, 18), trunc(c.Username+"@"+c.Host, leftW-26)))
		}
		i, c := i, c
		line = a.z.click(line, func(_ tea.Mouse, double bool) tea.Cmd {
			a.mgr.sel = i
			if double {
				return a.connect(c)
			}
			return nil
		})
		lb.WriteString(line + "\n")
	}
	if len(list) == 0 {
		lb.WriteString(s.Muted.Render("  Noch keine Verbindung.\n  n = neue Verbindung anlegen"))
	}
	left := lipgloss.NewStyle().Width(leftW).Height(h).Render(lb.String())
	sep := lipgloss.NewStyle().Foreground(s.T.Border).Render(strings.Repeat("│\n", max(0, h-1)) + "│")

	// rechte Spalte: Details
	var rb strings.Builder
	if a.mgr.sel < len(list) {
		c := list[a.mgr.sel]
		rb.WriteString(s.Accent.Render(" "+c.Alias) + "  " + s.Muted.Render(c.Group) + "   " +
			a.keyZone("⏎", s.Key.Render("⏎")+s.KeyDesc.Render("verbinden")) + " " + a.keyZone("e", s.Key.Render("e")+s.KeyDesc.Render("bearbeiten")) + "\n\n")
		row := func(k, v string) { rb.WriteString(" " + s.Label.Render(k) + s.Text.Render(v) + "\n") }
		row("Ziel", fmt.Sprintf("%s@%s", c.Username, c.Addr()))
		auth := map[domain.AuthMethod]string{domain.AuthPassword: "Passwort (verschlüsselt)", domain.AuthKeyFile: "Keyfile", domain.AuthAgent: "SSH-Agent"}[c.Auth]
		if c.Auth == domain.AuthKeyFile {
			if c.KeyPEM.IsSet() {
				auth += " (eingebettet, verschlüsselt)"
			} else {
				auth += " " + c.KeyPath
			}
		}
		row("Anmeldung", auth)
		if c.JumpHostID != "" {
			if j, ok := a.st.Connection(c.JumpHostID); ok {
				row("Jump-Host", j.Alias)
			}
		}
		if len(c.Tags) > 0 {
			row("Tags", strings.Join(c.Tags, ", "))
		}
		if !c.LastUsed.IsZero() {
			row("Zuletzt", c.LastUsed.Format("02.01.2006 15:04"))
		}
		tuns := a.st.Tunnels(c.ID)
		rb.WriteString("\n " + s.PanelTitle.Foreground(s.T.Accent).Render("Tunnel") + "  " + a.keyZone("t", s.Muted.Render("t = neuer Tunnel")) + "\n")
		if len(tuns) == 0 {
			rb.WriteString(s.Muted.Render("  keine") + "\n")
		}
		for _, t := range tuns {
			icon := "⇄"
			if t.IsWebService() {
				icon = "🌐"
			}
			auto := ""
			if t.AutoStart {
				auto = s.OK.Render(" auto")
			}
			rb.WriteString(fmt.Sprintf("  %s %-20s %s%s\n", icon, trunc(t.Name, 20), s.Muted.Render(t.Spec()), auto))
		}
		cfg := a.st.LLM()
		rb.WriteString("\n " + s.PanelTitle.Render("KI-Copilot") + "\n")
		rb.WriteString("  " + s.Muted.Render(cfg.Model+" @ "+cfg.BaseURL) + "\n")
	}
	if len(list) > 0 || a.st.SyncConfig().Enabled() {
		rb.WriteString("\n " + s.PanelTitle.Render("Sync") + "\n  " + a.syncStatus() + "\n")
	}
	right := lipgloss.NewStyle().Width(rightW).Height(h).Render(rb.String())
	return lipgloss.JoinHorizontal(lipgloss.Top, left, sep, right)
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// ---------------------------------------------------------------- Formulare

func (a *App) openConnForm(c domain.Connection) tea.Cmd {
	f := &form{title: "Neue Verbindung"}
	if c.ID != "" {
		f.title = "Verbindung bearbeiten: " + c.Alias
	}
	f.text("alias", "Alias", c.Alias, "Anzeigename, z. B. „DGX Spark“")
	f.text("host", "Host", c.Host, "Name oder IP")
	f.number("port", "Port", c.Port, "Standard 22")
	f.text("user", "Benutzer", c.Username, "")
	f.choice("auth", "Anmeldung", []string{"agent", "keyfile", "password"}, string(c.Auth), "agent = ssh-agent / Windows OpenSSH-Agent (empfohlen)")
	isAuth := func(v string) func(*form) bool { return func(f *form) bool { return f.sel("auth") != v } }
	f.secret("password", "Passwort", c.Password.Reveal(), "wird mit AES-256-GCM verschlüsselt gespeichert").hidden = isAuth("password")
	f.text("keypath", "Keyfile", c.KeyPath, "z. B. ~/.ssh/id_ed25519 (leer lassen bei eingebettetem Key)").hidden = isAuth("keyfile")
	f.secret("keypass", "Passphrase", c.KeyPassphrase.Reveal(), "optional; leer = beim Verbinden fragen").hidden = isAuth("keyfile")
	f.text("group", "Gruppe", c.Group, "z. B. Produktion, Homelab")
	f.text("tags", "Tags", strings.Join(c.Tags, ", "), "durch Komma getrennt")
	jumps := []string{"—"}
	cur := "—"
	for _, o := range a.st.Connections() {
		if o.ID != c.ID {
			jumps = append(jumps, o.Alias)
			if o.ID == c.JumpHostID {
				cur = o.Alias
			}
		}
	}
	f.choice("jump", "Jump-Host", jumps, cur, "wie ssh -J")
	f.choice("device", "Gerätetyp", deviceLabels, deviceLabel(c.Device), "Netzwerkgerät = Switch/Router (Aruba, Cisco …): keine Systemabfrage")
	a.form, a.formID, a.scr = f, c.ID, scrConnForm
	a.formBase = c
	return f.setFocus(0)
}

func (a *App) openTunnelForm(t domain.Tunnel) tea.Cmd {
	f := &form{title: "Tunnel / Port-Forwarding"}
	f.text("name", "Name", t.Name, "z. B. „Grafana“")
	f.choice("kind", "Art", []string{"local", "remote", "dynamic"}, string(t.Kind), "local = -L · remote = -R · dynamic = -D (SOCKS5)")
	f.text("bindhost", "Bind-Adresse", orDefault(t.BindHost, "127.0.0.1"), "127.0.0.1 = nur dieser Rechner (empfohlen)")
	f.number("bindport", "Port", t.BindPort, "local/dynamic: lokaler Port (0 = frei wählen) · remote: Port am Server")
	notDyn := func(f *form) bool { return f.sel("kind") == "dynamic" }
	f.text("thost", "Ziel-Host", t.TargetHost, "local: vom Server aus gesehen · remote: von hier aus gesehen").hidden = notDyn
	f.number("tport", "Ziel-Port", t.TargetPort, "").hidden = notDyn
	onlyLocal := func(f *form) bool { return f.sel("kind") != "local" }
	f.choice("web", "Web-Service", []string{"—", "http", "https"}, orDefault(t.WebScheme, "—"), "One-Click: zeigt die lokale URL und öffnet den Browser").hidden = onlyLocal
	f.text("path", "Pfad", t.WebPath, "z. B. /admin").hidden = func(f *form) bool { return f.sel("kind") != "local" || f.sel("web") == "—" }
	f.toggle("autoopen", "Browser öffnen", t.AutoOpen, "").hidden = func(f *form) bool { return f.sel("web") == "—" || f.sel("kind") != "local" }
	f.toggle("autostart", "Autostart", t.AutoStart, "mit der Verbindung starten")
	a.form, a.formID, a.scr = f, t.ID, scrTunnelForm
	a.formTunnel = t
	return f.setFocus(0)
}

func (a *App) openSettings() tea.Cmd {
	c := a.st.LLM()
	f := &form{title: "KI-Copilot (OpenAI-kompatibel: OpenAI, vLLM, SGLang, Ollama, LiteLLM, LocalAI)"}
	f.text("base", "Base-URL", c.BaseURL, "z. B. http://dgx-spark:8000/v1 · Ollama: http://localhost:11434/v1")
	f.secret("key", "API-Key", c.APIKey.Reveal(), "verschlüsselt gespeichert; leer für lokale Server")
	f.text("model", "Modell", c.Model, "Strg+L: Liste vom Endpunkt · Strg+T: Verbindung testen")
	f.number("tokens", "Max-Tokens", c.MaxTokens, "")
	f.text("temp", "Temperatur", fmt.Sprintf("%.2f", c.Temperature), "0.0–1.0; für Befehle niedrig (0.1–0.3)")
	f.number("timeout", "Timeout (s)", c.TimeoutSec, "")
	f.toggle("suggest", "Ghost-Text", c.SuggestEnabled, "Vorschläge während des Tippens")
	f.number("delay", "Verzögerung ms", c.SuggestDelayMS, "Pause nach dem letzten Tastendruck")
	f.toggle("screen", "Bildschirm senden", c.SendScreen, "letzte Zeilen als Kontext (Geheimnisse werden entfernt)")
	f.number("lines", "Kontext-Zeilen", c.ContextLines, "")
	f.number("steps", "Ziel: Schritte/min", c.GoalStepsMin, "Ratenlimit im Ziel-Modus")
	f.toggle("insecure", "http mit API-Key", c.AllowInsecure, "nur wenn du weißt, was du tust")
	a.form, a.scr = f, scrSettings
	a.llm = llmUI{}
	return f.setFocus(0)
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func (a *App) updateForm(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok && a.scr == scrSettings {
		if cmd, done := a.settingsKey(k); done {
			return cmd
		}
	}
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "esc" {
		a.form = nil
		a.scr = scrManager
		if a.returnToTerminal {
			a.scr, a.returnToTerminal = scrTerminal, false
		}
		return nil
	}
	submit, cmd := a.form.update(msg)
	if !submit {
		return cmd
	}
	var err error
	switch a.scr {
	case scrConnForm:
		err = a.saveConn()
	case scrTunnelForm:
		err = a.saveTunnel()
	case scrSettings:
		err = a.saveSettings()
	}
	if err != nil {
		a.form.err = err.Error()
		return nil
	}
	a.form = nil
	a.scr = scrManager
	if a.returnToTerminal {
		a.scr, a.returnToTerminal = scrTerminal, false
	}
	a.flash("Gespeichert (verschlüsselt)")
	return nil
}

func (a *App) saveConn() error {
	f := a.form
	c := a.formBase
	c.Alias, c.Host, c.Port, c.Username = f.str("alias"), f.str("host"), f.num("port"), f.str("user")
	if c.Port == 0 {
		c.Port = 22
	}
	c.Auth = domain.AuthMethod(f.sel("auth"))
	c.Password, c.KeyPath, c.KeyPassphrase = "", "", ""
	switch c.Auth {
	case domain.AuthPassword:
		c.Password = domain.Secret(f.raw("password"))
	case domain.AuthKeyFile:
		c.KeyPath, c.KeyPassphrase = f.str("keypath"), domain.Secret(f.raw("keypass"))
		c.KeyPEM = a.formBase.KeyPEM
	}
	c.Group = f.str("group")
	c.Tags = nil
	for _, t := range strings.Split(f.str("tags"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			c.Tags = append(c.Tags, t)
		}
	}
	c.Device = deviceFromLabel(f.sel("device"))
	c.JumpHostID = ""
	for _, o := range a.st.Connections() {
		if o.Alias == f.sel("jump") && o.ID != c.ID {
			c.JumpHostID = o.ID
		}
	}
	_, err := a.st.UpsertConnection(c)
	return err
}

func (a *App) saveTunnel() error {
	f := a.form
	t := a.formTunnel
	t.Name, t.Kind = f.str("name"), domain.TunnelKind(f.sel("kind"))
	t.BindHost, t.BindPort = f.str("bindhost"), f.num("bindport")
	t.TargetHost, t.TargetPort = f.str("thost"), f.num("tport")
	t.WebScheme, t.WebPath = "", ""
	if t.Kind == domain.TunnelLocal && f.sel("web") != "—" {
		t.WebScheme, t.WebPath = f.sel("web"), f.str("path")
	}
	t.AutoOpen, t.AutoStart = f.flag("autoopen"), f.flag("autostart")
	if t.Name == "" {
		t.Name = t.Spec()
	}
	if t.Kind != domain.TunnelDynamic && (t.TargetHost == "" || t.TargetPort == 0) {
		return fmt.Errorf("Ziel-Host und Ziel-Port fehlen")
	}
	if t.Kind == domain.TunnelRemote && t.BindPort == 0 {
		return fmt.Errorf("Remote-Port fehlt")
	}
	_, err := a.st.UpsertTunnel(t)
	return err
}

func (a *App) saveSettings() error {
	c := a.formLLM()
	if err := checkLLM(c); err != nil {
		return err
	}
	return a.st.SetLLM(c)
}

func (a *App) viewForm() string {
	if a.form == nil {
		return ""
	}
	w := min(100, a.w-4)
	body := a.form.view(a.s, w-4, &a.z)
	if a.scr == scrSettings {
		body += a.viewLLMExtra(w - 4)
	}
	return lipgloss.Place(a.w, a.h-2, lipgloss.Center, lipgloss.Center, a.s.Panel.Width(w).Render(body))
}

var _ = store.Dir

func newConnDefaults() domain.Connection { return domain.Connection{Port: 22, Auth: domain.AuthAgent} }

var deviceLabels = []string{"automatisch", "Server", "Netzwerkgerät"}

func deviceLabel(d domain.DeviceKind) string {
	switch d {
	case domain.DeviceServer:
		return deviceLabels[1]
	case domain.DeviceNetwork:
		return deviceLabels[2]
	}
	return deviceLabels[0]
}

func deviceFromLabel(l string) domain.DeviceKind {
	switch l {
	case deviceLabels[1]:
		return domain.DeviceServer
	case deviceLabels[2]:
		return domain.DeviceNetwork
	}
	return domain.DeviceAuto
}
