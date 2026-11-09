package psremote

import (
	"bytes"
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/termemu"
)

// fakePS spielt die Gegenstelle nach (Protokoll aus script.go).
type fakePS struct {
	mu      sync.Mutex
	starts  []string // Startordner je Start
	stopped int
	width   int
	raw     []string // Zeilen ohne \x1e (für Read-Host)
	ran     []string
	closed  bool
}

type fakeProc struct {
	f     *fakePS
	h     handlers
	dir   string
	boot  bool
	buf   []byte
	vars  map[string]string
	block bool
	pmu   sync.Mutex
	sleep bool     // „sleep 1“ läuft
	later []string // solange vorweg getippt
}

func (f *fakePS) start(dir string, h handlers) (proc, error) {
	f.mu.Lock()
	f.starts = append(f.starts, dir)
	f.mu.Unlock()
	if dir == "" {
		dir = `C:\Users\admin`
	}
	return &fakeProc{f: f, h: h, dir: dir, vars: map[string]string{}}, nil
}

func (f *fakePS) close() {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
}

func (p *fakeProc) stop() {
	p.f.mu.Lock()
	p.f.stopped++
	p.f.mu.Unlock()
}

func (p *fakeProc) prompt() {
	p.h.out([]byte("\x01D" + p.dir + "\x02\x01PPS " + p.dir + "> \x02"))
}

func (p *fakeProc) send(b []byte) error {
	p.pmu.Lock()
	defer p.pmu.Unlock()
	p.buf = append(p.buf, b...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			return nil
		}
		line := strings.TrimRight(string(p.buf[:i]), "\r")
		p.buf = p.buf[i+1:]
		p.line(line)
	}
}

func (p *fakeProc) line(l string) {
	if !p.boot {
		p.boot = true
		p.h.out([]byte("\x01IWindows Server 2022 Standard|5.1.20348|SRV01\x02"))
		p.prompt()
		return
	}
	if p.sleep {
		p.later = append(p.later, l)
		return
	}
	if !strings.HasPrefix(l, "\x1e") {
		p.f.mu.Lock()
		p.f.raw = append(p.f.raw, l)
		p.f.mu.Unlock()
		if p.block {
			p.h.out([]byte("Hallo " + l + "\r\n"))
			p.block = false
			p.prompt()
			return
		}
		if l == "" {
			return
		}
		p.h.out([]byte("\x01R" + l + "\x02"))
		l = "\x1eX" + base64.StdEncoding.EncodeToString([]byte(l))
	}
	k, a := l[1], l[2:]
	switch k {
	case 'W':
		p.f.mu.Lock()
		p.f.width, _ = strconv.Atoi(a)
		p.f.mu.Unlock()
	case 'T':
		pos, enc, _ := strings.Cut(a, ":")
		src, _ := base64.StdEncoding.DecodeString(enc)
		n, _ := strconv.Atoi(pos)
		word := string(src)[strings.LastIndex(string(src[:n]), " ")+1 : n]
		var ms []string
		for _, c := range []string{"Get-ChildItem", "Get-Content", "Get-Date", "Set-Location"} {
			if strings.HasPrefix(strings.ToLower(c), strings.ToLower(word)) {
				ms = append(ms, c)
			}
		}
		if len(ms) == 0 {
			p.h.out([]byte("\x01T\x02"))
			return
		}
		p.h.out([]byte("\x01T" + strconv.Itoa(n-len(word)) + "," + strconv.Itoa(len(word)) + "\x1f" + strings.Join(ms, "\x1f") + "\x02"))
	case 'C', 'X':
		src, _ := base64.StdEncoding.DecodeString(a)
		cmd := string(src)
		if k == 'C' && strings.Count(cmd, "{") > strings.Count(cmd, "}") {
			p.h.out([]byte("\x01M\x02"))
			return
		}
		p.f.mu.Lock()
		p.f.ran = append(p.f.ran, cmd)
		p.f.mu.Unlock()
		switch {
		case cmd == "Get-Date":
			p.h.out([]byte("\r\nMontag, 9. November 2026 11:00:00\r\n\r\n"))
		case strings.HasPrefix(cmd, "cd "):
			p.dir = strings.TrimSpace(cmd[3:])
		case cmd == "Read-Host Name":
			p.h.out([]byte("Name: "))
			p.block = true
			return
		case cmd == "sleep 100":
			p.block = true
			return
		case cmd == "sleep 1":
			p.sleep = true
			go func() {
				time.Sleep(150 * time.Millisecond)
				p.pmu.Lock()
				defer p.pmu.Unlock()
				p.sleep = false
				p.prompt()
				for _, l := range p.later {
					p.line(l)
				}
				p.later = nil
			}()
			return
		case strings.HasPrefix(cmd, "function"):
		default:
			p.h.err([]byte(cmd + " : The term '" + cmd + "' is not recognized\r\n"))
		}
		p.prompt()
	}
}

type harness struct {
	t    *testing.T
	f    *fakePS
	s    *Stream
	term *termemu.Term
	mu   sync.Mutex
}

func (h *harness) Write(p []byte) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.term.Write(p)
}

func (h *harness) screen() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.term.PlainLines(24)
}

func newHarness(t *testing.T, cols int) *harness {
	t.Helper()
	h := &harness{t: t, f: &fakePS{}}
	h.term = termemu.New(cols, 24, &bytes.Buffer{})
	s, err := newStream(h.f, cols, h, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	h.s = s
	t.Cleanup(func() { s.Close(); h.term.Close() })
	return h
}

func (h *harness) typ(s string) {
	_, _ = h.s.Write([]byte(s))
}

// wait: bis der Bildschirm want enthält.
func (h *harness) wait(want string) {
	h.t.Helper()
	want = strings.TrimRight(want, " ")
	dl := time.Now().Add(2 * time.Second)
	for time.Now().Before(dl) {
		if strings.Contains(h.screen(), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatalf("Bildschirm ohne %q:\n%s", want, h.screen())
}

// until: bis cond stimmt.
func (h *harness) until(what string, cond func() bool) {
	h.t.Helper()
	dl := time.Now().Add(2 * time.Second)
	for time.Now().Before(dl) {
		h.f.mu.Lock()
		ok := cond()
		h.f.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatalf("%s: nicht eingetreten\n%s", what, h.screen())
}

func TestPromptAndCommand(t *testing.T) {
	h := newHarness(t, 80)
	h.wait(`[SRV01]: PS C:\Users\admin> `)
	if i := h.s.Info(); i.OS != "Windows Server 2022 Standard" || i.Computer != "SRV01" || i.PSVersion != "5.1.20348" {
		t.Fatalf("Info = %+v", i)
	}
	h.typ("Get-Dat")
	h.typ("e\r")
	h.wait("Montag, 9. November 2026")
	h.typ("cd D:\\Daten\r")
	h.wait(`[SRV01]: PS D:\Daten> `)
	if h.s.Dir() != `D:\Daten` {
		t.Fatalf("Dir = %q", h.s.Dir())
	}
	h.f.mu.Lock()
	w := h.f.width
	h.f.mu.Unlock()
	if w != 80 {
		t.Fatalf("Breite drüben = %d", w)
	}
}

func TestLineEditing(t *testing.T) {
	h := newHarness(t, 80)
	h.wait("> ")
	// Tippfehler: „Gte-Date“, links, Backspace, Zeichen einfügen
	h.typ("Gt-Date")
	h.typ("\x1b[D\x1b[D\x1b[D\x1b[D\x1b[D\x1b[D") // hinter „G“
	h.typ("e")
	h.typ("\x1b[F\r") // Ende, Enter
	h.wait("Montag")
	h.f.mu.Lock()
	ran := append([]string(nil), h.f.ran...)
	h.f.mu.Unlock()
	if len(ran) != 1 || ran[0] != "Get-Date" {
		t.Fatalf("ausgeführt: %q", ran)
	}
	// Verlauf: Pfeil hoch holt den Befehl zurück
	h.typ("\x1b[A")
	h.wait("> Get-Date")
	h.typ("\x15") // Strg+U leert
	h.typ("dir\x17")
	h.typ("\r")
	h.f.mu.Lock()
	n := len(h.f.ran)
	h.f.mu.Unlock()
	if n != 1 {
		t.Fatal("leere Zeile ging an den Server")
	}
}

func TestTabCompletion(t *testing.T) {
	h := newHarness(t, 80)
	h.wait("> ")
	h.typ("Get-C\t")
	h.wait("> Get-ChildItem")
	h.typ("\t") // nächster Treffer
	h.wait("> Get-Content")
	h.typ("\x1b[Z") // ⇧Tab zurück
	h.wait("> Get-ChildItem")
}

func TestMultiLine(t *testing.T) {
	h := newHarness(t, 80)
	h.wait("> ")
	h.typ("function f {\r")
	h.wait(">> ")
	h.typ("'x' }\r")
	h.until("ausgeführt", func() bool { return len(h.f.ran) > 0 })
	h.f.mu.Lock()
	ran := append([]string(nil), h.f.ran...)
	h.f.mu.Unlock()
	if len(ran) != 1 || ran[0] != "function f {\n'x' }" {
		t.Fatalf("ausgeführt: %q", ran)
	}
}

func TestReadHostAndInterrupt(t *testing.T) {
	h := newHarness(t, 80)
	h.wait("> ")
	h.typ("Read-Host Name\r")
	h.wait("Name: ")
	h.typ("Dominik\r")
	h.wait("Hallo Dominik")
	h.typ("cd E:\\\r")
	h.wait(`PS E:\> `)
	h.typ("sleep 100\r")
	time.Sleep(50 * time.Millisecond)
	h.typ("\x03")
	h.wait("abgebrochen")
	h.until("Neustart", func() bool { return len(h.f.starts) == 2 })
	h.wait(`[SRV01]: PS E:\> `) // neu gestartet im selben Ordner
	h.f.mu.Lock()
	starts, stopped := append([]string(nil), h.f.starts...), h.f.stopped
	h.f.mu.Unlock()
	if len(starts) != 2 || starts[1] != `E:\` || stopped != 1 {
		t.Fatalf("Starts %q, gestoppt %d", starts, stopped)
	}
}

func TestExitEndsSession(t *testing.T) {
	h := newHarness(t, 80)
	h.wait("> ")
	h.typ("exit\r")
	select {
	case err := <-h.s.Done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("exit beendet die Sitzung nicht")
	}
}

func TestErrorsAndLongLine(t *testing.T) {
	h := newHarness(t, 30)
	h.wait("> ")
	long := strings.Repeat("x", 40)
	h.typ(long)
	h.typ("\x1b[H") // Pos1: Cursor über zwei Zeilen zurück
	h.typ("y")
	h.typ("\r")
	h.wait("is not recognized")
	h.f.mu.Lock()
	ran := h.f.ran
	h.f.mu.Unlock()
	if len(ran) != 1 || ran[0] != "y"+long {
		t.Fatalf("ausgeführt: %q", ran)
	}
}

func TestCLIXMLFiltered(t *testing.T) {
	h := newHarness(t, 80)
	h.wait("> ")
	h.s.remoteErr(h.s.gen, []byte("#< CLIXML\r\n<Objs Version=\"1.1.0.1\"></Objs>"))
	if strings.Contains(h.screen(), "CLIXML") {
		t.Fatal("CLIXML auf dem Bildschirm")
	}
}

func TestStartupFailure(t *testing.T) {
	be := &deadPS{}
	_, err := newStream(be, 80, &bytes.Buffer{}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "powershell.exe is not recognized") {
		t.Fatalf("err = %v", err)
	}
}

type deadPS struct{}

func (deadPS) start(dir string, h handlers) (proc, error) {
	go func() {
		h.err([]byte("'powershell.exe' : powershell.exe is not recognized\r\n"))
		h.end(&ExitError{Code: 1})
	}()
	return deadProc{}, nil
}
func (deadPS) close() {}

type deadProc struct{}

func (deadProc) send([]byte) error { return nil }
func (deadProc) stop()             {}

func TestParseEsc(t *testing.T) {
	for in, want := range map[string]keyKind{
		"\x1b[A": keyUp, "\x1b[1;5C": keyWordRight, "\x1b[3~": keyDel, "\x1bOH": keyHome,
		"\x1b[<0;3;4M": keyNone, "\x1b[200~": keyNone, "\x1bb": keyWordLeft,
	} {
		k, used, ok := parseEsc([]byte(in))
		if !ok || k != want || used != len(in) {
			t.Errorf("%q → %v %d %v", in, k, used, ok)
		}
	}
	if _, _, ok := parseEsc([]byte("\x1b[1;")); ok {
		t.Error("halbe Folge als fertig erkannt")
	}
}

func TestRuneIndex(t *testing.T) {
	b := []rune("a😀b")
	if runeIndex(b, 3) != 2 || runeIndex(b, 1) != 1 || runeIndex(b, 9) != 3 {
		t.Fatal("UTF-16 → Rune falsch")
	}
}

func TestEncodedCommand(t *testing.T) {
	if encodedCommand("ab") != "YQBiAA==" {
		t.Fatal(encodedCommand("ab"))
	}
	if psQuote("it's") != "'it''s'" {
		t.Fatal(psQuote("it's"))
	}
	if n := len(strings.Join(psArgs(), " ")); n > 2000 {
		t.Fatalf("Kommandozeile zu lang: %d", n)
	}
}

func TestTypeAhead(t *testing.T) {
	h := newHarness(t, 80)
	h.wait("> ")
	h.typ("sleep 1\r")
	h.typ("Get-Date\r") // während sleep getippt (oder eingefügt)
	h.typ("Get-")       // angefangen, noch ohne Enter
	h.wait("Montag")    // lief nach sleep
	h.wait(`PS C:\Users\admin> Get-Date`)
	h.wait(`PS C:\Users\admin> Get-`) // angefangene Zeile steht wieder im Prompt
	h.typ("Date\r")
	dl := time.Now().Add(2 * time.Second)
	for time.Now().Before(dl) {
		h.f.mu.Lock()
		n := len(h.f.ran)
		h.f.mu.Unlock()
		if n == 3 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("ausgeführt: %q\n%s", h.f.ran, h.screen())
}
