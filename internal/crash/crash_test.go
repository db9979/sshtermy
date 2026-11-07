package crash

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestCleanRemovesSecrets(t *testing.T) {
	t.Setenv("HOME", "/home/dominik")
	raw := "panic: dial web-01.example.com (10.1.2.3:22) as dominik with pw S3cr3t!pw failed\n" +
		"password=hunter2 token: abcdefgh Bearer abcdefghijklmnopqrstuvwxyz\n" +
		"open /home/dominik/.config/sshtermy/store.vault\n" +
		"-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----\n" +
		"ssh://root:topsecret@host fe80:0:0:0:1:2:3:4\n" +
		"goroutine 1 [running]:\nmain.main()\n\t/build/cmd/sshtermy/main.go:42 +0x1d\n"
	got := Clean(raw, []string{"S3cr3t!pw", "web-01.example.com", "dominik"})
	for _, bad := range []string{"S3cr3t", "web-01.example", "10.1.2.3", "hunter2", "abcdefghijklmnop", "dominik", "AAAA", "topsecret", "fe80"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q steht noch im Bericht:\n%s", bad, got)
		}
	}
	for _, keep := range []string{"main.main()", "main.go:42", "goroutine 1 [running]", "~/.config/sshtermy"} {
		if !strings.Contains(got, keep) {
			t.Errorf("%q fehlt:\n%s", keep, got)
		}
	}
}

func TestBuildAndIssueURL(t *testing.T) {
	stack := "Caught panic:\n\nruntime error: index out of range [3] with length 3\n\nRestoring terminal...\n\ngoroutine 1 [running]:\n" +
		strings.Repeat("github.com/db9979/sshtermy/internal/ui.(*App).Update(...)\n\t/src/internal/ui/app.go:123 +0x1\n", 400)
	r := Build(KindUI, stack, nil, time.Date(2026, 11, 6, 7, 0, 0, 0, time.UTC))
	if r.Title != "Crash: runtime error: index out of range [3] with length 3" {
		t.Errorf("Titel = %q", r.Title)
	}
	r.Path = "/x/sshtermy-crash-1.txt"
	u := IssueURL(r)
	if len(u) > maxURL {
		t.Fatalf("URL zu lang: %d", len(u))
	}
	p, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	body := p.Query().Get("body")
	if !strings.Contains(body, "index out of range") || !strings.Contains(body, "sshtermy-crash-1.txt") || !strings.HasSuffix(body, "```\n") {
		t.Errorf("Issue-Text unvollständig:\n%s", body[:min(len(body), 600)])
	}
	if p.Host != "github.com" || p.Path != "/db9979/sshtermy/issues/new" {
		t.Errorf("URL = %s", u)
	}
}

// panicModel stürzt bei der ersten Nachricht ab, wie ein Fehler in Update.
type panicModel struct{}

func (panicModel) Init() tea.Cmd { return func() tea.Msg { return "go" } }
func (panicModel) Update(m tea.Msg) (tea.Model, tea.Cmd) {
	if m == "go" {
		var s []int
		_ = s[3] // index out of range
	}
	return panicModel{}, nil
}
func (panicModel) View() tea.View { return tea.NewView("x") }

// TestUIPanicIsCaptured: bubbletea fängt die Panik, Guard liest den Stack mit.
func TestUIPanicIsCaptured(t *testing.T) {
	base := t.TempDir()
	g := Start(base)
	defer g.Close()
	r, w, _ := os.Pipe()
	defer w.Close()
	_, err := tea.NewProgram(panicModel{}, tea.WithInput(r), tea.WithOutput(discard{}), tea.WithoutSignals()).Run()
	if err == nil {
		t.Fatal("Run ohne Fehler")
	}
	raw := g.Panic()
	if !strings.Contains(raw, "index out of range") || !strings.Contains(raw, "crash.panicModel.Update") {
		t.Fatalf("Stack fehlt:\n%s", raw)
	}
	rep, err := Write(base, KindUI, raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(rep.Path)
	if !strings.Contains(string(b), "panicModel.Update") {
		t.Fatalf("Datei:\n%s", b)
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// TestFatalCrashNextStart: Panik in einer eigenen Goroutine beendet den
// Prozess; beim nächsten Start liegt der Stack bereit.
func TestFatalCrashNextStart(t *testing.T) {
	if os.Getenv("CRASH_CHILD") != "" {
		Start(os.Getenv("CRASH_CHILD"))
		done := make(chan bool)
		go func() { panic("fatal child password=hunter2") }()
		<-done
		return
	}
	base := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestFatalCrashNextStart")
	cmd.Env = append(os.Environ(), "CRASH_CHILD="+base)
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("Kind lief ohne Absturz durch:\n%s", out)
	}
	if Pending(base) != "" {
		t.Fatal("Pending vor dem nächsten Start")
	}
	g := Start(base) // nächster Start
	g.Close()
	raw := Pending(base)
	if !strings.Contains(raw, "fatal child") || !strings.Contains(raw, "goroutine") {
		t.Fatalf("Pending = %q", raw)
	}
	r, err := Write(base, KindFatal, raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	Resolve(base)
	if Pending(base) != "" {
		t.Fatal("Rohtext nach Resolve noch da")
	}
	b, _ := os.ReadFile(r.Path)
	if strings.Contains(string(b), "hunter2") {
		t.Fatalf("Passwort im Bericht:\n%s", b)
	}
	if filepath.Dir(r.Path) != Dir(base) {
		t.Fatalf("Pfad %s", r.Path)
	}
}

func TestPruneKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < keep+3; i++ {
		_ = os.WriteFile(filepath.Join(dir, "sshtermy-crash-2026010"+string(rune('a'+i))+".txt"), nil, 0o600)
	}
	prune(dir)
	m, _ := filepath.Glob(filepath.Join(dir, "sshtermy-crash-*.txt"))
	if len(m) != keep || !strings.HasSuffix(m[len(m)-1], "m.txt") {
		t.Fatalf("%d übrig: %v", len(m), m)
	}
}
