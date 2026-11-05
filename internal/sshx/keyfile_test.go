package sshx

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestKeyFileFallbackAndMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	blk, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(home, ".ssh", "id_ed25519")
	_ = os.MkdirAll(filepath.Dir(key), 0o700)
	if err := os.WriteFile(key, pem.EncodeToMemory(blk), 0o600); err != nil {
		t.Fatal(err)
	}

	// Windows-Pfad vom anderen Rechner: gleicher Ort unter dem eigenen Home.
	c := domain.Connection{ID: "x", Alias: "a", Auth: domain.AuthKeyFile, KeyPath: `C:\Users\db9\.ssh\id_ed25519`}
	if _, err := loadSigner(c, Options{}); err != nil {
		t.Fatalf("Rückfall auf ~/.ssh: %v", err)
	}
	if err := CheckKeyFile(key); err != nil {
		t.Fatal(err)
	}
	if got := LocalKeys(); len(got) != 1 || got[0] != "~/.ssh/id_ed25519" {
		t.Fatalf("LocalKeys = %q", got)
	}

	c.KeyPath = `C:\Users\db9\AppData\Local\NVIDIA Corporation\Sync\config\nvsync.key`
	_, err = loadSigner(c, Options{})
	var km *KeyMissingError
	if !errors.As(err, &km) || km.ConnID != "x" || km.Path != c.KeyPath || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("erwartet KeyMissingError, bekam %v", err)
	}
}
