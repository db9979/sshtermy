package ui

import (
	"errors"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Schlüsseldatei fehlt auf diesem Rechner (meist nach dem Sync von einem
// anderen Rechner, z. B. C:\Users\… am Mac): statt nur den Fehler zu zeigen,
// fragt sshTermy, wo der Key hier liegt. Der Pfad gilt nur für diesen
// Rechner, der andere behält seinen. Wahlweise wird der Key eingebettet und
// läuft dann verschlüsselt mit dem Sync mit.

const ownKeyPath = "eigener Pfad"

// openKeyFix öffnet die Abfrage für die Verbindung aus err (kann auch der
// Jump-Host sein) und verbindet danach die Sitzung sid neu.
func (a *App) openKeyFix(sid int, km *sshx.KeyMissingError) tea.Cmd {
	c, ok := a.st.Connection(km.ConnID)
	if !ok {
		return nil
	}
	f := &form{title: i18n.T("Schlüsseldatei für „") + c.Alias + i18n.T("“ auf diesem Rechner")}
	hint := i18n.T("gespeichert: ") + km.Path + i18n.T(" (gibt es hier nicht; andere Rechner behalten ihren Pfad)")
	found := sshx.LocalKeys()
	if len(found) > 0 {
		f.choice("found", "Key", append(found, ownKeyPath), found[0], hint)
		hint = i18n.T("z. B. ~/.ssh/id_ed25519")
	}
	pathField := f.text("keypath", i18n.T("Pfad"), "", hint)
	if len(found) > 0 {
		pathField.hidden = func(f *form) bool { return f.sel("found") != ownKeyPath }
	}
	f.toggle("embed", i18n.T("Key mitnehmen"), false, i18n.T("Key verschlüsselt im Store speichern und mitsynchronisieren: dann braucht kein Rechner mehr die Datei"))
	a.form, a.formID, a.formSID, a.scr = f, c.ID, sid, scrKeyForm
	a.returnToTerminal = true
	return f.setFocus(0)
}

// saveKeyFix übernimmt den gewählten Key und verbindet neu.
func (a *App) saveKeyFix() (tea.Cmd, error) {
	f := a.form
	p := f.str("keypath")
	if f.get("found") != nil && f.sel("found") != ownKeyPath {
		p = f.sel("found")
	}
	if p == "" {
		return nil, errors.New(i18n.T("Pfad zur Schlüsseldatei fehlt"))
	}
	home, _ := os.UserHomeDir()
	full := domain.ExpandKeyPath(p, home)
	if err := sshx.CheckKeyFile(full); err != nil {
		return nil, err
	}
	c, ok := a.st.Connection(a.formID)
	if !ok {
		return nil, errors.New(i18n.T("Verbindung nicht gefunden"))
	}
	if f.flag("embed") {
		b, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		c.KeyPEM, c.KeyPath = domain.Secret(b), ""
		if _, err := a.st.UpsertConnection(c); err != nil {
			return nil, err
		}
		a.flash(i18n.T("Key eingebettet (verschlüsselt, geht mit dem Sync mit)"))
	} else {
		if err := a.st.SetLocalKeyPath(c.ID, p); err != nil {
			return nil, err
		}
		a.flash(i18n.T("Schlüsseldatei für diesen Rechner gemerkt"))
	}
	s := a.byID(a.formSID)
	if s == nil || s.state != stClosed {
		return nil, nil
	}
	if fresh, ok := a.st.Connection(s.conn.ID); ok {
		s.conn = fresh
	}
	s.state, s.err, s.errv = stConnecting, "", nil
	return a.connectCmd(s), nil
}
