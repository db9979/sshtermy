// Package offlineai ist die kleine Offline-KI zum Nachladen: das Modell
// von lxask (Aaron Thomas, github.com/arxhr007/lxask; feinjustiertes
// Qwen2.5-Coder-1.5B, Apache 2.0) und llama-server aus llama.cpp
// (github.com/ggml-org/llama.cpp, MIT), der es auf dem eigenen Rechner als
// OpenAI-kompatiblen Endpunkt auf 127.0.0.1 bereitstellt. Nichts davon ist
// in sshTermy enthalten: Erst ein Knopfdruck lädt etwa 1 GB herunter.
//
// Läuft auf Windows, macOS und Linux (x86_64 und ARM64), ohne GPU. Der
// Server startet beim ersten Gebrauch und endet nach 30 Minuten Ruhe oder
// mit sshTermy.
package offlineai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Model: der Modellname, unter dem sshTermy den Endpunkt anspricht.
const Model = "lxask"

// IdleStop: so lange ohne Anfrage, dann endet der Server (Speicher frei).
const IdleStop = 30 * time.Minute

// Engine verwaltet Dateien und Server der Offline-KI.
type Engine struct {
	dir string

	mu       sync.Mutex
	cmd      *exec.Cmd
	port     int
	key      string // Zugangsschlüssel dieses Laufs (--api-key)
	lastUse  time.Time
	starting chan struct{} // offen, solange ein Start läuft
	startErr error
}

// New: Offline-KI mit Dateien im Ordner dir (wird bei Bedarf angelegt).
func New(dir string) *Engine { return &Engine{dir: dir} }

// Dir: Ordner der Dateien.
func (e *Engine) Dir() string { return e.dir }

// ModelPath: die Modelldatei.
func (e *Engine) ModelPath() string { return filepath.Join(e.dir, "lxask.gguf") }

func exeName(n string) string {
	if runtime.GOOS == "windows" {
		return n + ".exe"
	}
	return n
}

// ServerPath: llama-server, heruntergeladen oder schon auf dem Rechner.
func (e *Engine) ServerPath() string {
	if b, err := os.ReadFile(filepath.Join(e.dir, "server.path")); err == nil {
		if p := strings.TrimSpace(string(b)); p != "" && fileExists(p) {
			return p
		}
	}
	if p, err := exec.LookPath(exeName("llama-server")); err == nil {
		return p
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// Installed: Modell und Server sind da.
func (e *Engine) Installed() bool {
	if e == nil {
		return false
	}
	st, err := os.Stat(e.ModelPath())
	return err == nil && st.Size() > 100<<20 && e.ServerPath() != ""
}

// Size: Platz auf der Platte (Modell + Server).
func (e *Engine) Size() int64 {
	var n int64
	_ = filepath.Walk(e.dir, func(_ string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			n += fi.Size()
		}
		return nil
	})
	return n
}

// Running: Der Server läuft gerade.
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cmd != nil
}

// BaseURL des laufenden Servers ("" = läuft nicht).
func (e *Engine) BaseURL() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cmd == nil {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d/v1", e.port)
}

// Port: der feste Port dieses Laufs (wird beim ersten Aufruf gewählt, damit
// die Einstellung schon vor dem Start eine Adresse hat).
func (e *Engine) Port() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.port == 0 {
		e.port = freePort()
	}
	return e.port
}

// Key: Zugangsschlüssel des Servers für diesen Lauf. Ohne ihn beantwortet
// llama-server keine Anfragen, andere Programme oder Benutzer auf dem Rechner
// können die Offline-KI also nicht mitbenutzen.
func (e *Engine) Key() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.key == "" {
		b := make([]byte, 24)
		_, _ = rand.Read(b)
		e.key = hex.EncodeToString(b)
	}
	return e.key
}

func freePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 38471
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// Ensure startet den Server, falls nötig, und wartet, bis das Modell
// geladen ist.
func (e *Engine) Ensure(ctx context.Context) error {
	if !e.Installed() {
		return errors.New(i18n.T("Offline-KI ist nicht heruntergeladen"))
	}
	port := e.Port()
	key := e.Key()
	e.mu.Lock()
	e.lastUse = time.Now()
	if e.cmd != nil {
		e.mu.Unlock()
		return nil
	}
	if ch := e.starting; ch != nil {
		e.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
		e.mu.Lock()
		err := e.startErr
		e.mu.Unlock()
		return err
	}
	ch := make(chan struct{})
	e.starting = ch
	e.mu.Unlock()

	err := e.start(ctx, port, key)
	e.mu.Lock()
	e.startErr, e.starting = err, nil
	e.mu.Unlock()
	close(ch)
	return err
}

func (e *Engine) start(ctx context.Context, port int, key string) error {
	srv := e.ServerPath()
	cmd := exec.Command(srv, "-m", e.ModelPath(), "--host", "127.0.0.1", "--port", strconv.Itoa(port), "-c", "4096")
	cmd.Env = append(os.Environ(), "LLAMA_API_KEY="+key) // nicht in der Befehlszeile (ps)
	cmd.Dir = filepath.Dir(srv)
	hideWindow(cmd)
	logf, _ := os.Create(filepath.Join(e.dir, "server.log"))
	if logf != nil {
		cmd.Stdout, cmd.Stderr = logf, logf
	}
	if err := cmd.Start(); err != nil {
		if logf != nil {
			logf.Close()
		}
		return fmt.Errorf(i18n.T("llama-server startet nicht: %w"), err)
	}
	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
		if logf != nil {
			logf.Close()
		}
		e.mu.Lock()
		if e.cmd == cmd {
			e.cmd = nil
		}
		e.mu.Unlock()
	}()
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case err := <-exited:
			return fmt.Errorf(i18n.T("llama-server hat sich beendet (%v), siehe %s"), err, filepath.Join(e.dir, "server.log"))
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				if !e.ours(port, key) {
					_ = cmd.Process.Kill()
					return fmt.Errorf(i18n.T("Port %d ist von einem anderen Programm belegt"), port)
				}
				e.mu.Lock()
				e.cmd = cmd
				e.mu.Unlock()
				return nil
			}
		}
	}
	_ = cmd.Process.Kill()
	return errors.New(i18n.T("llama-server antwortet nicht (Zeitüberschreitung beim Laden des Modells)"))
}

// ours: Antwortet auf dem Port unser Server? Nur er kennt den Schlüssel:
// ohne Schlüssel lehnt er ab, mit Schlüssel nimmt er an.
func (e *Engine) ours(port int, key string) bool {
	get := func(withKey bool) int {
		// /tokenize: billig, und anders als /health und /v1/models nicht öffentlich
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/tokenize", port), strings.NewReader(`{"content":"x"}`))
		if err != nil {
			return 0
		}
		req.Header.Set("Content-Type", "application/json")
		if withKey {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		c := &http.Client{Timeout: 5 * time.Second}
		resp, err := c.Do(req)
		if err != nil {
			return 0
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	return get(false) == http.StatusUnauthorized && get(true) == http.StatusOK
}

// Touch: eine Anfrage lief (verschiebt das Ende im Leerlauf).
func (e *Engine) Touch() {
	e.mu.Lock()
	e.lastUse = time.Now()
	e.mu.Unlock()
}

// StopIdle beendet den Server nach IdleStop ohne Anfrage.
func (e *Engine) StopIdle(now time.Time) {
	e.mu.Lock()
	idle := e.cmd != nil && now.Sub(e.lastUse) > IdleStop
	e.mu.Unlock()
	if idle {
		e.Stop()
	}
}

// Stop beendet den Server.
func (e *Engine) Stop() {
	if e == nil {
		return
	}
	e.mu.Lock()
	cmd := e.cmd
	e.cmd = nil
	e.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// Remove löscht alle Dateien der Offline-KI.
func (e *Engine) Remove() error {
	e.Stop()
	return os.RemoveAll(e.dir)
}
