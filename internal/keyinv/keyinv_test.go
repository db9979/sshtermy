package keyinv

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func keyLine(t *testing.T, rsaBits int, comment string) string {
	t.Helper()
	var pub any
	if rsaBits > 0 {
		k, err := rsa.GenerateKey(rand.Reader, rsaBits)
		if err != nil {
			t.Fatal(err)
		}
		pub = &k.PublicKey
	} else {
		p, _, _ := ed25519.GenerateKey(rand.Reader)
		pub = p
	}
	pk, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pk))) + " " + comment
}

func TestParseMerge(t *testing.T) {
	alice := keyLine(t, 0, "alice@laptop")
	old := keyLine(t, 1024, "old")
	out1 := "root=1\nkey=root|/root/.ssh/authorized_keys|" + alice + "\nkey=deploy|/home/deploy/.ssh/authorized_keys|from=\"10.0.0.1\" " + old + "\nkey=root|/root/.ssh/authorized_keys|kaputt\n"
	out2 := "key=root|/root/.ssh/authorized_keys|" + alice + "\n"
	h1, h2 := Parse(out1), Parse(out2)
	if !h1.Root || len(h1.Entries) != 2 || h1.Bad != 1 || h2.Root {
		t.Fatalf("%+v %+v", h1, h2)
	}
	if h1.Entries[1].Options != `from="10.0.0.1"` || h1.Entries[1].Bits != 1024 || h1.Entries[1].Weak() != "RSA 1024" {
		t.Fatalf("%+v", h1.Entries[1])
	}
	own := map[string]string{h1.Entries[0].FP: "sshTermy (id_ed25519)"}
	keys := Merge(map[string]Host{"a": h1, "b": h2}, map[string]string{"a": "web01", "b": "web02"}, own)
	if len(keys) != 2 || keys[0].Weak() == "" {
		t.Fatalf("schwache zuerst: %+v", keys)
	}
	if keys[1].Hosts() != 2 || keys[1].Own == "" || keys[1].Label() != "sshTermy (id_ed25519)" || keys[1].Places[0].Host != "web01" {
		t.Fatalf("%+v", keys[1])
	}
}

func TestRemoveScript(t *testing.T) {
	b := "AAAAC3NzaC1lZDI1NTE5AAAAIExample+/="
	if !ValidBlob(b) || ValidBlob("x'; rm -rf /") {
		t.Fatal("ValidBlob")
	}
	s := RemoveScript("/home/o'neil/.ssh/authorized_keys", b, "20261114")
	if !strings.Contains(s, `f='/home/o'\''neil/.ssh/authorized_keys'`) || !strings.Contains(s, "cp -p") || !strings.Contains(s, `cat "$f.sshtermy-tmp" > "$f"`) {
		t.Fatal(s)
	}
}
