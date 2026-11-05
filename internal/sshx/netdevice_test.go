package sshx

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
)

// fakeSwitch verhält sich wie ein Aruba/ProCurve-Switch: genau eine Shell,
// und bei einer Exec-Anfrage legt er die ganze Verbindung auf.
type fakeSwitch struct {
	addr   string
	execs  atomic.Int32
	shells atomic.Int32
	// busy: so lange nach dem Auflegen lehnt der Switch neue Verbindungen
	// mitten im Handshake ab (Sitzung noch nicht freigegeben).
	busy      time.Duration
	busyUntil atomic.Int64
}

func startFakeSwitch(t *testing.T, banner string) *fakeSwitch {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil },
		ServerVersion:    banner,
	}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	fs := &fakeSwitch{addr: ln.Addr().String()}
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			if time.Now().UnixNano() < fs.busyUntil.Load() {
				nc.Close() // → "ssh: handshake failed: EOF" beim Client
				continue
			}
			go fs.serve(nc, cfg)
		}
	}()
	return fs
}

func (fs *fakeSwitch) serve(nc net.Conn, cfg *ssh.ServerConfig) {
	sc, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		ch, creqs, err := nch.Accept()
		if err != nil {
			return
		}
		go func() {
			for r := range creqs {
				switch r.Type {
				case "exec":
					fs.execs.Add(1)
					fs.busyUntil.Store(time.Now().Add(fs.busy).UnixNano())
					sc.Close() // wie der Switch: Verbindung weg → EOF beim Client
					return
				case "pty-req":
					r.Reply(true, nil)
				case "shell":
					fs.shells.Add(1)
					r.Reply(true, nil)
					ch.Write([]byte("switch# "))
				default:
					r.Reply(false, nil)
				}
			}
		}()
	}
}

func dialFake(t *testing.T, fs *fakeSwitch, dev domain.DeviceKind) *Client {
	t.Helper()
	hs, err := NewHostKeyStore(filepath.Join(t.TempDir(), "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(fs.addr)
	p, _ := strconv.Atoi(port)
	c := domain.Connection{Alias: "sw", Host: host, Port: p, Username: "admin",
		Auth: domain.AuthPassword, Password: "x", Device: dev}
	cl, err := Dial(context.Background(), c, Options{
		HostKeys:  hs,
		Prompt:    func(string, string, string) bool { return true },
		Timeout:   5 * time.Second,
		waitScale: 0.05,
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { cl.Close() })
	return cl
}

func expectPrompt(t *testing.T, cl *Client) {
	t.Helper()
	var buf safeBuf
	sh, err := cl.OpenShell(80, 24, &buf)
	if err != nil {
		t.Fatalf("OpenShell: %v", err)
	}
	defer sh.Close()
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(buf.String(), "switch#") {
		select {
		case err := <-sh.Done:
			t.Fatalf("Shell sofort beendet (EOF): %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("kein Switch-Prompt")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Unbekanntes Banner (z. B. Aruba mit eigener Kennung): die Systemabfrage
// killt die Verbindung, Dial muss das merken und ohne Abfrage neu verbinden.
func TestSwitchHangsUpOnExecFallsBack(t *testing.T) {
	fs := startFakeSwitch(t, "SSH-2.0-Unbekannt_1.0")
	cl := dialFake(t, fs, domain.DeviceAuto)
	if !cl.NetDevice {
		t.Fatal("Netzwerkgerät nicht erkannt")
	}
	expectPrompt(t, cl)
	if fs.execs.Load() != 1 {
		t.Fatalf("erwartet genau eine Exec-Probe, waren %d", fs.execs.Load())
	}
}

// Wie bei Dominiks Aruba: nach dem Auflegen ist der Switch kurz belegt. Der
// zweite Aufbau darf dann nicht mit "handshake failed: EOF" scheitern.
func TestSwitchBusyAfterHangupRetries(t *testing.T) {
	fs := startFakeSwitch(t, "SSH-2.0-Unbekannt_1.0")
	fs.busy = 150 * time.Millisecond // länger als die erste Wartezeit (75 ms)
	cl := dialFake(t, fs, domain.DeviceAuto)
	if !cl.NetDevice || !cl.Learned {
		t.Fatalf("NetDevice=%v Learned=%v, erwartet beides", cl.NetDevice, cl.Learned)
	}
	expectPrompt(t, cl)
}

// Bekanntes Banner oder Gerätetyp "network": gar keine Exec-Anfrage.
func TestNetworkDeviceSkipsProbe(t *testing.T) {
	for _, tc := range []struct {
		banner string
		dev    domain.DeviceKind
	}{
		{"SSH-2.0-RomSShell_5.40", domain.DeviceAuto},
		{"SSH-2.0-Cisco-1.25", domain.DeviceAuto},
		{"SSH-2.0-OpenSSH_8.9", domain.DeviceNetwork},
	} {
		fs := startFakeSwitch(t, tc.banner)
		cl := dialFake(t, fs, tc.dev)
		if !cl.NetDevice {
			t.Fatalf("%s: nicht als Netzwerkgerät behandelt", tc.banner)
		}
		expectPrompt(t, cl)
		if n := fs.execs.Load(); n != 0 {
			t.Fatalf("%s: %d Exec-Anfragen, erwartet keine", tc.banner, n)
		}
	}
}

func TestIsNetworkBanner(t *testing.T) {
	for _, b := range []string{"SSH-2.0-Cisco-1.25", "SSH-2.0-ROSSSH", "SSH-2.0-HUAWEI-1.5", "SSH-2.0-Mocana SSH 6.3"} {
		if !IsNetworkBanner(b) {
			t.Errorf("%s nicht erkannt", b)
		}
	}
	for _, b := range []string{"SSH-2.0-OpenSSH_9.6p1 Ubuntu-3ubuntu13", "SSH-2.0-dropbear_2022.83"} {
		if IsNetworkBanner(b) {
			t.Errorf("%s fälschlich als Netzwerkgerät", b)
		}
	}
}

type safeBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *safeBuf) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buf.String() }

// Systemprofil: ProCurve an der Mocana-Kennung erkannt, ohne Exec; ein
// früher als Netzwerkgerät erkanntes Profil verhindert die Abfrage ebenfalls.
func TestProfileFromBannerWithoutExec(t *testing.T) {
	fs := startFakeSwitch(t, "SSH-2.0-Mocana SSH 6.3")
	cl := dialFake(t, fs, domain.DeviceAuto)
	if cl.Profile.Dialect != "procurve" || cl.Profile.Family != domain.FamilyNetwork {
		t.Fatalf("Profil = %+v, erwartet ProCurve", cl.Profile)
	}
	expectPrompt(t, cl)
	if n := fs.execs.Load(); n != 0 {
		t.Fatalf("%d Exec-Anfragen, erwartet keine", n)
	}
}

func TestStoredNetworkProfileSkipsProbe(t *testing.T) {
	fs := startFakeSwitch(t, "SSH-2.0-OpenSSH_8.9") // z. B. ArubaOS-CX
	hs, _ := NewHostKeyStore(filepath.Join(t.TempDir(), "known_hosts"))
	host, port, _ := net.SplitHostPort(fs.addr)
	p, _ := strconv.Atoi(port)
	c := domain.Connection{Alias: "cx", Host: host, Port: p, Username: "admin", Auth: domain.AuthPassword, Password: "x",
		Profile: &domain.SystemProfile{Family: domain.FamilyNetwork, Dialect: "arubaos-cx"}}
	cl, err := Dial(context.Background(), c, Options{HostKeys: hs, Prompt: func(string, string, string) bool { return true },
		Timeout: 5 * time.Second, waitScale: 0.05})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if !cl.NetDevice || fs.execs.Load() != 0 {
		t.Fatalf("NetDevice=%v execs=%d", cl.NetDevice, fs.execs.Load())
	}
}
