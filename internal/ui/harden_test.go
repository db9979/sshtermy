package ui

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	pemlib "encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/keyinv"
)

func pumpEvents(t *testing.T, a *App, cond func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for !cond() {
		select {
		case m := <-a.events:
			a.Update(m)
		case <-deadline:
			t.Fatal("keine Antwort")
		}
	}
}

func TestHardenFolder(t *testing.T) {
	a := vulnTestApp(t)
	for _, n := range []string{"db01", "web01", "kaputt"} {
		if _, err := a.st.UpsertConnection(domain.Connection{Alias: n, Host: n, Username: "root", Group: "Prod"}); err != nil {
			t.Fatal(err)
		}
	}
	old := hardenRunHost
	defer func() { hardenRunHost = old }()
	hardenRunHost = func(*App) func(context.Context, domain.Connection, string) (string, error) {
		return func(_ context.Context, c domain.Connection, _ string) (string, error) {
			switch c.Alias {
			case "kaputt":
				return "", errors.New("Verbindung abgelehnt")
			case "db01":
				return "root=1\nsshd=permitrootlogin yes\nsshd=passwordauthentication yes\nlis=0.0.0.0:6379|\nlis=0.0.0.0:22|\n", nil
			}
			return "root=1\nsshd=permitrootlogin no\nsshd=passwordauthentication no\nfw=ufw Status: active\nauto=unit dnf-automatic.timer\n", nil
		}
	}
	a.openHardenFolder()
	a.pickerChoose(pickItem{value: "Prod"})
	pumpEvents(t, a, func() bool { return !a.hard.busy })
	txt := strings.Join(a.modal.lines, "\n")
	if !a.modal.harden || !strings.Contains(txt, "× Verbindung abgelehnt") {
		t.Fatalf("%s", txt)
	}
	if i, j := strings.Index(txt, "db01"), strings.Index(txt, "web01"); i < 0 || i > j {
		t.Fatalf("db01 (rot) nicht oben:\n%s", txt)
	}
	var db domain.Connection
	for _, c := range a.st.Connections() {
		if c.Alias == "db01" {
			db = c
		}
	}
	r := a.storedHarden(db.ID)
	if b, _, _ := r.Counts(); b < 2 {
		t.Fatalf("nicht gespeichert: %+v", r)
	}
	if len(a.st.VulnReports()) != 0 {
		t.Fatal("Härtung im Sicherheitscheck")
	}
}

func authLine(t *testing.T, comment string) (string, string) {
	t.Helper()
	p, priv, _ := ed25519.GenerateKey(rand.Reader)
	pk, _ := ssh.NewPublicKey(p)
	_ = priv
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pk))) + " " + comment, ssh.FingerprintSHA256(pk)
}

func TestKeyInventoryAndRemove(t *testing.T) {
	a := vulnTestApp(t)
	t.Setenv("HOME", t.TempDir())
	// eigener Schlüssel einer Verbindung: wird nie entfernt
	ownPub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pem, _ := ssh.MarshalPrivateKey(priv, "")
	ownPK, _ := ssh.NewPublicKey(ownPub)
	ownLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(ownPK))) + " sshtermy"
	keyFile := filepath.Join(t.TempDir(), "id_ed25519")
	_ = os.WriteFile(keyFile, pemlib.EncodeToMemory(pem), 0o600)

	for _, n := range []string{"web01", "web02"} {
		c := domain.Connection{Alias: n, Host: n, Username: "admin", Group: "Web"}
		if n == "web02" {
			c.Auth, c.KeyPath = domain.AuthKeyFile, keyFile
		}
		if _, err := a.st.UpsertConnection(c); err != nil {
			t.Fatal(err)
		}
	}
	bob, bobFP := authLine(t, "bob@alt")
	old, oldRm := keyinvRun, keyinvRemove
	defer func() { keyinvRun, keyinvRemove = old, oldRm }()
	keyinvRun = func(*App) func(context.Context, domain.Connection, string) (string, error) {
		return func(_ context.Context, c domain.Connection, _ string) (string, error) {
			out := "root=1\nkey=admin|/home/admin/.ssh/authorized_keys|" + bob + "\n"
			if c.Alias == "web02" {
				out += "key=admin|/home/admin/.ssh/authorized_keys|" + ownLine + "\n"
			}
			return out, nil
		}
	}
	var removed []string
	keyinvRemove = func(*App) func(context.Context, domain.Connection, []keyinv.Place, string) (int, error) {
		return func(_ context.Context, c domain.Connection, ps []keyinv.Place, blob string) (int, error) {
			removed = append(removed, c.Alias)
			return len(ps), nil
		}
	}
	a.openKeyinvFolder()
	a.pickerChoose(pickItem{value: "Web"})
	pumpEvents(t, a, func() bool { return !a.kinv.busy && len(a.kinv.keys) > 0 })
	if a.pick == nil || len(a.pick.items) != 2 {
		t.Fatalf("Liste: %+v", a.pick)
	}
	if a.pick.items[0].value != bobFP || !strings.Contains(a.pick.items[0].hint, "2 Hosts") {
		t.Fatalf("bob nicht oben: %+v", a.pick.items)
	}
	// eigener Schlüssel: Entfernen verweigert
	a.pickerChoose(a.pick.items[1])
	a.Update(keyMsg("d"))
	if a.modal == nil || a.modal.kind == mConfirm {
		t.Fatal("eigener Anmeldeschlüssel ließe sich entfernen")
	}
	// bob von beiden Hosts, web02 auslassen
	a.Update(keyMsg("left"))
	a.pickerChoose(a.pick.items[0])
	a.Update(keyMsg("d"))
	if a.modal == nil || a.modal.kind != mConfirm {
		t.Fatal("keine Bestätigung")
	}
	a.Update(keyMsg("j"))
	a.Update(keyMsg("j"))
	first := ""
	for _, n := range []string{"web01", "web02"} {
		if a.modal != nil && strings.Contains(a.modal.title, n) {
			first = n
		}
	}
	if first == "" {
		t.Fatalf("Host-Bestätigung fehlt: %q", a.modal.title)
	}
	_, cmd := a.Update(keyMsg("j"))
	if cmd != nil {
		a.Update(cmd())
	}
	if a.modal == nil || a.modal.kind != mConfirm || strings.Contains(a.modal.title, first) {
		t.Fatalf("zweiter Host: %q", a.modal.title)
	}
	a.Update(keyMsg("n"))
	if strings.Join(removed, ",") != first {
		t.Fatalf("entfernt: %v", removed)
	}
	k, _ := a.keyinvFind(bobFP)
	if k.Hosts() != 1 {
		t.Fatalf("Inventar nicht nachgeführt: %+v", k)
	}
}
