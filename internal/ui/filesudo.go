package ui

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/audit"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Dateien (F5) mit sudo: Meldet der Server beim Hochladen, Herunterladen,
// Löschen, Umbenennen, Ordner anlegen oder Rechte ändern „permission
// denied“, fragt sshTermy, ob es die Aktion mit sudo wiederholen soll. Weil
// jede Verbindung als produktiv gilt (Schutz an), muss j dafür zweimal
// gedrückt werden. Das sudo-Passwort kommt in dieser Reihenfolge aus:
// geprüftem Passwort dieser Sitzung, dem Anmeldepasswort (Store,
// Schlüsselbund, eben eingetippt), dem gemerkten sudo-Passwort im
// Schlüsselbund – sonst fragt ein verdecktes Eingabefeld. Ein getipptes
// Passwort, das stimmt, landet im Schlüsselbund, wenn die Verbindung ihn nutzt.

// sudoReadyMsg: sudo ist geprüft, für weitere Aktionen in diesem Tab merken.
type sudoReadyMsg struct {
	sid int
	c   *sshx.Client
	sx  *sshx.Sudo
}

// sudoDoneMsg: Aktion mit sudo ist fertig.
type sudoDoneMsg struct {
	sid   int
	err   error
	after func(error) tea.Cmd
}

// sudoPossible: Aktion auf dem Server über SSH (nicht lokal, nicht Telnet).
func sudoPossible(s *session, fsys sshx.FS) bool {
	_, remote := fsys.(*sshx.RemoteFS)
	return remote && s.client != nil
}

// offerSudo fragt nach einem Rechtefehler, ob what mit sudo laufen soll.
// Ohne Rechtefehler vom Server oder ohne SSH zeigt es nur den Fehler.
func (a *App) offerSudo(s *session, fsys sshx.FS, what string, err error, run func() tea.Cmd) bool {
	if !sshx.RemoteDenied(err) || !sudoPossible(s, fsys) {
		return false
	}
	body := fmt.Sprintf(i18n.T("%s\n\n%s meldet: keine Berechtigung (%s).\n\nMit sudo (als root) wiederholen?"), what, s.conn.Alias, errShort(err))
	m := confirmModal(i18n.T("Keine Berechtigung – mit sudo?"), body, run)
	if isProduction(s.conn) {
		m.twice = true
		m.body += "\n" + i18n.T("Schutz an: j zweimal drücken.")
	}
	a.showAsync(m) // auch wenn gerade ein anderer Dialog offen ist (Editor speichert im Hintergrund)
	return true
}

// errShort: Fehlertext ohne Pfad davor („open /opt/x: permission denied“ →
// „permission denied“).
func errShort(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

// sudoAuth liefert eine Funktion, die im Hintergrund sudo prüft (und dafür
// bei Bedarf nach dem Passwort fragt).
func (a *App) sudoAuth(s *session) func(context.Context) (*sshx.Sudo, error) {
	c, conn, ev, sid := s.client, s.conn, a.events, s.id
	if s.sudo != nil && s.sudoFor == c {
		sx := s.sudo
		return func(context.Context) (*sshx.Sudo, error) { return sx, nil }
	}
	var cands []string
	if conn.Password.IsSet() {
		cands = append(cands, conn.Password.Reveal())
	}
	if s.secrets != nil {
		if v, ok := s.secrets.get(sshx.PasswordLabel(conn)); ok {
			cands = append(cands, v)
		}
	}
	return func(ctx context.Context) (*sshx.Sudo, error) {
		need, err := sshx.SudoNeedsPassword(ctx, c)
		if err != nil {
			return nil, err
		}
		ready := func(sx *sshx.Sudo) (*sshx.Sudo, error) {
			ev <- sudoReadyMsg{sid, c, sx}
			return sx, nil
		}
		if !need {
			return ready(sshx.NoPasswordSudo(c))
		}
		if conn.Keychain {
			cands = append(cands, sshx.KeychainSecret(conn.ID, sshx.SecretSudo), sshx.KeychainSecret(conn.ID, sshx.SecretPassword))
		}
		tried := map[string]bool{}
		for _, pw := range cands {
			if pw == "" || tried[pw] {
				continue
			}
			tried[pw] = true
			sx, err := sshx.NewSudo(ctx, c, pw)
			if err == nil {
				return ready(sx)
			}
			if !errors.Is(err, sshx.ErrSudoPassword) {
				return nil, err
			}
			if len(tried) >= 2 { // nicht zu viele Fehlversuche ins Protokoll
				break
			}
		}
		label := fmt.Sprintf(i18n.T("sudo-Passwort für %s@%s"), conn.Username, conn.Host)
		for try := 0; try < 3; try++ {
			r := make(chan string, 1)
			ev <- secretAskMsg{label, r}
			pw := <-r
			if pw == "\x00cancel" {
				return nil, context.Canceled
			}
			sx, err := sshx.NewSudo(ctx, c, pw)
			if err == nil {
				if conn.Keychain && !tried[pw] {
					_ = sshx.SetKeychainSecret(conn, sshx.SecretSudo, pw)
				}
				return ready(sx)
			}
			if !errors.Is(err, sshx.ErrSudoPassword) {
				return nil, err
			}
			label = i18n.T("Falsches Passwort. ") + fmt.Sprintf(i18n.T("sudo-Passwort für %s@%s"), conn.Username, conn.Host)
		}
		return nil, sshx.ErrSudoPassword
	}
}

// runSudo führt op mit sudo im Hintergrund aus, after läuft danach im
// Hauptablauf (Liste neu laden, Meldung).
func (a *App) runSudo(s *session, what string, op func(context.Context, *sshx.Sudo) error, after func(error) tea.Cmd) tea.Cmd {
	auth, sid := a.sudoAuth(s), s.id
	if a.audit != nil {
		a.audit.Add(audit.EvCommand, auditConn(s.conn), "sudo (Dateien): "+what)
	}
	a.flash(i18n.T("sudo: ") + what + " …")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		sx, err := auth(ctx)
		if err == nil {
			err = op(ctx, sx)
		}
		return sudoDoneMsg{sid, err, after}
	}
}

// sudoMsg verarbeitet die Nachrichten oben.
func (a *App) sudoMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case sudoReadyMsg:
		if s := a.byID(m.sid); s != nil && s.client == m.c {
			s.sudo, s.sudoFor = m.sx, m.c
		}
		return a.listen(), true
	case sudoDoneMsg:
		if a.byID(m.sid) == nil {
			return nil, true
		}
		if m.err != nil && !errors.Is(m.err, context.Canceled) {
			a.flashErr(i18n.T("sudo: ") + m.err.Error())
		}
		if m.after != nil {
			return m.after(m.err), true
		}
		return nil, true
	}
	return nil, false
}

// startSudoTransfer kopiert wie startTransfer, aber über sudo und einen
// Zwischenordner auf dem Server (sshx.Sudo.Upload/Download).
func (a *App) startSudoTransfer(s *session, srcFS sshx.FS, paths []string, dstFS sshx.FS, dstDir string, up bool) tea.Cmd {
	f := &s.files
	if f.xfer != nil {
		a.flash(i18n.T("Es läuft schon eine Übertragung (x bricht ab)"))
		return nil
	}
	arrow := "↓"
	if up {
		arrow = "↑"
	}
	a.auditTransfer(s, paths, "sudo "+dstDir, up)
	name := baseName(srcFS, paths[0])
	if len(paths) > 1 {
		name = fmt.Sprintf(i18n.T("%d Einträge"), len(paths))
	}
	ctx, cancel := context.WithCancel(context.Background())
	x := &xfer{what: arrow + " " + name + " → " + dstDir + " (sudo)", pr: &sshx.Progress{}, cancel: cancel, start: time.Now()}
	x.files.Store(int64(len(paths)))
	f.xfer, f.last = x, ""
	auth, sid := a.sudoAuth(s), s.id
	run := func() tea.Msg {
		defer cancel()
		sx, err := auth(ctx)
		if err != nil {
			return filesDoneMsg{sid, err}
		}
		if up {
			err = sx.Upload(ctx, srcFS, paths, dstFS, dstDir, x.pr)
		} else {
			err = sx.Download(ctx, srcFS, paths, dstFS, dstDir, x.pr)
		}
		return filesDoneMsg{sid, err}
	}
	return tea.Batch(run, filesTick(sid))
}

// sudoTransferOffer: nach einem Rechtefehler beim Kopieren mit sudo
// wiederholen (nil, wenn sudo hier nicht in Frage kommt).
func (a *App) sudoTransferOffer(s *session, srcFS sshx.FS, paths []string, dstFS sshx.FS, dstDir string, up bool) func() tea.Cmd {
	remote := dstFS
	if !up {
		remote = srcFS
	}
	if !sudoPossible(s, remote) {
		return nil
	}
	return func() tea.Cmd { return a.startSudoTransfer(s, srcFS, paths, dstFS, dstDir, up) }
}

// sudoListNames: Pfade kurz für die Rückfrage.
func sudoListNames(paths []string) string {
	if len(paths) <= 3 {
		return strings.Join(paths, "\n")
	}
	return strings.Join(paths[:3], "\n") + fmt.Sprintf(i18n.T("\n… und %d weitere"), len(paths)-3)
}

// offerEditSudo: Datei aus dem eigenen Editor ließ sich nicht zurück-
// schreiben. Nach Bestätigung mit sudo, weitere Speichern dieser Datei dann
// ohne neue Rückfrage.
func (a *App) offerEditSudo(s *session, e *remoteEdit, err error) bool {
	fsys := s.editFSFor()
	if fsys == nil {
		return false
	}
	return a.offerSudo(s, fsys, i18n.T("Speichern aus dem Editor: ")+e.remote, err, func() tea.Cmd {
		data, rerr := os.ReadFile(e.local)
		if rerr != nil {
			a.flashErr(rerr.Error())
			return nil
		}
		e.busy = true
		return a.editSudoSave(s, e, data, sha256.Sum256(data))
	})
}

// editSudoSave schreibt die Datei mit sudo zurück.
func (a *App) editSudoSave(s *session, e *remoteEdit, data []byte, sum [32]byte) tea.Cmd {
	fsys := s.editFSFor()
	return a.runSudo(s, "cp → "+e.remote, func(ctx context.Context, sx *sshx.Sudo) error {
		return sx.WriteFile(ctx, fsys, e.remote, data)
	}, func(err error) tea.Cmd {
		e.busy = false
		if err != nil {
			return nil
		}
		e.sum, e.sudo = sum, true
		a.flashOK(i18n.T("Gespeichert (sudo) auf ") + e.host + ": " + e.remote)
		return nil
	})
}
