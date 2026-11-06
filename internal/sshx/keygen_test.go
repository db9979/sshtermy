package sshx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateKeyWithPassphrase(t *testing.T) {
	k, err := GenerateKey(KeyEd25519, "me@pc-sshtermy", []byte("pw"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(k.PublicLine, "ssh-ed25519 ") || !strings.HasSuffix(k.PublicLine, " me@pc-sshtermy") {
		t.Fatalf("PublicLine = %q", k.PublicLine)
	}
	if _, err := ssh.ParsePrivateKey(k.PrivatePEM); err == nil {
		t.Fatal("Key ohne Passphrase lesbar")
	}
	s, err := ssh.ParsePrivateKeyWithPassphrase(k.PrivatePEM, []byte("pw"))
	if err != nil {
		t.Fatal(err)
	}
	if string(s.PublicKey().Marshal()) != string(k.Signer.PublicKey().Marshal()) {
		t.Fatal("öffentlicher Teil passt nicht")
	}
	if _, err := LoadKey(k.PrivatePEM, nil); err == nil {
		t.Fatal("LoadKey ohne Passphrase müsste scheitern")
	}
	if l, err := LoadKey(k.PrivatePEM, []byte("pw")); err != nil || !strings.HasPrefix(l.PublicLine, "ssh-ed25519 ") {
		t.Fatal(l, err)
	}
}

func TestWriteKeyFilesNoOverwrite(t *testing.T) {
	k, err := GenerateKey(KeyEd25519, "x", nil)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "sub", "id_test")
	if err := WriteKeyFiles(p, k); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Errorf("Rechte %v", st.Mode().Perm())
	}
	if b, _ := os.ReadFile(p + ".pub"); strings.TrimSpace(string(b)) != k.PublicLine {
		t.Errorf(".pub = %q", b)
	}
	if err := CheckKeyFile(p); err != nil {
		t.Fatal(err)
	}
	if err := WriteKeyFiles(p, k); err == nil {
		t.Fatal("vorhandene Datei überschrieben")
	}
}

func TestInstallScript(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("keine sh")
	}
	k, _ := GenerateKey(KeyEd25519, "a@b-sshtermy", nil)
	script, err := InstallScript(k.PublicLine)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	// vorhandene Datei ohne Zeilenende am Schluss
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o755)
	os.WriteFile(filepath.Join(home, ".ssh", "authorized_keys"), []byte("ssh-rsa AAAAold old"), 0o644)
	run := func() string {
		cmd := exec.Command(sh, "-c", script)
		cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return string(out)
	}
	if out := run(); !strings.Contains(out, markAdded) {
		t.Fatalf("erster Lauf: %q", out)
	}
	if out := run(); !strings.Contains(out, markHave) {
		t.Fatalf("zweiter Lauf: %q", out)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	want := "ssh-rsa AAAAold old\n" + k.PublicLine + "\n"
	if string(b) != want {
		t.Fatalf("authorized_keys = %q", b)
	}
	if st, _ := os.Stat(filepath.Join(home, ".ssh")); st.Mode().Perm() != 0o700 {
		t.Errorf(".ssh Rechte %v", st.Mode().Perm())
	}
	if _, err := InstallScript("ssh-ed25519 AAAA'; rm -rf / #"); err == nil {
		t.Fatal("Anführungszeichen im Key durchgelassen")
	}
}
