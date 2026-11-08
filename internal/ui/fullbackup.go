package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/audit"
	"github.com/db9979/sshtermy/internal/debuglog"
	"github.com/db9979/sshtermy/internal/fullbackup"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/vault"
	"github.com/db9979/sshtermy/internal/version"
)

// Gesamtsicherung: der ganze Datenordner (Store, Protokolle, Sicherungen,
// known_hosts, Einstellungen) als eine verschlüsselte Datei, zum Umzug auf
// einen neuen Rechner oder als Sicherung. Verschlüsselt wird mit dem
// Master-Passwort (internal/fullbackup).

func defaultBackupPath() string {
	return filepath.Join(exportDir(), "sshtermy-sicherung-"+time.Now().Format("2006-01-02")+fullbackup.Ext)
}

func (a *App) openBackupForm(restore bool) tea.Cmd {
	f := &form{title: i18n.T("Gesamtsicherung speichern")}
	id := "export"
	if restore {
		id = "import"
		f.title = i18n.T("Gesamtsicherung einspielen")
		f.text("file", i18n.T("Datei"), "", i18n.T("Pfad zur .sshtbak-Datei"))
		f.secret("pw", i18n.T("Master-Passwort der Sicherung"), "", i18n.T("das Passwort, mit dem die Sicherung gespeichert wurde"))
	} else {
		f.text("file", i18n.T("Datei"), defaultBackupPath(), i18n.T("enthält alle Verbindungen, Schlüssel, Protokolle und Einstellungen"))
		f.secret("pw", i18n.T("Master-Passwort"), "", i18n.T("die Sicherung wird damit verschlüsselt"))
	}
	a.returnToTerminal = a.scr == scrTerminal
	a.form, a.formID, a.scr = f, id, scrBackupForm
	return f.setFocus(0)
}

func (a *App) saveBackupForm() (tea.Cmd, error) {
	f := a.form
	file := expandHome(f.str("file"))
	pw := []byte(f.raw("pw"))
	defer vault.Wipe(pw)
	if file == "" {
		return nil, errors.New(i18n.T("Datei fehlt"))
	}
	if len(pw) == 0 {
		return nil, errors.New(i18n.T("Master-Passwort fehlt"))
	}
	if a.formID == "import" {
		return a.prepareRestore(file, pw)
	}
	if err := a.st.V.CheckPassword(pw); err != nil {
		return nil, errors.New(i18n.T("Master-Passwort stimmt nicht"))
	}
	if !strings.HasSuffix(strings.ToLower(file), fullbackup.Ext) {
		file += fullbackup.Ext
	}
	m, err := writeBackup(a.st.Dir, file, pw)
	if err != nil {
		return nil, err
	}
	a.auditAdd(audit.EvBackup, "", "Gesamtsicherung gespeichert: "+file)
	a.flashOK(fmt.Sprintf(i18n.T("Gesamtsicherung gespeichert: %s (%d Dateien)"), file, m.Files))
	return nil, nil
}

func writeBackup(dir, file string, pw []byte) (fullbackup.Manifest, error) {
	tmp := file + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fullbackup.Manifest{}, err
	}
	m, err := fullbackup.Create(dir, pw, version.Version, out)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, file)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return m, err
}

// prepareRestore entpackt die Sicherung neben den Datenordner, prüft das
// Passwort am enthaltenen Store und fragt dann nach. Erst der Neustart
// tauscht die Ordner (fullbackup.ApplyPending in main).
func (a *App) prepareRestore(file string, pw []byte) (tea.Cmd, error) {
	dir := a.st.Dir
	check := func(vp string) error {
		v, err := vault.Open(vp)
		if err != nil {
			return err
		}
		return v.CheckPassword(pw)
	}
	stage, m, err := fullbackup.Prepare(dir, file, pw, check)
	switch {
	case errors.Is(err, fullbackup.ErrWrongPassword), errors.Is(err, vault.ErrWrongPassword):
		return nil, errors.New(i18n.T("Passwort stimmt nicht oder die Datei ist beschädigt"))
	case errors.Is(err, fullbackup.ErrNotBackup):
		return nil, errors.New(i18n.T("Keine sshTermy-Sicherung"))
	case err != nil:
		return nil, err
	}
	when := m.Created.Local().Format("02.01.2006 15:04")
	body := fmt.Sprintf(i18n.T("Sicherung vom %s (v%s, %s, %d Dateien).\n\nAlle Daten hier werden durch die Sicherung ersetzt. Der bisherige Stand bleibt im Ordner\n%s.vorher-…\n\nsshTermy startet dafür neu, offene Sitzungen werden getrennt."), when, m.Version, m.OS, m.Files, dir)
	a.modal = confirmModal(i18n.T("Gesamtsicherung einspielen"), body, func() tea.Cmd {
		if err := fullbackup.MarkPending(dir, stage); err != nil {
			os.RemoveAll(stage)
			a.flashErr(err.Error())
			return nil
		}
		a.auditAdd(audit.EvBackup, "", "Gesamtsicherung eingespielt: "+file)
		_ = debuglog.Set(false) // Windows: offene Datei verhindert das Umbenennen
		return a.doRestart()
	})
	return nil, nil
}

// StartNotice: Meldung nach dem Entsperren zeigen (z. B. Gesamtsicherung
// beim Start eingespielt, cmd/sshtermy).
func (a *App) StartNotice(ok bool, s string) { a.startNotice, a.startNoticeOK = s, ok }

func (a *App) showStartNotice() {
	if a.startNotice == "" {
		return
	}
	if a.startNoticeOK {
		a.flashOK(a.startNotice)
	} else {
		a.flashErr(a.startNotice)
	}
	a.startNotice = ""
}
