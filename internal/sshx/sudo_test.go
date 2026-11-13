package sshx

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// localRunner führt Befehle lokal mit sh aus, ein falsches sudo liegt vorn
// im PATH: es prüft -S-Passwort bzw. -n und führt den Rest aus.
type localRunner struct{ path string }

func (l localRunner) ExecInput(ctx context.Context, cmd string, in []byte) (RunResult, error) {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.Env = append(os.Environ(), "PATH="+l.path)
	c.Stdin = bytes.NewReader(in)
	out, err := c.CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return RunResult{Output: out, Exit: ee.ExitCode()}, nil
	}
	return RunResult{Output: out}, err
}

const fakeSudo = `#!/bin/sh
pw=
while [ $# -gt 0 ]; do
  case "$1" in
    -n) if [ -n "$FAKE_PW" ]; then echo "sudo: a password is required" >&2; exit 1; fi; shift;;
    -S) IFS= read -r pw; if [ "$pw" != "$FAKE_PW" ]; then echo "Sorry, try again." >&2; echo "sudo: 1 incorrect password attempt" >&2; exit 1; fi; shift;;
    -p) shift 2;;
    *) break;;
  esac
done
SUDO_UID=$(id -u) SUDO_GID=$(id -g) exec "$@"
`

func newFakeSudo(t *testing.T, pw string) Runner {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("braucht sh")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "sudo"), []byte(fakeSudo), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_PW", pw)
	t.Setenv("TMPDIR", t.TempDir())
	return localRunner{bin + string(os.PathListSeparator) + os.Getenv("PATH")}
}

func TestSudoPassword(t *testing.T) {
	r := newFakeSudo(t, "geheim 'x'")
	ctx := context.Background()
	need, err := SudoNeedsPassword(ctx, r)
	if err != nil || !need {
		t.Fatalf("need=%v err=%v", need, err)
	}
	if _, err := NewSudo(ctx, r, "falsch"); !errors.Is(err, ErrSudoPassword) {
		t.Fatalf("falsches Passwort: %v", err)
	}
	s, err := NewSudo(ctx, r, "geheim 'x'")
	if err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	if err := s.Mkdir(ctx, filepath.Join(d, "a b", "c")); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename(ctx, filepath.Join(d, "a b"), filepath.Join(d, "neu")); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(d, "x"), []byte("1"), 0o644)
	if err := s.Rename(ctx, filepath.Join(d, "x"), filepath.Join(d, "neu")); err == nil || !strings.Contains(err.Error(), "neu") {
		t.Fatalf("Umbenennen auf vorhandenes Ziel: %v", err)
	}
	if err := s.Chmod(ctx, filepath.Join(d, "x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(d, "x")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("Rechte %o", fi.Mode().Perm())
	}
	if err := s.Remove(ctx, filepath.Join(d, "neu"), filepath.Join(d, "x")); err != nil {
		t.Fatal(err)
	}
	if es, _ := os.ReadDir(d); len(es) != 0 {
		t.Fatalf("nicht gelöscht: %v", es)
	}
	if err := s.Run(ctx, "exit 4"); err == nil {
		t.Fatal("Fehler erwartet")
	}
}

func TestSudoNoPassword(t *testing.T) {
	r := newFakeSudo(t, "")
	need, err := SudoNeedsPassword(context.Background(), r)
	if err != nil || need {
		t.Fatalf("need=%v err=%v", need, err)
	}
	d := t.TempDir()
	if err := NoPasswordSudo(r).Mkdir(context.Background(), filepath.Join(d, "z")); err != nil {
		t.Fatal(err)
	}
}

func TestNoSudo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("braucht sh")
	}
	r := localRunner{t.TempDir()} // PATH ohne sudo
	if _, err := SudoNeedsPassword(context.Background(), r); !errors.Is(err, ErrNoSudo) {
		t.Fatalf("err=%v", err)
	}
}

func TestSudoUploadDownload(t *testing.T) {
	r := newFakeSudo(t, "pw")
	ctx := context.Background()
	s, err := NewSudo(ctx, r, "pw")
	if err != nil {
		t.Fatal(err)
	}
	src, dst := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(src, "a.py"), []byte("print(1)"), 0o755)
	os.MkdirAll(filepath.Join(src, "ordner", "unter"), 0o755)
	os.WriteFile(filepath.Join(src, "ordner", "unter", "b"), []byte("bb"), 0o644)
	os.WriteFile(filepath.Join(dst, "a.py"), []byte("alt"), 0o700) // wird überschrieben, Rechte bleiben
	pr := &Progress{}
	err = s.Upload(ctx, LocalFS{}, []string{filepath.Join(src, "a.py"), filepath.Join(src, "ordner")}, LocalFS{}, filepath.Join(dst, "neu"), pr)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "neu", "ordner", "unter", "b")); string(b) != "bb" {
		t.Fatalf("Ordner nicht hochgeladen: %q", b)
	}
	if pr.Done.Load() != pr.Total.Load() || pr.Total.Load() != 10 {
		t.Fatalf("Fortschritt %d/%d", pr.Done.Load(), pr.Total.Load())
	}
	if err := s.Upload(ctx, LocalFS{}, []string{filepath.Join(src, "a.py")}, LocalFS{}, dst, &Progress{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "a.py")); string(b) != "print(1)" {
		t.Fatalf("nicht überschrieben: %q", b)
	}
	if fi, _ := os.Stat(filepath.Join(dst, "a.py")); fi.Mode().Perm() != 0o700 {
		t.Fatalf("Rechte des Ziels geändert: %o", fi.Mode().Perm())
	}
	back := t.TempDir()
	if err := s.Download(ctx, LocalFS{}, []string{filepath.Join(dst, "neu", "ordner")}, LocalFS{}, back, &Progress{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(back, "ordner", "unter", "b")); string(b) != "bb" {
		t.Fatalf("nicht heruntergeladen: %q", b)
	}
	tmp, _ := os.ReadDir(os.Getenv("TMPDIR"))
	if len(tmp) != 0 {
		t.Fatalf("Zwischenordner nicht aufgeräumt: %v", tmp)
	}
}

func TestRemoteDenied(t *testing.T) {
	pe := &os.PathError{Op: "open", Path: "/opt/x", Err: os.ErrPermission}
	if RemoteDenied(pe) {
		t.Fatal("lokaler Fehler gilt nicht")
	}
	if !RemoteDenied(remoteErr(pe)) || RemoteDenied(remoteErr(os.ErrNotExist)) {
		t.Fatal("RemoteDenied")
	}
	if remoteErr(pe).Error() != pe.Error() {
		t.Fatal("Text muss gleich bleiben")
	}
}

func TestSudoWriteFile(t *testing.T) {
	r := newFakeSudo(t, "")
	d := t.TempDir()
	f := filepath.Join(d, "conf")
	os.WriteFile(f, []byte("alt"), 0o640)
	if err := NoPasswordSudo(r).WriteFile(context.Background(), LocalFS{}, f, []byte("neu")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f)
	fi, _ := os.Stat(f)
	if string(b) != "neu" || fi.Mode().Perm() != 0o640 {
		t.Fatalf("%q %o", b, fi.Mode().Perm())
	}
}
