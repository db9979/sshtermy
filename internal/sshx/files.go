package sshx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Dateiübertragung per SFTP über dieselbe SSH-Verbindung wie die Shell
// (eigener Kanal, kein zweiter Login). Lokale und entfernte Seite haben
// dasselbe kleine Interface, damit Kopieren in beide Richtungen ein Code ist.

// Entry ist ein Verzeichniseintrag.
type Entry struct {
	Name    string
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
	IsDir   bool
	Link    bool
}

// FS: lokale Platte oder SFTP.
type FS interface {
	Label() string
	ReadDir(p string) ([]Entry, error)
	Stat(p string) (Entry, error)
	Open(p string) (io.ReadCloser, error)
	Create(p string, mode fs.FileMode) (io.WriteCloser, error)
	Mkdir(p string) error
	RemoveAll(p string) error
	Join(elem ...string) string
	Dir(p string) string
	Home() string
}

func toEntry(fi fs.FileInfo) Entry {
	return Entry{Name: fi.Name(), Size: fi.Size(), Mode: fi.Mode(), ModTime: fi.ModTime(),
		IsDir: fi.IsDir(), Link: fi.Mode()&fs.ModeSymlink != 0}
}

func sortEntries(es []Entry) {
	sort.Slice(es, func(i, j int) bool {
		if es[i].IsDir != es[j].IsDir {
			return es[i].IsDir
		}
		return strings.ToLower(es[i].Name) < strings.ToLower(es[j].Name)
	})
}

// ---------------------------------------------------------------- lokal

type LocalFS struct{}

func (LocalFS) Label() string { return "Lokal" }
func (LocalFS) ReadDir(p string) ([]Entry, error) {
	des, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(des))
	for _, d := range des {
		fi, err := os.Stat(filepath.Join(p, d.Name())) // Symlinks auflösen
		if err != nil {
			if fi, err = d.Info(); err != nil {
				continue
			}
		}
		e := toEntry(fi)
		e.Name = d.Name()
		e.Link = d.Type()&fs.ModeSymlink != 0
		out = append(out, e)
	}
	sortEntries(out)
	return out, nil
}
func (LocalFS) Stat(p string) (Entry, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return Entry{}, err
	}
	return toEntry(fi), nil
}
func (LocalFS) Open(p string) (io.ReadCloser, error) { return os.Open(p) }
func (LocalFS) Create(p string, mode fs.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm()|0o200)
}
func (LocalFS) Mkdir(p string) error       { return os.MkdirAll(p, 0o755) }
func (LocalFS) RemoveAll(p string) error   { return os.RemoveAll(p) }
func (LocalFS) Join(elem ...string) string { return filepath.Join(elem...) }
func (LocalFS) Dir(p string) string        { return filepath.Dir(p) }
func (LocalFS) Home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	wd, _ := os.Getwd()
	return wd
}

// ---------------------------------------------------------------- SFTP

type RemoteFS struct {
	c    *sftp.Client
	home string
	name string
}

// OpenSFTP öffnet das sftp-Subsystem auf der bestehenden Verbindung.
func (c *Client) OpenSFTP() (*RemoteFS, error) {
	sc, err := sftp.NewClient(c.Client, sftp.UseConcurrentWrites(true), sftp.UseConcurrentReads(true))
	if err != nil {
		return nil, fmt.Errorf(i18n.T("SFTP nicht verfügbar (Server ohne sftp-Subsystem?): %w"), err)
	}
	home, err := sc.Getwd()
	if err != nil || home == "" {
		home = "/"
	}
	return &RemoteFS{c: sc, home: home, name: c.Conn.Alias}, nil
}

func (r *RemoteFS) Close() error  { return r.c.Close() }
func (r *RemoteFS) Label() string { return r.name }
func (r *RemoteFS) ReadDir(p string) ([]Entry, error) {
	fis, err := r.c.ReadDir(p)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(fis))
	for _, fi := range fis {
		e := toEntry(fi)
		if e.Link { // Ziel des Links: Ordner oder Datei?
			if t, err := r.c.Stat(path.Join(p, fi.Name())); err == nil {
				e.IsDir, e.Size = t.IsDir(), t.Size()
			}
		}
		out = append(out, e)
	}
	sortEntries(out)
	return out, nil
}
func (r *RemoteFS) Stat(p string) (Entry, error) {
	fi, err := r.c.Stat(p)
	if err != nil {
		return Entry{}, err
	}
	return toEntry(fi), nil
}
func (r *RemoteFS) Open(p string) (io.ReadCloser, error) { return r.c.Open(p) }
func (r *RemoteFS) Create(p string, mode fs.FileMode) (io.WriteCloser, error) {
	f, err := r.c.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err == nil {
		_ = f.Chmod(mode.Perm() | 0o200)
	}
	return f, err
}
// Rewrite überschreibt eine vorhandene Datei, ohne Rechte oder Besitzer
// anzufassen (Bearbeiten im eigenen Editor).
func (r *RemoteFS) Rewrite(p string, data []byte) error {
	f, err := r.c.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func (r *RemoteFS) Mkdir(p string) error       { return r.c.MkdirAll(p) }
func (r *RemoteFS) RemoveAll(p string) error   { return r.c.RemoveAll(p) }
func (r *RemoteFS) Join(elem ...string) string { return path.Join(elem...) }
func (r *RemoteFS) Dir(p string) string        { return path.Dir(p) }
func (r *RemoteFS) Home() string               { return r.home }

// ---------------------------------------------------------------- Kopieren

// Progress wird während einer Übertragung laufend aktualisiert (atomar,
// die Oberfläche liest es nur).
type Progress struct {
	Done, Total atomic.Int64
	Files       atomic.Int64
	current     atomic.Value // string
}

func (p *Progress) Current() string { s, _ := p.current.Load().(string); return s }

// Size zählt Bytes eines Pfads (rekursiv), für die Fortschrittsanzeige.
func Size(ctx context.Context, f FS, p string) (int64, int, error) {
	e, err := f.Stat(p)
	if err != nil {
		return 0, 0, err
	}
	if !e.IsDir {
		return e.Size, 1, nil
	}
	es, err := f.ReadDir(p)
	if err != nil {
		return 0, 0, err
	}
	var total int64
	n := 0
	for _, c := range es {
		if ctx.Err() != nil {
			return 0, 0, ctx.Err()
		}
		if c.Link && c.IsDir {
			continue // keine Link-Schleifen
		}
		s, k, err := Size(ctx, f, f.Join(p, c.Name))
		if err != nil {
			return 0, 0, err
		}
		total, n = total+s, n+k
	}
	return total, n, nil
}

// Copy kopiert src (Datei oder Ordner) nach dstDir/<name>.
func Copy(ctx context.Context, src FS, srcPath string, dst FS, dstDir string, pr *Progress) error {
	name := filepath.Base(filepath.FromSlash(srcPath))
	if src.Label() != "Lokal" {
		name = path.Base(srcPath)
	}
	if name == "" || name == "." || name == "/" || name == ".." {
		return errors.New(i18n.T("ungültiger Name"))
	}
	return copyAny(ctx, src, srcPath, dst, dst.Join(dstDir, name), pr)
}

func copyAny(ctx context.Context, src FS, sp string, dst FS, dp string, pr *Progress) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	e, err := src.Stat(sp)
	if err != nil {
		return err
	}
	if !e.IsDir {
		return copyFile(ctx, src, sp, dst, dp, e.Mode, pr)
	}
	if err := dst.Mkdir(dp); err != nil {
		return err
	}
	es, err := src.ReadDir(sp)
	if err != nil {
		return err
	}
	for _, c := range es {
		if c.Link && c.IsDir {
			continue
		}
		if err := copyAny(ctx, src, src.Join(sp, c.Name), dst, dst.Join(dp, c.Name), pr); err != nil {
			return err
		}
	}
	return nil
}

type ctxWriter struct {
	ctx context.Context
	w   io.Writer
	pr  *Progress
}

func (c ctxWriter) Write(b []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := c.w.Write(b)
	c.pr.Done.Add(int64(n))
	return n, err
}

func copyFile(ctx context.Context, src FS, sp string, dst FS, dp string, mode fs.FileMode, pr *Progress) error {
	pr.current.Store(sp)
	in, err := src.Open(sp)
	if err != nil {
		return err
	}
	defer in.Close()
	// erst in eine .part-Datei, dann umbenennen wäre schöner; SFTP-Rename
	// über bestehende Dateien ist aber nicht überall erlaubt. Deshalb direkt,
	// und bei Fehler/Abbruch die halbe Datei wieder löschen.
	out, err := dst.Create(dp, mode)
	if err != nil {
		return err
	}
	buf := make([]byte, 256*1024)
	_, err = io.CopyBuffer(ctxWriter{ctx, out, pr}, in, buf)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = dst.RemoveAll(dp)
		return err
	}
	pr.Files.Add(1)
	return nil
}
