package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/psremote"
	"github.com/db9979/sshtermy/internal/pwsource"
	"github.com/db9979/sshtermy/internal/sshx"
)

// PowerShell über WinRM: eine Sitzung wie Enter-PSSession, ohne SSH auf dem
// Server. Läuft wie Telnet/Konsole ohne SSH-Client (s.client == nil); die
// Eingabezeile (Verlauf, Tab-Ergänzung) baut psremote, weil WinRS keine
// Konsole hat. Profil: immer Windows mit PowerShell-Befehlssatz.

func (s *session) isWinRM() bool { return s.conn.Protocol == domain.ProtoWinRM }

// winrmPassword: gespeichertes Passwort, Schlüsselbund, Passwortmanager
// oder nachfragen (die Antwort merkt sich die Sitzung für Neuverbinden).
func winrmPassword(c domain.Connection, ask sshx.SecretPrompt, notice func(string)) (string, error) {
	pw := c.Password.Reveal()
	if pw == "" && c.Keychain {
		pw = sshx.KeychainSecret(c.ID, sshx.SecretPassword)
	}
	if pw == "" && c.PassSource != "" {
		v, err := pwsource.Get(context.Background(), c.PassSource, pwsource.Password, pwsource.Ask(ask))
		if err == nil {
			return v, nil
		}
		notice(pwsource.Label(c.PassSource) + ": " + err.Error())
	}
	if pw == "" && c.WinRMAuth == psremote.AuthKerberos && psremote.HasKerberosTicket() {
		return "", nil // Ticket von kinit
	}
	if pw == "" && ask != nil {
		return ask(sshx.PasswordLabel(c))
	}
	return pw, nil
}

func winrmConfig(c domain.Connection, pw string) psremote.Config {
	return psremote.Config{Host: strings.Trim(c.Host, "[]"), Port: c.Port, HTTPS: c.WinRMHTTPS, Insecure: c.WinRMInsecure,
		Auth: c.WinRMAuth, User: c.Username, Password: pw}
}

// dialWinRM: aus connectRawCmd (läuft im Hintergrund).
func (a *App) dialWinRM(s *session, c domain.Connection, cols int, out io.Writer) (*psremote.Stream, error) {
	ev := a.events
	ask := a.winrmAsk(s)
	sc := s.secrets
	pw, err := winrmPassword(c, ask, func(t string) { ev <- connectNoticeMsg{s.id, t} })
	if err != nil {
		return nil, err
	}
	psremote.SetInterruptText(i18n.T("Befehl abgebrochen · PowerShell neu gestartet (Variablen sind weg, Ordner bleibt)"))
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	st, err := psremote.Dial(ctx, winrmConfig(c, pw), cols, out)
	if err != nil {
		if errors.Is(err, psremote.ErrAuth) {
			sc.forget() // falsches Passwort nicht beim nächsten Versuch wiederholen
		}
		return nil, winrmError(err, c)
	}
	return st, nil
}

// winrmAsk: Passwort-Rückfrage der Sitzung (Antworten bleiben im Speicher,
// damit Neuverbinden und Abfragen wie der Log-Blick nicht noch einmal fragen).
func (a *App) winrmAsk(s *session) sshx.SecretPrompt {
	ev := a.events
	if s.secrets == nil {
		s.secrets = &secretCache{}
	}
	return s.secrets.wrap(func(label string) (string, error) {
		r := make(chan string, 1)
		ev <- secretAskMsg{label, r}
		v := <-r
		if v == "\x00cancel" {
			return "", context.Canceled
		}
		return v, nil
	})
}

// winrmRun: kurzes PowerShell-Skript neben der Sitzung ausführen (eigene
// WinRS-Shell, gleiche Anmeldung). Läuft im Hintergrund.
func (a *App) winrmRun(ctx context.Context, s *session, script string) (string, error) {
	pw, err := winrmPassword(s.conn, a.winrmAsk(s), func(string) {})
	if err != nil {
		return "", err
	}
	out, err := psremote.Run(ctx, winrmConfig(s.conn, pw), script)
	if err != nil {
		return out, winrmError(err, s.conn)
	}
	return out, nil
}

// winrmError: Fehler mit Hinweis, was auf dem Server fehlt.
func winrmError(err error, c domain.Connection) error {
	var hint string
	switch {
	case errors.Is(err, psremote.ErrAuth):
		hint = i18n.T("Anmeldung abgelehnt: Benutzer/Passwort prüfen (Domäne als DOMÄNE\\name oder name@domäne). Lokale Konten brauchen auf dem Server oft LocalAccountTokenFilterPolicy=1.")
	case errors.Is(err, psremote.ErrRefused):
		hint = i18n.F("WinRM antwortet nicht auf Port %d. Auf dem Server (als Administrator): Enable-PSRemoting -Force · Firewall-Regel „Windows-Remoteverwaltung“ prüfen.", orPort(c))
	case errors.Is(err, psremote.ErrCert):
		hint = i18n.T("Zertifikat des Servers ist nicht vertrauenswürdig (oft selbst signiert). In der Verbindung „Zertifikat nicht prüfen“ einschalten oder ein Zertifikat der eigenen CA einrichten.")
	case errors.Is(err, psremote.ErrUnreadable):
		hint = i18n.T("Unerwartete Antwort: passt HTTP/HTTPS zum Port? 5985 = HTTP, 5986 = HTTPS.")
	case errors.Is(err, psremote.ErrKerberosIP):
		return errors.New(i18n.T("Kerberos braucht den Rechnernamen (z. B. server.firma.local), keine IP-Adresse"))
	case errors.Is(err, psremote.ErrKerberosRealm):
		return errors.New(i18n.T("Kerberos: Domäne unbekannt, Benutzer als name@firma.local angeben"))
	case errors.Is(err, psremote.ErrKerberosSPN):
		hint = i18n.T("Kerberos: Der Domänencontroller kennt HTTP/<Rechnername> nicht. Den vollen Rechnernamen aus dem AD eintragen (kein Alias/CNAME).")
	case errors.Is(err, psremote.ErrKerberosKDC):
		hint = i18n.T("Kerberos: kein Domänencontroller erreichbar. DNS der Domäne nötig (SRV _kerberos._tcp) oder eine krb5.conf (KRB5_CONFIG).")
	case errors.Is(err, psremote.ErrBasicHTTP):
		return errors.New(i18n.T("Basic-Anmeldung nur über HTTPS (sonst geht das Passwort lesbar übers Netz)"))
	case errors.Is(err, psremote.ErrStartup):
		hint = i18n.T("Angemeldet, aber PowerShell startet drüben nicht")
	default:
		return err
	}
	return fmt.Errorf("%s · %w", hint, err)
}

func orPort(c domain.Connection) int {
	if c.Port > 0 {
		return c.Port
	}
	return c.DefaultPort()
}

// winrmProfile: Systemprofil aus der ersten Antwort (Windows-Version).
func winrmProfile(st *psremote.Stream) domain.SystemProfile {
	p := domain.SystemProfile{Family: domain.FamilyWindows, Dialect: "powershell", OS: "Windows", Shell: "powershell"}
	if st == nil {
		return p
	}
	in := st.Info()
	if in.OS != "" {
		p.OS = in.OS
	}
	if in.PSVersion != "" {
		p.Shell = "PowerShell " + in.PSVersion
	}
	return p
}

// winrmAuthText: Anmeldung in den Details der Verbindung.
func winrmAuthText(c domain.Connection) string {
	t := "NTLM"
	switch c.WinRMAuth {
	case "basic":
		t = "Basic"
	case "kerberos":
		t = "Kerberos"
	}
	if c.WinRMHTTPS {
		t += " · HTTPS"
		if c.WinRMInsecure {
			t += " · " + i18n.T("Zertifikat nicht geprüft")
		}
	} else {
		t += " · " + i18n.T("HTTP, Nachrichten verschlüsselt")
	}
	switch {
	case c.Keychain:
		t += " · " + i18n.T("Passwort") + " " + i18n.T(keychainName())
	case c.PassSource != "":
		t += " · " + pwsource.Label(c.PassSource)
	case !c.Password.IsSet():
		t += " · " + i18n.T("Passwort wird gefragt")
	}
	return t
}

// winrmBadge: Plakette in der Sitzungsleiste.
func (a *App) winrmBadge(s *session) string {
	st := a.s
	t := "PS · WinRM"
	if s.conn.WinRMHTTPS {
		t += " · HTTPS"
	}
	return st.Badge.Foreground(st.T.Accent).Render(t)
}

// ---------------------------------------------------------------- Palette

// parseQuickWinRM: „winrm [user@]host [port]“, „Enter-PSSession host“,
// „Enter-PSSession -ComputerName host [-Credential user] [-UseSSL]“, „etsn host“.
func parseQuickWinRM(q string) (domain.Connection, bool) {
	f := strings.Fields(strings.TrimSpace(q))
	if len(f) < 2 {
		return domain.Connection{}, false
	}
	switch strings.ToLower(f[0]) {
	case "winrm", "psremote", "enter-pssession", "etsn":
	default:
		return domain.Connection{}, false
	}
	var host, user string
	port, https := 0, false
	for i := 1; i < len(f); i++ {
		w := f[i]
		lw := strings.ToLower(w)
		next := func() string {
			if i+1 < len(f) {
				i++
				return f[i]
			}
			return ""
		}
		switch {
		case lw == "-computername" || lw == "-cn":
			host = next()
		case lw == "-credential" || lw == "-cred":
			user = next()
		case lw == "-port":
			port, _ = strconv.Atoi(next())
		case lw == "-usessl":
			https = true
		case strings.HasPrefix(w, "-"):
			return domain.Connection{}, false
		case host == "":
			host = w
		case port == 0:
			p, err := strconv.Atoi(w)
			if err != nil {
				return domain.Connection{}, false
			}
			port = p
		default:
			return domain.Connection{}, false
		}
	}
	user = strings.Trim(user, `"'`)
	if at := strings.LastIndex(host, "@"); at >= 0 && user == "" {
		user, host = host[:at], host[at+1:]
	}
	if host == "" || !validHost(host) || port < 0 || port > 65535 {
		return domain.Connection{}, false
	}
	if port == 5986 {
		https = true // WinRM-Standard für HTTPS
	}
	c := domain.Connection{Protocol: domain.ProtoWinRM, Host: host, Port: port, Username: user, WinRMHTTPS: https, Auth: domain.AuthPassword}
	if c.Port == 0 {
		c.Port = c.DefaultPort()
	}
	c.Alias = "PS " + host
	c.ID = "quick:winrm:" + host + ":" + strconv.Itoa(c.Port)
	return c, true
}

// quickWinRMItems: Palette-Einträge für eine getippte PowerShell-Sitzung.
func (a *App) quickWinRMItems(c domain.Connection) []palItem {
	conn := func() tea.Cmd {
		if c.Username == "" {
			// ohne Benutzer geht WinRM nicht: Formular mit allem Getippten
			n := c
			n.ID = ""
			n.Alias = c.Host
			return a.openConnForm(n)
		}
		return a.openOrConnect(c)
	}
	return []palItem{
		{group: pgConnect, icon: ic("connect"), label: i18n.T("PowerShell-Sitzung: ") + c.Host, hint: "WinRM " + strconv.Itoa(c.Port),
			run: conn},
		{group: pgConnect, icon: ic("new"), label: i18n.T("Als Verbindung speichern: ") + c.Host, hint: i18n.T("anlegen"),
			run: func() tea.Cmd {
				n := c
				n.ID = ""
				n.Alias = c.Host
				return a.openConnForm(n)
			}},
	}
}
