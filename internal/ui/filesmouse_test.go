package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/sshx"
)

// filesApp: Dateibrowser mit zwei lokalen Seiten (rechts spielt „Server“).
func filesApp(t *testing.T) (*App, *session, string, string) {
	t.Helper()
	a := dangerApp(t)
	s, _ := typedSession(domain.Connection{Alias: "web01"}, "dominik@web01:~$ ", "")
	a.sessions = []*session{s}
	a.scr = scrTerminal
	a.w, a.h, a.intro = 160, 40, nil
	a.resizeAll()
	l, r := t.TempDir(), t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(r, "conf.d"), 0o755))
	conf := "user www-data;\nworker_processes auto;\n\nhttp {\n    ssl_protocols TLSv1.2 TLSv1.3;\n    gzip on;\n}\n"
	must(os.WriteFile(filepath.Join(r, "nginx.conf"), []byte(conf), 0o644))
	must(os.WriteFile(filepath.Join(r, "big.bin"), append([]byte{0, 1, 2}, make([]byte, 5000)...), 0o644))
	must(os.WriteFile(filepath.Join(r, "aaa.txt"), []byte("x"), 0o644))
	must(os.WriteFile(filepath.Join(l, "notizen.md"), []byte("# Notizen\n"), 0o644))
	f := &s.files
	f.open = true
	lp := &pane{fs: sshx.LocalFS{}, path: l}
	rp := &pane{fs: sshx.LocalFS{}, path: r}
	lp.load(false)
	rp.load(false)
	f.panes, f.side = [2]*pane{lp, rp}, 1
	return a, s, l, r
}

func screenAt(t *testing.T, a *App, word string, nth int) (int, int) {
	t.Helper()
	lines := strings.Split(ansi.Strip(a.View().Content), "\n")
	n := 0
	for y, l := range lines {
		off := 0
		for {
			i := strings.Index(l[off:], word)
			if i < 0 {
				break
			}
			if n == nth {
				return ansi.StringWidth(l[:off+i]), y
			}
			n++
			off += i + len(word)
		}
	}
	t.Fatalf("%q nicht gefunden:\n%s", word, strings.Join(lines, "\n"))
	return 0, 0
}

// run führt Befehle samt Folgenachrichten aus (ohne echte Wartezeit).
func run(a *App, cmd tea.Cmd) {
	for i := 0; cmd != nil && i < 10; i++ {
		msg := cmd()
		switch m := msg.(type) {
		case tea.BatchMsg:
			for _, c := range m {
				run(a, c)
			}
			return
		case nil:
			return
		default:
			_, cmd = a.Update(m)
		}
	}
}

func dbl(a *App, x, y int) tea.Cmd {
	a.click = clickState{}
	a.mouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	a.mouse(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	cmd := a.mouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	a.mouse(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	return cmd
}

// Doppelklick auf eine Datei zeigt ihren Inhalt; Ziehen markiert und
// kopiert, Doppelklick auf ein Wort sucht, Esc schließt.
func TestFilesDoubleClickViewer(t *testing.T) {
	a, s, _, _ := filesApp(t)
	x, y := screenAt(t, a, "nginx.conf", 0)
	run(a, dbl(a, x, y))
	v := s.files.view
	if v == nil || v.busy || len(v.lines) != 7 {
		t.Fatalf("Betrachter nicht offen: %+v", v)
	}
	out := ansi.Strip(a.View().Content)
	if !strings.Contains(out, "worker_processes auto;") || !strings.Contains(out, "Ansehen") {
		t.Fatalf("Inhalt fehlt:\n%s", out)
	}

	// Ziehen über „TLSv1.2“ kopiert genau das
	tx, ty := screenAt(t, a, "TLSv1.2", 0)
	a.click = clickState{}
	a.mouse(tea.MouseClickMsg{X: tx, Y: ty, Button: tea.MouseLeft})
	a.mouse(tea.MouseMotionMsg{X: tx + 4, Y: ty, Button: tea.MouseLeft})
	cmd := a.mouse(tea.MouseReleaseMsg{X: tx + 7, Y: ty, Button: tea.MouseLeft})
	if got := v.selText(); got != "TLSv1.2" {
		t.Fatalf("Markierung = %q", got)
	}
	if cmd == nil {
		t.Fatal("Loslassen muss kopieren")
	}

	// Doppelklick auf „gzip“ sucht es
	gx, gy := screenAt(t, a, "gzip", 0)
	dbl(a, gx+1, gy)
	if v.query != "gzip" || v.hit != 5 {
		t.Fatalf("Suche: %q Treffer %d", v.query, v.hit)
	}
	// Rad blättert, Esc hebt erst die Suche auf, dann zu
	a.mouse(tea.MouseWheelMsg{X: gx, Y: gy, Button: tea.MouseWheelDown})
	if v.top == 0 {
		t.Fatal("Rad blättert nicht")
	}
	a.filesKey(s, tea.KeyPressMsg{Code: tea.KeyEscape})
	a.filesKey(s, tea.KeyPressMsg{Code: tea.KeyEscape})
	if s.files.view != nil || !s.files.open {
		t.Fatal("Esc schließt den Betrachter, nicht die Dateien")
	}

	// Binärdatei: Hex
	run(a, a.openViewer(s, 1, filepath.Join(s.files.panes[1].path, "big.bin")))
	if !s.files.view.binary || !strings.HasPrefix(s.files.view.lines[0], "00000000  00 01 02") {
		t.Fatalf("Hex fehlt: %q", s.files.view.lines[:1])
	}
}

// Mitlesen hängt neue Zeilen an (tail -f).
func TestFilesViewerFollow(t *testing.T) {
	a, s, _, r := filesApp(t)
	p := filepath.Join(r, "app.log")
	if err := os.WriteFile(p, []byte("start\nERROR kaputt\nhalbe"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(a, a.openViewer(s, 1, p))
	v := s.files.view
	a.filesKey(s, runeKey('f'))
	if !v.follow || !v.log {
		t.Fatal("f schaltet Mitlesen nicht an")
	}
	fh, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	fh.WriteString(" Zeile\nneu\n")
	fh.Close()
	_, cmd := a.Update(fileFollowTick{s.id, v.seq})
	msg := cmd().(fileFollowMsg)
	if _, ok := a.fileViewMsgs(msg); !ok {
		t.Fatal("Nachricht nicht verarbeitet")
	}
	if got := strings.Join(v.lines, "|"); got != "start|ERROR kaputt|halbe|halbe Zeile|neu" && got != "start|ERROR kaputt|halbe Zeile|neu" {
		t.Logf("Zeilen: %q", got)
	}
	if v.lines[len(v.lines)-1] != "neu" {
		t.Fatalf("neue Zeile fehlt: %q", v.lines)
	}
}

// Rechtsklick öffnet das Menü; Umbenennen und Rechte gehen darüber.
func TestFilesRightClickMenu(t *testing.T) {
	a, s, _, r := filesApp(t)
	x, y := screenAt(t, a, "nginx.conf", 0)
	a.mouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseRight})
	if a.modal == nil || a.modal.kind != mMenu {
		t.Fatal("kein Menü")
	}
	var labels []string
	for _, it := range a.modal.items {
		labels = append(labels, it.label)
	}
	all := strings.Join(labels, "|")
	for _, want := range []string{"Ansehen", "Umbenennen", "Rechte ändern", "Pfad kopieren", "Im Terminal: cd hierhin", "Löschen …"} {
		if !strings.Contains(all, want) {
			t.Fatalf("%q fehlt im Menü: %s", want, all)
		}
	}
	rename := func() {
		for _, it := range a.modal.items {
			if it.label == "Umbenennen" {
				a.modal = nil
				it.run()
				return
			}
		}
	}
	rename()
	if a.modal == nil || a.modal.kind != mInput {
		t.Fatal("Umbenennen fragt nicht nach dem Namen")
	}
	a.modal.onText("nginx.conf.bak")
	if _, err := os.Stat(filepath.Join(r, "nginx.conf.bak")); err != nil {
		t.Fatal("nicht umbenannt")
	}
	p := s.files.panes[1]
	if p.entries[p.sel].Name != "nginx.conf.bak" {
		t.Fatal("Auswahl folgt dem neuen Namen nicht")
	}
	// gibt es schon: kein Überschreiben
	if err := p.fs.Rename(filepath.Join(r, "aaa.txt"), filepath.Join(r, "nginx.conf.bak")); err == nil {
		t.Fatal("Umbenennen überschreibt")
	}
	// Rechte
	a.askChmod(s, 1, p.sel)
	a.modal.onText("600")
	if fi, _ := os.Stat(filepath.Join(r, "nginx.conf.bak")); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 { // Windows kennt nur schreibgeschützt
		t.Fatalf("Rechte = %o", fi.Mode().Perm())
	}
	// leerer Platz: Ordner-Menü
	a.modal = nil
	ex, ey := screenAt(t, a, "nginx.conf.bak", 0)
	a.mouse(tea.MouseClickMsg{X: ex, Y: ey + 8, Button: tea.MouseRight})
	if a.modal == nil || a.modal.items[0].label != "Neuer Ordner" {
		t.Fatal("Rechtsklick auf leeren Platz ohne Ordner-Menü")
	}
}

// Spaltenköpfe sortieren, Pfadleiste springt, Strg/Umschalt-Klick markiert,
// Maustaste „Zurück“ geht eine Ebene hoch.
func TestFilesSortCrumbsMarks(t *testing.T) {
	a, s, _, r := filesApp(t)
	p := s.files.panes[1]
	big := filepath.Join(r, "zzz.dat")
	os.WriteFile(big, make([]byte, 9000), 0o644)
	old := time.Now().Add(-48 * time.Hour)
	os.Chtimes(filepath.Join(r, "aaa.txt"), old, old)
	p.load(false)

	x, y := screenAt(t, a, "Größe", 1) // rechte Seite
	a.mouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if p.sortBy != sortSize || !p.desc {
		t.Fatalf("Sortierung %d desc=%v", p.sortBy, p.desc)
	}
	if p.entries[0].Name != ".." || p.entries[1].Name != "conf.d" || p.entries[2].Name != "zzz.dat" {
		t.Fatalf("nach Größe: %v", names(p.entries))
	}
	a.mouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if p.desc || p.entries[2].Name != "aaa.txt" {
		t.Fatalf("zweiter Klick dreht nicht um: %v", names(p.entries))
	}

	// Strg+Klick markiert einzeln, Umschalt+Klick einen Bereich
	ax, ay := screenAt(t, a, "aaa.txt", 0)
	a.click = clickState{}
	a.mouse(tea.MouseClickMsg{X: ax, Y: ay, Button: tea.MouseLeft, Mod: tea.ModCtrl})
	a.mouse(tea.MouseReleaseMsg{X: ax, Y: ay, Button: tea.MouseLeft})
	if !p.marked["aaa.txt"] || len(p.marked) != 1 {
		t.Fatalf("Strg+Klick: %v", p.marked)
	}
	zx, zy := screenAt(t, a, "zzz.dat", 0)
	a.click = clickState{}
	a.mouse(tea.MouseClickMsg{X: zx, Y: zy, Button: tea.MouseLeft, Mod: tea.ModShift})
	a.mouse(tea.MouseReleaseMsg{X: zx, Y: zy, Button: tea.MouseLeft})
	if len(p.marked) != 4 {
		t.Fatalf("Umschalt+Klick: %v", p.marked)
	}

	// in conf.d, dann Pfadleiste zurück
	cx, cy := screenAt(t, a, "conf.d/", 0)
	dbl(a, cx, cy)
	if filepath.Base(p.path) != "conf.d" {
		t.Fatalf("Doppelklick auf Ordner: %s", p.path)
	}
	bx, by := screenAt(t, a, filepath.Base(r), 0)
	a.click = clickState{}
	a.mouse(tea.MouseClickMsg{X: bx + 1, Y: by, Button: tea.MouseLeft})
	if p.path != r {
		t.Fatalf("Pfadleiste: %s", p.path)
	}
	// Maustaste „Zurück“
	a.mouse(tea.MouseClickMsg{X: bx + 1, Y: by + 3, Button: tea.MouseBackward})
	if p.path != filepath.Dir(r) {
		t.Fatalf("Zurück-Taste: %s", p.path)
	}
}

// Einfachklick auf eine Textdatei zeigt ihren Anfang unter der Liste.
func TestFilesPeek(t *testing.T) {
	a, s, _, _ := filesApp(t)
	x, y := screenAt(t, a, "nginx.conf", 0)
	a.click = clickState{}
	cmd := a.mouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	a.mouse(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("Klick stößt keine Vorschau an")
	}
	run(a, cmd)
	out := ansi.Strip(a.View().Content)
	if !strings.Contains(out, "Vorschau") || !strings.Contains(out, "worker_processes") {
		t.Fatalf("Vorschau fehlt:\n%s", out)
	}
	run(a, a.filesKey(s, runeKey('v')))
	if strings.Contains(ansi.Strip(a.View().Content), "worker_processes") {
		t.Fatal("v schaltet die Vorschau nicht aus")
	}
	if !a.st.Settings().FilesNoPeek {
		t.Fatal("Einstellung nicht gemerkt")
	}
}

func names(es []sshx.Entry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Name)
	}
	return out
}
