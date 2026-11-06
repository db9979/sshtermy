// Package termemu ist die Terminal-Emulation: der rohe PTY-Strom vom Server
// (ANSI/VT100/xterm-Sequenzen) wird von charmbracelet/x/vt in ein Zellraster
// mit Farben, Alt-Screen (vim, htop, tmux), Scrollback und Cursor übersetzt.
// Tastendrücke kodiert der Emulator selbst passend zum Modus (z. B.
// Application-Cursor-Keys), Antworten auf Terminal-Anfragen (DA, CPR) gehen
// automatisch zurück an den Server.
package termemu

import (
	"io"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

// Term ist ein emuliertes Terminal für genau eine SSH-Shell.
type Term struct {
	emu   *vt.SafeEmulator
	mu    sync.Mutex
	title string
	bell  bool
	mouse map[ansi.DECMode]bool // vom Programm angeforderte Maus-Modi

	host  io.Writer
	muted atomic.Bool // Antworten auf Abfragen verwerfen (MuteReplies)

	// Shell-Integration (OSC 133 / OSC 633): Exit-Code des letzten Befehls
	exitSeq  int
	exitCode int
}

// New erzeugt das Terminal. toHost bekommt alle Bytes, die zum Server sollen
// (kodierte Tasten, Paste, Antworten auf Abfragen).
func New(cols, rows int, toHost io.Writer) *Term {
	t := &Term{emu: vt.NewSafeEmulator(cols, rows), host: toHost}
	t.emu.SetScrollbackSize(5000)
	t.emu.SetCallbacks(vt.Callbacks{
		Title: func(s string) { t.mu.Lock(); t.title = s; t.mu.Unlock() },
		Bell:  func() { t.mu.Lock(); t.bell = true; t.mu.Unlock() },
		// läuft innerhalb von Write (Emulator gesperrt): nur eigene Felder anfassen
		EnableMode:  func(m ansi.Mode) { t.onMode(m, true) },
		DisableMode: func(m ansi.Mode) { t.onMode(m, false) },
	})
	// FinalTerm/VS-Code-Shell-Integration: "133;D;<exit>" bzw. "633;D;<exit>"
	// nach jedem Befehl. Liefert die Shell das, kennt sshTermy den echten
	// Exit-Code statt ihn aus der Ausgabe zu schätzen.
	for _, cmd := range []int{133, 633} {
		t.emu.RegisterOscHandler(cmd, func(data []byte) bool {
			if code, ok := parseCommandDone(string(data)); ok {
				t.mu.Lock()
				t.exitSeq++
				t.exitCode = code
				t.mu.Unlock()
			}
			return true // A/B/C (Prompt-Marker) still schlucken
		})
	}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := t.emu.Read(buf)
			if n > 0 && !t.muted.Load() {
				if _, werr := toHost.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return t
}

// MuteReplies: Ein anderes, echtes Terminal hängt am selben Strom (Tab in
// eigenem Fenster) und beantwortet Abfragen (DA, CPR …) selbst. Dann darf
// der Emulator nicht auch antworten. SendText und Paste gehen in der Zeit
// direkt an den Server.
func (t *Term) MuteReplies(on bool) { t.muted.Store(on) }

// parseCommandDone liest "133;D;1" (auch ohne Präfix oder ohne Code = 0).
func parseCommandDone(s string) (int, bool) {
	parts := strings.Split(s, ";")
	if len(parts) > 0 && (parts[0] == "133" || parts[0] == "633") {
		parts = parts[1:]
	}
	if len(parts) == 0 || parts[0] != "D" {
		return 0, false
	}
	if len(parts) < 2 || parts[1] == "" {
		return 0, true
	}
	n := 0
	for _, c := range parts[1] {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
		if n > 1<<16 {
			return 0, false
		}
	}
	return n, true
}

// LastExit: Zähler und Exit-Code der letzten OSC-133-D-Meldung. Ändert sich
// der Zähler zwischen Enter und Prompt, hat die Shell den Code gemeldet.
func (t *Term) LastExit() (seq, code int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.exitSeq, t.exitCode
}

// Write füttert Ausgabe des Servers in den Emulator.
func (t *Term) Write(p []byte) (int, error) { return t.emu.Write(p) }

func (t *Term) Resize(cols, rows int) {
	if cols < 10 || rows < 3 {
		return
	}
	t.emu.Resize(cols, rows)
}

func (t *Term) Size() (int, int) { return t.emu.Width(), t.emu.Height() }

// SendKey kodiert einen Tastendruck (bubbletea v2 und vt teilen den Key-Typ
// aus ultraviolet, daher ist die Umwandlung verlustfrei).
func (t *Term) SendKey(k uv.KeyPressEvent) { t.emu.SendKey(k) }

// SendText schickt Text unverändert (z. B. einen bestätigten Befehl).
func (t *Term) SendText(s string) {
	if t.muted.Load() {
		_, _ = io.WriteString(t.host, s)
		return
	}
	t.emu.SendText(s)
}

// Paste nutzt Bracketed Paste, wenn die Shell es eingeschaltet hat.
func (t *Term) Paste(s string) {
	if t.muted.Load() {
		_, _ = io.WriteString(t.host, s)
		return
	}
	t.emu.Paste(s)
}

func (t *Term) Title() string { t.mu.Lock(); defer t.mu.Unlock(); return t.title }

// TakeBell: gab es seit dem letzten Aufruf ein BEL?
func (t *Term) TakeBell() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	b := t.bell
	t.bell = false
	return b
}

func (t *Term) AltScreen() bool { return t.emu.IsAltScreen() }

// Cursor: Position im Raster (0-basiert) und ob sichtbar.
func (t *Term) Cursor() (x, y int, visible bool) {
	p := t.emu.CursorPosition()
	return p.X, p.Y, !t.emu.CursorHidden()
}

// Close beendet den Ausgabe-Kopierer.
func (t *Term) Close() { _ = t.emu.Close() }

// View rendert das Raster als ANSI-String mit genau Höhe Zeilen. ghost wird
// (gedimmt) hinter dem Cursor eingeblendet, wenn dort nichts steht.
func (t *Term) View(ghost string, ghostStyle func(string) string) string {
	w, h := t.Size()
	lines := strings.Split(strings.ReplaceAll(t.emu.Render(), "\r\n", "\n"), "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	lines = lines[:h]
	cx, cy, _ := t.Cursor()
	if ghost != "" && cy >= 0 && cy < h && !t.AltScreen() {
		row := lines[cy]
		plain := ansi.Strip(row)
		rest := ""
		if cx < ansi.StringWidth(plain) {
			rest = ansi.Cut(plain, cx, w)
		}
		if strings.TrimSpace(rest) == "" {
			room := w - cx
			g := ansi.Truncate(ghost, room, "…")
			lines[cy] = ansi.Truncate(row, cx, "") + strings.Repeat(" ", max(0, cx-ansi.StringWidth(ansi.Truncate(row, cx, "")))) + ghostStyle(g)
		}
	}
	for i, l := range lines {
		if lw := ansi.StringWidth(l); lw < w {
			lines[i] = l + strings.Repeat(" ", w-lw)
		} else if lw > w {
			lines[i] = ansi.Truncate(l, w, "")
		}
	}
	return strings.Join(lines, "\n")
}

// PlainLines: die letzten n Zeilen (Scrollback + Bildschirm) als Klartext,
// ohne nachlaufende Leerzeilen. Das ist der Kontext fürs Sprachmodell.
func (t *Term) PlainLines(n int) string {
	var all []string
	if !t.AltScreen() {
		if sb := t.emu.Scrollback(); sb != nil {
			ls := sb.Lines()
			if len(ls) > n {
				ls = ls[len(ls)-n:]
			}
			for _, l := range ls {
				all = append(all, strings.TrimRight(l.String(), " "))
			}
		}
	}
	for _, l := range strings.Split(t.emu.String(), "\n") {
		all = append(all, strings.TrimRight(l, " \r"))
	}
	for len(all) > 0 && all[len(all)-1] == "" {
		all = all[:len(all)-1]
	}
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return strings.Join(all, "\n")
}

// CursorLine: Klartext der Cursorzeile bis zum Cursor.
func (t *Term) CursorLine() string {
	_, cy, _ := t.Cursor()
	w, _ := t.Size()
	var b strings.Builder
	cx, _, _ := t.Cursor()
	for x := 0; x < w && x < cx; x++ {
		c := t.emu.CellAt(x, cy)
		if c == nil || c.Content == "" {
			b.WriteByte(' ')
			continue
		}
		b.WriteString(c.Content)
		if c.Width > 1 {
			x += c.Width - 1
		}
	}
	return b.String()
}

// Prompt-Erkennung: typische Enden von bash/zsh/fish/PowerShell-Prompts.
var promptEnd = regexp.MustCompile(`(?:[$#%>❯»]|PS [^>]*>)\s`)

// TypedInput schätzt, was der Nutzer hinter dem Prompt getippt hat. Das ist
// robuster als Tastendrücke mitzuzählen, weil Tab-Vervollständigung, Verlauf
// (↑) und Strg+R die Zeile auf dem Server verändern.
func (t *Term) TypedInput() (typed string, atPrompt bool) {
	if t.AltScreen() {
		return "", false
	}
	line := t.CursorLine()
	locs := promptEnd.FindAllStringIndex(line, -1)
	if len(locs) == 0 {
		return "", false
	}
	return line[locs[len(locs)-1][1]:], true
}

// AtIdlePrompt: steht ein leerer Prompt am Cursor (Befehl fertig)?
func (t *Term) AtIdlePrompt() bool {
	typed, ok := t.TypedInput()
	return ok && strings.TrimSpace(typed) == ""
}

// CommandOutput liefert die Ausgabe des zuletzt ausgeführten Befehls cmd:
// alles zwischen der Prompt-Zeile mit cmd und dem aktuellen Prompt (letzte
// Zeile). Gesucht wird in den letzten n Zeilen; ein umgebrochener langer Befehl
// darf über bis zu drei Zeilen gehen. line ist der Befehl, wie er am Prompt
// steht: beim Enter war das Echo der letzten Zeichen evtl. noch unterwegs
// (schnelles Tippen, Einfügen), dann ist cmd nur der Anfang davon. found =
// false, wenn die Befehlszeile nicht mehr zu sehen ist (z. B. nach clear oder
// sehr langer Ausgabe); dann kommen höchstens die letzten 60 Zeilen.
func (t *Term) CommandOutput(cmd string, n int) (out, line string, found bool) {
	cmd = strings.TrimSpace(cmd)
	lines := strings.Split(t.PlainLines(n), "\n")
	if len(lines) == 0 {
		return "", cmd, false
	}
	end := len(lines) - 1 // aktuelle Prompt-Zeile
	for i := end - 1; i >= 0 && cmd != ""; i-- {
		locs := promptEnd.FindAllStringIndex(lines[i], -1)
		if len(locs) == 0 {
			continue
		}
		joined := ""
		for k := 0; k < 3 && i+k < end; k++ {
			joined += lines[i+k]
			// nach irgendeinem Prompt-Ende (der Befehl selbst kann "$ " enthalten)
			for _, loc := range locs {
				if rest := strings.TrimSpace(joined[loc[1]:]); strings.HasPrefix(rest, cmd) {
					return strings.Join(lines[i+k+1:end], "\n"), rest, true
				}
			}
		}
	}
	return strings.Join(lines[max(0, end-60):end], "\n"), cmd, false
}
