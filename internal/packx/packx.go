// Package packx packt und entpackt Archive auf diesem Rechner (.zip,
// .tar.gz/.tgz, .tar) und listet den Inhalt für die Vorschau. Auf dem
// Server erledigen das tar/unzip per Befehl (Commands), damit nichts erst
// heruntergeladen werden muss.
//
// Entpacken legt immer einen neuen Ordner an und schreibt nie außerhalb
// davon: Namen mit „..“ oder absolute Pfade bricht es ab, Links, die
// hinausführen, lässt es weg.
package packx

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Format eines Archivs (aus der Endung).
type Format int

const (
	None Format = iota
	Zip
	TarGz
	Tar
	TarBz2
	TarXz
	TarZst
)

// FormatOf erkennt das Format an der Endung.
func FormatOf(name string) Format {
	l := strings.ToLower(name)
	switch {
	case strings.HasSuffix(l, ".zip"), strings.HasSuffix(l, ".jar"), strings.HasSuffix(l, ".war"):
		return Zip
	case strings.HasSuffix(l, ".tar.gz"), strings.HasSuffix(l, ".tgz"):
		return TarGz
	case strings.HasSuffix(l, ".tar.bz2"), strings.HasSuffix(l, ".tbz2"), strings.HasSuffix(l, ".tbz"):
		return TarBz2
	case strings.HasSuffix(l, ".tar.xz"), strings.HasSuffix(l, ".txz"):
		return TarXz
	case strings.HasSuffix(l, ".tar.zst"), strings.HasSuffix(l, ".tzst"):
		return TarZst
	case strings.HasSuffix(l, ".tar"):
		return Tar
	}
	return None
}

// IsArchive: Endung eines bekannten Archivs.
func IsArchive(name string) bool { return FormatOf(name) != None }

// Stem: Name ohne Archiv-Endung („logs.tar.gz“ → „logs“).
func Stem(name string) string {
	l := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tar.zst", ".tgz", ".tbz2", ".tbz", ".txz", ".tzst", ".tar", ".zip", ".jar", ".war"} {
		if strings.HasSuffix(l, ext) && len(name) > len(ext) {
			return name[:len(name)-len(ext)]
		}
	}
	return name
}

// ---------------------------------------------------------------- Packen

// Create packt srcs (Dateien oder Ordner) nach dst. Das Format kommt aus
// der Endung von dst (.zip, .tar.gz/.tgz, .tar). Im Archiv stehen die
// Einträge unter ihrem eigenen Namen. dst darf noch nicht existieren.
func Create(ctx context.Context, dst string, srcs []string) error {
	f := FormatOf(dst)
	if f != Zip && f != TarGz && f != Tar {
		return errors.New(i18n.T("Packen geht als .zip, .tar.gz oder .tar"))
	}
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf(i18n.T("„%s“ gibt es schon"), filepath.Base(dst))
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			out.Close()
			_ = os.Remove(dst)
		}
	}()
	absDst, _ := filepath.Abs(dst)
	switch f {
	case Zip:
		zw := zip.NewWriter(out)
		if err := walkAll(ctx, srcs, absDst, func(rel, full string, d fs.DirEntry, info fs.FileInfo) error {
			return addZip(zw, rel, full, info)
		}); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
	default:
		var w io.Writer = out
		var gz *gzip.Writer
		if f == TarGz {
			gz = gzip.NewWriter(out)
			w = gz
		}
		tw := tar.NewWriter(w)
		if err := walkAll(ctx, srcs, absDst, func(rel, full string, d fs.DirEntry, info fs.FileInfo) error {
			return addTar(tw, rel, full, info)
		}); err != nil {
			return err
		}
		if err := tw.Close(); err != nil {
			return err
		}
		if gz != nil {
			if err := gz.Close(); err != nil {
				return err
			}
		}
	}
	ok = true
	return out.Close()
}

// walkAll läuft durch alle Quellen; rel ist der Name im Archiv (mit /).
func walkAll(ctx context.Context, srcs []string, skip string, add func(rel, full string, d fs.DirEntry, info fs.FileInfo) error) error {
	for _, src := range srcs {
		base := filepath.Dir(src)
		err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if abs, _ := filepath.Abs(p); abs == skip {
				return nil // das Archiv selbst nicht mit einpacken
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(base, p)
			if err != nil {
				return err
			}
			return add(filepath.ToSlash(rel), p, d, info)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func addZip(zw *zip.Writer, rel, full string, info fs.FileInfo) error {
	if info.Mode()&fs.ModeSymlink != 0 || !(info.Mode().IsRegular() || info.IsDir()) {
		return nil // zip: nur Dateien und Ordner
	}
	h, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	h.Name = rel
	if info.IsDir() {
		h.Name += "/"
		h.Method = zip.Store
		_, err = zw.CreateHeader(h)
		return err
	}
	h.Method = zip.Deflate
	w, err := zw.CreateHeader(h)
	if err != nil {
		return err
	}
	return copyFile(w, full)
}

func addTar(tw *tar.Writer, rel, full string, info fs.FileInfo) error {
	link := ""
	if info.Mode()&fs.ModeSymlink != 0 {
		l, err := os.Readlink(full)
		if err != nil {
			return err
		}
		link = l
	} else if !info.Mode().IsRegular() && !info.IsDir() {
		return nil // Geräte, Sockets …
	}
	h, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return err
	}
	h.Name = rel
	if info.IsDir() {
		h.Name += "/"
	}
	h.Uname, h.Gname = "", ""
	if err := tw.WriteHeader(h); err != nil {
		return err
	}
	if info.Mode().IsRegular() {
		return copyFile(tw, full)
	}
	return nil
}

func copyFile(w io.Writer, p string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// ---------------------------------------------------------------- Entpacken

// Extract entpackt archive in den Ordner dst (wird angelegt).
func Extract(ctx context.Context, archive, dst string) error {
	f := FormatOf(archive)
	if f == None {
		return errors.New(i18n.T("kein bekanntes Archiv"))
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	if f == Zip {
		zr, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer zr.Close()
		for _, zf := range zr.File {
			if err := ctx.Err(); err != nil {
				return err
			}
			target, err := safeJoin(dst, zf.Name)
			if err != nil {
				return err
			}
			if zf.FileInfo().IsDir() {
				if err := os.MkdirAll(target, 0o755); err != nil {
					return err
				}
				continue
			}
			if zf.Mode()&fs.ModeSymlink != 0 {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			rc, err := zf.Open()
			if err != nil {
				return err
			}
			err = writeFile(target, rc, zf.Mode().Perm())
			rc.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}
	in, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer in.Close()
	r, err := decompress(in, f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(r)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeJoin(dst, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeFile(target, tr, fs.FileMode(h.Mode).Perm()); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// nur Links, die im Zielordner bleiben
			if filepath.IsAbs(h.Linkname) {
				continue
			}
			if _, err := safeJoin(dst, path.Join(path.Dir(h.Name), h.Linkname)); err != nil {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Symlink(h.Linkname, target)
		}
	}
}

func decompress(r io.Reader, f Format) (io.Reader, error) {
	switch f {
	case TarGz:
		return gzip.NewReader(r)
	case TarBz2:
		return bzip2.NewReader(r), nil
	case Tar:
		return r, nil
	}
	return nil, errors.New(i18n.T("dieses Format entpackt sshTermy hier nicht (.xz/.zst): bitte auf dem Server oder mit einem eigenen Programm"))
}

func writeFile(p string, r io.Reader, mode fs.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode|0o200)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// safeJoin: Name aus dem Archiv unter dst, nie außerhalb.
func safeJoin(dst, name string) (string, error) {
	n := strings.ReplaceAll(name, `\`, "/")
	if strings.HasPrefix(n, "/") || (len(n) > 1 && n[1] == ':') {
		return "", fmt.Errorf(i18n.T("unsicherer Name im Archiv: %s"), name)
	}
	clean := path.Clean("/" + n)
	for _, part := range strings.Split(n, "/") {
		if part == ".." {
			return "", fmt.Errorf(i18n.T("unsicherer Name im Archiv: %s"), name)
		}
	}
	return filepath.Join(dst, filepath.FromSlash(strings.TrimPrefix(clean, "/"))), nil
}

// ---------------------------------------------------------------- Inhalt

// Entry: ein Eintrag im Archiv (für die Vorschau).
type Entry struct {
	Name  string
	Size  int64
	IsDir bool
}

// List liest die Einträge eines Archivs, höchstens max. r ist der Inhalt;
// für zip braucht es ReaderAt und Größe (ra != nil), tar liest der Reihe
// nach. more = es gibt noch mehr.
func List(name string, r io.Reader, ra io.ReaderAt, size int64, max int) (out []Entry, more bool, err error) {
	f := FormatOf(name)
	if f == Zip {
		if ra == nil {
			return nil, false, errors.New(i18n.T("zip lässt sich hier nicht lesen"))
		}
		zr, err := zip.NewReader(ra, size)
		if err != nil {
			return nil, false, err
		}
		for i, zf := range zr.File {
			if i >= max {
				return out, true, nil
			}
			out = append(out, Entry{Name: zf.Name, Size: int64(zf.UncompressedSize64), IsDir: zf.FileInfo().IsDir()})
		}
		return out, false, nil
	}
	if f == None {
		return nil, false, errors.New(i18n.T("kein bekanntes Archiv"))
	}
	dr, err := decompress(r, f)
	if err != nil {
		return nil, false, err
	}
	tr := tar.NewReader(dr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out, false, nil
		}
		if err != nil {
			return out, false, err
		}
		if len(out) >= max {
			return out, true, nil
		}
		out = append(out, Entry{Name: h.Name, Size: h.Size, IsDir: h.Typeflag == tar.TypeDir})
	}
}

// ---------------------------------------------------------------- Server

// Q setzt einen Text für die POSIX-Shell in einfache Anführungszeichen.
func Q(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// PackCommand: Befehl, der im Ordner dir die Namen names nach archive
// (nur der Name, im selben Ordner) packt.
func PackCommand(dir, archive string, names []string) (string, error) {
	var parts []string
	for _, n := range names {
		parts = append(parts, Q("./"+n))
	}
	list := strings.Join(parts, " ")
	cd := "cd " + Q(dir) + " && "
	switch FormatOf(archive) {
	case Zip:
		return cd + "{ command -v zip >/dev/null 2>&1 || { echo '" + i18n.T("zip fehlt auf dem Server, bitte .tar.gz nehmen") + "' >&2; exit 127; }; } && zip -qry " + Q(archive) + " " + list, nil
	case TarGz:
		return cd + "tar -czf " + Q(archive) + " " + list, nil
	case Tar:
		return cd + "tar -cf " + Q(archive) + " " + list, nil
	case TarBz2:
		return cd + "tar -cjf " + Q(archive) + " " + list, nil
	case TarXz:
		return cd + "tar -cJf " + Q(archive) + " " + list, nil
	}
	return "", errors.New(i18n.T("Packen geht als .zip, .tar.gz, .tar, .tar.bz2 oder .tar.xz"))
}

// ExtractCommand: Befehl, der archive (voller Pfad) in den neuen Ordner
// dst entpackt.
func ExtractCommand(archive, dst string) (string, error) {
	mk := "mkdir -p " + Q(dst) + " && "
	a, d := Q(archive), Q(dst)
	switch FormatOf(archive) {
	case Zip:
		return mk + "if command -v unzip >/dev/null 2>&1; then unzip -q " + a + " -d " + d +
			"; elif command -v bsdtar >/dev/null 2>&1; then bsdtar -xf " + a + " -C " + d +
			"; elif command -v python3 >/dev/null 2>&1; then python3 -m zipfile -e " + a + " " + d +
			"; else echo '" + i18n.T("unzip fehlt auf dem Server") + "' >&2; exit 127; fi", nil
	case TarGz:
		return mk + "tar -xzf " + a + " -C " + d, nil
	case TarBz2:
		return mk + "tar -xjf " + a + " -C " + d, nil
	case TarXz:
		return mk + "tar -xJf " + a + " -C " + d, nil
	case TarZst:
		return mk + "tar --zstd -xf " + a + " -C " + d, nil
	case Tar:
		return mk + "tar -xf " + a + " -C " + d, nil
	}
	return "", errors.New(i18n.T("kein bekanntes Archiv"))
}
