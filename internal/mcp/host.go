package mcp

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/localsock"
)

// Verbindung Brücke ↔ App: erste Zeile das Token aus mcp.json, danach je
// Zeile eine Anfrage {id, tool, args} bzw. Antwort {id, text, error}.

const infoFile = "mcp.json"

type info struct {
	Addr  string `json:"addr"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

type wireReq struct {
	ID   int64           `json:"id"`
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args"`
}

type wireResp struct {
	ID    int64  `json:"id"`
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
}

// Handler: führt ein Werkzeug in der App aus.
type Handler func(tool string, args json.RawMessage) (string, error)

// Host: Socket der laufenden App.
type Host struct {
	ln      net.Listener
	sockDir string
	info    string
	token   []byte
	handle  Handler
}

// Listen öffnet den Socket und schreibt Adresse + Token nach dataDir/mcp.json
// (nur für den eigenen Benutzer lesbar).
func Listen(dataDir string, h Handler) (*Host, error) {
	sockDir, err := os.MkdirTemp("", "sshtermy-mcp-")
	if err != nil {
		return nil, err
	}
	ln, addr, err := localsock.Listen(sockDir)
	if err != nil {
		os.RemoveAll(sockDir)
		return nil, err
	}
	tok := make([]byte, 32)
	_, _ = rand.Read(tok)
	x := &Host{ln: ln, sockDir: sockDir, info: filepath.Join(dataDir, infoFile), token: []byte(hex.EncodeToString(tok)), handle: h}
	b, _ := json.Marshal(info{Addr: addr, Token: string(x.token), PID: os.Getpid()})
	if err := os.WriteFile(x.info, b, 0o600); err != nil {
		x.Close()
		return nil, err
	}
	go x.accept()
	return x, nil
}

// Close beendet den Socket und löscht mcp.json.
func (x *Host) Close() {
	x.ln.Close()
	os.Remove(x.info)
	os.RemoveAll(x.sockDir)
}

func (x *Host) accept() {
	for {
		nc, err := x.ln.Accept()
		if err != nil {
			return
		}
		go x.serve(nc)
	}
}

func (x *Host) serve(nc net.Conn) {
	defer nc.Close()
	r := bufio.NewReader(nc)
	_ = nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := r.ReadBytes('\n')
	if err != nil || subtle.ConstantTimeCompare(trimNL(line), x.token) != 1 {
		return
	}
	_ = nc.SetReadDeadline(time.Time{})
	var mu sync.Mutex
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		var q wireReq
		if json.Unmarshal(sc.Bytes(), &q) != nil {
			return
		}
		go func() {
			text, err := x.handle(q.Tool, q.Args)
			resp := wireResp{ID: q.ID, Text: text}
			if err != nil {
				resp.Error = err.Error()
			}
			b, _ := json.Marshal(resp)
			mu.Lock()
			nc.Write(append(b, '\n'))
			mu.Unlock()
		}()
	}
}

func trimNL(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

// ErrNotRunning: kein laufendes sshTermy mit MCP-Zugriff.
var ErrNotRunning = errors.New("sshTermy is not running or MCP access is off (sshTermy: Settings › Security › MCP access)")

// Remote: Backend der Brücke, verbindet bei Bedarf (auch wenn sshTermy erst
// später gestartet wird).
type Remote struct {
	DataDir string
	mu      sync.Mutex
	nc      net.Conn
	enc     *json.Encoder
	next    int64
	wait    map[int64]chan wireResp
}

func (r *Remote) connect() error {
	b, err := os.ReadFile(filepath.Join(r.DataDir, infoFile))
	if err != nil {
		return ErrNotRunning
	}
	var in info
	if json.Unmarshal(b, &in) != nil {
		return ErrNotRunning
	}
	nc, err := localsock.Dial(in.Addr)
	if err != nil {
		return ErrNotRunning
	}
	if _, err := nc.Write([]byte(in.Token + "\n")); err != nil {
		nc.Close()
		return ErrNotRunning
	}
	r.nc, r.enc, r.wait = nc, json.NewEncoder(nc), map[int64]chan wireResp{}
	go r.read(nc)
	return nil
}

func (r *Remote) read(nc net.Conn) {
	sc := bufio.NewScanner(nc)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		var resp wireResp
		if json.Unmarshal(sc.Bytes(), &resp) != nil {
			continue
		}
		r.mu.Lock()
		ch := r.wait[resp.ID]
		delete(r.wait, resp.ID)
		r.mu.Unlock()
		if ch != nil {
			ch <- resp
		}
	}
	r.mu.Lock()
	if r.nc == nc {
		for id, ch := range r.wait {
			ch <- wireResp{ID: id, Error: "sshTermy was closed"}
		}
		r.nc, r.wait = nil, nil
	}
	r.mu.Unlock()
}

// Call: Werkzeug in der App ausführen (wartet auch auf Rückfragen beim Nutzer).
func (r *Remote) Call(tool string, args json.RawMessage) (string, error) {
	r.mu.Lock()
	if r.nc == nil {
		if err := r.connect(); err != nil {
			r.mu.Unlock()
			return "", err
		}
	}
	r.next++
	id := r.next
	ch := make(chan wireResp, 1)
	r.wait[id] = ch
	err := r.enc.Encode(wireReq{ID: id, Tool: tool, Args: args})
	r.mu.Unlock()
	if err != nil {
		return "", ErrNotRunning
	}
	resp := <-ch
	if resp.Error != "" {
		return "", errors.New(resp.Error)
	}
	return resp.Text, nil
}
