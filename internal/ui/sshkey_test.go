package ui

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/keychain"
	"github.com/db9979/sshtermy/internal/sshx"
	"github.com/db9979/sshtermy/internal/store"
)

func newKeyTestApp(t *testing.T) (*App, *keychain.Memory) {
	t.Helper()
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	mem := &keychain.Memory{}
	old := keychain.Use(mem)
	t.Cleanup(func() { keychain.Use(old) })
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
	return a, mem
}

func setField(f *form, key, val string) { f.get(key).in.SetValue(val) }

// Passwort im Schlüsselbund: nicht im Store, beim Bearbeiten vorbelegt,
// Schalter aus holt es zurück in den Store.
func TestConnFormKeychain(t *testing.T) {
	a, mem := newKeyTestApp(t)
	a.openConnForm(domain.Connection{Port: 22, Auth: domain.AuthPassword})
	f := a.form
	setField(f, "alias", "Spark")
	setField(f, "host", "spark")
	setField(f, "user", "dominik")
	setField(f, "password", "geheim")
	f.get("keychain").on = true
	if err := a.saveConn(); err != nil {
		t.Fatal(err)
	}
	cs := a.st.Connections()
	if len(cs) != 1 || cs[0].Password.IsSet() || !cs[0].Keychain {
		t.Fatalf("Store: %+v", cs)
	}
	id := cs[0].ID
	if mem.M[id+"/password"] != "geheim" {
		t.Fatalf("Schlüsselbund: %v", mem.M)
	}

	// erneut öffnen: Feld zeigt das Passwort aus dem Schlüsselbund
	a.openConnForm(cs[0])
	if a.form.raw("password") != "geheim" || !a.form.flag("keychain") {
		t.Fatal("Formular nicht aus dem Schlüsselbund vorbelegt")
	}
	a.form.get("keychain").on = false
	if err := a.saveConn(); err != nil {
		t.Fatal(err)
	}
	c, _ := a.st.Connection(id)
	if c.Password.Reveal() != "geheim" || c.Keychain {
		t.Fatalf("zurück in den Store: %+v", c)
	}
	if len(mem.M) != 0 {
		t.Fatalf("Eintrag nicht gelöscht: %v", mem.M)
	}
}

// Fehlt der Eintrag (anderer Rechner nach dem Sync), wird die Antwort auf
// die Rückfrage nach dem Verbinden gemerkt.
func TestLearnKeychain(t *testing.T) {
	a, mem := newKeyTestApp(t)
	c, err := a.st.UpsertConnection(domain.Connection{Alias: "x", Host: "h", Port: 22, Username: "u", Auth: domain.AuthPassword, Keychain: true})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{conn: c, secrets: &secretCache{}}
	ask := s.secrets.wrap(func(string) (string, error) { return "pw1", nil })
	ask(sshx.PasswordLabel(c))
	a.learnKeychain(s)
	if mem.M[c.ID+"/password"] != "pw1" {
		t.Fatalf("nicht gemerkt: %v", mem.M)
	}
}

// keyServer: SSH-Server, der Passwort „pw“ und Keys aus HOME/.ssh/
// authorized_keys annimmt und Exec-Befehle mit sh im Test-HOME ausführt.
func keyServer(t *testing.T, home string) (host string, port int) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("keine sh")
	}
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	hk, _ := ssh.NewSignerFromKey(priv)
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
			if string(p) == "pw" {
				return nil, nil
			}
			return nil, ssh.ErrNoAuth
		},
		PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			b, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
			for len(b) > 0 {
				pk, _, _, rest, err := ssh.ParseAuthorizedKey(b)
				if err != nil {
					break
				}
				if bytes.Equal(pk.Marshal(), k.Marshal()) {
					return nil, nil
				}
				b = rest
			}
			return nil, ssh.ErrNoAuth
		},
	}
	cfg.AddHostKey(hk)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				sc, chans, reqs, err := ssh.NewServerConn(nc, cfg)
				if err != nil {
					return
				}
				defer sc.Close()
				go ssh.DiscardRequests(reqs)
				for nch := range chans {
					ch, creqs, _ := nch.Accept()
					go func() {
						for r := range creqs {
							if r.Type != "exec" {
								r.Reply(false, nil)
								continue
							}
							r.Reply(true, nil)
							l := binary.BigEndian.Uint32(r.Payload)
							cmd := exec.Command(sh, "-c", string(r.Payload[4:4+l]))
							cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
							cmd.Stdout, cmd.Stderr = ch, ch.Stderr()
							code := uint32(0)
							if err := cmd.Run(); err != nil {
								code = 1
							}
							st := make([]byte, 4)
							binary.BigEndian.PutUint32(st, code)
							ch.SendRequest("exit-status", false, st)
							ch.Close()
							return
						}
					}()
				}
			}()
		}
	}()
	h, p, _ := net.SplitHostPort(ln.Addr().String())
	port, _ = strconv.Atoi(p)
	return h, port
}

// Ganzer Ablauf: Key erzeugen, per Passwort übertragen, mit dem Key
// probeweise anmelden, Verbindung umstellen, Passphrase im Schlüsselbund.
func TestSSHKeyGenerateInstallSwitch(t *testing.T) {
	a, mem := newKeyTestApp(t)
	remoteHome := t.TempDir()
	host, port := keyServer(t, remoteHome)
	c, err := a.st.UpsertConnection(domain.Connection{Alias: "Test Box", Host: host, Port: port, Username: "u",
		Auth: domain.AuthPassword, Device: domain.DeviceServer})
	if err != nil {
		t.Fatal(err)
	}
	// Rückfragen beantworten: Host-Key ja, Passwort „pw“
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case m := <-a.events:
				switch m := m.(type) {
				case hostKeyAskMsg:
					m.reply <- true
				case secretAskMsg:
					m.reply <- "pw"
				}
			case <-stop:
				return
			}
		}
	}()

	keyFile := filepath.Join(t.TempDir(), "id_test")
	a.scr = scrManager
	a.openSSHKeyForm(c)
	setField(a.form, "file", keyFile)
	setField(a.form, "pass", "kp")
	cmd, err := a.startSSHKey()
	if err != nil {
		t.Fatal(err)
	}
	msg := cmd()
	done, ok := msg.(sshKeyDoneMsg)
	if !ok {
		t.Fatalf("msg %T", msg)
	}
	if done.err != nil {
		t.Fatalf("%s: %v", done.step, done.err)
	}
	if !done.wrote || !done.installed || !done.switched {
		t.Fatalf("Ergebnis: %+v", done)
	}
	var _ tea.Cmd = a.sshKeyDone(done)
	if a.modal == nil || !strings.Contains(a.modal.body, "authorized_keys") {
		t.Fatalf("Dialog: %+v", a.modal)
	}
	got, _ := a.st.Connection(c.ID)
	if got.Auth != domain.AuthKeyFile || got.KeyPath == "" || got.Password.IsSet() || got.KeyPassphrase.IsSet() || !got.Keychain {
		t.Fatalf("Verbindung nicht umgestellt: %+v", got)
	}
	if mem.M[c.ID+"/passphrase"] != "kp" {
		t.Fatalf("Passphrase nicht im Schlüsselbund: %v", mem.M)
	}
	ak, _ := os.ReadFile(filepath.Join(remoteHome, ".ssh", "authorized_keys"))
	pub, _ := os.ReadFile(keyFile + ".pub")
	if strings.TrimSpace(string(ak)) != strings.TrimSpace(string(pub)) {
		t.Fatalf("authorized_keys = %q, pub = %q", ak, pub)
	}

	// zweiter Lauf mit dem vorhandenen Key: steht schon drin
	a.openSSHKeyForm(got)
	a.form.get("mode").opt = 1
	found := a.form.get("found")
	found.opt = len(found.options) - 1 // eigener Pfad
	setField(a.form, "keypath", keyFile)
	setField(a.form, "pass", "kp")
	a.form.get("switch").on = false
	cmd, err = a.startSSHKey()
	if err != nil {
		t.Fatal(err)
	}
	done = cmd().(sshKeyDoneMsg)
	if done.err != nil || !done.already {
		t.Fatalf("zweiter Lauf: %+v", done)
	}
}
