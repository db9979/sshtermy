package sshx

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
)

// startSFTPServer: SSH-Server mit sftp-Subsystem auf einem echten Ordner.
func startSFTPServer(t *testing.T, root string) string {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	cfg := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil }}
	cfg.AddHostKey(signer)
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
					ch, creqs, err := nch.Accept()
					if err != nil {
						continue
					}
					go func() {
						for r := range creqs {
							ok := r.Type == "subsystem" && len(r.Payload) > 4 && string(r.Payload[4:]) == "sftp"
							_ = r.Reply(ok, nil)
							if ok {
								srv, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(root))
								if err == nil {
									_ = srv.Serve()
								}
								ch.Close()
								return
							}
						}
					}()
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func TestSFTPRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Server-Pfade im Test sind Unix-Pfade")
	}
	remote := t.TempDir()
	cl := dialAddr(t, startSFTPServer(t, remote), domain.DeviceServer)
	r, err := cl.OpenSFTP()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.Label() != "x" || r.Home() == "" {
		t.Fatalf("Label/Home: %q %q", r.Label(), r.Home())
	}
	dir := r.Join(remote, "ordner", "unter")
	if err := r.Mkdir(dir); err != nil {
		t.Fatal(err)
	}
	if r.Dir(dir) != filepath.ToSlash(filepath.Join(remote, "ordner")) && r.Dir(dir) != r.Join(remote, "ordner") {
		t.Fatalf("Dir: %q", r.Dir(dir))
	}
	f := r.Join(dir, "a.txt")
	w, err := r.Create(f, 0o640)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(w, "hallo welt")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Rewrite(f, []byte("neu")); err != nil {
		t.Fatal(err)
	}
	rc, err := r.Open(f)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "neu" {
		t.Fatalf("Inhalt: %q", b)
	}
	e, err := r.Stat(f)
	if err != nil || e.IsDir || e.Size != 3 {
		t.Fatalf("Stat: %+v %v", e, err)
	}
	if err := r.Chmod(f, 0o600); err != nil {
		t.Fatal(err)
	}
	// Umbenennen überschreibt nie
	g := r.Join(dir, "b.txt")
	if err := r.Rewrite(g, []byte("b")); err != nil {
		t.Fatal(err)
	}
	if err := r.Rename(f, g); err == nil || !strings.Contains(err.Error(), "b.txt") {
		t.Fatalf("Umbenennen auf vorhandene Datei: %v", err)
	}
	h := r.Join(dir, "c.txt")
	if err := r.Rename(f, h); err != nil {
		t.Fatal(err)
	}
	es, err := r.ReadDir(dir)
	if err != nil || len(es) != 2 {
		t.Fatalf("ReadDir: %+v %v", es, err)
	}
	if _, err := r.Stat(r.Join(remote, "gibt-es-nicht")); err == nil {
		t.Fatal("Stat auf fehlende Datei ohne Fehler")
	}

	// Kopieren in beide Richtungen mit Fortschritt
	local := t.TempDir()
	var pr Progress
	if err := Copy(context.Background(), r, r.Join(remote, "ordner"), LocalFS{}, local, &pr); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(local, "ordner", "unter", "c.txt"))
	if err != nil || string(got) != "neu" {
		t.Fatalf("heruntergeladen: %q %v", got, err)
	}
	if pr.Files.Load() != 2 {
		t.Fatalf("Fortschritt Dateien: %d", pr.Files.Load())
	}
	up := r.Join(remote, "hoch")
	_ = r.Mkdir(up)
	if err := Copy(context.Background(), LocalFS{}, filepath.Join(local, "ordner"), r, up, &Progress{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(remote, "hoch", "ordner", "unter", "b.txt")); err != nil {
		t.Fatalf("hochgeladen: %v", err)
	}
	n, files, err := Size(context.Background(), r, up)
	if err != nil || files != 2 || n != 4 {
		t.Fatalf("Size: %d %d %v", n, files, err)
	}
	// Abbrechen
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Copy(ctx, r, up, LocalFS{}, t.TempDir(), &Progress{}); err == nil {
		t.Fatal("abgebrochene Kopie ohne Fehler")
	}
	if err := r.RemoveAll(r.Join(remote, "ordner")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(remote, "ordner")); !os.IsNotExist(err) {
		t.Fatalf("RemoveAll: %v", err)
	}
}
