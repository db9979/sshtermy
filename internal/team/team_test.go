package team

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

func sample() *domain.Data {
	d := domain.NewData()
	d.Connections = []domain.Connection{
		{ID: "jump", Alias: "Bastion", Host: "bastion", Username: "db9", Auth: domain.AuthPassword, Password: "geheim1"},
		{ID: "web", Alias: "Web", Host: "web1", Username: "db9", Auth: domain.AuthKeyFile, KeyPath: `C:\Users\db9\key`, KeyPassphrase: "passphrase-xyz",
			JumpHostID: "jump", Group: "Prod", QuickCmds: []string{"uptime"},
			Profile: &domain.SystemProfile{OS: "Ubuntu", Notes: "nie reboot", Failed: []string{"rm -rf /"}}},
		{ID: "home", Alias: "NAS", Host: "nas", Username: "admin", Auth: domain.AuthKeyFile, KeyPEM: "-----BEGIN"},
	}
	d.Tunnels = []domain.Tunnel{{ID: "t1", ConnectionID: "web", Kind: domain.TunnelLocal, BindPort: 8080}, {ID: "t2", ConnectionID: "home"}}
	d.Runbooks = []domain.Runbook{{ID: "r1", Name: "Logs", ConnectionID: "web", Uses: 9}, {ID: "r2", Name: "Allgemein"}, {ID: "r3", ConnectionID: "home"}}
	d.LLM.APIKey = "sk-geheim"
	return d
}

func TestRoundTripWithoutSecrets(t *testing.T) {
	d := sample()
	p, jumps := Build(d, []string{"web"}, "Dominik", Options{Runbooks: true, QuickCmds: true, Tunnels: true}, time.Now())
	if len(jumps) != 1 || jumps[0] != "jump" || len(p.Connections) != 2 {
		t.Fatalf("Jump-Host fehlt: %v %d", jumps, len(p.Connections))
	}
	raw, err := Seal(p, []byte("team-passwort"))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"geheim1", "passphrase-xyz", "sk-geheim", "BEGIN", "Dominik", "web1", `C:\\Users`, "rm -rf"} {
		if strings.Contains(string(raw), bad) {
			t.Fatalf("Klartext %q in der Datei", bad)
		}
	}
	if _, err := Open(raw, []byte("falsch-falsch")); err != ErrWrongPass {
		t.Fatalf("falsches Passwort: %v", err)
	}
	q, err := Open(raw, []byte("team-passwort"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range q.Connections {
		if c.Password.IsSet() || c.KeyPEM.IsSet() || c.KeyPassphrase.IsSet() || c.KeyPath != "" || c.Username != "" {
			t.Fatalf("persönliche Daten mitgegeben: %#v", c)
		}
		if c.ID == "web" && (len(c.QuickCmds) != 1 || c.Profile.Notes != "nie reboot" || len(c.Profile.Failed) != 0) {
			t.Fatalf("web: %#v %#v", c, c.Profile)
		}
	}
	if len(q.Tunnels) != 1 || len(q.Runbooks) != 2 || q.Runbooks[0].Uses != 0 || q.From != "Dominik" {
		t.Fatalf("Tunnel/Runbooks: %+v", q)
	}
	if _, err := Seal(p, []byte("kurz")); err == nil {
		t.Fatal("kurzes Passwort angenommen")
	}
	if len(Passphrase()) != 19 {
		t.Fatal(Passphrase())
	}
}

func TestApplyKeepsOwnCredentials(t *testing.T) {
	p, _ := Build(sample(), []string{"web"}, "", Options{Runbooks: true, Tunnels: true, QuickCmds: true}, time.Now())
	// Empfänger hat „web“ schon (früher übernommen) mit eigenem Passwort.
	d := domain.NewData()
	d.Connections = []domain.Connection{{ID: "web", Alias: "alt", Host: "alt", Username: "kollege", Auth: domain.AuthPassword, Password: "meins", Group: "Team / Prod", Order: 1}}
	d.Deleted = map[string]time.Time{"jump": time.Now()}
	st := Apply(d, p, nil, "Team", "kollege", time.Now())
	if st.Added != 1 || st.Updated != 1 || st.Tunnels != 1 || st.Runbooks != 2 {
		t.Fatalf("%+v", st)
	}
	var web, jump domain.Connection
	for _, c := range d.Connections {
		switch c.ID {
		case "web":
			web = c
		case "jump":
			jump = c
		}
	}
	if web.Password != "meins" || web.Username != "kollege" || web.Host != "web1" || web.Alias != "Web" || web.Group != "Team / Prod" || web.JumpHostID != "jump" {
		t.Fatalf("web: %#v", web)
	}
	if jump.Username != "kollege" || jump.Group != "Team" || jump.Auth != domain.AuthPassword || jump.Password.IsSet() {
		t.Fatalf("jump: %#v", jump)
	}
	if _, gone := d.Deleted["jump"]; gone {
		t.Fatal("Grabstein bleibt, Sync würde den Eintrag wieder löschen")
	}
	if st.NeedPw != 1 {
		t.Fatalf("NeedPw=%d", st.NeedPw)
	}
	// Nur gewählte übernehmen
	d2 := domain.NewData()
	st = Apply(d2, p, map[string]bool{"jump": true}, "", "", time.Now())
	if st.Added != 1 || st.Tunnels != 0 || st.Runbooks != 1 {
		t.Fatalf("Auswahl: %+v", st)
	}
}

func TestLinkPublishPull(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	src := domain.NewData()
	src.Connections = []domain.Connection{
		{ID: "a", Alias: "web", Host: "10.0.0.1", Group: "Kunden / ACME", Password: "geheim", Auth: domain.AuthPassword},
		{ID: "b", Alias: "db", Host: "10.0.0.2", Group: "Kunden / ACME / DB"},
		{ID: "c", Alias: "privat", Host: "10.0.0.3", Group: "Privat"},
	}
	pub := domain.TeamLink{Role: domain.TeamPublish, Folder: "Kunden / ACME", TeamPass: "team-pass-123"}
	p, sum1 := BuildLink(src, pub, now)
	if len(p.Connections) != 2 {
		t.Fatalf("Ordner: %d Verbindungen", len(p.Connections))
	}
	if p.Connections[0].Group != "ACME" || p.Connections[1].Group != "ACME / DB" {
		t.Fatalf("Ordner relativ: %q %q", p.Connections[0].Group, p.Connections[1].Group)
	}
	if _, again := BuildLink(src, pub, now.Add(time.Hour)); again != sum1 {
		t.Fatal("Prüfsumme hängt an der Uhrzeit")
	}
	dir := t.TempDir()
	loc := filepath.Join(dir, "acme.sshteam")
	raw, err := Seal(p, []byte(pub.TeamPass))
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteLoc(context.Background(), loc, raw); err != nil {
		t.Fatal(err)
	}

	dst := domain.NewData()
	sub := domain.TeamLink{Role: domain.TeamSubscribe, Path: loc, Into: "Team", DefaultUser: "kollege"}
	got, err := ReadLoc(context.Background(), loc)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := Open(got, []byte("team-pass-123"))
	if err != nil {
		t.Fatal(err)
	}
	st, have := Pull(dst, pk, sub, now)
	if st.Added != 2 || len(have) != 2 || dst.Connections[0].Group != "Team / ACME" || dst.Connections[0].Username != "kollege" {
		t.Fatalf("erste Übernahme: %+v %v %+v", st, have, dst.Connections)
	}
	sub.Have = have

	// Absender löscht db, benennt web um, legt neue an
	src.Connections = []domain.Connection{
		{ID: "a", Alias: "web-neu", Host: "10.0.0.1", Group: "Kunden / ACME"},
		{ID: "d", Alias: "mail", Host: "10.0.0.4", Group: "Kunden / ACME"},
	}
	p2, sum2 := BuildLink(src, pub, now)
	if sum2 == sum1 {
		t.Fatal("Änderung nicht erkannt")
	}
	st, have = Pull(dst, p2, sub, now)
	if st.Added != 1 || st.Updated != 1 || st.Removed != 1 || len(have) != 2 {
		t.Fatalf("zweite Übernahme: %+v %v", st, have)
	}
	if _, ok := dst.Deleted["b"]; !ok {
		t.Fatal("kein Grabstein für die entfernte Verbindung")
	}
	for _, c := range dst.Connections {
		if c.ID == "a" && (c.Alias != "web-neu" || c.Username != "kollege") {
			t.Fatalf("Aktualisierung: %+v", c)
		}
	}

	// Abgewählte Verbindung bleibt draußen und wird nicht gelöscht
	sub.Have, sub.Skip = have, []string{"d"}
	dst.Connections = append(dst.Connections, domain.Connection{ID: "x", Alias: "eigene"})
	st, _ = Pull(dst, p2, sub, now)
	if st.Removed != 0 || st.Updated != 1 {
		t.Fatalf("mit Skip: %+v", st)
	}
	if len(dst.Connections) != 3 {
		t.Fatalf("eigene Verbindung angefasst: %+v", dst.Connections)
	}
	if _, err := ReadLoc(context.Background(), filepath.Join(dir, "fehlt.sshteam")); err != ErrNotFound {
		t.Fatalf("fehlende Datei: %v", err)
	}
}

func TestLocHTTP(t *testing.T) {
	var stored []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "nas" || p != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodPut:
			stored, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			if stored == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Write(stored)
		}
	}))
	defer srv.Close()
	loc := strings.Replace(srv.URL, "http://", "http://nas:pw@", 1) + "/team/acme.sshteam"
	if ShowLoc(loc) != strings.Replace(srv.URL, "http://", "http://nas@", 1)+"/team/acme.sshteam" {
		t.Fatalf("Passwort sichtbar: %s", ShowLoc(loc))
	}
	if _, err := ReadLoc(context.Background(), loc); err != ErrNotFound {
		t.Fatalf("leer: %v", err)
	}
	if err := WriteLoc(context.Background(), loc, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if b, err := ReadLoc(context.Background(), loc); err != nil || string(b) != "x" {
		t.Fatalf("lesen: %q %v", b, err)
	}
	if _, err := ReadLoc(context.Background(), srv.URL+"/team/acme.sshteam"); err == nil {
		t.Fatal("ohne Anmeldung gelesen")
	}
}
