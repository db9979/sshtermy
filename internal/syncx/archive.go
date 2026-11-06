package syncx

// Archiv: Config-Sicherungen der Netzwerkgeräte und Sitzungsprotokolle
// zwischen Rechnern abgleichen (zuschaltbar, SyncConfig.Archive).
//
// Die Sync-Datei bleibt klein: das Archiv liegt daneben im Unterordner
// sshtermy-archiv/, eine Datei je Sicherung bzw. Protokoll. Jede Datei wird
// genau einmal geschrieben und danach nie geändert, deshalb gibt es keine
// Konflikte zwischen Rechnern:
//
//	b-20261106-3f9a…e1.sta    Sicherung (b) vom 06.11.2026
//	l-20261105-77c0…2d.sta    Sitzungsprotokoll (l) vom 05.11.2026
//
//   - Inhalt: AES-256-GCM mit einem aus dem Sync-Schlüssel abgeleiteten
//     Schlüssel, AAD = Dateiname. Der Speicherort sieht weder Hostnamen noch
//     IDs, nur Art und Tag.
//   - Name: HMAC über Art und ID (eigener abgeleiteter Schlüssel), damit jeder
//     Rechner für seine Einträge denselben Namen berechnet.
//   - Aufbewahrung: Dateien älter als Days werden nicht mehr hochgeladen und
//     von jedem Rechner im Archiv entfernt. Lokale Kopien bleiben.
//   - Löschen: Grabstein "archiv:<Art>:<ID>" in Payload.Deleted (wird mit der
//     Sync-Datei verteilt). Dann verschwindet die Datei im Archiv und auf den
//     anderen Rechnern.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/vault"
)

const (
	ArchiveDir         = "sshtermy-archiv"
	archiveExt         = ".sta"
	archiveFormat      = "sshtermy-archive"
	archiveVer         = 1
	DefaultArchiveDays = 90

	// Obergrenzen je Abgleich; der Rest folgt beim nächsten Mal.
	archiveMaxPut = 100
	archiveMaxGet = 200
)

// Arten im Archiv.
const (
	KindBackup = "backup" // ID: <Verbindungs-ID>/<Sicherungs-ID>
	KindLog    = "log"    // ID: Protokoll-ID
)

// ArchiveTombstone: Schlüssel in Payload.Deleted für einen gelöschten Eintrag.
func ArchiveTombstone(kind, id string) string { return "archiv:" + kind + ":" + id }

// ArchiveStore: Ablage des Archivs (Ordner oder WebDAV).
type ArchiveStore interface {
	List(ctx context.Context) ([]string, error)
	Get(ctx context.Context, name string) ([]byte, error)
	Put(ctx context.Context, name string, data []byte) error
	Remove(ctx context.Context, name string) error
}

// Archiver: Backends, die ein Archiv neben der Sync-Datei anbieten.
type Archiver interface {
	Archive() ArchiveStore
}

// LocalItem: ein Eintrag auf diesem Rechner. Load wird nur aufgerufen, wenn
// der Eintrag wirklich hochgeladen werden muss.
type LocalItem struct {
	Kind  string
	ID    string
	Taken time.Time
	Load  func() ([]byte, error)
}

// ArchiveItem: ein Eintrag aus dem Archiv (Klartext).
type ArchiveItem struct {
	Kind   string    `json:"kind"`
	ID     string    `json:"id"`
	Taken  time.Time `json:"taken"`
	Device string    `json:"device,omitempty"`
	Data   []byte    `json:"data"`
}

// ArchiveResult: was der Abgleich ergeben hat.
type ArchiveResult struct {
	Got      []ArchiveItem // neu von anderen Rechnern: lokal ablegen
	Drop     []LocalItem   // anderswo gelöscht: lokal entfernen
	Uploaded int
	Removed  int // im Archiv entfernt (Aufbewahrung, Grabstein)
	Pending  bool
}

type archiveFile struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	KeyID      string `json:"key_id"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

func (k *Key) sub(purpose string) []byte {
	m := hmac.New(sha256.New, k.DEK)
	m.Write([]byte("sshtermy/v1/sync/archive/" + purpose))
	return m.Sum(nil)
}

func kindLetter(kind string) string {
	if kind == KindBackup {
		return "b"
	}
	return "l"
}

// ArchiveName: Dateiname eines Eintrags im Archiv.
func ArchiveName(k *Key, kind, id string, taken time.Time) string {
	nk := k.sub("name")
	defer vault.Wipe(nk)
	m := hmac.New(sha256.New, nk)
	m.Write([]byte(kind + "\x00" + id))
	return kindLetter(kind) + "-" + taken.UTC().Format("20060102") + "-" + hex.EncodeToString(m.Sum(nil)[:12]) + archiveExt
}

// archiveDay liest den Tag aus dem Dateinamen.
func archiveDay(name string) (time.Time, bool) {
	parts := strings.Split(strings.TrimSuffix(name, archiveExt), "-")
	if len(parts) != 3 || !strings.HasSuffix(name, archiveExt) {
		return time.Time{}, false
	}
	t, err := time.Parse("20060102", parts[1])
	return t, err == nil
}

func sealItem(k *Key, name string, it ArchiveItem) ([]byte, error) {
	plain, err := json.Marshal(it)
	if err != nil {
		return nil, err
	}
	defer vault.Wipe(plain)
	dk := k.sub("data")
	defer vault.Wipe(dk)
	n, ct, err := vault.SealGCM(dk, plain, []byte(archiveFormat+"/"+name))
	if err != nil {
		return nil, err
	}
	return json.Marshal(archiveFile{Format: archiveFormat, Version: archiveVer, KeyID: k.ID, Nonce: n, Ciphertext: ct})
}

func openItem(k *Key, name string, raw []byte) (*ArchiveItem, error) {
	var f archiveFile
	if err := json.Unmarshal(raw, &f); err != nil || f.Format != archiveFormat {
		return nil, ErrNotSyncFile
	}
	if f.Version > archiveVer {
		return nil, ErrTooNew
	}
	if f.KeyID != k.ID {
		return nil, ErrKeyMismatch
	}
	dk := k.sub("data")
	defer vault.Wipe(dk)
	plain, err := vault.OpenGCM(dk, f.Nonce, f.Ciphertext, []byte(archiveFormat+"/"+name))
	if err != nil {
		return nil, errors.New(i18n.T("Archivdatei beschädigt oder verändert"))
	}
	defer vault.Wipe(plain)
	it := &ArchiveItem{}
	if err := json.Unmarshal(plain, it); err != nil {
		return nil, err
	}
	// Name muss zum Inhalt passen (keine umbenannten Dateien unterschieben)
	if ArchiveName(k, it.Kind, it.ID, it.Taken) != name {
		return nil, errors.New(i18n.T("Archivdatei beschädigt oder verändert"))
	}
	return it, nil
}

// SyncArchive gleicht das Archiv ab:
//
//  1. eigene Einträge, die im Archiv fehlen, hochladen (nur innerhalb der Aufbewahrung)
//  2. fremde Einträge herunterladen
//  3. abgelaufene und gelöschte Einträge im Archiv entfernen
func SyncArchive(ctx context.Context, a ArchiveStore, k *Key, local []LocalItem, deleted map[string]time.Time, days int, device string, now time.Time) (ArchiveResult, error) {
	var res ArchiveResult
	if days <= 0 {
		days = DefaultArchiveDays
	}
	cutoff := now.UTC().AddDate(0, 0, -days)
	remote, err := a.List(ctx)
	if err != nil {
		return res, err
	}
	have := map[string]bool{}
	for _, n := range remote {
		have[n] = true
	}
	tomb := func(kind, id string) bool {
		_, ok := deleted[ArchiveTombstone(kind, id)]
		return ok
	}

	mine := map[string]bool{}
	for _, it := range local {
		name := ArchiveName(k, it.Kind, it.ID, it.Taken)
		mine[name] = true
		switch {
		case tomb(it.Kind, it.ID):
			res.Drop = append(res.Drop, it)
			if have[name] {
				if err := a.Remove(ctx, name); err == nil {
					res.Removed++
				}
			}
		case have[name] || it.Taken.Before(cutoff):
		case res.Uploaded >= archiveMaxPut:
			res.Pending = true
		default:
			data, err := it.Load()
			if err != nil {
				continue // z. B. unlesbar: beim nächsten Mal erneut
			}
			blob, err := sealItem(k, name, ArchiveItem{Kind: it.Kind, ID: it.ID, Taken: it.Taken.UTC(), Device: device, Data: data})
			vault.Wipe(data)
			if err != nil {
				return res, err
			}
			if err := a.Put(ctx, name, blob); err != nil {
				return res, err
			}
			res.Uploaded++
		}
	}

	got := 0
	for _, name := range remote {
		if mine[name] {
			continue
		}
		day, ok := archiveDay(name)
		if !ok {
			continue
		}
		if day.Before(cutoff.Truncate(24 * time.Hour)) {
			if err := a.Remove(ctx, name); err == nil {
				res.Removed++
			}
			continue
		}
		if got >= archiveMaxGet {
			res.Pending = true
			continue
		}
		raw, err := a.Get(ctx, name)
		if err != nil {
			return res, err
		}
		it, err := openItem(k, name, raw)
		if err != nil {
			continue // anderer Schlüssel oder beschädigt: liegen lassen
		}
		if tomb(it.Kind, it.ID) {
			if err := a.Remove(ctx, name); err == nil {
				res.Removed++
			}
			continue
		}
		got++
		res.Got = append(res.Got, *it)
	}
	return res, nil
}

// ---------------------------------------------------------------- Ordner

// Archive: Unterordner neben der Sync-Datei.
func (f Folder) Archive() ArchiveStore { return folderArchive{filepath.Join(f.Dir, ArchiveDir)} }

type folderArchive struct{ dir string }

func validArchiveName(n string) bool {
	_, ok := archiveDay(n)
	return ok && path.Base(n) == n && !strings.ContainsAny(n, `/\`)
}

func (f folderArchive) List(_ context.Context) ([]string, error) {
	ents, err := os.ReadDir(f.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() && validArchiveName(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

func (f folderArchive) Get(_ context.Context, name string) ([]byte, error) {
	if !validArchiveName(name) {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(filepath.Join(f.dir, name))
}

func (f folderArchive) Put(_ context.Context, name string, data []byte) error {
	if !validArchiveName(name) {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(f.dir, "."+name+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(f.dir, name))
}

func (f folderArchive) Remove(_ context.Context, name string) error {
	if !validArchiveName(name) {
		return os.ErrInvalid
	}
	err := os.Remove(filepath.Join(f.dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ---------------------------------------------------------------- WebDAV

// Archive: Unterordner sshtermy-archiv/ in der WebDAV-Ablage.
func (w *WebDAV) Archive() ArchiveStore { return &davArchive{w: w} }

type davArchive struct {
	w     *WebDAV
	ready bool
}

func (d *davArchive) dirURL() string {
	return strings.TrimSuffix(d.w.URL, "/") + "/" + ArchiveDir + "/"
}

func (d *davArchive) fileURL(name string) string { return d.dirURL() + url.PathEscape(name) }

func (d *davArchive) List(ctx context.Context) ([]string, error) {
	body := []byte(`<?xml version="1.0" encoding="utf-8"?><d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/></d:prop></d:propfind>`)
	resp, b, err := d.w.do(ctx, "PROPFIND", d.dirURL(), body, map[string]string{"Depth": "1", "Content-Type": "application/xml"})
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, nil
	case http.StatusMultiStatus, http.StatusOK:
	default:
		return nil, &HTTPError{Method: "PROPFIND", Status: resp.StatusCode, HTML: isHTML(resp)}
	}
	d.ready = true
	return davNames(b), nil
}

// davNames liest die Dateinamen aus einer PROPFIND-Antwort.
func davNames(b []byte) []string {
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	var out []string
	inHref := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			inHref = t.Name.Local == "href"
		case xml.EndElement:
			inHref = false
		case xml.CharData:
			if !inHref {
				continue
			}
			h := strings.TrimSpace(string(t))
			if u, err := url.Parse(h); err == nil {
				h = u.Path
			}
			n := path.Base(strings.TrimSuffix(h, "/"))
			if validArchiveName(n) {
				out = append(out, n)
			}
		}
	}
	return out
}

func (d *davArchive) Get(ctx context.Context, name string) ([]byte, error) {
	resp, b, err := d.w.do(ctx, http.MethodGet, d.fileURL(name), nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{Method: "GET", Status: resp.StatusCode, HTML: isHTML(resp)}
	}
	return b, nil
}

func (d *davArchive) Put(ctx context.Context, name string, data []byte) error {
	if !d.ready {
		if code, err := d.w.mkcol(ctx, d.dirURL()); err != nil {
			return err
		} else if code >= 300 && code != http.StatusMethodNotAllowed { // 405: gibt es schon
			return &HTTPError{Method: "MKCOL", Status: code}
		}
		d.ready = true
	}
	resp, _, err := d.w.do(ctx, http.MethodPut, d.fileURL(name), data, map[string]string{"Content-Type": "application/octet-stream"})
	if err != nil {
		return err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	}
	return &HTTPError{Method: "PUT", Status: resp.StatusCode, HTML: isHTML(resp)}
}

func (d *davArchive) Remove(ctx context.Context, name string) error {
	return d.w.del(ctx, d.fileURL(name))
}
