package syncx

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

func conn(id, alias string, at time.Time) domain.Connection {
	return domain.Connection{ID: id, Alias: alias, Host: alias + ".lan", Username: "root", CreatedAt: at, UpdatedAt: at}
}

func TestMergeNewerWinsAndTombstones(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	a := &Payload{Connections: []domain.Connection{conn("1", "alt", t0), conn("2", "nur-a", t0)}}
	b := &Payload{Connections: []domain.Connection{conn("1", "neu", t0.Add(time.Minute)), conn("3", "nur-b", t0)},
		Deleted: map[string]time.Time{"2": t0.Add(time.Hour)}}
	now := t0.Add(2 * time.Hour)
	m1, m2 := Merge(a, b, now), Merge(b, a, now)
	if !equalContent(m1, m2) {
		t.Fatal("Merge ist nicht kommutativ")
	}
	if !equalContent(Merge(m1, a, now), m1) {
		t.Fatal("Merge ist nicht idempotent")
	}
	got := map[string]string{}
	for _, c := range m1.Connections {
		got[c.ID] = c.Alias
	}
	if got["1"] != "neu" || got["3"] != "nur-b" || len(got) != 2 {
		t.Fatalf("falsches Ergebnis: %v", got)
	}
	// Bearbeitung nach dem Löschen gewinnt gegen den Grabstein
	c := Merge(&Payload{Connections: []domain.Connection{conn("2", "wieder", t0.Add(3*time.Hour))}}, m1, now)
	if len(c.Connections) != 3 {
		t.Fatalf("späte Bearbeitung verloren: %d", len(c.Connections))
	}
}

func TestMergeDropsOrphanTunnelsAndUnionsHostKeys(t *testing.T) {
	t0 := time.Now().UTC()
	a := &Payload{Connections: []domain.Connection{conn("c", "x", t0)},
		Tunnels:    []domain.Tunnel{{ID: "t1", ConnectionID: "c"}, {ID: "t2", ConnectionID: "weg"}},
		KnownHosts: []string{"a ssh-ed25519 AAA", "# Kommentar"}}
	b := &Payload{KnownHosts: []string{"b ssh-ed25519 BBB", "a ssh-ed25519 AAA"}}
	m := Merge(a, b, t0)
	if len(m.Tunnels) != 1 || m.Tunnels[0].ID != "t1" {
		t.Fatalf("Tunnel: %+v", m.Tunnels)
	}
	rb := Merge(&Payload{Runbooks: []domain.Runbook{{ID: "r", Goal: "alt", CreatedAt: t0, Uses: 5}}},
		&Payload{Runbooks: []domain.Runbook{{ID: "r", Goal: "neu", CreatedAt: t0, UpdatedAt: t0.Add(time.Minute), Uses: 1}}}, t0)
	if len(rb.Runbooks) != 1 || rb.Runbooks[0].Goal != "neu" || rb.Runbooks[0].Uses != 5 {
		t.Fatalf("Runbooks: %+v", rb.Runbooks)
	}
	if len(m.KnownHosts) != 2 {
		t.Fatalf("known_hosts: %v", m.KnownHosts)
	}
}

func TestEncryptNoPlaintextWrongPassAndKeyMismatch(t *testing.T) {
	k, err := NewKey([]byte("sync-passwort"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := Encode(k, &Payload{Connections: []domain.Connection{conn("1", "geheimer-host", time.Now())}})
	if strings.Contains(string(raw), "geheimer-host") {
		t.Fatal("Klartext in der Sync-Datei")
	}
	if _, err := UnlockKey(raw, []byte("falsches-pw")); err != ErrWrongPass {
		t.Fatalf("falsches Passwort: %v", err)
	}
	k2, err := UnlockKey(raw, []byte("sync-passwort"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Decode(k2, raw)
	if err != nil || p.Connections[0].Alias != "geheimer-host" {
		t.Fatalf("Decode: %v", err)
	}
	k3, _ := ParseKey(k2.Marshal())
	if _, err := Decode(k3, raw); err != nil {
		t.Fatal("Schlüssel nach Marshal/Parse unbrauchbar")
	}
	other, _ := NewKey([]byte("anderes-pw"))
	if _, err := Decode(other, raw); err != ErrKeyMismatch {
		t.Fatalf("Key-Mismatch nicht erkannt: %v", err)
	}
	// manipulierter Inhalt
	bad := []byte(strings.Replace(string(raw), `"ciphertext": "`, `"ciphertext": "AAAA`, 1))
	if _, err := Decode(k, bad); err == nil {
		t.Fatal("Manipulation nicht erkannt")
	}
}

// Zwei Rechner über einen gemeinsamen Ordner, inklusive Konfliktkopie.
func TestRunTwoDevicesFolder(t *testing.T) {
	dir := t.TempDir()
	k, _ := NewKey([]byte("sync-passwort"))
	ctx := context.Background()
	t0 := time.Now().UTC().Add(-time.Hour)

	laptop := &Payload{Connections: []domain.Connection{conn("a", "laptop-host", t0)}}
	m, st, err := Run(ctx, Folder{dir}, k, laptop, "laptop")
	if err != nil || !st.Uploaded || len(m.Connections) != 1 {
		t.Fatalf("erster Sync: %v %+v", err, st)
	}
	desktop := &Payload{Connections: []domain.Connection{conn("b", "desktop-host", t0)}}
	m, st, err = Run(ctx, Folder{dir}, k, desktop, "desktop")
	if err != nil || len(m.Connections) != 2 || st.Added != 1 {
		t.Fatalf("zweiter Sync: %v %+v", err, st)
	}
	// unveränderter Stand: kein erneutes Hochladen
	if _, st, _ = Run(ctx, Folder{dir}, k, m, "desktop"); st.Uploaded {
		t.Fatal("ohne Änderung neu hochgeladen")
	}
	// Konfliktkopie wie von Dropbox
	extra, _ := Encode(k, &Payload{Connections: []domain.Connection{conn("c", "aus-konflikt", t0)}})
	os.WriteFile(filepath.Join(dir, "sshtermy-sync (Konfliktkopie).vault"), extra, 0o600)
	m, st, err = Run(ctx, Folder{dir}, k, m, "desktop")
	if err != nil || len(m.Connections) != 3 || st.Merged != 1 {
		t.Fatalf("Konfliktkopie: %v %+v", err, st)
	}
	if _, err := os.Stat(filepath.Join(dir, "sshtermy-sync (Konfliktkopie).vault.merged")); err != nil {
		t.Fatal("Konfliktkopie nicht umbenannt")
	}
}

func TestCheckFolder(t *testing.T) {
	r := CheckFolder(context.Background(), filepath.Join(t.TempDir(), "Dropbox", "sshtermy"))
	if !r.OK() || r.RemoteExists {
		t.Fatalf("%+v", r.Steps)
	}
	if r := CheckFolder(context.Background(), "relativ/pfad"); r.OK() {
		t.Fatal("relativer Pfad akzeptiert")
	}
}

// ---------------------------------------------------------------- WebDAV

// davServer: minimaler WebDAV-Server im Speicher (PROPFIND, MKCOL, GET, PUT
// mit If-Match/If-None-Match, DELETE) mit Basic-Auth.
type davServer struct {
	mu    sync.Mutex
	files map[string][]byte
	dirs  map[string]bool
}

func newDAV() *davServer {
	return &davServer{files: map[string][]byte{}, dirs: map[string]bool{"/": true, "/home/": true}}
}

func etag(b []byte) string { return fmt.Sprintf(`"%x"`, sha256.Sum256(b)) }

func (d *davServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if u, p, ok := r.BasicAuth(); !ok || u != "dominik" || p != "nas-pw" {
		w.WriteHeader(401)
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	p, _ := url.PathUnescape(r.URL.Path)
	parent := p[:strings.LastIndex(strings.TrimSuffix(p, "/"), "/")+1]
	switch r.Method {
	case "PROPFIND":
		if d.dirs[p] || d.files[p] != nil {
			w.WriteHeader(207)
			return
		}
		w.WriteHeader(404)
	case "MKCOL":
		if !d.dirs[parent] {
			w.WriteHeader(409)
			return
		}
		d.dirs[p] = true
		w.WriteHeader(201)
	case "GET":
		b, ok := d.files[p]
		if !ok {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("ETag", etag(b))
		w.Write(b)
	case "PUT":
		cur, exists := d.files[p]
		if im := r.Header.Get("If-Match"); im != "" && (!exists || im != etag(cur)) {
			w.WriteHeader(412)
			return
		}
		if r.Header.Get("If-None-Match") == "*" && exists {
			w.WriteHeader(412)
			return
		}
		b, _ := io.ReadAll(r.Body)
		d.files[p] = b
		w.WriteHeader(201)
	case "DELETE":
		delete(d.files, p)
		w.WriteHeader(204)
	default:
		w.WriteHeader(405)
	}
}

func TestWebDAVCheckAndSync(t *testing.T) {
	dav := newDAV()
	srv := httptest.NewServer(dav)
	defer srv.Close()
	ctx := context.Background()

	in := WebDAVInput{Preset: "synology", Address: srv.URL, User: "dominik", Password: "nas-pw", Remote: "/home/sshtermy"}
	r := CheckWebDAV(ctx, in)
	if !r.OK() {
		t.Fatalf("Check: %+v", r.Steps)
	}
	if !strings.HasSuffix(r.URL, "/home/sshtermy/") || !dav.dirs["/home/sshtermy/"] {
		t.Fatalf("Ordner nicht angelegt: %s", r.URL)
	}

	bad := in
	bad.Password = "falsch"
	if r := CheckWebDAV(ctx, bad); r.OK() || !strings.Contains(r.Steps[len(r.Steps)-1].Hint, "WebDAV") {
		t.Fatalf("falsches Passwort: %+v", r.Steps)
	}
	noParent := in
	noParent.Remote = "/gibtsnicht/sshtermy"
	if r := CheckWebDAV(ctx, noParent); r.OK() || !strings.Contains(r.Steps[len(r.Steps)-1].Detail, "409") {
		t.Fatalf("fehlender Elternordner: %+v", r.Steps)
	}

	k, _ := NewKey([]byte("sync-passwort"))
	w := &WebDAV{URL: r.URL, User: "dominik", Password: "nas-pw"}
	t0 := time.Now().UTC()
	if _, _, err := Run(ctx, w, k, &Payload{Connections: []domain.Connection{conn("a", "eins", t0)}}, "pc1"); err != nil {
		t.Fatal(err)
	}
	m, _, err := Run(ctx, w, k, &Payload{Connections: []domain.Connection{conn("b", "zwei", t0)}}, "pc2")
	if err != nil || len(m.Connections) != 2 {
		t.Fatalf("WebDAV-Sync: %v", err)
	}
	// Zweiter Rechner richtet sich ein: Datei vorhanden, Passwort passt
	r = CheckWebDAV(ctx, in)
	if !r.RemoteExists {
		t.Fatal("vorhandene Sync-Datei nicht erkannt")
	}
	if _, err := UnlockKey(r.RemoteRaw, []byte("sync-passwort")); err != nil {
		t.Fatal(err)
	}
	// Konflikt: veraltetes ETag
	data, _ := Encode(k, &Payload{})
	if err := w.Write(ctx, data, `etag:"veraltet"`); err != ErrConflict {
		t.Fatalf("Konflikt nicht erkannt: %v", err)
	}
}

func TestWebDAVSelfSignedPinning(t *testing.T) {
	srv := httptest.NewTLSServer(newDAV())
	defer srv.Close()
	ctx := context.Background()
	in := WebDAVInput{Preset: "andere", Address: srv.URL, User: "dominik", Password: "nas-pw", Remote: "/home/x"}
	r := CheckWebDAV(ctx, in)
	if r.OK() || r.Cert == nil || r.Cert.Pin == "" {
		t.Fatalf("selbstsigniertes Zertifikat nicht gemeldet: %+v", r.Steps)
	}
	in.CertPin = r.Cert.Pin
	if r := CheckWebDAV(ctx, in); !r.OK() {
		t.Fatalf("mit Pin: %+v", r.Steps)
	}
	in.CertPin = strings.Repeat("0", 64)
	if r := CheckWebDAV(ctx, in); r.OK() {
		t.Fatal("falscher Pin akzeptiert")
	}
}

func TestCandidates(t *testing.T) {
	c, _, _ := candidates("synology", "diskstation")
	if c[0].url("/home/") != "https://diskstation:5006/home/" || c[1].url("/") != "http://diskstation:5005/" {
		t.Fatalf("%+v", c)
	}
	c, p, _ := candidates("qnap", "https://nas.local:8443/Public/x")
	if len(c) != 1 || c[0].port != "8443" || p != "/Public/x" {
		t.Fatalf("%+v %s", c, p)
	}
	if fp := folderPath("nextcloud", "", "dominik", "sshtermy"); fp != "/remote.php/dav/files/dominik/sshtermy/" {
		t.Fatal(fp)
	}
}
