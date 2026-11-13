package sshx

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Dateiaktionen mit sudo, wenn SFTP „permission denied“ meldet: SFTP läuft
// mit den Rechten des Anmeldebenutzers. Hochladen geht deshalb erst in einen
// eigenen Ordner unter /tmp (mktemp, 0700), von dort kopiert „sudo cp“ ans
// Ziel. Herunterladen umgekehrt: sudo kopiert in den eigenen Ordner und
// übergibt die Kopie dem Benutzer ($SUDO_UID), SFTP holt sie dort ab.
// Löschen, Umbenennen, Ordner anlegen und Rechte ändern laufen direkt per
// sudo. Das Passwort geht über stdin (sudo -S), nie in die Befehlszeile.

// Runner führt einen Befehl auf dem Server aus, stdin = in.
type Runner interface {
	ExecInput(ctx context.Context, cmd string, in []byte) (RunResult, error)
}

// ExecInput wie Exec, aber mit Eingabe auf stdin (für sudo -S).
func (c *Client) ExecInput(ctx context.Context, cmd string, in []byte) (RunResult, error) {
	s, err := c.NewSession()
	if err != nil {
		return RunResult{Exit: -1}, err
	}
	defer s.Close()
	var buf cappedBuf
	s.Stdout, s.Stderr = &buf, &buf
	s.Stdin = strings.NewReader(string(in))
	if err := s.Start(cmd); err != nil {
		return RunResult{Exit: -1}, err
	}
	done := make(chan error, 1)
	go func() { done <- s.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = s.Signal(ssh.SIGTERM)
		s.Close()
		out, _ := buf.snapshot()
		return RunResult{Output: out, Exit: -1}, ctx.Err()
	}
	out, _ := buf.snapshot()
	if err == nil {
		return RunResult{Output: out}, nil
	}
	var ee *ssh.ExitError
	if errors.As(err, &ee) {
		return RunResult{Output: out, Exit: ee.ExitStatus()}, nil
	}
	return RunResult{Output: out, Exit: -1}, err
}

var (
	// ErrSudoPassword: sudo hat das Passwort abgelehnt.
	ErrSudoPassword = i18n.NewError("sudo hat das Passwort nicht angenommen")
	// ErrNoSudo: kein sudo auf dem Server oder der Benutzer darf es nicht.
	ErrNoSudo = i18n.NewError("sudo geht hier nicht (nicht installiert oder für diesen Benutzer nicht erlaubt)")
)

// RemoteDenied: Fehler kommt vom Server (SFTP) und heißt „keine Berechtigung“.
// Nur dann hilft sudo; lokale Rechtefehler bleiben, wie sie sind.
func RemoteDenied(err error) bool {
	var re *RemoteError
	if !errors.As(err, &re) {
		return false
	}
	// Manche Server melden beim Umbenennen nur SSH_FX_FAILURE mit dem Text.
	return errors.Is(err, fs.ErrPermission) || strings.Contains(strings.ToLower(err.Error()), "permission denied")
}

// RemoteError markiert Fehler des SFTP-Servers (Text bleibt unverändert).
type RemoteError struct{ Err error }

func (e *RemoteError) Error() string { return e.Err.Error() }
func (e *RemoteError) Unwrap() error { return e.Err }

func remoteErr(err error) error {
	if err == nil {
		return nil
	}
	return &RemoteError{err}
}

// Sudo: geprüfter Zugang zu sudo auf einem Server.
type Sudo struct {
	r      Runner
	pw     string
	withPw bool
}

// SudoNeedsPassword prüft, ob sudo ohne Passwort geht. ErrNoSudo, wenn es
// sudo nicht gibt oder der Benutzer nicht in der sudoers-Liste steht.
func SudoNeedsPassword(ctx context.Context, r Runner) (bool, error) {
	res, err := r.ExecInput(ctx, "LC_ALL=C sudo -n true", nil)
	if err != nil {
		return false, err
	}
	if res.Exit == 0 {
		return false, nil
	}
	out := strings.ToLower(string(res.Output))
	switch {
	case res.Exit == 127, strings.Contains(out, "not found"), strings.Contains(out, "not in the sudoers"),
		strings.Contains(out, "not allowed"), strings.Contains(out, "may not run sudo"):
		return false, sudoErr(res, ErrNoSudo)
	}
	return true, nil
}

// NoPasswordSudo: sudo ohne Passwort (NOPASSWD), nach SudoNeedsPassword.
func NoPasswordSudo(r Runner) *Sudo { return &Sudo{r: r} }

// NewSudo prüft das Passwort mit „sudo -S true“. ErrSudoPassword, wenn es
// nicht stimmt.
func NewSudo(ctx context.Context, r Runner, pw string) (*Sudo, error) {
	s := &Sudo{r: r, pw: pw, withPw: true}
	res, err := r.ExecInput(ctx, "LC_ALL=C sudo -S -p '' true", []byte(pw+"\n"))
	if err != nil {
		return nil, err
	}
	if res.Exit == 0 {
		return s, nil
	}
	out := strings.ToLower(string(res.Output))
	if strings.Contains(out, "not in the sudoers") || strings.Contains(out, "not allowed") || res.Exit == 127 {
		return nil, sudoErr(res, ErrNoSudo)
	}
	return nil, ErrSudoPassword
}

// Password: das geprüfte Passwort ("" bei NOPASSWD).
func (s *Sudo) Password() string { return s.pw }

func sudoErr(res RunResult, base error) error {
	msg := strings.TrimSpace(string(res.Output))
	if msg == "" {
		return base
	}
	if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
		msg = msg[i+1:]
	}
	return fmt.Errorf("%w: %s", base, msg)
}

// Run führt ein sh-Skript als root aus. Fehler enthält die letzte
// Ausgabezeile (z. B. „cp: cannot create …“).
func (s *Sudo) Run(ctx context.Context, script string) error {
	cmd := "LC_ALL=C sudo -n sh -c " + Quote(script)
	var in []byte
	if s.withPw {
		cmd = "LC_ALL=C sudo -S -p '' sh -c " + Quote(script)
		in = []byte(s.pw + "\n")
	}
	res, err := s.r.ExecInput(ctx, cmd, in)
	if err != nil {
		return err
	}
	if res.Exit != 0 {
		msg := strings.TrimSpace(string(res.Output))
		if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
			msg = msg[i+1:]
		}
		if msg == "" {
			msg = fmt.Sprintf("exit %d", res.Exit)
		}
		return errors.New("sudo: " + msg)
	}
	return nil
}

// Quote: ein Wort für sh in einfachen Anführungszeichen.
func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func quoteAll(ps []string) string {
	q := make([]string, len(ps))
	for i, p := range ps {
		q[i] = Quote(p)
	}
	return strings.Join(q, " ")
}

// Remove löscht Pfade (Ordner mit allem darin).
func (s *Sudo) Remove(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	return s.Run(ctx, "rm -rf -- "+quoteAll(paths))
}

// Rename benennt um und überschreibt wie RemoteFS.Rename nie ein Ziel.
func (s *Sudo) Rename(ctx context.Context, from, to string) error {
	err := s.Run(ctx, "if [ -e "+Quote(to)+" ] || [ -L "+Quote(to)+" ]; then echo exists; exit 3; fi; mv -- "+Quote(from)+" "+Quote(to))
	if err != nil && strings.HasSuffix(err.Error(), "exists") {
		return fmt.Errorf(i18n.T("„%s“ gibt es schon"), path.Base(to))
	}
	return err
}

// Mkdir legt einen Ordner an (mit allen fehlenden darüber).
func (s *Sudo) Mkdir(ctx context.Context, p string) error {
	return s.Run(ctx, "mkdir -p -- "+Quote(p))
}

// Chmod setzt Rechte (mit setuid/setgid/sticky).
func (s *Sudo) Chmod(ctx context.Context, p string, mode fs.FileMode) error {
	return s.Run(ctx, fmt.Sprintf("chmod %04o -- %s", octalMode(mode), Quote(p)))
}

func octalMode(m fs.FileMode) uint32 {
	n := uint32(m.Perm())
	if m&fs.ModeSetuid != 0 {
		n |= 0o4000
	}
	if m&fs.ModeSetgid != 0 {
		n |= 0o2000
	}
	if m&fs.ModeSticky != 0 {
		n |= 0o1000
	}
	return n
}

// stage: eigener Ordner unter /tmp (0700, gehört dem Anmeldebenutzer).
// Das Aufräumen läuft auch nach Abbruch, mit eigener kurzer Frist.
func stage(ctx context.Context, r Runner) (string, func(), error) {
	res, err := r.ExecInput(ctx, "mktemp -d \"${TMPDIR:-/tmp}/sshtermy-sudo.XXXXXX\"", nil)
	if err != nil {
		return "", nil, err
	}
	dir := strings.TrimSpace(string(res.Output))
	if res.Exit != 0 || !strings.HasPrefix(dir, "/") || strings.ContainsAny(dir, "\n'") {
		return "", nil, fmt.Errorf(i18n.T("Zwischenordner auf dem Server ging nicht: %s"), dir)
	}
	return dir, func() {
		c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = r.ExecInput(c, "rm -rf -- "+Quote(dir), nil)
	}, nil
}

// Upload lädt paths von src nach dstDir auf dem Server hoch: erst per SFTP
// in einen eigenen Zwischenordner, dann mit sudo cp ans Ziel. Vorhandene
// Dateien behalten so Besitzer und Rechte, neue gehören root.
func (s *Sudo) Upload(ctx context.Context, src FS, paths []string, dst FS, dstDir string, pr *Progress) error {
	tmp, clean, err := stage(ctx, s.r)
	if err != nil {
		return err
	}
	defer clean()
	var total int64
	for _, p := range paths {
		n, _, err := Size(ctx, src, p)
		if err != nil {
			return err
		}
		total += n
	}
	pr.Total.Store(total)
	names := make([]string, 0, len(paths))
	for _, p := range paths {
		if err := Copy(ctx, src, p, dst, tmp, pr); err != nil {
			return err
		}
		names = append(names, path.Join(tmp, baseOf(src, p)))
	}
	pr.current.Store("sudo cp → " + dstDir)
	return s.Run(ctx, "mkdir -p -- "+Quote(dstDir)+" && cp -R -- "+quoteAll(names)+" "+Quote(strings.TrimSuffix(dstDir, "/")+"/"))
}

// Download holt paths vom Server nach dstDir auf dst: sudo kopiert sie in
// den Zwischenordner und übergibt sie dem Anmeldebenutzer, SFTP lädt sie
// von dort herunter.
func (s *Sudo) Download(ctx context.Context, src FS, paths []string, dst FS, dstDir string, pr *Progress) error {
	tmp, clean, err := stage(ctx, s.r)
	if err != nil {
		return err
	}
	defer clean()
	names := make([]string, 0, len(paths))
	for _, p := range paths {
		names = append(names, path.Join(tmp, path.Base(p)))
	}
	pr.current.Store("sudo cp ← " + strings.Join(paths, ", "))
	if err := s.Run(ctx, "cp -RH -- "+quoteAll(paths)+" "+Quote(tmp+"/")+
		" && chown -R \"${SUDO_UID:-$(id -u)}:${SUDO_GID:-$(id -g)}\" -- "+quoteAll(names)); err != nil {
		return err
	}
	var total int64
	for _, n := range names {
		b, _, err := Size(ctx, src, n)
		if err != nil {
			return err
		}
		total += b
	}
	pr.Total.Store(total)
	for _, n := range names {
		if err := Copy(ctx, src, n, dst, dstDir, pr); err != nil {
			return err
		}
	}
	return nil
}

// baseOf: Name wie in Copy (lokal mit \ oder /, Server nur /).
func baseOf(f FS, p string) string {
	if f.Label() == "Lokal" {
		return filepath.Base(filepath.FromSlash(p))
	}
	return path.Base(p)
}

// WriteFile überschreibt dst mit data (Editor speichert). Erst in den
// Zwischenordner, dann „cp“ auf die vorhandene Datei: Besitzer und Rechte
// bleiben wie beim Speichern ohne sudo.
func (s *Sudo) WriteFile(ctx context.Context, fsys FS, dst string, data []byte) error {
	tmp, clean, err := stage(ctx, s.r)
	if err != nil {
		return err
	}
	defer clean()
	p := path.Join(tmp, "neu")
	w, err := fsys.Create(p, 0o600)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return s.Run(ctx, "cp -- "+Quote(p)+" "+Quote(dst))
}
