package syncx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Backend speichert genau eine (bereits verschlüsselte) Datei.
//
// Version ist ein undurchsichtiger Wert (ETag oder Prüfsumme). Write mit
// ifVersion schreibt nur, wenn die Datei seit dem Lesen unverändert ist
// (optimistisches Sperren); ifVersion "" heißt: Datei darf noch nicht existieren.
type Backend interface {
	Describe() string
	Read(ctx context.Context) (data []byte, version string, err error)
	Write(ctx context.Context, data []byte, ifVersion string) error
}

// ConflictCopies liefern Backends, deren Cloud-Dienst bei gleichzeitigen
// Änderungen Kopien anlegt (OneDrive, Dropbox, Syncthing, Nextcloud).
type ConflictCopies interface {
	Copies(ctx context.Context) (map[string][]byte, error)
	Retire(name string) error
}

func sum(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }

// ---------------------------------------------------------------- Ordner

// Folder: ein Ordner, den ein Sync-Dienst zwischen Rechnern abgleicht.
type Folder struct{ Dir string }

func (f Folder) Describe() string { return "Ordner " + f.Dir }
func (f Folder) path() string     { return filepath.Join(f.Dir, FileName) }

func (f Folder) Read(_ context.Context) ([]byte, string, error) {
	b, err := os.ReadFile(f.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	return b, sum(b), nil
}

func (f Folder) Write(_ context.Context, data []byte, ifVersion string) error {
	cur, err := os.ReadFile(f.path())
	switch {
	case errors.Is(err, os.ErrNotExist):
		if ifVersion != "" {
			return ErrConflict
		}
	case err != nil:
		return err
	case sum(cur) != ifVersion:
		return ErrConflict
	}
	tmp := filepath.Join(f.Dir, "."+FileName+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path())
}

// Copies findet Konfliktkopien, z. B.
//
//	sshtermy-sync (Konfliktkopie …).vault       Dropbox / Nextcloud
//	sshtermy-sync-LAPTOP.vault                  OneDrive
//	sshtermy-sync.sync-conflict-2026….vault     Syncthing
func (f Folder) Copies(_ context.Context) (map[string][]byte, error) {
	ents, err := os.ReadDir(f.Dir)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	base := strings.TrimSuffix(FileName, ".vault")
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || n == FileName || !strings.HasPrefix(n, base) || !strings.HasSuffix(n, ".vault") {
			continue
		}
		if b, err := os.ReadFile(filepath.Join(f.Dir, n)); err == nil {
			out[n] = b
		}
	}
	return out, nil
}

// Retire benennt eine eingearbeitete Kopie um (nicht löschen: falls doch
// etwas schiefgeht, bleibt sie erhalten).
func (f Folder) Retire(name string) error {
	p := filepath.Join(f.Dir, name)
	return os.Rename(p, p+".merged")
}
