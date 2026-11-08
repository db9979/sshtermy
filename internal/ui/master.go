package ui

import (
	"errors"
	"fmt"
	"runtime"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/audit"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/vault"
)

// Master-Passwort ändern und (unter Windows) das Entsperren über die
// Windows-Anmeldung (DPAPI) ein- oder ausschalten. Beides verlangt das
// bisherige Passwort; der Datenschlüssel bleibt gleich, Sitzungsprotokolle
// und gesicherte Configs bleiben also lesbar.

func (a *App) openMasterForm() tea.Cmd {
	f := &form{title: i18n.T("Master-Passwort und Entsperren")}
	f.secret("old", i18n.T("Bisheriges Passwort"), "", i18n.T("zur Bestätigung, auch wenn du nur die Windows-Anmeldung umschaltest"))
	f.secret("new", i18n.T("Neues Passwort"), "", i18n.T("mindestens 8 Zeichen · leer lassen = Passwort bleibt"))
	f.secret("again", i18n.T("Neues Passwort wiederholen"), "", "")
	if vault.DPAPIAvailable() || runtime.GOOS == "windows" {
		f.toggle("dpapi", i18n.T("Mit der Windows-Anmeldung entsperren (DPAPI)"), a.st.V.HasDPAPI(),
			i18n.T("an: sshTermy öffnet sich nach der Windows-Anmeldung ohne Eingabe · das Master-Passwort bleibt der Weg auf anderen Rechnern"))
	}
	a.returnToTerminal = a.scr == scrTerminal
	a.form, a.formID, a.scr = f, "", scrMasterForm
	return f.setFocus(0)
}

func (a *App) saveMasterForm() error {
	f := a.form
	old, nw := []byte(f.raw("old")), []byte(f.raw("new"))
	defer vault.Wipe(old)
	defer vault.Wipe(nw)
	if len(old) == 0 {
		return errors.New(i18n.T("Bisheriges Passwort fehlt"))
	}
	if f.raw("new") != f.raw("again") {
		return errors.New(i18n.T("Die neuen Passwörter sind nicht gleich"))
	}
	dpapi := a.st.V.HasDPAPI()
	if f.get("dpapi") != nil {
		dpapi = f.flag("dpapi")
	}
	if len(nw) == 0 && dpapi == a.st.V.HasDPAPI() {
		return errors.New(i18n.T("Nichts geändert: neues Passwort eintragen oder die Windows-Anmeldung umschalten"))
	}
	if err := a.st.ChangeMaster(old, nw, dpapi); err != nil {
		if errors.Is(err, vault.ErrWrongPassword) {
			a.auditAdd(audit.EvSettings, "", "Master-Passwort ändern: falsches Passwort")
			return errors.New(i18n.T("Bisheriges Passwort stimmt nicht"))
		}
		return err
	}
	a.auditAdd(audit.EvSettings, "", fmt.Sprintf("Master-Passwort geändert=%t, Windows-Anmeldung=%t", len(nw) > 0, dpapi))
	return nil
}
