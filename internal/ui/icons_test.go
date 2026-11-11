package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
)

// Zeichen, die Cascadia Mono (Standardschrift im Windows Terminal) selbst
// hat. Alles andere holt Windows aus einer Ersatzschrift: andere Breite und
// Strichstärke, der Rahmen verrutscht. ⇧ und ⌘ sind Tastennamen und dürfen
// bleiben. Nerd-Font-Zeichen stehen nur in icons.go.
const cascadiaGlyphs = "·»«ÄÖ×ÜßäöøØüé–—‘’“”„•…‹›←↑→↓↔↕⇡∆−≈≠≡⏎␣─━│┃┄┆┌┐└┘┏┓┗┛├┤┬┴┼║╔╗╦╩╭╮╰╯▁▂▃▀▄▅▆▇█▉▊▋▌▍▎▏▐░▒▓■□▣▤▦▪▲▶▸►▾◂◆◇◈◉○◌◎●◐◦◷♥♦✓❯⠇⠋⠏⠙⠦⠧⠴⠸⠹⠼⠿" + "⇧⌘"

func TestGlyphsInCascadia(t *testing.T) {
	var files []string
	for _, dir := range []string{".", "../i18n"} {
		m, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		files = append(files, m...)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || filepath.Base(f) == "icons.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[rune]bool{}
		for _, r := range string(b) {
			if r > 0x7e && !strings.ContainsRune(cascadiaGlyphs, r) && !seen[r] {
				seen[r] = true
				t.Errorf("%s: %q (U+%04X) fehlt in Cascadia Mono, bitte ein Zeichen aus cascadiaGlyphs oder ic() nehmen", f, r, r)
			}
		}
	}
}

// Jede Zeile der Palette hat eine gleich breite Symbolspalte: Text-Tags
// [x] drei Spalten, Nerd Font eine.
func TestPaletteIconColumn(t *testing.T) {
	defer func() { iconSet = "" }()
	for _, set := range []string{"", iconUni, iconMin, iconNerd} {
		a := newPaletteTestApp(t)
		a.st.AddConnections([]domain.Connection{{Alias: "web-01", Host: "10.0.0.1", Port: 22, Username: "u", Auth: domain.AuthPassword}})
		a.st.UpdateSettings(func(s *domain.Settings) { s.Icons = set })
		a.applyLook()
		items := a.paletteItems()
		a.palEnter("settings")
		items = append(items, a.paletteItems()...)
		for _, it := range items {
			if w := ansi.StringWidth(padIcon(it.icon)); w != iconCol() {
				t.Errorf("Stil %q: %q hat Symbol %q mit Breite %d", set, it.label, it.icon, w)
			}
		}
	}
	for name, d := range iconDefs {
		if len(d.tag) != 3 || d.tag[0] != '[' || d.tag[2] != ']' {
			t.Errorf("%s: Tag %q ist nicht [x]", name, d.tag)
		}
		if ansi.StringWidth(d.nerd) != 1 {
			t.Errorf("%s: Nerd-Symbol %q nicht eine Spalte", name, d.nerd)
		}
	}
}

func TestIconStyles(t *testing.T) {
	defer func() { iconSet = "" }()
	a := newPaletteTestApp(t)
	if ic("settings") != "[:]" {
		t.Fatalf("Standard sind Text-Tags, nicht %q", ic("settings"))
	}
	// Unicode und Minimal schalten sofort um.
	a.chooseIcons(iconUni)
	if a.modal != nil || a.st.Settings().Icons != iconUni || ic("settings") != "◈" || devGlyph(devLinux) != "❯" {
		t.Fatalf("Unicode: %q %q", a.st.Settings().Icons, ic("settings"))
	}
	a.chooseIcons(iconMin)
	if ic("settings") != "»" || ic("manager") != "▸" || ic("ai") != "◆" || devGlyph(devSwitch) != "■" {
		t.Fatalf("Minimal: %q %q", ic("settings"), ic("manager"))
	}
	// Nerd Font fragt erst mit einer Probezeile; Nein lässt den Stil.
	a.chooseIcons(iconNerd)
	if a.modal == nil || a.modal.kind != mConfirm || !strings.Contains(a.modal.body, devNerd[devLinux]) || !strings.Contains(a.modal.body, "Nerd Font") {
		t.Fatalf("keine Probe: %+v", a.modal)
	}
	a.modal = nil
	if a.st.Settings().Icons != iconMin {
		t.Fatal("ohne Ja darf nichts umschalten")
	}
	a.chooseIcons(iconNerd)
	a.modal.onYes()
	a.modal = nil
	if a.st.Settings().Icons != iconNerd || ic("settings") != "\uf013" {
		t.Fatalf("Nerd an: %q %q", a.st.Settings().Icons, ic("settings"))
	}
	a.chooseIcons("")
	if a.st.Settings().Icons != "" || ic("settings") != "[:]" {
		t.Fatal("zurück zu Text-Tags")
	}
	// Die Palette zeigt einen Durchschalt-Eintrag; die Suche findet alle
	// vier Stile einzeln, den aktiven mit Haken.
	a.palEnter("settings")
	if it, ok := hasLabel(a.paletteItems(), "Symbole"); !ok || it.cycle == nil || it.hint != "Text-Tags" {
		t.Fatalf("Durchschalt-Eintrag Symbole: %+v", it)
	}
	a.pal.in.SetValue("symbole")
	n := 0
	for _, it := range a.paletteItems() {
		if strings.HasPrefix(it.label, "Symbole: ") {
			n++
		}
	}
	if n != len(iconStyles) {
		t.Fatalf("%d Symbol-Stile in der Palette", n)
	}
}

func TestCustomTheme(t *testing.T) {
	th := CustomTheme(domain.CustomColors{Base: "matrix", Accent: "#ff0000", Text: "kaputt"})
	if hexOf(th.Accent) != "#ff0000" || hexOf(th.Text) != hexOf(Matrix.Text) || hexOf(th.Bg) != hexOf(Matrix.Bg) {
		t.Fatalf("Akzent %s Text %s", hexOf(th.Accent), hexOf(th.Text))
	}
	light := CustomTheme(domain.CustomColors{Base: "nacht", Bg: "#ffffff"})
	if light.Dark {
		t.Fatal("weißer Hintergrund ist hell")
	}
	if p := contrastProblems(light); len(p) == 0 {
		t.Fatal("helle Schrift auf Weiß muss auffallen")
	}
	for _, th := range Themes {
		if p := contrastProblems(th); len(p) > 0 {
			t.Errorf("%s: %v", th.Name, p)
		}
	}
	for _, hex := range []string{"#39ff14", "#7aa2f7", "#808080", "#000000", "#ffffff", "#c4122f"} {
		c, _ := parseHex(hex)
		h, s, l := toHSL(c)
		if got := hexOf(fromHSL(h, s, l)); got != hex {
			t.Errorf("HSL %s → %s", hex, got)
		}
	}
}

func TestColorEditor(t *testing.T) {
	a := newPaletteTestApp(t)
	a.pal, a.intro = nil, nil // Startanimation schluckt sonst die erste Taste
	a.openColors()
	press := func(keys ...string) {
		for _, k := range keys {
			var m tea.KeyPressMsg
			switch k {
			case "up":
				m = tea.KeyPressMsg{Code: tea.KeyUp}
			case "down":
				m = tea.KeyPressMsg{Code: tea.KeyDown}
			case "right":
				m = tea.KeyPressMsg{Code: tea.KeyRight}
			case "enter":
				m = tea.KeyPressMsg{Code: tea.KeyEnter}
			case "esc":
				m = tea.KeyPressMsg{Code: tea.KeyEscape}
			case "bs":
				m = tea.KeyPressMsg{Code: tea.KeyBackspace}
			case "ctrl+s":
				m = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
			default:
				m = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
			}
			a.Update(m)
		}
	}
	// Vorlage wechseln: Nacht → nächstes Theme, Vorschau gilt sofort.
	press("up", "right")
	if a.colEd.c.Base != Themes[1].Name || hexOf(a.s.T.Bg) != hexOf(Themes[1].Bg) {
		t.Fatalf("Vorlage %q, Bg %s", a.colEd.c.Base, hexOf(a.s.T.Bg))
	}
	// Esc ohne Speichern: altes Aussehen zurück.
	press("esc")
	if a.scr != scrManager || hexOf(a.s.T.Bg) != hexOf(Nacht.Bg) || a.st.Settings().Theme != "nacht" {
		t.Fatalf("Esc: scr %v Bg %s Theme %q", a.scr, hexOf(a.s.T.Bg), a.st.Settings().Theme)
	}
	// Akzent per Hex setzen und speichern.
	a.openColors()
	press("down", "down", "down", "down", "enter") // Hintergrund → Akzent
	if !a.colEd.editing || colorFields[a.colEd.sel-1].label != "Akzent" {
		t.Fatalf("Hex-Eingabe für %v", a.colEd.sel)
	}
	for range 7 {
		press("bs")
	}
	press("#", "f", "f", "8", "8", "0", "0", "enter")
	press("ctrl+s")
	set := a.st.Settings()
	if set.Theme != CustomName || set.Colors == nil || set.Colors.Accent != "#ff8800" || set.Colors.Base != "nacht" {
		t.Fatalf("gespeichert: %q %+v", set.Theme, set.Colors)
	}
	if hexOf(a.s.T.Accent) != "#ff8800" || a.scr != scrManager {
		t.Fatalf("Akzent %s", hexOf(a.s.T.Accent))
	}
	if !strings.Contains(ansi.Strip(a.View().Content), ">_ sshTermy") {
		t.Fatal("Logo fehlt")
	}
}
