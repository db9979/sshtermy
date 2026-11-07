package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
)

// footerOrderOK: nach der Palette erst alle F-Tasten aufsteigend, dann ⇧F,
// danach nur noch andere Tasten – nie eine Strg-Taste zwischen F-Tasten.
func footerOrderOK(t *testing.T, name string, keys [][2]string) {
	t.Helper()
	fre := regexp.MustCompile(`^(⇧)?F(\d+)`)
	last, phase := 0, 0 // 0 = Kopf, 1 = F, 2 = ⇧F, 3 = Rest
	for _, k := range keys {
		m := fre.FindStringSubmatch(k[0])
		switch {
		case k[0] == "Strg+Space" || k[0] == "Alt+0":
			if phase > 0 {
				t.Fatalf("%s: %s steht nicht vorn: %v", name, k[0], keys)
			}
		case m == nil:
			phase = 3
		default:
			n := 0
			for _, c := range m[2] {
				n = n*10 + int(c-'0')
			}
			p := 1
			if m[1] != "" {
				p, n = 2, n+100
			}
			if phase > p || n < last {
				t.Fatalf("%s: %s falsch einsortiert: %v", name, k[0], keys)
			}
			phase, last = p, n
		}
	}
}

func TestFooterKeyOrder(t *testing.T) {
	a := dangerApp(t)
	s, _ := typedSession(domain.Connection{Alias: "web01", Host: "10.0.0.1"}, "$ ", "")
	a.sessions = []*session{s, {id: 2, conn: domain.Connection{Alias: "db"}, state: stConnected}}
	for _, w := range []int{240, 160, 120, 80} {
		a.w = w
		footerOrderOK(t, "Terminal", a.sortedFooter(a.terminalKeys(s)))
	}
	a.scr = scrManager
	a.w = 240
	footerOrderOK(t, "Manager", a.sortedFooter(a.managerKeys()))
	if p := os.Getenv("SSHTERMY_FOOTER_DUMP"); p != "" {
		a.scr, a.w = scrTerminal, 180
		var b strings.Builder
		b.WriteString(a.footer(a.terminalKeys(s)) + "\n\n")
		a.scr = scrManager
		b.WriteString(a.footer(a.managerKeys()) + "\n")
		_ = os.WriteFile(p, []byte(b.String()), 0o644)
	}
	got := ansi.Strip(a.footer(a.managerKeys()))
	if !strings.HasPrefix(strings.TrimSpace(got), "Strg+Space") {
		t.Fatalf("Manager-Fußzeile beginnt nicht mit der Palette: %q", got)
	}
}

// TestDumpScreens schreibt für Screenshots ganze Bildschirme (nur mit SSHTERMY_SCREEN_DUMP).
func TestDumpScreens(t *testing.T) {
	dir := os.Getenv("SSHTERMY_SCREEN_DUMP")
	if dir == "" {
		t.Skip()
	}
	a := dangerApp(t)
	a.w, a.h, a.intro = 200, 14, nil
	s, _ := typedSession(domain.Connection{Alias: "web01", Host: "10.0.0.1"}, "web01:~$ ", "")
	a.sessions = []*session{s}
	dump := func(name string) {
		v := a.View()
		_ = os.WriteFile(dir+"/"+name+".ansi", []byte(regexp.MustCompile(`\x1b\[\d+z`).ReplaceAllString(v.Content, "")), 0o644)
	}
	dump("terminal")
	a.modal = a.helpModal()
	dump("hilfe")
	a.modal = nil
	a.scr = scrManager
	dump("manager")
}
