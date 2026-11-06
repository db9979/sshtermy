package ui

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/atotto/clipboard"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/keychain"
	"github.com/db9979/sshtermy/internal/sshx"
)

// SSH-Schlüssel verwalten: neu erzeugen (wie ssh-keygen) oder einen
// vorhandenen nehmen, auf den Server übertragen (wie ssh-copy-id), mit dem
// Key probeweise anmelden und erst dann die Verbindung umstellen.
// Passwörter und Passphrasen können statt im Store im Schlüsselbund des
// Betriebssystems liegen (Connection.Keychain).

const (
	keyModeNew   = "neu erzeugen"
	keyModeOld   = "vorhandenen nehmen"
	keyWhereFile = "Datei"
	keyWhereEmb  = "eingebettet"
)

// keychainName: wie der Schlüsselbund auf diesem System heißt.
func keychainName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS-Schlüsselbund"
	case "windows":
		return "Windows-Anmeldeinformationen"
	}
	return "Schlüsselbund"
}

// keychainIn: „im macOS-Schlüsselbund“ usw. (für Sätze).
func keychainIn() string {
	switch runtime.GOOS {
	case "darwin":
		return i18n.T("im macOS-Schlüsselbund")
	case "windows":
		return i18n.T("in den Windows-Anmeldeinformationen")
	}
	return i18n.T("im Schlüsselbund des Systems")
}

func keychainHelp() string {
	return i18n.F("Passwort/Passphrase %s statt in sshTermy", keychainIn())
}

// sshKeyTarget: für welche Verbindung (Terminal: aktueller Tab, Manager: Auswahl).
func (a *App) sshKeyTarget() (domain.Connection, bool) {
	if a.scr == scrTerminal {
		if s := a.current(); s != nil {
			return a.st.Connection(s.conn.ID)
		}
		return domain.Connection{}, false
	}
	rows := a.mgrRows()
	if a.mgr.sel < len(rows) && !rows[a.mgr.sel].isFolder() {
		return a.st.Connection(rows[a.mgr.sel].conn.ID)
	}
	return domain.Connection{}, false
}

// keySlug: Dateiname aus dem Alias („DGX Spark“ → „dgx-spark“).
func keySlug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "host"
	}
	return out
}

func (a *App) openSSHKeyForm(c domain.Connection) tea.Cmd {
	f := &form{title: i18n.T("SSH-Schlüssel für „") + c.Alias + "“ (" + c.Username + "@" + c.Host + ")"}
	f.choice("mode", i18n.T("Schlüssel"), []string{keyModeNew, keyModeOld}, keyModeNew, i18n.T("neu wie ssh-keygen, oder einen vorhandenen Key nehmen"))
	notNew := func(f *form) bool { return f.sel("mode") != keyModeNew }
	notOld := func(f *form) bool { return f.sel("mode") != keyModeOld }
	f.choice("type", i18n.T("Typ"), []string{string(sshx.KeyEd25519), string(sshx.KeyRSA4096)}, string(sshx.KeyEd25519),
		i18n.T("ed25519 = modern und kurz · rsa-4096 für alte Server und Switches")).hidden = notNew
	f.choice("where", i18n.T("Ablage"), []string{keyWhereFile, keyWhereEmb}, keyWhereFile,
		i18n.T("Datei: auch für ssh nutzbar · eingebettet: verschlüsselt, mit Sync")).hidden = notNew
	f.text("file", i18n.T("Datei"), "~/.ssh/sshtermy_"+keySlug(c.Alias), i18n.T("privater Key; der öffentliche kommt daneben als .pub")).hidden = func(f *form) bool {
		return f.sel("mode") != keyModeNew || f.sel("where") != keyWhereFile
	}
	found := sshx.LocalKeys()
	f.choice("found", "Key", append(found, ownKeyPath), "", i18n.T("private Keys aus ~/.ssh")).hidden = notOld
	f.text("keypath", i18n.T("Pfad"), "", i18n.T("z. B. ~/.ssh/id_ed25519")).hidden = func(f *form) bool {
		return f.sel("mode") != keyModeOld || f.sel("found") != ownKeyPath
	}
	f.secret("pass", "Passphrase", "", i18n.T("neu: schützt den Key · vorhanden: nur nötig, wenn verschlüsselt"))
	if keychain.Available() {
		f.toggle("keychain", i18n.T("Im Schlüsselbund"), true, i18n.F("Passphrase %s statt in sshTermy ablegen", keychainIn())).hidden = func(f *form) bool {
			return f.raw("pass") == ""
		}
	}
	f.toggle("install", i18n.T("Auf Server kopieren"), true, i18n.T("wie ssh-copy-id: trägt den Key in ~/.ssh/authorized_keys ein"))
	f.toggle("switch", i18n.T("Key benutzen"), true, i18n.T("erst nach geglückter Probe-Anmeldung mit dem Key: Anmeldung = Keyfile"))
	a.returnToTerminal = a.scr == scrTerminal
	a.form, a.formID, a.scr = f, c.ID, scrSSHKeyForm
	return f.setFocus(0)
}

// sshKeyJob: was der Hintergrundlauf tun soll.
type sshKeyJob struct {
	conn       domain.Connection
	gen        bool
	typ        sshx.KeyType
	embed      bool
	path       string // wie gespeichert (~/…)
	full       string // auf der Platte
	pass       string
	keychain   bool
	install    bool
	switchAuth bool
}

type sshKeyDoneMsg struct {
	job       sshKeyJob
	pub       string
	pem       []byte // nur bei eingebettetem Key
	wrote     bool
	installed bool
	already   bool
	netdev    bool
	switched  bool
	err       error
	step      string
}

func (a *App) startSSHKey() (tea.Cmd, error) {
	f := a.form
	c, ok := a.st.Connection(a.formID)
	if !ok {
		return nil, errors.New(i18n.T("Verbindung nicht gefunden"))
	}
	home, _ := os.UserHomeDir()
	j := sshKeyJob{conn: c, gen: f.sel("mode") == keyModeNew, typ: sshx.KeyType(f.sel("type")), pass: f.raw("pass"),
		install: f.flag("install"), switchAuth: f.flag("switch")}
	j.keychain = j.pass != "" && f.flag("keychain")
	if j.gen {
		j.embed = f.sel("where") == keyWhereEmb
		if !j.embed {
			j.path = f.str("file")
			if j.path == "" {
				return nil, errors.New(i18n.T("Dateiname fehlt"))
			}
		}
	} else {
		j.path = f.str("keypath")
		if f.sel("found") != ownKeyPath {
			j.path = f.sel("found")
		}
		if j.path == "" {
			return nil, errors.New(i18n.T("Pfad zur Schlüsseldatei fehlt"))
		}
	}
	if j.path != "" {
		j.full = domain.ExpandKeyPath(j.path, home)
		j.path = domain.PortableKeyPath(j.full, home)
	}
	if j.gen && !j.embed {
		if _, err := os.Stat(j.full); err == nil {
			return nil, errors.New(i18n.F("%s gibt es schon, bitte anderen Namen wählen", j.path))
		}
	}
	if !j.gen {
		if err := sshx.CheckKeyFile(j.full); err != nil {
			return nil, err
		}
	}
	if !j.install && !j.switchAuth && !j.gen {
		return nil, errors.New(i18n.T("Nichts zu tun: übertragen oder umstellen einschalten"))
	}
	a.modal = &modal{kind: mBusy, title: i18n.T("SSH-Schlüssel"), body: i18n.T("Arbeite für ") + c.Alias + " …"}
	return a.sshKeyCmd(j), nil
}

// dialOpts: wie ein Tab, Rückfragen über die normalen Dialoge.
func (a *App) dialOpts() sshx.Options {
	ev := a.events
	return sshx.Options{
		HostKeys: a.hostKeys,
		Resolve:  a.st.Connection,
		Relay:    a.relayDialer(),
		Prompt: func(host, fp, typ string) bool {
			r := make(chan bool, 1)
			ev <- hostKeyAskMsg{host, fp, typ, r}
			return <-r
		},
		AskSecret: func(label string) (string, error) {
			r := make(chan string, 1)
			ev <- secretAskMsg{label, r}
			v := <-r
			if v == "\x00cancel" {
				return "", context.Canceled
			}
			return v, nil
		},
	}
}

func (a *App) sshKeyCmd(j sshKeyJob) tea.Cmd {
	opts := a.dialOpts()
	return func() tea.Msg {
		res := sshKeyDoneMsg{job: j}
		fail := func(step string, err error) tea.Msg {
			res.step, res.err = step, err
			return res
		}
		var key *sshx.NewKey
		var err error
		if j.gen {
			key, err = sshx.GenerateKey(j.typ, sshx.KeyComment(), []byte(j.pass))
			if err != nil {
				return fail(i18n.T("Erzeugen"), err)
			}
			if j.embed {
				res.pem = append([]byte(nil), key.PrivatePEM...)
			} else {
				if err := sshx.WriteKeyFiles(j.full, key); err != nil {
					return fail(i18n.T("Speichern"), err)
				}
				res.wrote = true
			}
		} else {
			b, err := os.ReadFile(j.full)
			if err != nil {
				return fail(i18n.T("Lesen"), err)
			}
			key, err = sshx.LoadKey(b, []byte(j.pass))
			for i := range b {
				b[i] = 0
			}
			if err != nil {
				return fail(i18n.T("Lesen"), err)
			}
		}
		defer key.Wipe()
		res.pub = key.PublicLine

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if j.install {
			cl, err := sshx.Dial(ctx, j.conn, opts)
			if err != nil {
				return fail(i18n.T("Anmelden"), err)
			}
			if cl.NetDevice || j.conn.Device == domain.DeviceNetwork {
				cl.Close()
				res.netdev = true
				return res // Switch: Key von Hand am Gerät eintragen, nichts umstellen
			}
			res.already, err = cl.InstallKey(ctx, key.PublicLine)
			cl.Close()
			if err != nil {
				return fail(i18n.T("Übertragen"), err)
			}
			res.installed = true
		}
		if j.switchAuth {
			t := j.conn
			t.Auth, t.Password, t.Keychain = domain.AuthKeyFile, "", false
			t.KeyPath, t.KeyPEM, t.KeyPassphrase = j.full, "", domain.Secret(j.pass)
			if j.embed {
				t.KeyPath, t.KeyPEM = "", domain.Secret(res.pem)
			}
			o := opts
			o.AskSecret = nil // Probe nur mit dem Key, kein Passwort-Rückfall
			t.Device = domain.DeviceServer
			cl, err := sshx.Dial(ctx, t, o)
			if err != nil {
				return fail(i18n.T("Probe-Anmeldung mit dem Key"), err)
			}
			cl.Close()
			res.switched = true
		}
		return res
	}
}

// sshKeyDone: Ergebnis anzeigen und (nach geglückter Probe) die Verbindung
// auf den Key umstellen.
func (a *App) sshKeyDone(m sshKeyDoneMsg) tea.Cmd {
	j := m.job
	var lines []string
	ok := func(s string) { lines = append(lines, a.s.OK.Render("✓ ")+s) }
	if j.gen {
		where := j.path
		if j.embed {
			where = i18n.T("eingebettet (verschlüsselt, geht mit dem Sync mit)")
		}
		if m.wrote || j.embed && m.pub != "" {
			ok(i18n.T("Schlüssel erzeugt: ") + string(j.typ) + " · " + where)
		}
	}
	if m.installed {
		if m.already {
			ok(i18n.T("Stand schon in authorized_keys auf ") + j.conn.Host)
		} else {
			ok(i18n.T("Eingetragen in ~/.ssh/authorized_keys auf ") + j.conn.Host)
		}
	}
	if m.netdev {
		lines = append(lines, a.s.Warn.Render("! ")+i18n.T("Netzwerkgerät: kein authorized_keys. Bitte den Key am Gerät eintragen (er liegt in der Zwischenablage), dann hier mit „vorhandenen nehmen“ umstellen."))
	}
	if m.switched && m.err == nil {
		if err := a.switchToKey(m); err != nil {
			m.step, m.err = i18n.T("Verbindung umstellen"), err
		} else {
			ok(i18n.T("Probe-Anmeldung mit dem Key klappt: die Verbindung nutzt ihn ab jetzt"))
			if j.keychain {
				ok(i18n.F("Passphrase liegt %s", keychainIn()))
			}
		}
	}
	if m.err != nil {
		lines = append(lines, a.s.Danger.Render("✗ ")+m.step+": "+m.err.Error())
		if m.wrote {
			lines = append(lines, a.s.Muted.Render(i18n.T("Die Schlüsseldatei bleibt liegen; nochmal mit „vorhandenen nehmen“ versuchen.")))
		} else if j.embed && j.gen {
			lines = append(lines, a.s.Muted.Render(i18n.T("Der eingebettete Key wurde verworfen, die Verbindung bleibt, wie sie war.")))
		}
	}
	var cmd tea.Cmd
	if m.pub != "" {
		pub := m.pub
		go func() { _ = clipboard.WriteAll(pub) }()
		cmd = tea.SetClipboard(pub)
		lines = append(lines, "", i18n.T("Öffentlicher Schlüssel (in der Zwischenablage):"), a.s.Muted.Render(pub))
	}
	a.modal = &modal{kind: mInfo, title: i18n.T("SSH-Schlüssel für ") + j.conn.Alias, body: strings.Join(lines, "\n")}
	return cmd
}

// switchToKey stellt die gespeicherte Verbindung auf den neuen Key um.
func (a *App) switchToKey(m sshKeyDoneMsg) error {
	j := m.job
	c, ok := a.st.Connection(j.conn.ID)
	if !ok {
		return errors.New(i18n.T("Verbindung nicht gefunden"))
	}
	wasKC := c.Keychain
	c.Auth, c.Password = domain.AuthKeyFile, ""
	c.KeyPath, c.KeyPEM = j.path, ""
	if j.embed {
		c.KeyPath, c.KeyPEM = "", domain.Secret(m.pem)
	}
	c.KeyPassphrase, c.Keychain = domain.Secret(j.pass), false
	if j.keychain {
		if err := sshx.SetKeychainSecret(c, sshx.SecretPassphrase, j.pass); err != nil {
			return err
		}
		c.KeyPassphrase, c.Keychain = "", true
	}
	if _, err := a.st.UpsertConnection(c); err != nil {
		return err
	}
	if wasKC {
		_ = sshx.SetKeychainSecret(c, sshx.SecretPassword, "") // altes Passwort hier nicht mehr nötig
		if !c.Keychain {
			sshx.ForgetKeychain(c.ID)
		}
	}
	for _, s := range a.sessions {
		if s.conn.ID == c.ID {
			if fresh, ok := a.st.Connection(c.ID); ok {
				s.conn = fresh // Auto-Reconnect nimmt schon den Key
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------- Schlüsselbund im Verbindungsformular

// connFormKeychain: Liegt das Geheimnis im Schlüsselbund, kommt es nicht in
// den Store. Liefert das Geheimnis aus dem Formular für saveConnKeychain.
func (a *App) connFormKeychain(c *domain.Connection) string {
	use := a.formBase.Keychain
	if fl := a.form.get("keychain"); fl.key != "" {
		use = fl.on
	}
	if c.Protocol.Raw() || c.Auth != domain.AuthPassword && c.Auth != domain.AuthKeyFile {
		use = false // Telnet/Konsole tippen das Passwort selbst ein
	}
	c.Keychain = use
	secret := c.Password.Reveal()
	if c.Auth == domain.AuthKeyFile {
		secret = c.KeyPassphrase.Reveal()
	}
	if use {
		c.Password, c.KeyPassphrase = "", ""
	}
	return secret
}

// saveConnKeychain schreibt nach dem Speichern in den Schlüsselbund bzw.
// räumt ihn auf, wenn der Schalter aus ist.
func (a *App) saveConnKeychain(c domain.Connection, secret string) error {
	if !keychain.Available() {
		return nil
	}
	base := a.formBase
	if !c.Keychain {
		if base.Keychain {
			sshx.ForgetKeychain(c.ID)
		}
		return nil
	}
	kind, other, prev := sshx.SecretPassword, sshx.SecretPassphrase, a.formKC[0]
	if c.Auth == domain.AuthKeyFile {
		kind, other, prev = sshx.SecretPassphrase, sshx.SecretPassword, a.formKC[1]
	}
	if !base.Keychain || secret != prev {
		if err := sshx.SetKeychainSecret(c, kind, secret); err != nil {
			a.formBase = c // gespeichert ist die Verbindung schon: nicht doppelt anlegen
			a.formKC = [2]string{}
			return errors.New(i18n.T(keychainName()) + ": " + err.Error())
		}
	}
	_ = sshx.SetKeychainSecret(c, other, "")
	return nil
}

// learnKeychain: Fehlte das Geheimnis im Schlüsselbund (z. B. erster Start
// auf einem anderen Rechner nach dem Sync), hat sshTermy gefragt. Nach der
// geglückten Anmeldung kommt die Antwort in den Schlüsselbund.
func (a *App) learnKeychain(s *session) {
	c := s.conn
	if !c.Keychain || s.secrets == nil || strings.HasPrefix(c.ID, quickPrefix) || !keychain.Available() {
		return
	}
	kind, label := sshx.SecretPassword, sshx.PasswordLabel(c)
	if c.Auth == domain.AuthKeyFile {
		kind, label = sshx.SecretPassphrase, sshx.PassphraseLabel(c)
	} else if c.Auth != domain.AuthPassword {
		return
	}
	v, ok := s.secrets.get(label)
	if !ok || v == "" || sshx.KeychainSecret(c.ID, kind) == v {
		return
	}
	if err := sshx.SetKeychainSecret(c, kind, v); err != nil {
		a.flash(i18n.T(keychainName()) + ": " + err.Error())
		return
	}
	a.flash(i18n.F("Für %s %s gemerkt", c.Alias, keychainIn()))
}
