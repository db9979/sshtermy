package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Datei ansehen (Dateien, F5)
//
// Doppelklick oder ⏎ auf eine Datei öffnet sie im eingebauten Betrachter,
// auf beiden Seiten (lokal und Server). Er zeigt Zeilennummern, färbt
// Kommentare, Zahlen und Texte in Anführungszeichen und in Logs Fehler und
// Warnungen. Das Rad blättert, Ziehen markiert Text und kopiert ihn beim
// Loslassen, ein Doppelklick auf ein Wort sucht es. / sucht, f liest mit wie
// tail -f, w bricht lange Zeilen um. Große Dateien lädt er nur am Anfang und
// am Ende. Bilder, PDFs und Office-Dateien öffnet der Doppelklick gleich mit
// dem Standardprogramm des Rechners (Server-Dateien vorher heruntergeladen).
//
// Einfachklick auf eine Textdatei zeigt ihren Anfang unter der Liste
// (Vorschau, v schaltet sie aus).

const (
	viewMax   = 4 << 20 // bis dahin ganz laden
	viewHead  = 1 << 20 // sonst: so viel vom Anfang …
	viewTail  = 1 << 20 // … und so viel vom Ende
	viewHex   = 4 << 10 // Binärdatei: so viel als Hex
	peekBytes = 16 << 10
	followMax = 1 << 20 // pro Schritt beim Mitlesen höchstens
)

// fileView: eine Datei im Betrachter.
type fileView struct {
	side   int
	fs     sshx.FS
	path   string
	name   string
	size   int64
	lines  []string
	binary bool
	note   string // „nur Anfang und Ende“ …
	log    bool   // Log-Datei: Fehler/Warnungen färben
	err    string
	busy   bool
	seq    int

	top  int // erste sichtbare Zeile
	hoff int // ohne Umbruch: Spalten nach links geschoben
	wrap bool

	follow bool
	off    int64  // so viele Bytes gelesen (für f)
	carry  string // angefangene letzte Zeile beim Mitlesen

	search *textinput.Model
	query  string
	hit    int // Zeile des aktuellen Treffers (-1 = keiner)

	sel   *viewSel
	vrows []vrow // zuletzt gezeichnete Zeilen (für die Maus)
	gut   int    // Breite der Zeilennummern samt Abstand
	bodyH int
}

type vrow struct{ line, from, to int } // Zeile, Spalten [from, to) in Runen

type viewPos struct{ line, col int }

func (p viewPos) before(q viewPos) bool {
	return p.line < q.line || (p.line == q.line && p.col < q.col)
}

type viewSel struct {
	a, b     viewPos
	dragging bool
}

func (v *viewSel) norm() (viewPos, viewPos) {
	if v.b.before(v.a) {
		return v.b, v.a
	}
	return v.a, v.b
}

// filePeek: Vorschau unter der Liste.
type filePeek struct {
	name    string // voller Pfad
	lines   []string
	binary  bool
	err     string
	loading bool
	seq     int
}

type fileViewMsg struct {
	sid, seq int
	lines    []string
	binary   bool
	note     string
	size     int64
	off      int64
	carry    string
	err      error
}

type fileFollowMsg struct {
	sid, seq int
	lines    []string // neu dazugekommen
	carry    string
	off      int64
	reset    bool // Datei kürzer geworden (rotiert): neu laden
	err      error
}

type fileFollowTick struct{ sid, seq int }

type filePeekTick struct{ sid, side, seq int }

type filePeekMsg struct {
	sid, side, seq int
	name           string
	lines          []string
	binary         bool
	err            error
}

// ---------------------------------------------------------------- Laden

// systemExt: Dateien, die der Doppelklick mit dem Standardprogramm öffnet.
var systemExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".webp": true, ".ico": true, ".tif": true, ".tiff": true, ".heic": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true, ".odt": true, ".ods": true, ".odp": true,
	".mp3": true, ".wav": true, ".flac": true, ".mp4": true, ".mkv": true, ".mov": true, ".avi": true, ".webm": true,
}

func opensWithSystem(name string) bool { return systemExt[strings.ToLower(filepath.Ext(name))] }

// isLogName: Datei, deren Zeilen nach Fehlern/Warnungen gefärbt werden.
func isLogName(p string) bool {
	l := strings.ToLower(p)
	return strings.HasSuffix(l, ".log") || strings.Contains(l, "/log/") || strings.Contains(l, `\log\`) ||
		strings.HasSuffix(l, "syslog") || strings.HasSuffix(l, "messages") || strings.Contains(l, ".log.")
}

// looksBinary: NUL-Bytes oder viele Steuerzeichen am Anfang.
func looksBinary(b []byte) bool {
	b = b[:min(len(b), 8000)]
	if bytes.IndexByte(b, 0) >= 0 {
		return true
	}
	ctl := 0
	for _, c := range b {
		if c < 0x20 && c != '\n' && c != '\r' && c != '\t' && c != '\f' && c != 0x1b {
			ctl++
		}
	}
	return len(b) > 0 && ctl*20 > len(b)
}

// decodeText: UTF-8, sonst Latin-1 (alte Windows-/Switch-Dateien).
func decodeText(b []byte) string {
	b = bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
	if utf8.Valid(b) {
		return string(b)
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// textLines: Zeilen ohne \r, Tabs als Leerzeichen, Steuerzeichen sichtbar.
func textLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return []string{""}
	}
	parts := strings.Split(s, "\n")
	for i, l := range parts {
		parts[i] = cleanLine(l)
	}
	return parts
}

func cleanLine(l string) string {
	l = strings.TrimSuffix(l, "\r")
	if !strings.ContainsAny(l, "\t\x1b\r\f\v\b\x00") && !hasCtl(l) {
		return l
	}
	var b strings.Builder
	col := 0
	for _, r := range l {
		switch {
		case r == '\t':
			n := 8 - col%8
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		case r < 0x20 || r == 0x7f:
			r = '·'
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

func hasCtl(l string) bool {
	for i := 0; i < len(l); i++ {
		if l[i] < 0x20 || l[i] == 0x7f {
			return true
		}
	}
	return false
}

// hexLines: klassischer Hex-Dump.
func hexLines(b []byte) []string {
	var out []string
	for off := 0; off < len(b); off += 16 {
		row := b[off:min(off+16, len(b))]
		var hx, as strings.Builder
		for i := 0; i < 16; i++ {
			if i < len(row) {
				fmt.Fprintf(&hx, "%02x ", row[i])
				if row[i] >= 0x20 && row[i] < 0x7f {
					as.WriteByte(row[i])
				} else {
					as.WriteByte('.')
				}
			} else {
				hx.WriteString("   ")
			}
			if i == 7 {
				hx.WriteByte(' ')
			}
		}
		out = append(out, fmt.Sprintf("%08x  %s %s", off, hx.String(), as.String()))
	}
	return out
}

// readForView liest eine Datei für den Betrachter: klein ganz, groß Anfang
// und Ende. off = bis wohin gelesen wurde (für das Mitlesen).
func readForView(fsys sshx.FS, p string) (m fileViewMsg) {
	st, err := fsys.Stat(p)
	if err != nil {
		m.err = err
		return
	}
	if st.IsDir {
		m.err = errors.New(i18n.T("Das ist ein Ordner"))
		return
	}
	m.size = st.Size
	r, err := fsys.Open(p)
	if err != nil {
		m.err = err
		return
	}
	defer r.Close()
	var data, tail []byte
	if st.Size <= viewMax {
		data, err = io.ReadAll(io.LimitReader(r, viewMax+1))
		m.off = int64(len(data))
	} else {
		data, err = io.ReadAll(io.LimitReader(r, viewHead))
		m.off = st.Size
		if sk, ok := r.(io.Seeker); ok && err == nil {
			if _, err = sk.Seek(st.Size-viewTail, io.SeekStart); err == nil {
				tail, err = io.ReadAll(io.LimitReader(r, viewTail))
			}
			m.note = fmt.Sprintf(i18n.T("groß: Anfang und Ende, %s dazwischen ausgelassen"), humanBytes(st.Size-viewHead-viewTail))
		} else {
			m.note = fmt.Sprintf(i18n.T("groß: nur die ersten %s"), humanBytes(viewHead))
			m.off = int64(len(data))
		}
	}
	if err != nil {
		m.err = err
		return
	}
	if looksBinary(data) {
		m.binary = true
		m.lines = hexLines(data[:min(len(data), viewHex)])
		return
	}
	if tail == nil {
		txt := decodeText(data)
		if !strings.HasSuffix(txt, "\n") { // letzte Zeile ohne Umbruch: beim Mitlesen fortsetzen
			if i := strings.LastIndexByte(txt, '\n'); i >= 0 {
				m.carry = txt[i+1:]
			} else {
				m.carry = txt
			}
		}
		m.lines = textLines(txt)
		return
	}
	head := textLines(decodeText(data))
	if i := bytes.IndexByte(tail, '\n'); i >= 0 { // angeschnittene erste Zeile weg
		tail = tail[i+1:]
	}
	gap := fmt.Sprintf(i18n.T("··· %s ausgelassen (f liest ab dem Ende mit) ···"), humanBytes(st.Size-viewHead-viewTail))
	m.lines = append(append(head, gap), textLines(decodeText(tail))...)
	return
}

// readMore: beim Mitlesen das Neue ab off.
func readMore(fsys sshx.FS, p string, off int64, carry string) (m fileFollowMsg) {
	st, err := fsys.Stat(p)
	if err != nil {
		m.err = err
		return
	}
	if st.Size < off {
		m.reset = true
		return
	}
	m.off, m.carry = off, carry
	if st.Size == off {
		return
	}
	r, err := fsys.Open(p)
	if err != nil {
		m.err = err
		return
	}
	defer r.Close()
	sk, ok := r.(io.Seeker)
	if !ok {
		m.err = errors.New(i18n.T("Mitlesen geht hier nicht"))
		return
	}
	if _, err := sk.Seek(off, io.SeekStart); err != nil {
		m.err = err
		return
	}
	data, err := io.ReadAll(io.LimitReader(r, followMax))
	if err != nil {
		m.err = err
		return
	}
	m.off = off + int64(len(data))
	txt := carry + decodeText(data)
	i := strings.LastIndexByte(txt, '\n')
	if i < 0 {
		m.carry = txt
		return
	}
	m.carry = txt[i+1:]
	m.lines = textLines(txt[:i+1])
	return
}

// readPeek: die ersten Zeilen für die Vorschau.
func readPeek(fsys sshx.FS, p string) (lines []string, binary bool, err error) {
	r, err := fsys.Open(p)
	if err != nil {
		return nil, false, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, peekBytes))
	if err != nil {
		return nil, false, err
	}
	if looksBinary(data) {
		return nil, true, nil
	}
	return textLines(decodeText(data)), false, nil
}

// ---------------------------------------------------------------- Öffnen

// openEntry: Doppelklick/⏎ auf Eintrag i der Seite side.
func (a *App) openEntry(s *session, side, i int) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil || i < 0 || i >= len(p.entries) {
		return nil
	}
	e := p.entries[i]
	f.side, p.sel = side, i
	if e.IsDir {
		p.cd(e.Name, f.hidden)
		return nil
	}
	if opensWithSystem(e.Name) {
		return a.openWithSystem(s, side, i)
	}
	return a.openViewer(s, side, p.fs.Join(p.path, e.Name))
}

func (a *App) openViewer(s *session, side int, full string) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	v := &fileView{side: side, fs: p.fs, path: full, name: baseName(p.fs, full), busy: true, hit: -1, log: isLogName(full)}
	if f.view != nil {
		v.seq = f.view.seq + 1
	}
	f.view = v
	f.drag = nil
	return a.loadView(s)
}

func (a *App) loadView(s *session) tea.Cmd {
	v := s.files.view
	v.busy, v.err = true, ""
	sid, seq, fsys, p := s.id, v.seq, v.fs, v.path
	return func() tea.Msg {
		m := readForView(fsys, p)
		m.sid, m.seq = sid, seq
		return m
	}
}

// openWithSystem: mit dem Standardprogramm des Rechners öffnen. Server-
// Dateien kommen vorher in einen privaten Ordner (beim Beenden gelöscht).
func (a *App) openWithSystem(s *session, side, i int) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil || i <= 0 || i >= len(p.entries) {
		return nil
	}
	full := p.fs.Join(p.path, p.entries[i].Name)
	if _, local := p.fs.(sshx.LocalFS); local {
		if err := sshx.OpenBrowser(full); err != nil {
			a.flashErr(i18n.T("Ließ sich nicht öffnen: ") + err.Error())
		} else {
			a.flash(i18n.T("Öffne mit dem Standardprogramm: ") + p.entries[i].Name)
		}
		return nil
	}
	if p.entries[i].IsDir {
		a.flash(i18n.T("Ordner vom Server lassen sich nicht direkt öffnen, erst kopieren (c)"))
		return nil
	}
	dir, err := os.MkdirTemp("", "sshtermy-open-")
	if err != nil {
		a.flashErr(err.Error())
		return nil
	}
	a.openTmp = append(a.openTmp, dir)
	local := filepath.Join(dir, p.entries[i].Name)
	cmd := a.startTransfer(s, p.fs, []string{full}, sshx.LocalFS{}, dir, false, true)
	if f.xfer != nil && cmd != nil {
		f.xfer.then = func() {
			if err := sshx.OpenBrowser(local); err != nil {
				a.flashErr(i18n.T("Ließ sich nicht öffnen: ") + err.Error())
			}
		}
	}
	return cmd
}

func (a *App) cleanupOpenTmp() {
	for _, d := range a.openTmp {
		_ = os.RemoveAll(d)
	}
	a.openTmp = nil
}

// ---------------------------------------------------------------- Nachrichten

func (a *App) fileViewMsgs(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case fileViewMsg:
		s := a.byID(m.sid)
		if s == nil || s.files.view == nil || s.files.view.seq != m.seq {
			return nil, true
		}
		v := s.files.view
		v.busy = false
		if m.err != nil {
			v.err = m.err.Error()
			return nil, true
		}
		v.lines, v.binary, v.note, v.size, v.off, v.carry = m.lines, m.binary, m.note, m.size, m.off, m.carry
		v.top = min(v.top, max(0, len(v.lines)-1))
		if v.follow {
			v.toEnd()
			return fileFollowAfter(s.id, v.seq), true
		}
		return nil, true
	case fileFollowTick:
		s := a.byID(m.sid)
		if s == nil || !s.files.open || s.files.view == nil || s.files.view.seq != m.seq || !s.files.view.follow {
			return nil, true
		}
		v := s.files.view
		sid, seq, fsys, p, off, carry := s.id, v.seq, v.fs, v.path, v.off, v.carry
		return func() tea.Msg {
			r := readMore(fsys, p, off, carry)
			r.sid, r.seq = sid, seq
			return r
		}, true
	case fileFollowMsg:
		s := a.byID(m.sid)
		if s == nil || s.files.view == nil || s.files.view.seq != m.seq || !s.files.view.follow {
			return nil, true
		}
		v := s.files.view
		switch {
		case m.err != nil:
			v.follow = false
			a.flashErr(i18n.T("Mitlesen beendet: ") + m.err.Error())
			return nil, true
		case m.reset:
			v.seq++
			return a.loadView(s), true
		}
		atEnd := v.atEnd()
		v.off, v.carry = m.off, m.carry
		if len(m.lines) > 0 {
			v.lines = append(v.lines, m.lines...)
			if atEnd {
				v.toEnd()
			}
		}
		return fileFollowAfter(s.id, v.seq), true
	case filePeekTick:
		s := a.byID(m.sid)
		if s == nil || !s.files.open || s.files.panes[m.side] == nil {
			return nil, true
		}
		p := s.files.panes[m.side]
		pk := p.peek
		if pk == nil || pk.seq != m.seq || !pk.loading {
			return nil, true
		}
		sid, side, seq, fsys, name := s.id, m.side, m.seq, p.fs, pk.name
		return func() tea.Msg {
			lines, bin, err := readPeek(fsys, name)
			return filePeekMsg{sid, side, seq, name, lines, bin, err}
		}, true
	case filePeekMsg:
		s := a.byID(m.sid)
		if s == nil || s.files.panes[m.side] == nil {
			return nil, true
		}
		pk := s.files.panes[m.side].peek
		if pk == nil || pk.seq != m.seq {
			return nil, true
		}
		pk.loading, pk.lines, pk.binary = false, m.lines, m.binary
		if m.err != nil {
			pk.err = m.err.Error()
		}
		return nil, true
	}
	return nil, false
}

func fileFollowAfter(sid, seq int) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return fileFollowTick{sid, seq} })
}

// ---------------------------------------------------------------- Vorschau

func (a *App) peekOn() bool { return !a.st.Settings().FilesNoPeek }

// filesPeek: Vorschau für die Auswahl der aktiven Seite anstoßen (kurz
// verzögert, damit Blättern mit den Pfeilen nicht jede Datei liest).
func (a *App) filesPeek(s *session) tea.Cmd {
	f := &s.files
	if !a.peekOn() || f.panes[0] == nil || f.view != nil {
		return nil
	}
	p := f.panes[f.side]
	if p.sel <= 0 || p.sel >= len(p.entries) || p.entries[p.sel].IsDir {
		return nil
	}
	e := p.entries[p.sel]
	full := p.fs.Join(p.path, e.Name)
	if p.peek != nil && p.peek.name == full {
		return nil
	}
	seq := 1
	if p.peek != nil {
		seq = p.peek.seq + 1
	}
	p.peek = &filePeek{name: full, loading: true, seq: seq}
	if opensWithSystem(e.Name) {
		p.peek.loading = false
		return nil
	}
	sid, side := s.id, f.side
	return tea.Tick(180*time.Millisecond, func(time.Time) tea.Msg { return filePeekTick{sid, side, seq} })
}

func (a *App) togglePeek(s *session) tea.Cmd {
	on := !a.peekOn()
	if err := a.st.UpdateSettings(func(st *domain.Settings) { st.FilesNoPeek = !on }); err != nil {
		a.flashErr(err.Error())
		return nil
	}
	if on {
		a.flash(i18n.T("Vorschau an: ein Klick auf eine Textdatei zeigt ihren Anfang"))
		return a.filesPeek(s)
	}
	for _, p := range s.files.panes {
		if p != nil {
			p.peek = nil
		}
	}
	a.flash(i18n.T("Vorschau aus (v schaltet sie wieder an)"))
	return nil
}

// viewPeek: Vorschau unter der Liste (h Zeilen).
func (a *App) viewPeek(s *session, p *pane, side int, w, h int) string {
	st := a.s
	pk := p.peek
	e := p.entries[p.sel]
	info := humanBytes(e.Size)
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(st.T.Border).Render(strings.Repeat("─", w)) + "\n")
	head := st.TabActive.Foreground(st.T.Accent).Render(i18n.T("Vorschau")) + " " + st.Text.Render(trunc(e.Name, max(8, w/2))) + st.Muted.Render(" · "+info)
	b.WriteString(ansiTruncate(head, w) + "\n")
	rows := h - 2
	var body []string
	switch {
	case opensWithSystem(e.Name):
		body = []string{st.Muted.Render(i18n.T(" Doppelklick öffnet mit dem Standardprogramm"))}
	case pk.loading:
		body = []string{st.Muted.Render(" …")}
	case pk.err != "":
		body = []string{st.Danger.Render(trunc(" × "+pk.err, w))}
	case pk.binary:
		body = []string{st.Muted.Render(i18n.T(" Binärdatei · Doppelklick zeigt Hex, o öffnet mit dem Standardprogramm"))}
	default:
		gw := len(fmt.Sprint(min(len(pk.lines), rows))) + 1
		for i, l := range pk.lines {
			if i >= rows {
				break
			}
			if i == rows-1 && len(pk.lines) > rows {
				body = append(body, st.Muted.Render(i18n.T(" … Doppelklick oder ⏎ öffnet ganz")))
				break
			}
			r := []rune(l)
			if len(r) > w-gw-1 {
				r = r[:max(0, w-gw-1)]
			}
			body = append(body, st.Muted.Render(fmt.Sprintf("%*d ", gw, i+1))+a.colorCode(string(r), false))
		}
	}
	for len(body) < rows {
		body = append(body, "")
	}
	b.WriteString(strings.Join(body[:rows], "\n"))
	return a.z.mark(lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(b.String()), zoneAction{block: true, click: func(tea.Mouse, bool) tea.Cmd {
		return a.openEntry(s, side, p.sel)
	}})
}

// ---------------------------------------------------------------- Betrachter: Bild

func (v *fileView) gutterW() int { return len(fmt.Sprint(max(1, len(v.lines)))) + 2 }

func (v *fileView) toEnd() {
	v.top = max(0, len(v.lines)-max(1, v.bodyH))
}

func (v *fileView) atEnd() bool { return v.top >= len(v.lines)-max(1, v.bodyH) }

func (v *fileView) scroll(d int) {
	v.top = clamp(v.top+d, 0, max(0, len(v.lines)-1))
}

// layoutRows: sichtbare Zeilen ab top für eine Textbreite tw.
func (v *fileView) layoutRows(tw, rows int) []vrow {
	var out []vrow
	for li := v.top; li < len(v.lines) && len(out) < rows; li++ {
		n := utf8.RuneCountInString(v.lines[li])
		if !v.wrap {
			out = append(out, vrow{li, v.hoff, v.hoff + tw})
			continue
		}
		for from := 0; len(out) < rows; from += tw {
			out = append(out, vrow{li, from, from + tw})
			if from+tw >= n {
				break
			}
		}
	}
	return out
}

func (a *App) viewFileView(s *session, w, h int) string {
	st := a.s
	f := &s.files
	v := f.view
	label := i18n.T("Lokal")
	if v.side == 1 && f.panes[1] != nil {
		label = f.panes[1].fs.Label()
	}
	info := label + " · " + v.path
	if !v.busy && v.err == "" {
		info += " · " + humanBytes(v.size)
		if v.binary {
			info += i18n.T(" · Binärdatei")
		} else {
			info += fmt.Sprintf(i18n.T(" · %d Zeilen"), len(v.lines))
		}
		if v.note != "" {
			info += " · " + v.note
		}
	}
	if v.follow {
		info += i18n.T(" · liest mit")
	}
	closeBtn := a.z.click(st.Danger.Render(i18n.T(" × zu ")), func(tea.Mouse, bool) tea.Cmd { f.view = nil; return nil })
	tag := st.TabActive.Foreground(st.T.Accent).Render(i18n.T("Ansehen"))
	if v.follow {
		tag = st.TabActive.Foreground(st.T.OK).Render(i18n.T("Mitlesen"))
	}
	head := tag + " " + st.Text.Bold(true).Render(v.name) + "  " + st.Muted.Render(info)
	head = ansiTruncate(head, w-lipgloss.Width(closeBtn)-1)
	head += strings.Repeat(" ", max(0, w-lipgloss.Width(head)-lipgloss.Width(closeBtn))) + closeBtn
	rule := lipgloss.NewStyle().Foreground(st.T.Border).Render(strings.Repeat("─", w))

	// unten: Suche oder Hinweis
	var status string
	switch {
	case v.search != nil:
		v.search.SetWidth(w - 12)
		status = st.Accent.Render(i18n.T(" Suchen: ")) + v.search.View()
	case v.query != "":
		status = st.Muted.Render(fmt.Sprintf(i18n.T(" Suche „%s“ · n weiter, N zurück, Esc hebt auf"), v.query))
		if v.hit < 0 {
			status = st.Warn.Render(fmt.Sprintf(i18n.T(" „%s“ nicht gefunden"), v.query))
		}
	case v.binary:
		status = st.Muted.Render(i18n.T(" Binärdatei: die ersten 4 KB als Hex · o öffnet mit dem Standardprogramm"))
	default:
		status = st.Muted.Render(i18n.T(" Rad blättert · Ziehen markiert und kopiert · Doppelklick auf ein Wort sucht es · Rechtsklick kopiert"))
	}

	bodyH := max(1, h-4)
	v.bodyH = bodyH
	var lines []string
	switch {
	case v.busy && len(v.lines) == 0:
		lines = []string{st.AI.Render(i18n.T(" ⠿ lade …"))}
		v.vrows = nil
	case v.err != "":
		lines = []string{st.Danger.Render(trunc(" × "+v.err, w))}
		v.vrows = nil
	default:
		v.gut = v.gutterW()
		tw := max(4, w-v.gut)
		v.vrows = v.layoutRows(tw, bodyH)
		var sa, sb viewPos
		hasSel := v.sel != nil && v.sel.a != v.sel.b
		if hasSel {
			sa, sb = v.sel.norm()
		}
		prev := -1
		for _, r := range v.vrows {
			num := strings.Repeat(" ", v.gut)
			if r.line != prev {
				num = fmt.Sprintf("%*d  ", v.gut-2, r.line+1)
			}
			prev = r.line
			runes := []rune(v.lines[r.line])
			from, to := min(r.from, len(runes)), min(r.to, len(runes))
			seg := string(runes[from:to])
			var body string
			switch {
			case hasSel && r.line >= sa.line && r.line <= sb.line:
				c0, c1 := from, to
				if r.line == sa.line {
					c0 = clamp(sa.col, from, to)
				}
				if r.line == sb.line {
					c1 = clamp(sb.col, from, to)
				}
				if c1 < c0 {
					c1 = c0
				}
				body = st.Text.Render(string(runes[from:c0])) + st.Selected.Render(string(runes[c0:c1])) + st.Text.Render(string(runes[c1:to]))
			case v.query != "" && strings.Contains(strings.ToLower(seg), strings.ToLower(v.query)):
				body = a.markHits(seg, v.query, r.line == v.hit)
			case v.binary:
				body = st.Text.Render(seg)
			default:
				body = a.colorLine(seg, v.lines[r.line], v.log)
			}
			numSt := st.Muted
			if r.line == v.hit {
				numSt = st.Warn
			}
			lines = append(lines, numSt.Render(num)+body)
		}
	}
	for len(lines) < bodyH {
		lines = append(lines, "")
	}
	text := lipgloss.NewStyle().Width(w).Height(bodyH).MaxHeight(bodyH).Render(strings.Join(lines[:bodyH], "\n"))
	text = a.z.mark(text, zoneAction{
		block: true,
		click: func(m tea.Mouse, double bool) tea.Cmd { return a.viewClick(s, m, double) },
		menu: func(tea.Mouse) tea.Cmd {
			if v.sel != nil && v.sel.a != v.sel.b {
				cmd := a.copyViewSel(v)
				v.sel = nil
				return cmd
			}
			return nil
		},
		wheel: func(up bool) tea.Cmd {
			if up {
				v.scroll(-3)
			} else {
				v.scroll(3)
			}
			return nil
		},
	})
	return head + "\n" + rule + "\n" + text + "\n" + rule + "\n" + ansiTruncate(status, w)
}

// markHits: Treffer der Suche hervorheben.
func (a *App) markHits(seg, q string, current bool) string {
	st := a.s
	hit := lipgloss.NewStyle().Foreground(st.T.Bg).Background(st.T.Warn)
	if current {
		hit = lipgloss.NewStyle().Foreground(st.T.Bg).Background(st.T.Danger).Bold(true)
	}
	low, lq := strings.ToLower(seg), strings.ToLower(q)
	var b strings.Builder
	for {
		i := strings.Index(low, lq)
		if i < 0 || len(low) != len(seg) { // Kleinschreibung hat Längen geändert: schlicht
			b.WriteString(st.Text.Render(seg))
			break
		}
		b.WriteString(st.Text.Render(seg[:i]) + hit.Render(seg[i:i+len(q)]))
		seg, low = seg[i+len(q):], low[i+len(q):]
		if seg == "" {
			break
		}
	}
	return b.String()
}

// colorLine: Kommentare, Zahlen, Texte; in Logs Fehler/Warnungen.
func (a *App) colorLine(seg, whole string, log bool) string {
	st := a.s
	if log {
		l := strings.ToLower(whole)
		switch {
		case containsAny(l, "error", "fehler", "fail", "crit", "fatal", "panic", "emerg", "alert", "denied", "refused"):
			return st.Danger.Render(seg)
		case containsAny(l, "warn", "timeout", "retry"):
			return st.Warn.Render(seg)
		}
		return st.Text.Render(seg)
	}
	t := strings.TrimSpace(whole)
	if strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//") || strings.HasPrefix(t, ";") || strings.HasPrefix(t, "--") || strings.HasPrefix(t, "<!--") || strings.HasPrefix(t, "REM ") || strings.HasPrefix(t, "!") {
		return st.Muted.Render(seg)
	}
	return a.colorCode(seg, seg == whole || strings.HasPrefix(whole, seg))
}

func containsAny(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// colorCode: einfache Färbung ohne Sprachkenntnis: erstes Wort (Schlüssel/
// Befehl) violett, Zahlen gelb, Texte in Anführungszeichen grün.
func (a *App) colorCode(seg string, lineStart bool) string {
	st := a.s
	t := strings.TrimSpace(seg)
	if strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//") {
		return st.Muted.Render(seg)
	}
	var b strings.Builder
	r := []rune(seg)
	first := lineStart
	for i := 0; i < len(r); {
		c := r[i]
		switch {
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(r) && r[j] != c {
				j++
			}
			j = min(j+1, len(r))
			b.WriteString(st.OK.Render(string(r[i:j])))
			i = j
			first = false
		case unicode.IsDigit(c) && (i == 0 || !isWordRune(r[i-1])):
			j := i
			for j < len(r) && (unicode.IsDigit(r[j]) || r[j] == '.' || r[j] == ':') {
				j++
			}
			if j < len(r) && isWordRune(r[j]) { // 2048M, v1: als Wort lassen
				for j < len(r) && isWordRune(r[j]) {
					j++
				}
				b.WriteString(st.Text.Render(string(r[i:j])))
			} else {
				b.WriteString(st.Warn.Render(string(r[i:j])))
			}
			i = j
			first = false
		case isWordRune(c):
			j := i
			for j < len(r) && (isWordRune(r[j]) || r[j] == '-' || r[j] == '.') {
				j++
			}
			if first {
				b.WriteString(st.AI.Render(string(r[i:j])))
			} else {
				b.WriteString(st.Text.Render(string(r[i:j])))
			}
			i = j
			first = false
		default:
			j := i
			for j < len(r) && !isWordRune(r[j]) && r[j] != '"' && r[j] != '\'' && !unicode.IsDigit(r[j]) {
				j++
			}
			b.WriteString(st.Text.Render(string(r[i:j])))
			i = j
		}
	}
	return b.String()
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// ---------------------------------------------------------------- Betrachter: Maus

// viewPosAt: Stelle im Text unter x/y (begrenzt auf die Fläche); dy sagt,
// ob oben (-1) oder unten (+1) hinausgezogen wurde.
func (a *App) viewPosAt(s *session, x, y int) (p viewPos, dy int, ok bool) {
	v := s.files.view
	if v == nil || len(v.vrows) == 0 {
		return p, 0, false
	}
	r := a.termRect(s)
	row := y - (r.y + 2)
	if row < 0 {
		dy = -1
	} else if row >= len(v.vrows) {
		dy = 1
	}
	row = clamp(row, 0, len(v.vrows)-1)
	vr := v.vrows[row]
	n := utf8.RuneCountInString(v.lines[vr.line])
	col := clamp(vr.from+x-(r.x+v.gut), vr.from, min(vr.to, n))
	return viewPos{vr.line, max(0, col)}, dy, true
}

func (a *App) viewClick(s *session, m tea.Mouse, double bool) tea.Cmd {
	v := s.files.view
	p, _, ok := a.viewPosAt(s, m.X, m.Y)
	if !ok {
		return nil
	}
	if double {
		v.sel = nil
		if w := wordAt(v.lines[p.line], p.col); w != "" {
			v.query = w
			v.findNext(p.line, 1, true)
		}
		return nil
	}
	v.sel = &viewSel{a: p, b: p, dragging: true}
	return nil
}

// fileViewDrag: Ziehen und Loslassen im Betrachter.
func (a *App) fileViewDrag(s *session, msg tea.MouseMsg) tea.Cmd {
	v := s.files.view
	m := msg.Mouse()
	switch msg.(type) {
	case tea.MouseMotionMsg:
		p, dy, ok := a.viewPosAt(s, m.X, m.Y)
		if !ok {
			return nil
		}
		if dy != 0 {
			v.scroll(dy)
		}
		v.sel.b = p
	case tea.MouseReleaseMsg:
		if p, _, ok := a.viewPosAt(s, m.X, m.Y); ok {
			v.sel.b = p
		}
		v.sel.dragging = false
		if v.sel.a == v.sel.b {
			v.sel = nil
			return nil
		}
		return a.copyViewSel(v)
	}
	return nil
}

func (v *fileView) selText() string {
	if v.sel == nil {
		return ""
	}
	sa, sb := v.sel.norm()
	var out []string
	for li := sa.line; li <= sb.line && li < len(v.lines); li++ {
		r := []rune(v.lines[li])
		c0, c1 := 0, len(r)
		if li == sa.line {
			c0 = min(sa.col, len(r))
		}
		if li == sb.line {
			c1 = min(sb.col, len(r))
		}
		if c1 < c0 {
			c1 = c0
		}
		out = append(out, string(r[c0:c1]))
	}
	return strings.Join(out, "\n")
}

func (a *App) copyViewSel(v *fileView) tea.Cmd {
	t := v.selText()
	if t == "" {
		return nil
	}
	go func() { _ = clipboard.WriteAll(t) }()
	n := strings.Count(t, "\n") + 1
	if n > 1 {
		a.flash(fmt.Sprintf(i18n.T("%d Zeilen kopiert"), n))
	} else {
		a.flash(i18n.T("Kopiert: ") + trunc(t, 50))
	}
	return tea.SetClipboard(t)
}

func wordAt(line string, col int) string {
	r := []rune(line)
	if col < 0 || col >= len(r) || !isWordRune(r[col]) {
		return ""
	}
	i, j := col, col
	for i > 0 && (isWordRune(r[i-1]) || r[i-1] == '-' || r[i-1] == '.') {
		i--
	}
	for j < len(r) && (isWordRune(r[j]) || r[j] == '-' || r[j] == '.') {
		j++
	}
	return strings.Trim(string(r[i:j]), ".-")
}

// findNext: nächster Treffer ab Zeile from in Richtung dir (inklusive from).
func (v *fileView) findNext(from, dir int, incl bool) {
	q := strings.ToLower(v.query)
	if q == "" || len(v.lines) == 0 {
		v.hit = -1
		return
	}
	n := len(v.lines)
	start := from
	if !incl {
		start = from + dir
	}
	for k := 0; k < n; k++ {
		i := ((start+k*dir)%n + n) % n
		if strings.Contains(strings.ToLower(v.lines[i]), q) {
			v.hit = i
			if i < v.top || i >= v.top+v.bodyH {
				v.top = max(0, i-v.bodyH/3)
			}
			return
		}
	}
	v.hit = -1
}

// ---------------------------------------------------------------- Betrachter: Tasten

func (a *App) fileViewKey(s *session, k tea.KeyPressMsg) tea.Cmd {
	f := &s.files
	v := f.view
	key := k.String()
	if v.search != nil {
		switch key {
		case "esc":
			v.search = nil
		case "enter":
			v.query = strings.TrimSpace(v.search.Value())
			v.search = nil
			v.findNext(v.top, 1, true)
		default:
			var cmd tea.Cmd
			*v.search, cmd = v.search.Update(k)
			return cmd
		}
		return nil
	}
	if v.sel != nil && !v.sel.dragging {
		v.sel = nil
	}
	page := max(1, v.bodyH-1)
	switch key {
	case "esc":
		if v.query != "" {
			v.query, v.hit = "", -1
			return nil
		}
		f.view = nil
	case "q", "backspace":
		f.view = nil
	case "up", "k":
		v.scroll(-1)
	case "down", "j":
		v.scroll(1)
	case "pgup":
		v.scroll(-page)
	case "pgdown", "space":
		v.scroll(page)
	case "home", "g":
		v.top = 0
	case "end", "G":
		v.toEnd()
	case "left", "h":
		v.hoff = max(0, v.hoff-8)
	case "right", "l":
		if !v.wrap {
			v.hoff += 8
		}
	case "w":
		v.wrap = !v.wrap
		v.hoff = 0
	case "/", "ctrl+f":
		in := newInput(v.query, false)
		in.Placeholder = i18n.T("Text im Betrachter suchen")
		v.search = &in
		return v.search.Focus()
	case "n":
		v.findNext(max(v.hit, v.top), 1, false)
	case "N":
		v.findNext(max(v.hit, v.top), -1, false)
	case "f":
		if v.binary {
			return nil
		}
		v.follow = !v.follow
		if v.follow {
			v.toEnd()
			if v.note != "" { // groß: nur Ende im Speicher halten reicht
				v.seq++
				return a.loadView(s)
			}
			return fileFollowAfter(s.id, v.seq)
		}
	case "r":
		v.seq++
		return a.loadView(s)
	case "e":
		return a.editPath(s, v.side, v.path)
	case "o":
		return a.openPathWithSystem(s, v.side, v.path)
	case "c", "f6":
		p, dst := f.panes[v.side], f.panes[1-v.side]
		return a.startTransfer(s, p.fs, []string{v.path}, dst.fs, dst.path, v.side == 0, false)
	case "y":
		return a.copyPath(v.path)
	}
	return nil
}

func (a *App) fileViewKeys(v *fileView) [][2]string {
	if v.search != nil {
		return [][2]string{{"⏎", i18n.T("suchen")}, {"Esc", i18n.T("abbrechen")}}
	}
	return [][2]string{{"Esc", i18n.T("zurück")}, {"/", i18n.T("suchen")}, {"n", i18n.T("weiter")}, {"f", i18n.T("mitlesen")}, {"w", i18n.T("umbrechen")},
		{"e", i18n.T("bearbeiten")}, {"o", i18n.T("Programm")}, {"c", i18n.T("kopieren ↔")}, {"y", i18n.T("Pfad")}, {"g/G", i18n.T("Anfang/Ende")}}
}

// editPath: lokale Datei im eigenen Editor, Server-Datei wie e im Dateibrowser.
func (a *App) editPath(s *session, side int, full string) tea.Cmd {
	if side == 1 {
		return a.editRemote(s, full)
	}
	return a.launchEditor(full)
}

// openPathWithSystem: wie openWithSystem, aber für einen Pfad (Betrachter).
func (a *App) openPathWithSystem(s *session, side int, full string) tea.Cmd {
	p := s.files.panes[side]
	if p == nil {
		return nil
	}
	name := baseName(p.fs, full)
	for i, e := range p.entries {
		if e.Name == name && p.fs.Join(p.path, e.Name) == full {
			return a.openWithSystem(s, side, i)
		}
	}
	if _, local := p.fs.(sshx.LocalFS); local {
		if err := sshx.OpenBrowser(full); err != nil {
			a.flashErr(i18n.T("Ließ sich nicht öffnen: ") + err.Error())
		}
	}
	return nil
}

func (a *App) copyPath(p string) tea.Cmd {
	go func() { _ = clipboard.WriteAll(p) }()
	a.flash(i18n.T("Pfad kopiert: ") + trunc(p, 60))
	return tea.SetClipboard(p)
}
