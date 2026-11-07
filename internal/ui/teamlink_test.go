package ui

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/store"
	"github.com/db9979/sshtermy/internal/team"
)

func teamApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	a, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// run führt einen Befehl aus und wendet das Ergebnis an wie Update.
func runTL(t *testing.T, a *App, c tea.Cmd) {
	t.Helper()
	if c == nil {
		t.Fatal("kein Befehl")
	}
	m, ok := c().(teamLinkMsg)
	if !ok {
		t.Fatal("keine teamLinkMsg")
	}
	m.apply(a)
}

func TestTeamFolderPublishSubscribe(t *testing.T) {
	loc := filepath.Join(t.TempDir(), "acme.sshteam")

	pub := teamApp(t)
	cs, err := pub.st.AddConnections([]domain.Connection{
		{Alias: "web", Host: "10.0.0.1", Port: 22, Group: "Kunden / ACME", Username: "db9", Auth: domain.AuthPassword, Password: "geheim"},
		{Alias: "db", Host: "10.0.0.2", Port: 22, Group: "Kunden / ACME / DB", Username: "root", Auth: domain.AuthAuto},
		{Alias: "privat", Host: "10.0.0.3", Port: 22, Group: "Privat", Username: "x", Auth: domain.AuthAuto},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := pub.addPublishLink(loc, "team-pass-123", "Dominik", "Kunden / ACME", nil, team.Options{}, ""); err != nil {
		t.Fatal(err)
	}
	runTL(t, pub, pub.publishLink(pub.st.TeamLinks()[0], false))
	if l := pub.st.TeamLinks()[0]; l.LastErr != "" || l.LastSum == "" {
		t.Fatalf("veröffentlichen: %+v", l)
	}
	if pub.publishLink(pub.st.TeamLinks()[0], false) != nil {
		t.Fatal("ohne Änderung darf nicht neu geschrieben werden")
	}

	sub := teamApp(t)
	if err := sub.addSubscribeLink(loc, "team-pass-123", "Team", "kollege", nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	runTL(t, sub, sub.pullLink(sub.st.TeamLinks()[0], false))
	got := map[string]domain.Connection{}
	for _, c := range sub.st.Connections() {
		got[c.Alias] = c
	}
	if len(got) != 2 || got["web"].Group != "Team / ACME" || got["db"].Group != "Team / ACME / DB" || got["web"].Username != "kollege" {
		t.Fatalf("Abo erste Übernahme: %+v", got)
	}
	if got["web"].Password.IsSet() {
		t.Fatal("Passwort ist mitgekommen")
	}

	// Absender: db löschen, neue Verbindung im Ordner
	if err := pub.st.DeleteConnection(cs[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pub.st.AddConnections([]domain.Connection{{Alias: "mail", Host: "10.0.0.4", Port: 22, Group: "Kunden / ACME", Username: "y", Auth: domain.AuthAuto}}); err != nil {
		t.Fatal(err)
	}
	runTL(t, pub, pub.publishLink(pub.st.TeamLinks()[0], false))
	runTL(t, sub, sub.pullLink(sub.st.TeamLinks()[0], false))
	got = map[string]domain.Connection{}
	for _, c := range sub.st.Connections() {
		got[c.Alias] = c
	}
	if len(got) != 2 || got["mail"].ID == "" || got["db"].ID != "" {
		t.Fatalf("Abo nach Änderung: %+v", got)
	}

	// falsches Passwort → Fehler am Link, nichts kaputt
	l := sub.st.TeamLinks()[0]
	l.TeamPass, l.LastSum = "falsch-falsch", ""
	if _, err := sub.st.UpsertTeamLink(l); err != nil {
		t.Fatal(err)
	}
	runTL(t, sub, sub.pullLink(sub.st.TeamLinks()[0], false))
	if sub.st.TeamLinks()[0].LastErr == "" || len(sub.st.Connections()) != 2 {
		t.Fatal("falsches Passwort nicht gemeldet")
	}
}
