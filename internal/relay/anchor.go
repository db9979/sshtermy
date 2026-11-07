package relay

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/i18n"
)

// AnchorConfig: was der Anker auf dem Server weiß (anker.json, 0600).
type AnchorConfig struct {
	Base   string `json:"base"`
	FP     string `json:"fp,omitempty"`
	ID     string `json:"id"`
	Key    string `json:"key"`
	Name   string `json:"name"`
	Target string `json:"ziel"` // sshd, zu dem der Anker Kanäle öffnet (Standard 127.0.0.1:22)
}

// DefaultTarget: das eigene sshd. Der Anker öffnet Kanäle nur hierhin, egal
// was der Vermittler schickt.
const DefaultTarget = "127.0.0.1:22"

// DefaultAnchorPath: als root /etc/sshtermy-anker/anker.json, sonst im
// Konfigurationsordner des Benutzers.
func DefaultAnchorPath() string {
	if runtime.GOOS != "windows" && os.Geteuid() == 0 {
		return "/etc/sshtermy-anker/anker.json"
	}
	d, err := os.UserConfigDir()
	if err != nil {
		d = "."
	}
	return filepath.Join(d, "sshtermy", "anker.json")
}

func LoadAnchor(path string) (AnchorConfig, error) {
	var c AnchorConfig
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.Target == "" {
		c.Target = DefaultTarget
	}
	return c, nil
}

func SaveAnchor(path string, c AnchorConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return writeAtomic(path, b)
}

var errStop = errors.New("stop")

// ProbeHostKey fragt das sshd nach seinem Host-Key (ohne Anmeldung), damit
// der Vermittler ihn sshTermy zum Prüfen mitgeben kann.
func ProbeHostKey(target string) (string, error) {
	c, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf(i18n.T("sshd unter %s nicht erreichbar: %w"), target, err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	var got ssh.PublicKey
	cfg := &ssh.ClientConfig{User: "sshtermy-anker",
		HostKeyAlgorithms: []string{ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256},
		HostKeyCallback: func(_ string, _ net.Addr, k ssh.PublicKey) error {
			got = k
			return errStop
		}}
	_, _, _, err = ssh.NewClientConn(c, target, cfg)
	if got == nil {
		return "", fmt.Errorf(i18n.T("Host-Key nicht lesbar: %w"), err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(got))), nil
}

// OSName: kurze Beschreibung des Systems (Anzeige in sshTermy).
func OSName() string {
	switch runtime.GOOS {
	case "linux":
		if f, err := os.Open("/etc/os-release"); err == nil {
			defer f.Close()
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				if v, ok := strings.CutPrefix(sc.Text(), "PRETTY_NAME="); ok {
					return clean(strings.Trim(v, `"`), 60)
				}
			}
		}
		return "Linux"
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	}
	return runtime.GOOS
}

// PairAnchor koppelt diesen Server mit dem Vermittler.
func PairAnchor(ctx context.Context, line PairLine, nm, target string) (AnchorConfig, error) {
	if target == "" {
		target = DefaultTarget
	}
	hk, err := ProbeHostKey(target)
	if err != nil {
		return AnchorConfig{}, err
	}
	if nm == "" {
		nm, _ = os.Hostname()
	}
	cl, err := NewClient(line.Base, "", line.FP)
	if err != nil {
		return AnchorConfig{}, err
	}
	var r struct{ ID, Key string }
	err = cl.do(ctx, http.MethodPost, PathAnchorPair, map[string]string{"code": line.Code, "name": nm, "os": OSName(), "hostkey": hk}, &r)
	if err != nil {
		return AnchorConfig{}, err
	}
	if r.ID == "" || r.Key == "" {
		return AnchorConfig{}, i18n.NewError("Vermittler hat keinen Schlüssel geschickt")
	}
	return AnchorConfig{Base: cl.Base, FP: line.FP, ID: r.ID, Key: r.Key, Name: name(nm, "Server"), Target: target}, nil
}

// Anchor hält die Leitung zum Vermittler und öffnet Kanäle zum eigenen sshd.
type Anchor struct {
	Cfg  AnchorConfig
	Logf func(string, ...any)
	cl   *Client
}

// Run läuft, bis ctx endet; nach Abbrüchen verbindet er neu (1 s bis 2 min).
func (a *Anchor) Run(ctx context.Context) error {
	if a.Logf == nil {
		a.Logf = log.Printf
	}
	cl, err := NewClient(a.Cfg.Base, a.Cfg.Key, a.Cfg.FP)
	if err != nil {
		return err
	}
	a.cl = cl
	if a.Cfg.Target == "" {
		a.Cfg.Target = DefaultTarget
	}
	wait := time.Second
	for {
		start := time.Now()
		err := a.once(ctx)
		if ctx.Err() != nil {
			return nil
		}
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden) {
			a.Logf("anker: abgewiesen | %d", se.Code)
			wait = 5 * time.Minute // entfernt: selten nachfragen
		} else {
			a.Logf("anker: getrennt | %s", errWord(err))
			if time.Since(start) > time.Minute {
				wait = time.Second
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		if wait < 2*time.Minute {
			wait *= 2
		}
	}
}

func errWord(err error) string {
	if err == nil {
		return "ende"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "zeit"
	}
	return "fehler"
}

func (a *Anchor) dialWS(ctx context.Context, path string) (*websocket.Conn, error) {
	h := http.Header{}
	a.cl.auth(h)
	dctx, c := context.WithTimeout(ctx, 20*time.Second)
	defer c()
	ws, resp, err := websocket.Dial(dctx, a.cl.Base+path, &websocket.DialOptions{HTTPClient: a.cl.http, HTTPHeader: h})
	if err != nil && resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
		return nil, &StatusError{Code: resp.StatusCode}
	}
	return ws, err
}

func (a *Anchor) once(ctx context.Context) error {
	ws, err := a.dialWS(ctx, PathAnchor)
	if err != nil {
		return err
	}
	defer ws.Close(websocket.StatusNormalClosure, "")
	ws.SetReadLimit(ctrlLimit)
	a.Logf("anker: verbunden | 1")
	hk, _ := ProbeHostKey(a.Cfg.Target)
	hello, _ := json.Marshal(map[string]string{"typ": "hallo", "os": OSName(), "hostkey": hk})
	if err := ws.Write(ctx, websocket.MessageText, hello); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		t := time.NewTicker(pingEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, c := context.WithTimeout(ctx, 20*time.Second)
				err := ws.Ping(pctx)
				c()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	for {
		typ, b, err := ws.Read(ctx)
		if err != nil {
			return err
		}
		if typ != websocket.MessageText {
			continue
		}
		var m struct{ Typ, ID string }
		if json.Unmarshal(b, &m) != nil || m.Typ != "kanal" || len(m.ID) != 32 {
			continue
		}
		go a.channel(ctx, m.ID)
	}
}

// channel: ein Kanal, immer nur zum eigenen sshd (Cfg.Target).
func (a *Anchor) channel(ctx context.Context, id string) {
	tc, err := net.DialTimeout("tcp", a.Cfg.Target, 5*time.Second)
	if err != nil {
		a.Logf("anker: sshd nicht erreichbar | 0")
		return
	}
	defer tc.Close()
	ws, err := a.dialWS(ctx, PathAnchorChannel+"?id="+url.QueryEscape(id))
	if err != nil {
		a.Logf("anker: kanal_fehler | 0")
		return
	}
	ws.SetReadLimit(-1)
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wc := websocket.NetConn(cctx, ws, websocket.MessageBinary)
	defer wc.Close()
	a.Logf("anker: kanal_auf | 1")
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(wc, tc); done <- struct{}{} }()
	go func() { _, _ = io.Copy(tc, wc); done <- struct{}{} }()
	select {
	case <-done:
	case <-cctx.Done():
	}
	a.Logf("anker: kanal_zu | 1")
}

// SystemdUnit: Dienst-Datei für Linux (systemctl enable --now sshtermy-anker).
func SystemdUnit(binary, cfgPath string) string {
	return `[Unit]
Description=sshTermy-Anker (Verbindung zum Vermittler)
After=network-online.target sshd.service
Wants=network-online.target

[Service]
ExecStart=` + binary + ` anker start --datei ${CREDENTIALS_DIRECTORY}/anker.json
Restart=always
RestartSec=5
DynamicUser=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
NoNewPrivileges=yes
LoadCredential=anker.json:` + cfgPath + `

[Install]
WantedBy=multi-user.target
`
}
