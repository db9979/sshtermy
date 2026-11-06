package team

import (
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
