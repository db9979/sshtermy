package syncx

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func item(kind, id string, taken time.Time, data string) LocalItem {
	return LocalItem{Kind: kind, ID: id, Taken: taken, Load: func() ([]byte, error) { return []byte(data), nil }}
}

func TestArchiveTwoDevicesFolder(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	k, _ := NewKey([]byte("sync-passwort"))
	ar := Folder{Dir: dir}.Archive()
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

	// Rechner A: eine Sicherung, ein Protokoll, ein uraltes Protokoll
	a := []LocalItem{
		item(KindBackup, "conn1/20261105T080000Z_120", now.Add(-26*time.Hour), "hostname core-sw-1\npassword secret-hash"),
		item(KindLog, "20261106-090000-aabbccddeeff", now.Add(-time.Hour), `{"id":"x"}`),
		item(KindLog, "20250101-090000-000000000000", now.AddDate(-1, 0, 0), `{"id":"alt"}`),
	}
	res, err := SyncArchive(ctx, ar, k, a, nil, 90, "laptop-a", now)
	if err != nil || res.Uploaded != 2 || len(res.Got) != 0 {
		t.Fatalf("A: %+v %v", res, err)
	}
	// kein Klartext im Archiv (weder Inhalt noch Hostname/ID)
	ents, _ := os.ReadDir(filepath.Join(dir, ArchiveDir))
	for _, e := range ents {
		b, _ := os.ReadFile(filepath.Join(dir, ArchiveDir, e.Name()))
		for _, s := range []string{"core-sw-1", "secret-hash", "conn1", "aabbccddeeff"} {
			if bytes.Contains(b, []byte(s)) || strings.Contains(e.Name(), s) {
				t.Fatalf("Klartext %q in %s", s, e.Name())
			}
		}
	}

	// Rechner B bekommt beides mit Rechnername
	res, err = SyncArchive(ctx, ar, k, nil, nil, 90, "mac-b", now)
	if err != nil || len(res.Got) != 2 || res.Uploaded != 0 {
		t.Fatalf("B: %+v %v", res, err)
	}
	for _, g := range res.Got {
		if g.Device != "laptop-a" {
			t.Fatalf("Rechner fehlt: %+v", g)
		}
		if g.Kind == KindBackup && !strings.Contains(string(g.Data), "core-sw-1") {
			t.Fatalf("Inhalt: %q", g.Data)
		}
	}
	// B hat jetzt dieselben Einträge lokal: nichts mehr zu tun
	b := []LocalItem{}
	for _, g := range res.Got {
		b = append(b, item(g.Kind, g.ID, g.Taken, string(g.Data)))
	}
	res, err = SyncArchive(ctx, ar, k, b, nil, 90, "mac-b", now)
	if err != nil || res.Uploaded != 0 || len(res.Got) != 0 {
		t.Fatalf("B erneut: %+v %v", res, err)
	}

	// A löscht das Protokoll: Grabstein entfernt es im Archiv und bei B
	del := map[string]time.Time{ArchiveTombstone(KindLog, "20261106-090000-aabbccddeeff"): now}
	res, err = SyncArchive(ctx, ar, k, a[:1], del, 90, "laptop-a", now)
	if err != nil || res.Removed != 1 {
		t.Fatalf("A löschen: %+v %v", res, err)
	}
	res, err = SyncArchive(ctx, ar, k, b, del, 90, "mac-b", now)
	if err != nil || len(res.Drop) != 1 || res.Drop[0].Kind != KindLog {
		t.Fatalf("B löschen: %+v %v", res, err)
	}

	// Aufbewahrung: 100 Tage später verschwindet die Sicherung im Archiv
	res, err = SyncArchive(ctx, ar, k, nil, nil, 90, "mac-b", now.AddDate(0, 0, 100))
	if err != nil || res.Removed != 1 || len(res.Got) != 0 {
		t.Fatalf("Aufbewahrung: %+v %v", res, err)
	}
	if names, _ := ar.List(ctx); len(names) != 0 {
		t.Fatalf("Archiv nicht leer: %v", names)
	}

	// anderer Sync-Schlüssel: fremde Dateien werden ignoriert, nicht gelöscht
	_, _ = SyncArchive(ctx, ar, k, a[:1], nil, 90, "laptop-a", now)
	other, _ := NewKey([]byte("anderes-pw"))
	res, err = SyncArchive(ctx, ar, other, nil, nil, 90, "x", now)
	if err != nil || len(res.Got) != 0 || res.Removed != 0 {
		t.Fatalf("fremder Schlüssel: %+v %v", res, err)
	}
}

func TestArchiveRejectsRenamedFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	k, _ := NewKey([]byte("sync-passwort"))
	ar := Folder{Dir: dir}.Archive()
	now := time.Now().UTC()
	_, _ = SyncArchive(ctx, ar, k, []LocalItem{item(KindLog, "20261106-090000-aabbccddeeff", now, "x")}, nil, 90, "a", now)
	names, _ := ar.List(ctx)
	if len(names) != 1 {
		t.Fatal(names)
	}
	d := filepath.Join(dir, ArchiveDir)
	moved := "l-" + now.Format("20060102") + "-000000000000000000000000" + archiveExt
	if err := os.Rename(filepath.Join(d, names[0]), filepath.Join(d, moved)); err != nil {
		t.Fatal(err)
	}
	res, _ := SyncArchive(ctx, ar, k, nil, nil, 90, "b", now)
	if len(res.Got) != 0 {
		t.Fatalf("umbenannte Datei übernommen: %+v", res.Got)
	}
}

func TestArchiveWebDAV(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(&fakeDAV{files: map[string][]byte{}})
	defer srv.Close()
	k, _ := NewKey([]byte("sync-passwort"))
	now := time.Now().UTC()
	w := &WebDAV{URL: srv.URL + "/dav/"}
	res, err := SyncArchive(ctx, w.Archive(), k, []LocalItem{item(KindBackup, "c/20261106T080000Z_3", now, "cfg")}, nil, 90, "a", now)
	if err != nil || res.Uploaded != 1 {
		t.Fatalf("hochladen: %+v %v", res, err)
	}
	w2 := &WebDAV{URL: srv.URL + "/dav/"}
	res, err = SyncArchive(ctx, w2.Archive(), k, nil, nil, 90, "b", now)
	if err != nil || len(res.Got) != 1 || string(res.Got[0].Data) != "cfg" {
		t.Fatalf("holen: %+v %v", res, err)
	}
}

// fakeDAV: kleinster WebDAV-Server für den Test (ein Unterordner).
type fakeDAV struct {
	mu    sync.Mutex
	files map[string][]byte
	dirOK bool
}

func (f *fakeDAV) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	const dir = "/dav/" + ArchiveDir + "/"
	switch r.Method {
	case "MKCOL":
		f.dirOK = true
		w.WriteHeader(http.StatusCreated)
	case "PROPFIND":
		if !f.dirOK {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var b strings.Builder
		b.WriteString(`<?xml version="1.0"?><D:multistatus xmlns:D="DAV:"><D:response><D:href>` + dir + `</D:href></D:response>`)
		for n := range f.files {
			b.WriteString(`<D:response><D:href>` + dir + n + `</D:href></D:response>`)
		}
		b.WriteString(`</D:multistatus>`)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(b.String()))
	case http.MethodPut:
		if !f.dirOK {
			w.WriteHeader(http.StatusConflict)
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.files[strings.TrimPrefix(r.URL.Path, dir)] = body
		w.WriteHeader(http.StatusCreated)
	case http.MethodGet:
		b, ok := f.files[strings.TrimPrefix(r.URL.Path, dir)]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(b)
	case http.MethodDelete:
		delete(f.files, strings.TrimPrefix(r.URL.Path, dir))
		w.WriteHeader(http.StatusNoContent)
	}
}
