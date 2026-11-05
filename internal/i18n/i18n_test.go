package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// needsEntry: Texte ohne Wörter (Symbole, Zahlen) und reine
// Tastenbeschriftungen („Strg+S“) brauchen keinen Eintrag.
func needsEntry(s string) bool {
	words := regexp.MustCompile(`[A-Za-zÄÖÜäöüß]{2,}`).FindAllString(s, -1)
	for _, w := range words {
		switch w {
		case "Strg", "Alt", "Shift", "Space", "Tab", "Esc", "Enter":
			continue
		}
		if regexp.MustCompile(`^F\d+$`).MatchString(w) {
			continue
		}
		return true
	}
	return false
}

// TestCatalogComplete: jeder Text, der im Code mit i18n.T/F/NewError
// übersetzt wird, hat eine englische Fassung.
func TestCatalogComplete(t *testing.T) {
	root := filepath.Join("..", "..")
	var missing []string
	seen := map[string]bool{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && (info.Name() == ".git" || info.Name() == "tools") {
			return filepath.SkipDir
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok || len(c.Args) == 0 {
				return true
			}
			sel, ok := c.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "i18n" {
				return true
			}
			switch sel.Sel.Name {
			case "T", "F", "NewError":
			default:
				return true
			}
			bl, ok := c.Args[0].(*ast.BasicLit)
			if !ok || bl.Kind != token.STRING {
				return true
			}
			s, _ := strconv.Unquote(bl.Value)
			if !seen[s] && needsEntry(s) && !Has(s) {
				missing = append(missing, s)
			}
			seen[s] = true
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(missing)
	for _, s := range missing {
		t.Errorf("keine englische Fassung für %q (internal/i18n/en.go)", s)
	}
}

// TestPlaceholders: Formatangaben (%s, %d …) bleiben in Zahl und Reihenfolge gleich.
func TestPlaceholders(t *testing.T) {
	verb := regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z%]`)
	for de, e := range en {
		a, b := verb.FindAllString(de, -1), verb.FindAllString(e, -1)
		if strings.Join(a, " ") != strings.Join(b, " ") {
			t.Errorf("%q: Platzhalter %v ≠ %v in %q", de, a, b, e)
		}
	}
}

func TestSwitch(t *testing.T) {
	defer Set(DE)
	if T("Verbindung") != "Verbindung" {
		t.Fatal("Deutsch muss unverändert bleiben")
	}
	Set(EN)
	if T("Verbindung") != "connection" || T("Strg+S") != "Ctrl+S" || Keys("Strg+Pfeil") != "Ctrl+Arrow" {
		t.Fatalf("Englisch: %q %q %q", T("Verbindung"), T("Strg+S"), Keys("Strg+Pfeil"))
	}
	if NewError("Sync-Passwort falsch").Error() != "Wrong sync password" {
		t.Fatal("Fehler nicht übersetzt")
	}
	Set("fr")
	if Lang() != DE {
		t.Fatal("unbekannte Sprache muss Deutsch sein")
	}
}

func TestLocale(t *testing.T) {
	for loc, want := range map[string]string{"de_DE.UTF-8": DE, "de-AT": DE, "en_US.UTF-8": EN, "en-GB": EN, "fr_FR": EN, "C": DE, "": DE, "C.UTF-8": DE} {
		if got := fromLocale(loc); got != want {
			t.Errorf("%q: %s, erwartet %s", loc, got, want)
		}
	}
}

func TestLoadSave(t *testing.T) {
	defer Set(DE)
	dir := t.TempDir()
	if err := Save(dir, EN); err != nil {
		t.Fatal(err)
	}
	Set(DE)
	if Load(dir) != EN || !English() {
		t.Fatal("gespeicherte Sprache nicht geladen")
	}
}
