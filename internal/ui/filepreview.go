package ui

import (
	"encoding/csv"
	"fmt"
	"image"
	_ "image/gif" // Bildgröße in der Vorschau
	_ "image/jpeg"
	_ "image/png"
	"io"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/packx"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Schönere Vorschau (F5): die Vorschau unter der Liste und der Betrachter
// kennen jetzt mehr als Text.
//
//   - Ordner: ihr Inhalt.
//   - Archive (.zip, .tar.gz …): die Dateien darin, ⏎ zeigt die ganze Liste,
//     u entpackt.
//   - Bilder (PNG, JPEG, GIF): Format und Größe in Pixeln.
//   - Markdown: Überschriften, Listen, Code und Zitate farbig.
//   - CSV/TSV: als Tabelle mit ausgerichteten Spalten (t schaltet im
//     Betrachter zwischen Tabelle und Rohtext).
//   - Programmcode: Schlüsselwörter je Sprache (Go, Python, JavaScript/
//     TypeScript, Shell, SQL, C/C++/Java/C#, Rust, PHP, YAML).

const (
	kindText    = ""
	kindDir     = "dir"
	kindArchive = "archive"
	kindImage   = "image"
	kindMD      = "md"
	kindCSV     = "csv"
)

const (
	archivePeekMax = 2000      // so viele Einträge listet der Betrachter
	archiveReadMax = 256 << 20 // so viel eines tar-Archivs liest er höchstens
	imagePeekMax   = 4 << 20   // Bildkopf
	csvColMax      = 32        // breiteste Spalte (Zeichen)
	csvRowsMax     = 5000      // so viele Zeilen werden zur Tabelle
	csvSep         = " │ "
)

// previewKind: Art der Datei aus dem Namen.
func previewKind(name string) string {
	l := strings.ToLower(name)
	switch {
	case packx.IsArchive(l):
		return kindArchive
	case strings.HasSuffix(l, ".png"), strings.HasSuffix(l, ".jpg"), strings.HasSuffix(l, ".jpeg"), strings.HasSuffix(l, ".gif"):
		return kindImage
	case strings.HasSuffix(l, ".md"), strings.HasSuffix(l, ".markdown"):
		return kindMD
	case strings.HasSuffix(l, ".csv"), strings.HasSuffix(l, ".tsv"):
		return kindCSV
	}
	return kindText
}

// ---------------------------------------------------------------- Ordner, Archiv, Bild

func dirLines(fsys sshx.FS, p string, max int) ([]string, error) {
	es, err := fsys.ReadDir(p)
	if err != nil {
		return nil, err
	}
	if len(es) == 0 {
		return []string{i18n.T("(leer)")}, nil
	}
	var dirs, files int
	var size int64
	var out []string
	for _, e := range es {
		if e.IsDir {
			dirs++
		} else {
			files++
			size += e.Size
		}
		if len(out) < max {
			if e.IsDir {
				out = append(out, "▸ "+e.Name+"/")
			} else {
				out = append(out, fmt.Sprintf("  %-*s %9s", 28, trunc(e.Name, 28), humanBytes(e.Size)))
			}
		}
	}
	head := fmt.Sprintf(i18n.T("%d Ordner, %d Dateien (%s)"), dirs, files, humanBytes(size))
	return append([]string{head}, out...), nil
}

func archiveLines(fsys sshx.FS, p string, max int) ([]string, error) {
	r, err := fsys.Open(p)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var size int64
	if st, err := fsys.Stat(p); err == nil {
		size = st.Size
	}
	ra, _ := r.(io.ReaderAt)
	es, more, err := packx.List(p, io.LimitReader(r, archiveReadMax), ra, size, max)
	if err != nil && len(es) == 0 {
		return nil, err
	}
	var total int64
	out := []string{""}
	for _, e := range es {
		if e.IsDir {
			out = append(out, "▸ "+e.Name)
			continue
		}
		total += e.Size
		out = append(out, fmt.Sprintf("  %s  %s", e.Name, humanBytes(e.Size)))
	}
	n := fmt.Sprint(len(es))
	if more {
		n = fmt.Sprintf(i18n.T("mehr als %d"), len(es))
	}
	out[0] = fmt.Sprintf(i18n.T("Archiv: %s Einträge, %s entpackt · u entpackt"), n, humanBytes(total))
	return out, nil
}

func imageLines(fsys sshx.FS, p string) ([]string, error) {
	r, err := fsys.Open(p)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	cfg, format, err := image.DecodeConfig(io.LimitReader(r, imagePeekMax))
	if err != nil {
		return []string{i18n.T("Bild (Größe unbekannt) · o öffnet mit dem Standardprogramm")}, nil
	}
	return []string{
		fmt.Sprintf(i18n.T("Bild: %s, %d × %d Pixel"), strings.ToUpper(format), cfg.Width, cfg.Height),
		i18n.T("Doppelklick oder o öffnet es mit dem Standardprogramm"),
	}, nil
}

// ---------------------------------------------------------------- CSV

// csvTable: Zeilen einer CSV-/TSV-Datei als Tabelle mit ausgerichteten
// Spalten. ok = false, wenn es keine sinnvolle Tabelle ist.
func csvTable(lines []string, name string) ([]string, bool) {
	if len(lines) == 0 {
		return nil, false
	}
	sep := ','
	if strings.HasSuffix(strings.ToLower(name), ".tsv") {
		sep = '\t'
	} else {
		head := lines[0]
		best := strings.Count(head, ",")
		for _, c := range []rune{';', '\t', '|'} {
			if n := strings.Count(head, string(c)); n > best {
				sep, best = c, n
			}
		}
	}
	rd := csv.NewReader(strings.NewReader(strings.Join(lines[:min(len(lines), csvRowsMax)], "\n")))
	rd.Comma = sep
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	var rows [][]string
	for {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false
		}
		rows = append(rows, rec)
	}
	if len(rows) == 0 || len(rows[0]) < 2 {
		return nil, false
	}
	var width []int
	for _, r := range rows {
		for i, c := range r {
			for len(width) <= i {
				width = append(width, 0)
			}
			width[i] = max(width[i], min(csvColMax, utf8.RuneCountInString(c)))
		}
	}
	out := make([]string, 0, len(rows)+1)
	for ri, r := range rows {
		var b strings.Builder
		for i := range width {
			c := ""
			if i < len(r) {
				c = strings.ReplaceAll(r[i], "\n", " ")
			}
			if i > 0 {
				b.WriteString(csvSep)
			}
			c = trunc(c, width[i])
			if i < len(width)-1 {
				c += strings.Repeat(" ", width[i]-utf8.RuneCountInString(c))
			}
			b.WriteString(c)
		}
		out = append(out, strings.TrimRight(b.String(), " "))
		if ri == 0 {
			var rule strings.Builder
			for i, w := range width {
				if i > 0 {
					rule.WriteString("─┼─")
				}
				rule.WriteString(strings.Repeat("─", w))
			}
			out = append(out, rule.String())
		}
	}
	if len(lines) > csvRowsMax {
		out = append(out, fmt.Sprintf(i18n.T("… nur die ersten %d Zeilen als Tabelle (t zeigt alles als Text)"), csvRowsMax))
	}
	return out, true
}

// colorCSV: Kopfzeile hervorheben, Trennstriche gedämpft.
func (a *App) colorCSV(seg string, line int) string {
	st := a.s
	if line == 1 && strings.Trim(seg, "─┼") == "" {
		return st.Muted.Render(seg)
	}
	base := st.Text
	if line == 0 {
		base = st.Accent.Bold(true)
	}
	parts := strings.Split(seg, "│")
	for i, p := range parts {
		parts[i] = base.Render(p)
	}
	return strings.Join(parts, st.Muted.Render("│"))
}

// ---------------------------------------------------------------- Markdown

// mdCodeLines: welche Zeilen in einem ``` Codeblock stehen.
func mdCodeLines(lines []string) []bool {
	in := make([]bool, len(lines))
	code := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			in[i] = true
			code = !code
			continue
		}
		in[i] = code
	}
	return in
}

// colorMD färbt eine Markdown-Zeile, ohne den Text zu ändern (Markieren
// und Kopieren bleiben spaltengenau).
func (a *App) colorMD(seg, whole string, code bool) string {
	st := a.s
	if code {
		return st.OK.Render(seg)
	}
	t := strings.TrimLeft(whole, " ")
	switch {
	case strings.HasPrefix(t, "#"):
		return st.Accent.Bold(true).Render(seg)
	case strings.HasPrefix(t, ">"):
		return st.Muted.Italic(true).Render(seg)
	case t == "---" || t == "***" || t == "___":
		return st.Muted.Render(seg)
	}
	// Listenzeichen, `Code`, **fett**
	var b strings.Builder
	r := []rune(seg)
	lead := len(whole) - len(t)
	if strings.HasPrefix(whole, seg) && (strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "+ ") || isNumbered(t)) {
		end := lead + strings.IndexByte(t, ' ') + 1
		end = min(end, len(r))
		b.WriteString(st.AI.Render(string(r[:end])))
		r = r[end:]
	}
	for i := 0; i < len(r); {
		switch {
		case r[i] == '`':
			j := i + 1
			for j < len(r) && r[j] != '`' {
				j++
			}
			j = min(j+1, len(r))
			b.WriteString(st.OK.Render(string(r[i:j])))
			i = j
		case r[i] == '*' && i+1 < len(r) && r[i+1] == '*':
			j := i + 2
			for j+1 < len(r) && !(r[j] == '*' && r[j+1] == '*') {
				j++
			}
			j = min(j+2, len(r))
			b.WriteString(st.Text.Bold(true).Render(string(r[i:j])))
			i = j
		case r[i] == '[':
			j := i + 1
			rest := string(r[i:])
			if mid := strings.Index(rest, "]("); mid > 0 {
				if end := strings.IndexByte(rest[mid:], ')'); end > 0 {
					j = i + utf8.RuneCountInString(rest[:mid+end+1])
					b.WriteString(st.Accent.Underline(true).Render(string(r[i:j])))
					i = j
					continue
				}
			}
			b.WriteString(st.Text.Render(string(r[i:j])))
			i = j
		default:
			j := i
			for j < len(r) && r[j] != '`' && r[j] != '[' && !(r[j] == '*' && j+1 < len(r) && r[j+1] == '*') {
				j++
			}
			b.WriteString(st.Text.Render(string(r[i:j])))
			i = j
		}
	}
	return b.String()
}

func isNumbered(t string) bool {
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(t) && (t[i] == '.' || t[i] == ')') && t[i+1] == ' '
}

// ---------------------------------------------------------------- Programmcode

type langDef struct {
	kw       map[string]bool
	comments []string // Zeilenkommentare
}

func words(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

var (
	langGo    = langDef{words("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var nil true false iota"), []string{"//"}}
	langPy    = langDef{words("and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield None True False self"), []string{"#"}}
	langJS    = langDef{words("async await break case catch class const continue debugger default delete do else export extends finally for function if import in instanceof let new of return super switch this throw try typeof var void while with yield null undefined true false interface type enum implements"), []string{"//"}}
	langSh    = langDef{words("if then else elif fi for while until do done case esac in function return local export readonly set unset shift exit echo source declare"), []string{"#"}}
	langSQL   = langDef{words("select from where and or not insert into values update set delete create table index view drop alter join left right inner outer on group by order having limit as distinct union null is in like between primary key foreign references SELECT FROM WHERE AND OR NOT INSERT INTO VALUES UPDATE SET DELETE CREATE TABLE INDEX VIEW DROP ALTER JOIN LEFT RIGHT INNER OUTER ON GROUP BY ORDER HAVING LIMIT AS DISTINCT UNION NULL IS IN LIKE BETWEEN PRIMARY KEY FOREIGN REFERENCES"), []string{"--"}}
	langC     = langDef{words("auto break case char const continue default do double else enum extern float for goto if int long register return short signed sizeof static struct switch typedef union unsigned void volatile while class public private protected virtual template typename namespace using new delete this throw try catch bool true false null nullptr final override abstract interface extends implements package import var string"), []string{"//"}}
	langRust  = langDef{words("as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait true type unsafe use where while"), []string{"//"}}
	langPHP   = langDef{words("abstract and array as break case catch class clone const continue declare default do echo else elseif empty endif endforeach endwhile extends final finally fn for foreach function global if implements include interface isset list namespace new or print private protected public require return static switch throw trait try unset use var while yield null true false"), []string{"//", "#"}}
	langYAML  = langDef{words("true false null yes no on off"), []string{"#"}}
	langByExt = map[string]*langDef{".go": &langGo, ".py": &langPy, ".js": &langJS, ".mjs": &langJS, ".cjs": &langJS, ".ts": &langJS, ".tsx": &langJS, ".jsx": &langJS, ".sh": &langSh, ".bash": &langSh, ".zsh": &langSh, ".sql": &langSQL, ".c": &langC, ".h": &langC, ".cpp": &langC, ".cc": &langC, ".hpp": &langC, ".java": &langC, ".cs": &langC, ".kt": &langC, ".rs": &langRust, ".php": &langPHP, ".yml": &langYAML, ".yaml": &langYAML}
	shebangSh = []string{"#!/bin/sh", "#!/bin/bash", "#!/usr/bin/env bash", "#!/usr/bin/env sh"}
	shebangPy = []string{"#!/usr/bin/env python", "#!/usr/bin/python"}
)

// langFor: Sprache aus Endung oder #!-Zeile (nil = allgemeine Färbung).
func langFor(name string, first string) *langDef {
	if l, ok := langByExt[strings.ToLower(filepath.Ext(name))]; ok {
		return l
	}
	for _, s := range shebangSh {
		if strings.HasPrefix(first, s) {
			return &langSh
		}
	}
	for _, s := range shebangPy {
		if strings.HasPrefix(first, s) {
			return &langPy
		}
	}
	return nil
}

// colorLang: Schlüsselwörter violett, Texte grün, Zahlen gelb, Kommentare grau.
func (a *App) colorLang(seg, whole string, l *langDef) string {
	st := a.s
	t := strings.TrimSpace(whole)
	for _, c := range l.comments {
		if strings.HasPrefix(t, c) {
			return st.Muted.Render(seg)
		}
	}
	if l == &langYAML { // Schlüssel vor dem Doppelpunkt
		if k, rest, ok := strings.Cut(seg, ":"); ok && strings.HasPrefix(whole, seg) && !strings.ContainsAny(strings.TrimSpace(k), " \"'") {
			return st.AI.Render(k) + st.Muted.Render(":") + a.colorLang(rest, rest, &langDef{kw: l.kw})
		}
	}
	var b strings.Builder
	r := []rune(seg)
	for i := 0; i < len(r); {
		c := r[i]
		switch {
		case c == '"' || c == '\'' || c == '`':
			j := i + 1
			for j < len(r) && r[j] != c {
				if r[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(r))
			b.WriteString(st.OK.Render(string(r[i:j])))
			i = j
		case c == '/' && i+1 < len(r) && r[i+1] == '/' && hasComment(l, "//"),
			c == '#' && hasComment(l, "#") && (i == 0 || r[i-1] == ' '),
			c == '-' && i+1 < len(r) && r[i+1] == '-' && hasComment(l, "--"):
			b.WriteString(st.Muted.Render(string(r[i:])))
			i = len(r)
		case unicode.IsDigit(c) && (i == 0 || !isWordRune(r[i-1])):
			j := i
			for j < len(r) && (isWordRune(r[j]) || r[j] == '.') {
				j++
			}
			b.WriteString(st.Warn.Render(string(r[i:j])))
			i = j
		case isWordRune(c):
			j := i
			for j < len(r) && isWordRune(r[j]) {
				j++
			}
			w := string(r[i:j])
			switch {
			case l.kw[w]:
				b.WriteString(st.AI.Bold(true).Render(w))
			case j < len(r) && r[j] == '(':
				b.WriteString(st.Accent.Render(w)) // Aufruf
			default:
				b.WriteString(st.Text.Render(w))
			}
			i = j
		default:
			j := i + 1
			for j < len(r) && !isWordRune(r[j]) && !strings.ContainsRune("\"'`/#-", r[j]) && !unicode.IsDigit(r[j]) {
				j++
			}
			b.WriteString(st.Text.Render(string(r[i:j])))
			i = j
		}
	}
	return b.String()
}

func hasComment(l *langDef, c string) bool {
	for _, x := range l.comments {
		if x == c {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- Betrachter

// setKind: nach dem Laden Art, Tabelle, Codeblöcke und Sprache festlegen.
func (v *fileView) setKind(kind string) {
	v.kind, v.table, v.raw, v.code, v.lang = kind, false, nil, nil, nil
	if v.binary {
		return
	}
	switch kind {
	case kindCSV:
		v.raw = v.lines
		if t, ok := csvTable(v.lines, v.name); ok {
			v.lines, v.table = t, true
		}
	case kindMD:
		v.code = mdCodeLines(v.lines)
	case kindText:
		if len(v.lines) > 0 && !v.log {
			v.lang = langFor(v.name, v.lines[0])
		}
	}
}

// toggleTable: t im Betrachter (CSV als Tabelle oder Text).
func (v *fileView) toggleTable() {
	if v.raw == nil {
		return
	}
	if v.table {
		v.lines, v.table = v.raw, false
	} else if t, ok := csvTable(v.raw, v.name); ok {
		v.lines, v.table = t, true
	}
	v.top, v.hit, v.sel = 0, -1, nil
}

// colorView: Färbung einer Zeile im Betrachter je nach Art der Datei.
func (a *App) colorView(v *fileView, seg string, line int) string {
	whole := v.lines[line]
	switch {
	case v.log || v.kind == kindArchive:
		return a.colorLine(seg, whole, v.log)
	case v.kind == kindMD && line < len(v.code):
		return a.colorMD(seg, whole, v.code[line])
	case v.kind == kindCSV && v.table:
		return a.colorCSV(seg, line)
	case v.lang != nil:
		return a.colorLang(seg, whole, v.lang)
	}
	return a.colorLine(seg, whole, false)
}
