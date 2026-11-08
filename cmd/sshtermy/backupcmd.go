package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/db9979/sshtermy/internal/fullbackup"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/store"
	"github.com/db9979/sshtermy/internal/vault"
	"github.com/db9979/sshtermy/internal/version"
)

// sshtermy backup [datei]   → Gesamtsicherung des Datenordners
// sshtermy restore <datei>  → beim nächsten Start einspielen
// Beide fragen das Master-Passwort ab (kein Schalter, nichts in der Shell-History).

func readPass(prompt string) ([]byte, error) {
	fmt.Fprint(os.Stderr, prompt)
	pw, err := term.ReadPassword(os.Stdin.Fd())
	fmt.Fprintln(os.Stderr)
	return pw, err
}

func runBackup(args []string) int {
	dir, err := store.Dir()
	if err != nil {
		return cliErr(err)
	}
	file := "sshtermy-sicherung-" + time.Now().Format("2006-01-02") + fullbackup.Ext
	if len(args) > 0 {
		file = args[0]
	}
	v, err := vault.Open(filepath.Join(dir, "store.vault"))
	if err != nil || !v.Exists() {
		return cliErr(errors.New(i18n.T("Kein Store in ") + dir))
	}
	pw, err := readPass(i18n.T("Master-Passwort: "))
	if err != nil {
		return cliErr(err)
	}
	defer vault.Wipe(pw)
	if err := v.CheckPassword(pw); err != nil {
		return cliErr(errors.New(i18n.T("Master-Passwort stimmt nicht")))
	}
	tmp := file + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return cliErr(err)
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
		return cliErr(err)
	}
	fmt.Printf(i18n.T("Gesamtsicherung gespeichert: %s (%d Dateien)")+"\n", file, m.Files)
	return 0
}

func runRestore(args []string) int {
	if len(args) != 1 {
		return cliErr(errors.New(i18n.T("Aufruf: sshtermy restore <datei>")))
	}
	dir, err := store.Dir()
	if err != nil {
		return cliErr(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return cliErr(err)
	}
	pw, err := readPass(i18n.T("Master-Passwort der Sicherung: "))
	if err != nil {
		return cliErr(err)
	}
	defer vault.Wipe(pw)
	check := func(vp string) error {
		v, err := vault.Open(vp)
		if err != nil {
			return err
		}
		return v.CheckPassword(pw)
	}
	stage, m, err := fullbackup.Prepare(dir, args[0], pw, check)
	switch {
	case errors.Is(err, fullbackup.ErrWrongPassword), errors.Is(err, vault.ErrWrongPassword):
		return cliErr(errors.New(i18n.T("Passwort stimmt nicht oder die Datei ist beschädigt")))
	case errors.Is(err, fullbackup.ErrNotBackup):
		return cliErr(errors.New(i18n.T("Keine sshTermy-Sicherung")))
	case err != nil:
		return cliErr(err)
	}
	fmt.Printf(i18n.T("Sicherung vom %s (v%s, %s, %d Dateien).")+"\n", m.Created.Local().Format("02.01.2006 15:04"), m.Version, m.OS, m.Files)
	fmt.Printf(i18n.T("Alle Daten in %s ersetzen? [j/N] "), dir)
	var ans string
	fmt.Scanln(&ans)
	if a := strings.ToLower(strings.TrimSpace(ans)); a != "j" && a != "y" && a != "ja" && a != "yes" {
		os.RemoveAll(stage)
		fmt.Println(i18n.T("Abgebrochen."))
		return 1
	}
	if err := fullbackup.MarkPending(dir, stage); err != nil {
		os.RemoveAll(stage)
		return cliErr(err)
	}
	old, err := fullbackup.ApplyPending(dir)
	if err != nil {
		return cliErr(err)
	}
	fmt.Printf(i18n.T("Eingespielt. Bisheriger Stand: %s")+"\n", old)
	return 0
}

func cliErr(err error) int {
	fmt.Fprintln(os.Stderr, "sshtermy:", err)
	return 1
}
