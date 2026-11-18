package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Ideen aus nukri im Dateien-Fenster (F5). Beide Seiten sind in den Tests
// lokale Ordner; was auf dem Server per Befehl läuft, prüfen die Tests der
// Befehle und ihrer Auswertung.

func fkey(s string) tea.KeyPressMsg {
	switch s {
	case "delete":
		return tea.KeyPressMsg{Code: tea.KeyDelete}
	case "shift+delete":
		return tea.KeyPressMsg{Code: tea.KeyDelete, Mod: tea.ModShift}
	case "alt+left":
		return tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt}
	case "alt+right":
		return tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt}
	case "shift+f2":
		return tea.KeyPressMsg{Code: tea.KeyF2, Mod: tea.ModShift}
	case "ctrl+z":
		return tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl}
	}
	r := []rune(s)
	return runeKey(r[0])
}

func selectEntry(t *testing.T, p *pane, name string) {
	t.Helper()
	p.selectName(name)
	if p.entries[p.sel].Name != name {
		t.Fatalf("%s nicht in der Liste", name)
	}
}

func TestFilesTrashAndUndo(t *testing.T) {
	a, s, _, r := filesApp(t)
	trashOverride = filepath.Join(t.TempDir(), "Trash")
	defer func() { trashOverride = "" }()
	p := s.files.panes[1]
	selectEntry(t, p, "aaa.txt")
	a.filesKey(s, fkey("d"))
	if _, err := os.Stat(filepath.Join(r, "aaa.txt")); err == nil {
		t.Fatal("d muss in den Papierkorb verschieben")
	}
	if _, err := os.Stat(filepath.Join(trashOverride, "files", "aaa.txt")); err != nil {
		t.Fatal("nicht im Papierkorb:", err)
	}
	info, err := os.ReadFile(filepath.Join(trashOverride, "info", "aaa.txt.trashinfo"))
	if err != nil || !strings.Contains(string(info), "Path="+filepath.ToSlash(filepath.Join(r, "aaa.txt"))) {
		t.Fatalf(".trashinfo: %q %v", info, err)
	}
	if a.undo == nil || a.modal != nil {
		t.Fatal("ohne Rückfrage, mit Rückgängig")
	}
	a.filesKey(s, fkey("ctrl+z"))
	if _, err := os.Stat(filepath.Join(r, "aaa.txt")); err != nil {
		t.Fatal("Strg+Z holt nicht zurück:", err)
	}
	if _, err := os.Stat(filepath.Join(trashOverride, "info", "aaa.txt.trashinfo")); err == nil {
		t.Fatal(".trashinfo bleibt liegen")
	}
	// zweimal gleicher Name: .2
	selectEntry(t, p, "aaa.txt")
	a.filesKey(s, fkey("d"))
	os.WriteFile(filepath.Join(r, "aaa.txt"), []byte("y"), 0o644)
	p.load(false)
	selectEntry(t, p, "aaa.txt")
	a.filesKey(s, fkey("delete"))
	if _, err := os.Stat(filepath.Join(trashOverride, "files", "aaa.txt.2")); err != nil {
		t.Fatal("zweiter Name:", err)
	}
	// ⇧D fragt und löscht endgültig
	selectEntry(t, p, "nginx.conf")
	a.filesKey(s, fkey("D"))
	if a.modal == nil || a.modal.kind != mConfirm {
		t.Fatal("⇧D muss fragen")
	}
	run(a, a.modal.onYes())
	if _, err := os.Stat(filepath.Join(r, "nginx.conf")); err == nil {
		t.Fatal("nicht gelöscht")
	}
	// im Papierkorb löscht d endgültig (mit Rückfrage)
	a.modal = nil
	run(a, a.openTrash(s, 1))
	if !inTrash(p.fs, p.path) {
		t.Fatalf("Papierkorb nicht geöffnet: %s", p.path)
	}
	selectEntry(t, p, "aaa.txt.2")
	a.filesKey(s, fkey("d"))
	if a.modal == nil || a.modal.kind != mConfirm {
		t.Fatal("im Papierkorb muss d fragen")
	}
}

func TestFilesPackUnpackLocal(t *testing.T) {
	a, s, _, r := filesApp(t)
	p := s.files.panes[1]
	os.WriteFile(filepath.Join(r, "conf.d", "site.conf"), []byte("server {}\n"), 0o644)
	selectEntry(t, p, "conf.d")
	a.filesKey(s, fkey("z"))
	if a.modal == nil || a.modal.kind != mInput || a.modal.in.Value() != "conf.d.zip" {
		t.Fatalf("Packen fragt nach dem Namen: %+v", a.modal)
	}
	submit := a.modal.onText
	a.modal = nil
	run(a, submit("conf.zip"))
	if s.files.job != nil {
		t.Fatal("Job hängt")
	}
	if _, err := os.Stat(filepath.Join(r, "conf.zip")); err != nil {
		t.Fatal("kein Archiv:", err)
	}
	if p.entries[p.sel].Name != "conf.zip" {
		t.Fatalf("Archiv nicht ausgewählt: %s", p.entries[p.sel].Name)
	}
	// Vorschau listet den Inhalt
	lines, _, kind, err := readPeek(p.fs, filepath.Join(r, "conf.zip"), false)
	if err != nil || kind != kindArchive || !strings.Contains(strings.Join(lines, "\n"), "conf.d/site.conf") {
		t.Fatalf("Archiv-Vorschau: %q %v", lines, err)
	}
	run(a, a.filesKey(s, fkey("u")))
	if _, err := os.Stat(filepath.Join(r, "conf", "conf.d", "site.conf")); err != nil {
		t.Fatal("u entpackt nicht:", err)
	}
}

func TestFilesUnpackRuns(t *testing.T) {
	a, s, _, r := filesApp(t)
	p := s.files.panes[1]
	os.WriteFile(filepath.Join(r, "conf.d", "site.conf"), []byte("server {}\n"), 0o644)
	selectEntry(t, p, "conf.d")
	run(a, a.pack(s, 1, p.picked(), "conf.tar.gz"))
	selectEntry(t, p, "conf.tar.gz")
	run(a, a.unpack(s, 1, p.sel))
	b, err := os.ReadFile(filepath.Join(r, "conf", "conf.d", "site.conf"))
	if err != nil || string(b) != "server {}\n" {
		t.Fatalf("entpackt: %q %v", b, err)
	}
	if p.entries[p.sel].Name != "conf" {
		t.Fatalf("neuer Ordner nicht ausgewählt: %s", p.entries[p.sel].Name)
	}
	// noch einmal: conf-2
	selectEntry(t, p, "conf.tar.gz")
	run(a, a.unpack(s, 1, p.sel))
	if _, err := os.Stat(filepath.Join(r, "conf-2")); err != nil {
		t.Fatal("zweites Entpacken überschreibt?", err)
	}
}

func TestFindParsingAndFuzzy(t *testing.T) {
	es := parseFind("./conf.d/\n./\n./nginx.conf\n./conf.d/site.conf\n")
	if len(es) != 3 || !es[0].isDir || es[1].rel != "nginx.conf" || es[2].rel != "conf.d/site.conf" {
		t.Fatalf("%+v", es)
	}
	c := findCommand("/etc", false)
	if !strings.Contains(c, "cd '/etc' && find . -xdev -maxdepth 12 \\( -name '.?*' -prune \\) -o -type d") {
		t.Fatalf("%s", c)
	}
	if strings.Contains(findCommand("/etc", true), "prune") {
		t.Fatal("mit versteckten kein prune")
	}
	items := []pickItem{{label: "conf.d/nginx-ganz-lang/config.txt"}, {label: "nginx.conf"}, {label: "x/ngcfa"}, {label: "readme"}}
	got := fuzzyPick(items, "ngcf")
	if len(got) != 3 || got[0].label != "x/ngcfa" || got[1].label != "nginx.conf" {
		t.Fatalf("Reihenfolge: %+v", got)
	}
}

func TestFilesFindLocal(t *testing.T) {
	a, s, _, r := filesApp(t)
	os.MkdirAll(filepath.Join(r, "conf.d", "tief"), 0o755)
	os.WriteFile(filepath.Join(r, "conf.d", "tief", "site.conf"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(r, ".geheim"), []byte("x"), 0o644)
	cmd := a.filesKey(s, fkey("/"))
	if a.pick == nil || !a.pick.fuzzy {
		t.Fatal("/ öffnet die Suche nicht")
	}
	run(a, cmd)
	labels := []string{}
	for _, it := range a.pick.items {
		labels = append(labels, it.label)
	}
	all := strings.Join(labels, "|")
	if !strings.Contains(all, "conf.d/tief/site.conf") || strings.Contains(all, ".geheim") {
		t.Fatalf("Baum: %s", all)
	}
	a.pick.in.SetValue("sitcf")
	vis := a.pick.visible()
	if len(vis) == 0 || vis[0].label != "conf.d/tief/site.conf" {
		t.Fatalf("unscharf: %+v", vis)
	}
	run(a, a.pickerChoose(vis[0]))
	p := s.files.panes[1]
	if p.path != filepath.Join(r, "conf.d", "tief") || p.entries[p.sel].Name != "site.conf" {
		t.Fatalf("Sprung: %s %s", p.path, p.entries[p.sel].Name)
	}
}

func TestFilesDirSizesAndDups(t *testing.T) {
	a, s, _, r := filesApp(t)
	big := strings.Repeat("x", 50000)
	os.WriteFile(filepath.Join(r, "conf.d", "a.bin"), []byte(big), 0o644)
	os.WriteFile(filepath.Join(r, "conf.d", "b.bin"), []byte(big), 0o644)
	os.WriteFile(filepath.Join(r, "c.bin"), []byte(big), 0o644)
	p := s.files.panes[1]
	run(a, a.filesKey(s, fkey("i")))
	if p.sizes["conf.d"] != 100000 || p.sortBy != sortSize || !p.desc {
		t.Fatalf("Größen: %v sort %d", p.sizes, p.sortBy)
	}
	if p.entries[1].Name != "conf.d" {
		t.Fatalf("größter Eintrag oben: %s", p.entries[1].Name)
	}
	view := ansi.Strip(a.View().Content)
	if !strings.Contains(view, "97.7 KB") || !strings.Contains(view, "Belegt:") {
		t.Fatalf("Ordnergröße fehlt in der Ansicht")
	}
	// anderer Ordner: keine alten Größen
	p.cd("conf.d", false)
	if _, ok := p.sizeOf(sshx.Entry{Name: "x", IsDir: true}); ok {
		t.Fatal("Größen gelten nur für ihren Ordner")
	}
	p.cd("..", false)

	run(a, a.filesKey(s, fkey("I")))
	if a.pick == nil {
		t.Fatalf("keine Liste doppelter Dateien: %s", ansi.Strip(s.files.last))
	}
	var files []string
	for _, it := range a.pick.items {
		if it.value != "" {
			files = append(files, it.value)
		}
	}
	if strings.Join(files, ",") != "c.bin,conf.d/a.bin,conf.d/b.bin" {
		t.Fatalf("Gruppe: %v", files)
	}
	if !strings.Contains(a.pick.title, "97.7 KB") {
		t.Fatalf("frei möglich: %s", a.pick.title)
	}
}

func TestSpaceParsers(t *testing.T) {
	sizes, total := parseDu("4\t./a\n1024\t./log\n12\t./.cache\n1040\t.\n")
	if sizes["log"] != 1024*1024 || sizes[".cache"] != 12*1024 || total != 1040*1024 {
		t.Fatalf("%v %d", sizes, total)
	}
	fs := parseSizeList("10 ./a\n10 ./b c\n0 ./leer\n5 ./d\n")
	by := sameSize(fs)
	if len(by) != 1 || len(by[10]) != 2 || by[10][1] != "b c" {
		t.Fatalf("%v", by)
	}
	sums := parseHashes("abc  ./a\nabc  ./b c\n")
	gs := groupDups(by, sums)
	if len(gs) != 1 || gs[0].wasted() != 10 {
		t.Fatalf("%+v", gs)
	}
	if c := hashCommand("/srv", []string{"a", "b c"}); !strings.HasSuffix(c, `$H -- './a' './b c' 2>/dev/null`) {
		t.Fatalf("%s", c)
	}
	if c := dupListCommand("/srv", true); !strings.Contains(c, "-printf '%s %p\\n'") || !strings.Contains(c, "stat -c '%s %n'") {
		t.Fatalf("%s", c)
	}
}

func TestFilesHistoryAndMarks(t *testing.T) {
	a, s, l, r := filesApp(t)
	p := s.files.panes[1]
	selectEntry(t, p, "conf.d")
	a.filesKey(s, fkey("l"))
	if p.path != filepath.Join(r, "conf.d") {
		t.Fatal("cd")
	}
	a.filesKey(s, fkey("alt+left"))
	if p.path != r {
		t.Fatalf("Alt+← : %s", p.path)
	}
	a.filesKey(s, fkey("alt+right"))
	if p.path != filepath.Join(r, "conf.d") {
		t.Fatalf("Alt+→ : %s", p.path)
	}
	// Seite links: m merkt lokal
	s.files.side = 0
	a.filesKey(s, fkey("m"))
	if got := a.st.Settings().FileMarks; len(got) != 1 || got[0] != l {
		t.Fatalf("gemerkt: %v", got)
	}
	run(a, a.filesKey(s, fkey("g")))
	if a.pick == nil || !strings.HasPrefix(a.pick.items[0].label, "♦ ") {
		t.Fatalf("Orte: %+v", a.pick)
	}
	a.pick = nil
	a.filesKey(s, fkey("m"))
	if len(a.st.Settings().FileMarks) != 0 {
		t.Fatal("zweites m vergisst")
	}
	// breites Fenster: Orte-Leiste
	a.w = 170
	a.resizeAll()
	if !strings.Contains(ansi.Strip(a.View().Content), " Orte ") {
		t.Fatal("Orte-Leiste fehlt bei breitem Fenster")
	}
}

func TestParseMounts(t *testing.T) {
	m := parseMounts("/dev/sda1 / ext4 rw 0 0\nproc /proc proc rw 0 0\n/dev/sdb1 /mnt/daten\\040neu xfs rw 0 0\ntmpfs /run tmpfs rw 0 0\n/dev/sdc1 /run/media/usb vfat rw 0 0\n/dev/loop0 /snap/core/1 squashfs ro 0 0\n")
	if strings.Join(m, ",") != "/mnt/daten neu,/run/media/usb" {
		t.Fatalf("%v", m)
	}
}

func TestPlanRenames(t *testing.T) {
	exists := func(n string) bool { return n == "fremd" }
	old := []string{"a", "b", "c"}
	pairs, errs := planRenames(old, []string{"b", "a", "c", ""}, exists)
	if len(errs) > 0 || len(pairs) != 2 {
		t.Fatalf("Tausch: %v %v", pairs, errs)
	}
	for _, c := range []struct {
		lines []string
		want  string
	}{
		{[]string{"a", "b"}, "3 Zeilen"},
		{[]string{"x", "x", "c"}, "Zeile 2"},
		{[]string{"a/b", "b", "c"}, "enthält /"},
		{[]string{"fremd", "b", "c"}, "gibt es schon"},
		{[]string{"", "b", "c"}, "leer"},
	} {
		_, errs := planRenames(old, c.lines, exists)
		if len(errs) == 0 || !strings.Contains(errs[0], c.want) {
			t.Errorf("%v: %v", c.lines, errs)
		}
	}
	// Tauschen auf der Platte
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "a"), []byte("A"), 0o644)
	os.WriteFile(filepath.Join(d, "b"), []byte("B"), 0o644)
	if err := applyRenames(sshx.LocalFS{}, d, []renPair{{"a", "b"}, {"b", "a"}}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(d, "a")); string(b) != "B" {
		t.Fatalf("a = %q", b)
	}
	// Fehler mittendrin: alles zurück
	if err := applyRenames(sshx.LocalFS{}, d, []renPair{{"a", "x"}, {"fehlt", "y"}}); err == nil {
		t.Fatal("fehlende Datei muss scheitern")
	}
	if _, err := os.Stat(filepath.Join(d, "a")); err != nil {
		t.Fatal("nicht zurückgedreht")
	}
	es, _ := os.ReadDir(d)
	if len(es) != 2 {
		t.Fatalf("Zwischennamen liegen herum: %v", es)
	}
}

func TestBulkRenameFlow(t *testing.T) {
	a, s, _, r := filesApp(t)
	a.st.UpdateSettings(func(st *domain.Settings) { st.Editor = "true" })
	p := s.files.panes[1]
	p.marked = map[string]bool{"aaa.txt": true, "big.bin": true}
	a.filesKey(s, fkey("shift+f2"))
	b := a.bulkRen
	if b == nil {
		t.Fatal("kein Umbenennen gestartet")
	}
	data, _ := os.ReadFile(b.file)
	if string(data) != "aaa.txt\nbig.bin\n" {
		t.Fatalf("Liste: %q", data)
	}
	os.WriteFile(b.file, []byte("eins.txt\nbig.bin\n"), 0o600)
	b.mod = b.mod.Add(-1e9)
	a.bulkRenameTick()
	if a.modal == nil || a.modal.kind != mConfirm || !strings.Contains(a.modal.body, "aaa.txt → eins.txt") {
		t.Fatalf("Rückfrage: %+v", a.modal)
	}
	run(a, a.modal.onYes())
	if _, err := os.Stat(filepath.Join(r, "eins.txt")); err != nil {
		t.Fatal("nicht umbenannt")
	}
	if a.bulkRen != nil {
		t.Fatal("bleibt aktiv")
	}
}

func TestPreviewKinds(t *testing.T) {
	tab, ok := csvTable([]string{"name;alter", "anna;31", "bernd;4"}, "x.csv")
	if !ok || tab[0] != "name  │ alter" || !strings.HasPrefix(tab[1], "──────┼─") || tab[2] != "anna  │ 31" {
		t.Fatalf("%q", tab)
	}
	if _, ok := csvTable([]string{"nur eine spalte"}, "x.csv"); ok {
		t.Fatal("eine Spalte ist keine Tabelle")
	}
	code := mdCodeLines([]string{"# T", "```", "x := 1", "```", "text"})
	if code[0] || !code[2] || code[4] {
		t.Fatalf("%v", code)
	}
	if langFor("a.go", "") != &langGo || langFor("run", "#!/bin/bash") != &langSh || langFor("a.txt", "x") != nil {
		t.Fatal("Sprache")
	}
	a := dangerApp(t)
	for _, l := range []string{"func main() { // hi", `x := "s" + 3`, "key: wert # k", "- [link](http://x) **fett** `c`"} {
		for _, f := range []func() string{
			func() string { return a.colorLang(l, l, &langGo) },
			func() string { return a.colorLang(l, l, &langYAML) },
			func() string { return a.colorMD(l, l, false) },
		} {
			if got := ansi.Strip(f()); got != l {
				t.Errorf("Färbung ändert den Text: %q → %q", l, got)
			}
		}
	}
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "sub"), 0o755)
	os.WriteFile(filepath.Join(d, "f.txt"), []byte("12345"), 0o644)
	lines, _, kind, err := readPeek(sshx.LocalFS{}, d, true)
	if err != nil || kind != kindDir || !strings.Contains(lines[0], "1 Ordner, 1 Dateien") {
		t.Fatalf("Ordner: %q %v", lines, err)
	}
}

func TestFilesViewerCSVToggle(t *testing.T) {
	a, s, _, r := filesApp(t)
	os.WriteFile(filepath.Join(r, "liste.csv"), []byte("a,b\n1,2\n"), 0o644)
	run(a, a.openViewer(s, 1, filepath.Join(r, "liste.csv")))
	v := s.files.view
	if !v.table || !strings.Contains(v.lines[0], "│") {
		t.Fatalf("Tabelle: %q", v.lines)
	}
	a.filesKey(s, fkey("t"))
	if v.table || v.lines[0] != "a,b" {
		t.Fatalf("t zeigt Rohtext: %q", v.lines)
	}
}

func TestFilesCutPaste(t *testing.T) {
	a, s, l, r := filesApp(t)
	p := s.files.panes[1]
	selectEntry(t, p, "aaa.txt")
	a.filesKey(s, fkey("x"))
	if s.files.cut == nil || !strings.Contains(ansi.Strip(a.View().Content), "» aaa.txt") {
		t.Fatal("x schneidet nicht aus")
	}
	selectEntry(t, p, "conf.d")
	a.filesKey(s, fkey("l"))
	a.filesKey(s, fkey("p"))
	if _, err := os.Stat(filepath.Join(r, "conf.d", "aaa.txt")); err != nil {
		t.Fatal("nicht verschoben:", err)
	}
	if _, err := os.Stat(filepath.Join(r, "aaa.txt")); err == nil {
		t.Fatal("Original noch da")
	}
	if s.files.cut != nil {
		t.Fatal("Ausgeschnittenes bleibt hängen")
	}
	// auf die andere Seite: kopieren, dann Original weg
	selectEntry(t, p, "aaa.txt")
	a.filesKey(s, fkey("x"))
	s.files.side = 0
	run(a, a.filesKey(s, fkey("p")))
	if _, err := os.Stat(filepath.Join(l, "aaa.txt")); err != nil {
		t.Fatal("nicht kopiert:", err)
	}
	if _, err := os.Stat(filepath.Join(r, "conf.d", "aaa.txt")); err == nil {
		t.Fatal("Original nach dem Kopieren noch da")
	}
}
