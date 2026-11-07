package localsh

import (
	"bytes"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestStartEchoExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ConPTY wird von Hand getestet")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("kein sh")
	}
	var out syncBuf
	s, err := Start(sh, t.TempDir(), 100, 30, &out)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.Write([]byte("echo sp$((40+2))cols:$(stty size) $TERM; exit\r"))
	select {
	case err := <-s.Done:
		if err != nil {
			t.Fatalf("Done: %v", err)
		}
	case <-time.After(5 * time.Second):
		s.Close()
		t.Fatalf("Shell endet nicht, Ausgabe: %q", out.String())
	}
	got := out.String()
	if !strings.Contains(got, "sp42") || !strings.Contains(got, "30 100") || !strings.Contains(got, "xterm-256color") {
		t.Fatalf("Ausgabe: %q", got)
	}
}

func TestCloseEndsShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("kein sh")
	}
	s, err := Start(sh, "", 80, 24, &syncBuf{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	s.Close()
	select {
	case err := <-s.Done:
		if err != nil {
			t.Fatalf("Done nach Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close beendet die Shell nicht")
	}
}

func TestDialect(t *testing.T) {
	cases := []struct{ goos, shell, want string }{
		{"windows", `C:\Program Files\PowerShell\7\pwsh.exe`, "powershell"},
		{"windows", `C:\WINDOWS\System32\WindowsPowerShell\v1.0\powershell.exe`, "powershell"},
		{"windows", `C:\WINDOWS\system32\cmd.exe`, "cmd"},
		{"windows", `C:\WINDOWS\system32\wsl.exe`, "posix"},
		{"darwin", "/bin/zsh", "macos"},
		{"linux", "/usr/bin/fish", "posix"},
		{"freebsd", "/bin/sh", "bsd"},
		{"linux", "/usr/bin/pwsh", "powershell"},
	}
	for _, c := range cases {
		if got := Dialect(c.goos, c.shell); got != c.want {
			t.Errorf("Dialect(%s, %s) = %s, want %s", c.goos, c.shell, got, c.want)
		}
	}
}

func TestDefaultWindows(t *testing.T) {
	old := lookPath
	defer func() { lookPath = old }()
	have := map[string]bool{}
	lookPath = func(f string) (string, error) {
		if have[f] {
			return `C:\x\` + f, nil
		}
		return "", errors.New("nicht da")
	}
	env := func(k string) string {
		if k == "COMSPEC" {
			return `C:\WINDOWS\system32\cmd.exe`
		}
		return ""
	}
	if got := defaultFor("windows", env); got != `C:\WINDOWS\system32\cmd.exe` {
		t.Errorf("ohne PowerShell: %s", got)
	}
	have["powershell.exe"] = true
	if got := defaultFor("windows", env); got != `C:\x\powershell.exe` {
		t.Errorf("Windows PowerShell: %s", got)
	}
	have["pwsh.exe"] = true
	if got := defaultFor("windows", env); got != `C:\x\pwsh.exe` {
		t.Errorf("PowerShell 7: %s", got)
	}
}

func TestSplitArgsAndEnv(t *testing.T) {
	got := splitArgs(`wsl.exe -d "Ubuntu 24.04"  --cd ~`)
	want := []string{"wsl.exe", "-d", "Ubuntu 24.04", "--cd", "~"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("splitArgs: %q", got)
	}
	env := strings.Join(Env([]string{"TERM=dumb", "PATH=/bin", "term=x"}), " ")
	if strings.Contains(env, "dumb") || !strings.Contains(env, "PATH=/bin") || !strings.Contains(env, "TERM=xterm-256color") {
		t.Fatalf("Env: %s", env)
	}
	if ShortName(`C:\a b\pwsh.exe`) != "pwsh" || ShortName("/bin/zsh") != "zsh" {
		t.Fatal("ShortName")
	}
}
