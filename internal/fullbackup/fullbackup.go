// Package fullbackup: Gesamtsicherung des Datenordners in eine einzige
// verschlüsselte Datei, zum Sichern und für den Umzug auf einen neuen Rechner.
//
// Inhalt ist ein ZIP aller Dateien im Datenordner (Store, gesicherte
// Switch-Configs, Sitzungsprotokolle, known_hosts, Sprache …) ohne Absturz-
// und Debug-Berichte. Das ZIP wird mit einem Schlüssel aus dem Passwort
// (Argon2id) in Blöcken mit AES-256-GCM verschlüsselt; jeder Block trägt
// seine Nummer und ob er der letzte ist, so fällt Kürzen, Umsortieren oder
// Ändern auf. Das Passwort ist das Master-Passwort: die Oberfläche prüft es
// beim Erstellen, und der Store in der Sicherung öffnet sich damit.
//
// Einspielen geschieht in zwei Schritten: Prepare entschlüsselt in einen
// Ordner neben dem Datenordner und merkt ihn vor; ApplyPending tauscht beim
// nächsten Start (bevor der Store geöffnet wird) die Ordner. Der bisherige
// Datenordner bleibt als <ordner>.vorher-<zeit> liegen.
package fullbackup

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	magic        = "SSHTERMY-SICHERUNG\x01"
	chunkSize    = 1 << 20
	manifestName = "sshtermy-sicherung.json"
	pendingName  = "restore-pending"
	// Ext: Dateiendung der Sicherung.
	Ext = ".sshtbak"
	// Schutz beim Einspielen: höchstens so viel entpacken.
	maxTotal = 8 << 30
	maxFiles = 200000
)

// skip: was nicht in die Sicherung gehört (rechnerbezogen oder Diagnose).
func skip(rel string, d fs.DirEntry) bool {
	top := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
	switch top {
	case "crash", pendingName, "restore-tabs.json":
		return true
	}
	n := d.Name()
	return strings.HasPrefix(n, "debug.log") || strings.HasSuffix(n, ".lock") || strings.HasSuffix(n, ".tmp")
}

// Manifest steht unverschlüsselt im ZIP (also verschlüsselt in der Datei).
type Manifest struct {
	Format  int       `json:"format"`
	Created time.Time `json:"created"`
	Version string    `json:"version"`
	OS      string    `json:"os"`
	Files   int       `json:"files"`
	Bytes   int64     `json:"bytes"`
}

func deriveKey(pass, salt []byte) []byte {
	return argon2.IDKey(pass, salt, 3, 64*1024, 4, 32)
}

// Create schreibt die Sicherung von dir nach w.
func Create(dir string, pass []byte, version string, w io.Writer) (Manifest, error) {
	if len(pass) == 0 {
		return Manifest{}, errors.New("fullbackup: Passwort fehlt")
	}
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	m := Manifest{Format: 1, Created: time.Now().UTC().Truncate(time.Second), Version: version, OS: runtime.GOOS}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == "." {
			return nil
		}
		if skip(rel, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // Ordner entstehen beim Entpacken, Links bleiben draußen
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name, h.Method = filepath.ToSlash(rel), zip.Deflate
		fw, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		n, err := io.Copy(fw, f)
		f.Close()
		m.Files++
		m.Bytes += n
		return err
	})
	if err != nil {
		return Manifest{}, err
	}
	if m.Files == 0 {
		return Manifest{}, errors.New("fullbackup: Datenordner ist leer")
	}
	mj, _ := json.MarshalIndent(m, "", "  ")
	fw, err := zw.Create(manifestName)
	if err == nil {
		_, err = fw.Write(mj)
	}
	if err == nil {
		err = zw.Close()
	}
	if err != nil {
		return Manifest{}, err
	}
	return m, seal(w, pass, zbuf.Bytes())
}

func seal(w io.Writer, pass, plain []byte) error {
	salt := make([]byte, 16)
	base := make([]byte, 7)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	if _, err := rand.Read(base); err != nil {
		return err
	}
	aead, err := newAEAD(deriveKey(pass, salt))
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(w)
	bw.WriteString(magic)
	bw.Write(salt)
	bw.Write(base)
	for i := 0; ; i++ {
		n := min(chunkSize, len(plain))
		chunk, rest := plain[:n], plain[n:]
		ct := aead.Seal(nil, nonce(base, i, len(rest) == 0), chunk, []byte(magic))
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(ct)))
		bw.Write(l[:])
		if _, err := bw.Write(ct); err != nil {
			return err
		}
		plain = rest
		if len(rest) == 0 {
			break
		}
	}
	return bw.Flush()
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

func nonce(base []byte, i int, last bool) []byte {
	n := make([]byte, 12)
	copy(n, base)
	binary.BigEndian.PutUint32(n[7:11], uint32(i))
	if last {
		n[11] = 1
	}
	return n
}

// ErrWrongPassword: Passwort falsch oder Datei verändert.
var ErrWrongPassword = errors.New("fullbackup: Passwort falsch oder Datei beschädigt")

// ErrNotBackup: keine sshTermy-Sicherung.
var ErrNotBackup = errors.New("fullbackup: keine sshTermy-Sicherung")

// open entschlüsselt die ganze Datei.
func open(r io.Reader, pass []byte) ([]byte, error) {
	br := bufio.NewReader(r)
	head := make([]byte, len(magic)+16+7)
	if _, err := io.ReadFull(br, head); err != nil || string(head[:len(magic)]) != magic {
		return nil, ErrNotBackup
	}
	salt, base := head[len(magic):len(magic)+16], head[len(magic)+16:]
	aead, err := newAEAD(deriveKey(pass, salt))
	if err != nil {
		return nil, err
	}
	var out []byte
	for i := 0; ; i++ {
		var l [4]byte
		if _, err := io.ReadFull(br, l[:]); err != nil {
			return nil, ErrWrongPassword // abgeschnitten
		}
		n := binary.BigEndian.Uint32(l[:])
		if n > chunkSize+uint32(aead.Overhead()) {
			return nil, ErrWrongPassword
		}
		ct := make([]byte, n)
		if _, err := io.ReadFull(br, ct); err != nil {
			return nil, ErrWrongPassword
		}
		_, peekErr := br.Peek(1)
		last := peekErr == io.EOF
		pt, err := aead.Open(nil, nonce(base, i, last), ct, []byte(magic))
		if err != nil {
			return nil, ErrWrongPassword
		}
		out = append(out, pt...)
		if int64(len(out)) > maxTotal {
			return nil, errors.New("fullbackup: Sicherung zu groß")
		}
		if last {
			return out, nil
		}
	}
}

// Prepare entschlüsselt die Sicherung in einen neuen Ordner neben dir und
// prüft, dass ein Store darin liegt. check bekommt den Pfad der Vault-Datei
// (die Oberfläche prüft damit das Passwort). Rückgabe: Ordner und Manifest.
func Prepare(dir, file string, pass []byte, check func(vaultPath string) error) (string, Manifest, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", Manifest{}, err
	}
	defer f.Close()
	plain, err := open(f, pass)
	if err != nil {
		return "", Manifest{}, err
	}
	zr, err := zip.NewReader(bytes.NewReader(plain), int64(len(plain)))
	if err != nil {
		return "", Manifest{}, ErrNotBackup
	}
	if len(zr.File) > maxFiles {
		return "", Manifest{}, errors.New("fullbackup: zu viele Dateien")
	}
	stage := fmt.Sprintf("%s.neu-%s", filepath.Clean(dir), time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(stage, 0o700); err != nil {
		return "", Manifest{}, err
	}
	var m Manifest
	fail := func(err error) (string, Manifest, error) { os.RemoveAll(stage); return "", Manifest{}, err }
	for _, zf := range zr.File {
		name := filepath.FromSlash(zf.Name)
		// nur relative Pfade innerhalb des Ordners (kein ../, kein C:\)
		if !filepath.IsLocal(name) || strings.HasSuffix(zf.Name, "/") {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return fail(err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxTotal))
		rc.Close()
		if err != nil {
			return fail(err)
		}
		if zf.Name == manifestName {
			_ = json.Unmarshal(data, &m)
			continue
		}
		dst := filepath.Join(stage, name)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			return fail(err)
		}
	}
	vp := filepath.Join(stage, "store.vault")
	if _, err := os.Stat(vp); err != nil {
		return fail(errors.New("fullbackup: kein Store in der Sicherung"))
	}
	if check != nil {
		if err := check(vp); err != nil {
			return fail(err)
		}
	}
	return stage, m, nil
}

// MarkPending: beim nächsten Start stage einspielen.
func MarkPending(dir, stage string) error {
	return os.WriteFile(filepath.Join(dir, pendingName), []byte(stage), 0o600)
}

// ApplyPending tauscht den Datenordner gegen die vorbereitete Sicherung
// (vor dem Öffnen des Stores aufrufen). Rückgabe: Ordner mit dem bisherigen
// Stand ("" = nichts zu tun).
func ApplyPending(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, pendingName))
	if err != nil {
		return "", nil
	}
	stage := strings.TrimSpace(string(b))
	// nur ein Ordner, den Prepare neben dir angelegt hat
	if filepath.Dir(stage) != filepath.Dir(filepath.Clean(dir)) || !strings.HasPrefix(filepath.Base(stage), filepath.Base(filepath.Clean(dir))+".neu-") {
		os.Remove(filepath.Join(dir, pendingName))
		return "", errors.New("fullbackup: vorgemerkter Ordner ungültig")
	}
	if _, err := os.Stat(filepath.Join(stage, "store.vault")); err != nil {
		os.Remove(filepath.Join(dir, pendingName))
		return "", errors.New("fullbackup: vorgemerkte Sicherung fehlt")
	}
	os.Remove(filepath.Join(dir, pendingName))
	old := fmt.Sprintf("%s.vorher-%s", filepath.Clean(dir), time.Now().Format("20060102-150405"))
	if err := os.Rename(dir, old); err != nil {
		return "", err
	}
	if err := os.Rename(stage, dir); err != nil {
		_ = os.Rename(old, dir) // zurück, nichts verloren
		return "", err
	}
	return old, nil
}
