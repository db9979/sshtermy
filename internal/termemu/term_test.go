package termemu

import (
	"io"
	"strings"
	"testing"
)

func TestTypedInputAndGhost(t *testing.T) {
	tm := New(60, 5, io.Discard)
	defer tm.Close()
	tm.Write([]byte("\x1b[1;32mdominik@spark\x1b[0m:~$ docker p"))
	typed, ok := tm.TypedInput()
	if !ok || typed != "docker p" {
		t.Fatalf("TypedInput = %q %v", typed, ok)
	}
	v := tm.View("s -a", func(s string) string { return "[" + s + "]" })
	if !strings.Contains(v, "docker p[s -a]") {
		t.Fatalf("Ghost fehlt: %q", strings.Split(v, "\n")[0])
	}
	tm.Write([]byte("\r\nzeile 2\r\n"))
	if !strings.Contains(tm.PlainLines(10), "zeile 2") {
		t.Fatal("PlainLines")
	}
}

func TestShellIntegrationExitCode(t *testing.T) {
	tm := New(40, 5, io.Discard)
	defer tm.Close()
	seq0, _ := tm.LastExit()
	_, _ = tm.Write([]byte("ls /nope\r\nls: cannot access\r\n\x1b]133;D;2\x07\x1b]133;A\x07$ "))
	seq, code := tm.LastExit()
	if seq != seq0+1 || code != 2 {
		t.Fatalf("seq %d→%d code %d", seq0, seq, code)
	}
	_, _ = tm.Write([]byte("\x1b]633;D\x1b\\"))
	if seq2, code := tm.LastExit(); seq2 != seq+1 || code != 0 {
		t.Fatalf("633 without code: seq %d code %d", seq2, code)
	}
	if strings.Contains(tm.PlainLines(10), "133") {
		t.Fatal("OSC leaked into screen")
	}
}

func TestCommandOutput(t *testing.T) {
	tm := New(50, 10, io.Discard)
	defer tm.Close()
	_, _ = tm.Write([]byte("u@h:~$ ls\r\nold\r\nu@h:~$ cat /etc/nope\r\ncat: /etc/nope: No such file or directory\r\nu@h:~$ "))
	out, line, ok := tm.CommandOutput("cat /etc/nope", 100)
	if !ok || out != "cat: /etc/nope: No such file or directory" || line != "cat /etc/nope" {
		t.Fatalf("got %q %q %v", out, line, ok)
	}
	// Echo beim Enter noch nicht vollständig: Befehl vom Bildschirm nehmen
	if _, line, ok := tm.CommandOutput("cat /etc/no", 100); !ok || line != "cat /etc/nope" {
		t.Fatalf("partial: %q %v", line, ok)
	}
	// umgebrochener Befehl
	long := "echo " + strings.Repeat("x", 60)
	_, _ = tm.Write([]byte(long + "\r\n" + strings.Repeat("x", 40) + "\r\nu@h:~$ "))
	out, line, ok = tm.CommandOutput(long, 100)
	if !ok || out != strings.Repeat("x", 40) || line != long {
		t.Fatalf("wrapped: %q %v", out, ok)
	}
}

// OSC 10/11 merkt sich das Terminal (eigenes Fenster malt damit), 110/111
// setzt zurück, Abfragen ändern nichts.
func TestDefaultColorsOSC(t *testing.T) {
	tm := New(20, 5, io.Discard)
	defer tm.Close()
	_, _ = tm.Write([]byte("\x1b]11;#f5f5f7\x07\x1b]10;rgb:1f/23/35\x1b\\"))
	fg, bg := tm.DefaultColors()
	if bg == nil || fg == nil {
		t.Fatalf("nicht gemerkt: %v %v", fg, bg)
	}
	if r, g, b, _ := bg.RGBA(); r>>8 != 0xf5 || g>>8 != 0xf5 || b>>8 != 0xf7 {
		t.Errorf("bg %v", bg)
	}
	if r, _, _, _ := fg.RGBA(); r>>8 != 0x1f {
		t.Errorf("fg %v", fg)
	}
	_, _ = tm.Write([]byte("\x1b]11;?\x07"))
	if _, bg := tm.DefaultColors(); bg == nil {
		t.Error("Abfrage hat Farbe gelöscht")
	}
	_, _ = tm.Write([]byte("\x1b]111\x07"))
	if _, bg := tm.DefaultColors(); bg != nil {
		t.Errorf("111 setzt nicht zurück: %v", bg)
	}
}
