package psremote

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/termemu"
)

// Echtes PowerShell (Windows: powershell.exe, sonst pwsh, falls da) über
// Pipes statt WinRS: prüft die Schleife aus script.go so, wie sie drüben
// läuft. In der CI auf dem Windows-Runner immer dabei.

type localPS struct{ exe string }

type localProc struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	mu  sync.Mutex
}

func (l localPS) start(dir string, h handlers) (proc, error) {
	cmd := exec.Command(l.exe, psArgs()...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	so, _ := cmd.StdoutPipe()
	se, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	pump := func(r io.Reader, f func([]byte)) {
		defer wg.Done()
		b := make([]byte, 4096)
		for {
			n, err := r.Read(b)
			if n > 0 {
				f(append([]byte(nil), b[:n]...))
			}
			if err != nil {
				return
			}
		}
	}
	wg.Add(2)
	go pump(so, h.out)
	go pump(se, h.err)
	go func() {
		wg.Wait()
		h.end(cmd.Wait())
	}()
	return &localProc{cmd: cmd, in: in}, nil
}

func (localPS) close() {}

func (p *localProc) send(b []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.in.Write(b)
	return err
}

func (p *localProc) stop() { _ = p.cmd.Process.Kill() }

func findPS(t *testing.T) string {
	for _, n := range []string{"powershell.exe", "pwsh"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	t.Skip("kein PowerShell auf diesem Rechner")
	return ""
}

func TestRealPowerShell(t *testing.T) {
	exe := findPS(t)
	h := &harness{t: t, f: &fakePS{}}
	h.term = termemu.New(100, 40, &bytes.Buffer{})
	s, err := newStream(localPS{exe}, 100, h, 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	h.s = s
	t.Cleanup(func() { s.Close(); h.term.Close() })
	slow := func(want string) {
		t.Helper()
		dl := time.Now().Add(30 * time.Second)
		want = strings.TrimRight(want, " ")
		for time.Now().Before(dl) {
			if strings.Contains(h.term2(), want) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("Bildschirm ohne %q:\n%s", want, h.term2())
	}
	slow("PS ")
	if i := s.Info(); i.PSVersion == "" || i.Computer == "" {
		t.Fatalf("Info = %+v", i)
	}
	t.Logf("drüben: %+v", s.Info())

	// Variablen bleiben zwischen Befehlen erhalten (dot-sourced)
	h.typ("$zahl = 40 + 2\r")
	h.typ("\"Ergebnis=$zahl\"\r")
	slow("Ergebnis=42")

	// mehrzeilig: Funktion über zwei Zeilen, dann aufrufen
	h.typ("function Sag-Hallo {\r")
	slow(">> ")
	h.typ("'Hallo ' + $args[0] }\r")
	h.typ("Sag-Hallo Welt\r")
	slow("Hallo Welt")

	// Fehler und Warnungen kommen sichtbar an
	h.typ("Gibt-EsNicht\r")
	slow("is not recognized")
	h.typ("Write-Warning 'Achtung'\r")
	slow("WARNING: Achtung")

	// Tabellen in Bildschirmbreite
	h.typ("[pscustomobject]@{Name='a';Wert=1} | Format-Table\r")
	slow("Wert")

	// Tab-Ergänzung über TabExpansion2
	idle := func() {
		t.Helper()
		dl := time.Now().Add(30 * time.Second)
		for time.Now().Before(dl) {
			s.mu.Lock()
			r := s.ready
			s.mu.Unlock()
			if r {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("kein Prompt")
	}
	slow("a       1")
	idle() // Tab gilt nur am Prompt
	h.typ("Get-ChildIte\t")
	slow("Get-ChildItem")
	h.typ("\x15")

	// Ordnerwechsel landet im Prompt
	h.typ("Set-Location ..\r")
	before := s.Dir()
	h.typ("'nach cd'\r")
	slow("nach cd")
	if s.Dir() == "" || before == "" {
		t.Fatalf("Ordner fehlt: %q", s.Dir())
	}

	h.typ("exit\r")
	select {
	case err := <-s.Done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("exit beendet die Sitzung nicht")
	}
}

func (h *harness) term2() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.term.PlainLines(40)
}
