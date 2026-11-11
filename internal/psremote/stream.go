package psremote

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// backend: wo PowerShell läuft (WinRM, in Tests ein lokaler Prozess).
type backend interface {
	// start startet PowerShell im Ordner dir. Ausgabe kommt über h.
	start(dir string, h handlers) (proc, error)
	close()
}

// proc: ein laufendes PowerShell.
type proc interface {
	send(b []byte) error
	stop()
}

type handlers struct {
	out func([]byte)
	err func([]byte)
	end func(error)
}

// Info: was drüben läuft (aus der ersten Marke).
type Info struct {
	OS, PSVersion, Computer string
}

// Stream: eine interaktive PowerShell-Sitzung mit eigener Eingabezeile.
// Tasten kommen über Write (wie bei einer SSH-Shell), die Zeile wird hier
// bearbeitet und erst mit Enter verschickt.
type Stream struct {
	Done chan error // nil = exit bzw. von uns geschlossen

	be  backend
	out io.Writer

	mu       sync.Mutex
	p        proc
	gen      int
	cols     int
	sentW    int
	info     Info
	cwd      string
	prompt   string // fertiger Prompt samt [Rechner]:
	ready    bool   // Prompt steht, Eingabe gehört der Zeile
	started  bool   // erster Prompt war da
	firstErr []byte // stderr vor dem ersten Prompt (für die Fehlermeldung)
	more     []string
	buf      []rune
	cur      int
	drawn    int  // Cursor-Spalte (Prompt + Zeile) seit dem letzten Zeichnen
	shown    bool // Prompt + Zeile stehen auf dem Schirm
	atBOL    bool // letzte Ausgabe endete mit Zeilenumbruch
	hist     []string
	hidx     int
	hsave    []rune
	busyBuf  []rune // Eingabe für Read-Host & Co., solange ein Befehl läuft
	ver      int    // Zähler für Änderungen an buf (Tab-Antwort noch gültig?)
	comp     *compState
	compWait int // ver der offenen Tab-Anfrage, -1 = keine
	pend     []byte
	marker   []byte
	inMarker bool
	esc      []byte
	lastCR   bool // Ausgabe
	keyCR    bool // Eingabe: letzte Taste war \r
	closed   bool
	ready1   chan struct{}
	sendq    chan sendItem
}

type compState struct {
	start, n int // ersetzter Bereich in buf (Runen)
	matches  []string
	i        int
	ver      int
}

type sendItem struct {
	gen int
	b   []byte
}

// ErrStartup: PowerShell kam nicht bis zum ersten Prompt.
var ErrStartup = errors.New("PowerShell startet nicht")

func newStream(be backend, cols int, out io.Writer, timeout time.Duration) (*Stream, error) {
	if cols < 20 {
		cols = 80
	}
	s := &Stream{Done: make(chan error, 1), be: be, out: out, cols: cols, atBOL: true, compWait: -1,
		ready1: make(chan struct{}), sendq: make(chan sendItem, 256)}
	go s.sender()
	if err := s.startProc(""); err != nil {
		close(s.sendq)
		be.close()
		return nil, err
	}
	select {
	case <-s.ready1:
		return s, nil
	case err := <-s.Done:
		s.Close()
		if err == nil {
			err = ErrStartup
		}
		return nil, err
	case <-time.After(timeout):
		s.Close()
		return nil, fmt.Errorf("%w: keine Antwort nach %s", ErrStartup, timeout)
	}
}

// startProc startet PowerShell (neu) und schickt die Schleife hinterher.
func (s *Stream) startProc(dir string) error {
	s.mu.Lock()
	s.gen++
	gen := s.gen
	s.mu.Unlock()
	p, err := s.be.start(dir, handlers{
		out: func(b []byte) { s.remoteOut(gen, b) },
		err: func(b []byte) { s.remoteErr(gen, b) },
		end: func(err error) { s.remoteEnd(gen, err) },
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed || gen != s.gen {
		s.mu.Unlock()
		p.stop()
		return nil
	}
	s.p = p
	s.sentW = 0
	s.queue(bootLine(dir))
	s.syncWidth()
	s.mu.Unlock()
	return nil
}

// sender schickt Eingaben der Reihe nach (WinRM-Anfragen blockieren kurz,
// die Oberfläche soll das nicht merken).
func (s *Stream) sender() {
	for it := range s.sendq {
		s.mu.Lock()
		p, ok := s.p, it.gen == s.gen && !s.closed
		s.mu.Unlock()
		if ok && p != nil {
			_ = p.send(it.b)
		}
	}
}

// queue: unter s.mu aufrufen.
func (s *Stream) queue(b []byte) {
	if s.closed {
		return
	}
	select {
	case s.sendq <- sendItem{s.gen, b}:
	default: // Rückstau: lieber verwerfen als die Oberfläche anhalten
	}
}

func (s *Stream) syncWidth() {
	if s.cols != s.sentW {
		s.sentW = s.cols
		s.queue(cmdLine('W', strconv.Itoa(s.cols)))
	}
}

// Info: Systemangaben der Gegenstelle.
func (s *Stream) Info() Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info
}

// Dir: aktueller Ordner drüben.
func (s *Stream) Dir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cwd
}

// Resize: neue Breite (Ausgabe von Tabellen richtet sich danach).
func (s *Stream) Resize(cols, rows int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cols >= 20 {
		s.cols = cols
	}
	if s.ready {
		s.syncWidth()
	}
	return nil
}

// Close beendet PowerShell und gibt die Sitzung drüben frei.
func (s *Stream) Close() error {
	s.finish(nil)
	return nil
}

func (s *Stream) finish(err error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	p := s.p
	s.p = nil
	close(s.sendq)
	s.mu.Unlock()
	go func() {
		if p != nil {
			p.stop()
		}
		s.be.close()
	}()
	s.Done <- err
}

// ---------------------------------------------------------------- Ausgabe

func (s *Stream) write(b []byte) {
	if len(b) == 0 {
		return
	}
	_, _ = s.out.Write(b)
}

func (s *Stream) remoteOut(gen int, b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if gen != s.gen || s.closed {
		return
	}
	var plain []byte
	flush := func() {
		if len(plain) > 0 {
			s.emit(plain)
			plain = plain[:0]
		}
	}
	b = append(s.pend, b...)
	s.pend = nil
	for len(b) > 0 {
		c := b[0]
		if s.inMarker {
			if c == 2 {
				s.inMarker = false
				m := s.marker
				s.marker = nil
				flush()
				s.handleMarker(m)
			} else {
				s.marker = append(s.marker, c)
			}
			b = b[1:]
			continue
		}
		if c == 1 {
			s.inMarker = true
			b = b[1:]
			continue
		}
		plain = append(plain, c)
		b = b[1:]
	}
	flush()
}

// emit: normale Ausgabe, \n → \r\n (WinRS liefert meist \r\n, nicht immer).
func (s *Stream) emit(b []byte) {
	out := make([]byte, 0, len(b)+8)
	for _, c := range b {
		if c == '\n' && !s.lastCR {
			out = append(out, '\r')
		}
		s.lastCR = c == '\r'
		out = append(out, c)
	}
	s.write(out)
	if n := len(b); n > 0 {
		s.atBOL = b[n-1] == '\n'
	}
}

func (s *Stream) remoteErr(gen int, b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if gen != s.gen || s.closed {
		return
	}
	t := string(b)
	if strings.Contains(t, "#< CLIXML") || strings.HasPrefix(strings.TrimSpace(t), "<Objs ") {
		return // Fortschrittsbalken als XML: nichts für Menschen
	}
	if !s.started {
		s.firstErr = append(s.firstErr, b...)
	}
	s.write([]byte("\x1b[91m"))
	s.emit(b)
	s.write([]byte("\x1b[0m"))
}

func (s *Stream) remoteEnd(gen int, err error) {
	s.mu.Lock()
	if gen != s.gen || s.closed {
		s.mu.Unlock()
		return
	}
	var ee *ExitError
	if s.started && errors.As(err, &ee) {
		err = nil // „exit 1“ drüben: gewolltes Ende, kein Leitungsabriss
	}
	if !s.started {
		if msg := strings.TrimSpace(string(s.firstErr)); msg != "" {
			err = fmt.Errorf("%w: %s", ErrStartup, msg)
		} else if err == nil {
			err = ErrStartup
		}
	}
	s.mu.Unlock()
	s.finish(err)
}

func (s *Stream) handleMarker(m []byte) {
	if len(m) == 0 {
		return
	}
	v := string(m[1:])
	switch m[0] {
	case 'I':
		f := strings.SplitN(v, "|", 3)
		for len(f) < 3 {
			f = append(f, "")
		}
		s.info = Info{OS: strings.TrimSpace(f[0]), PSVersion: f[1], Computer: f[2]}
	case 'D':
		s.cwd = v
	case 'P':
		p := v
		if s.info.Computer != "" {
			p = "[" + s.info.Computer + "]: " + p
		}
		s.prompt = p
		s.more = nil
		s.showPrompt()
		if !s.started {
			s.started = true
			close(s.ready1)
		}
	case 'M':
		s.prompt = ">> "
		s.showPrompt()
	case 'R':
		// vorweg getippte Zeile läuft jetzt: hinter dem Prompt zeigen
		if s.ready {
			typed := s.buf
			s.buf, s.cur = []rune(v), len([]rune(v))
			s.redraw()
			s.write([]byte("\r\n"))
			s.buf, s.cur, s.busyBuf = nil, 0, typed
			s.ready, s.shown, s.atBOL = false, false, true
		}
		// Vorweg getippte Zeile wie eine normale prüfen: ist sie unvollständig
		// (function x {), meldet das Skript 'M' und die nächsten Zeilen hängen
		// sich an, statt dass sie einzeln mit Parserfehler ausgeführt wird.
		s.more = []string{v}
		if t := strings.TrimSpace(v); t != "" && (len(s.hist) == 0 || s.hist[len(s.hist)-1] != v) {
			s.hist = append(s.hist, v)
		}
	case 'T':
		s.completion(v)
	}
}

// showPrompt: Befehl fertig, Prompt (und schon Getipptes) zeichnen.
func (s *Stream) showPrompt() {
	if !s.atBOL {
		s.write([]byte("\r\n"))
	}
	if len(s.busyBuf) > 0 && len(s.buf) == 0 {
		s.buf, s.cur = s.busyBuf, len(s.busyBuf) // angefangene Zeile übernehmen
	}
	s.ready, s.shown, s.drawn, s.busyBuf = true, false, 0, nil
	s.syncWidth()
	s.redraw()
}

// ---------------------------------------------------------------- Eingabezeile

func width(r []rune) int { return ansi.StringWidth(string(r)) }

// redraw zeichnet Prompt und Zeile neu, auch über mehrere Bildschirmzeilen.
func (s *Stream) redraw() {
	cols := s.cols
	var b strings.Builder
	if s.shown {
		b.WriteString("\r")
		if up := s.drawn / cols; up > 0 {
			fmt.Fprintf(&b, "\x1b[%dA", up)
		}
	}
	b.WriteString(s.prompt)
	b.WriteString(string(s.buf))
	b.WriteString("\x1b[J")
	pw := ansi.StringWidth(s.prompt)
	end := pw + width(s.buf)
	cur := pw + width(s.buf[:s.cur])
	if end > 0 && end%cols == 0 {
		b.WriteString("\r\n") // Terminal wartet am Rand mit dem Umbruch
	}
	if up := end/cols - cur/cols; up > 0 {
		fmt.Fprintf(&b, "\x1b[%dA", up)
	}
	b.WriteString("\r")
	if c := cur % cols; c > 0 {
		fmt.Fprintf(&b, "\x1b[%dC", c)
	}
	s.write([]byte(b.String()))
	s.drawn, s.shown, s.atBOL = cur, true, false
}

// Write: Tasten aus dem Terminal.
func (s *Stream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, io.ErrClosedPipe
	}
	n := len(p)
	if len(s.esc) > 0 {
		p = append(s.esc, p...)
		s.esc = nil
	}
	for len(p) > 0 {
		if s.closed {
			break
		}
		c := p[0]
		if c == 0x1b {
			k, used, complete := parseEsc(p)
			if !complete {
				if len(p) == 1 {
					s.key(keyEsc, 0)
					p = nil
					break
				}
				s.esc = append([]byte(nil), p...)
				break
			}
			s.key(k, 0)
			p = p[used:]
			continue
		}
		if c < 0x20 || c == 0x7f {
			if c == '\n' && s.keyCR {
				s.keyCR = false
				p = p[1:]
				continue // \r\n aus Einfügen: nur einmal Enter
			}
			s.key(ctrlKey(c), 0)
			s.keyCR = c == '\r'
			p = p[1:]
			continue
		}
		s.keyCR = false
		r, size := utf8.DecodeRune(p)
		if r == utf8.RuneError && size <= 1 && !utf8.FullRune(p) {
			s.esc = append([]byte(nil), p...) // halbes Zeichen: auf den Rest warten
			break
		}
		s.key(keyRune, r)
		p = p[size:]
	}
	return n, nil
}

type keyKind int

const (
	keyNone keyKind = iota
	keyRune
	keyEnter
	keyBack
	keyDel
	keyLeft
	keyRight
	keyWordLeft
	keyWordRight
	keyHome
	keyEnd
	keyUp
	keyDown
	keyTab
	keyBackTab
	keyCtrlC
	keyCtrlD
	keyKillStart
	keyKillEnd
	keyKillWord
	keyClear
	keyEsc
)

func ctrlKey(c byte) keyKind {
	switch c {
	case '\r', '\n':
		return keyEnter
	case 0x7f, 0x08:
		return keyBack
	case 0x03:
		return keyCtrlC
	case 0x04:
		return keyCtrlD
	case 0x01:
		return keyHome
	case 0x05:
		return keyEnd
	case 0x02:
		return keyLeft
	case 0x06:
		return keyRight
	case 0x10:
		return keyUp
	case 0x0e:
		return keyDown
	case 0x15:
		return keyKillStart
	case 0x0b:
		return keyKillEnd
	case 0x17:
		return keyKillWord
	case 0x0c:
		return keyClear
	case 0x09:
		return keyTab
	}
	return keyNone
}

// parseEsc liest eine Escape-Folge (CSI/SS3) am Anfang von p.
func parseEsc(p []byte) (k keyKind, used int, complete bool) {
	if len(p) < 2 {
		return keyNone, 0, false
	}
	switch p[1] {
	case '[':
		i := 2
		for i < len(p) && (p[i] < 0x40 || p[i] > 0x7e) {
			i++
		}
		if i >= len(p) {
			return keyNone, 0, false
		}
		params, final := string(p[2:i]), p[i]
		used = i + 1
		mod := ""
		if j := strings.IndexByte(params, ';'); j >= 0 {
			mod = params[j+1:]
		}
		ctrl := mod == "5" || mod == "3" // Strg bzw. Alt
		switch final {
		case 'A':
			return keyUp, used, true
		case 'B':
			return keyDown, used, true
		case 'C':
			if ctrl {
				return keyWordRight, used, true
			}
			return keyRight, used, true
		case 'D':
			if ctrl {
				return keyWordLeft, used, true
			}
			return keyLeft, used, true
		case 'H':
			return keyHome, used, true
		case 'F':
			return keyEnd, used, true
		case 'Z':
			return keyBackTab, used, true
		case '~':
			switch strings.SplitN(params, ";", 2)[0] {
			case "1", "7":
				return keyHome, used, true
			case "4", "8":
				return keyEnd, used, true
			case "3":
				return keyDel, used, true
			}
		}
		return keyNone, used, true // Maus, Fokus, Einfügen-Klammern …
	case 'O':
		if len(p) < 3 {
			return keyNone, 0, false
		}
		switch p[2] {
		case 'A':
			return keyUp, 3, true
		case 'B':
			return keyDown, 3, true
		case 'C':
			return keyRight, 3, true
		case 'D':
			return keyLeft, 3, true
		case 'H':
			return keyHome, 3, true
		case 'F':
			return keyEnd, 3, true
		}
		return keyNone, 3, true
	case 'b':
		return keyWordLeft, 2, true
	case 'f':
		return keyWordRight, 2, true
	case 0x7f:
		return keyKillWord, 2, true
	}
	return keyEsc, 1, true
}

func (s *Stream) key(k keyKind, r rune) {
	if k == keyNone {
		return
	}
	if !s.ready {
		s.busyKey(k, r)
		return
	}
	if k != keyTab && k != keyBackTab {
		s.comp = nil
	}
	switch k {
	case keyRune:
		s.buf = append(s.buf[:s.cur], append([]rune{r}, s.buf[s.cur:]...)...)
		s.cur++
	case keyEnter:
		s.enter()
		return
	case keyBack:
		if s.cur == 0 {
			return
		}
		s.buf = append(s.buf[:s.cur-1], s.buf[s.cur:]...)
		s.cur--
	case keyDel:
		if s.cur >= len(s.buf) {
			return
		}
		s.buf = append(s.buf[:s.cur], s.buf[s.cur+1:]...)
	case keyCtrlD:
		if len(s.buf) == 0 && len(s.more) == 0 {
			s.write([]byte("\r\n"))
			s.finishLater()
			return
		}
		if s.cur < len(s.buf) {
			s.buf = append(s.buf[:s.cur], s.buf[s.cur+1:]...)
		}
	case keyLeft:
		if s.cur > 0 {
			s.cur--
		}
	case keyRight:
		if s.cur < len(s.buf) {
			s.cur++
		}
	case keyWordLeft:
		s.cur = wordLeft(s.buf, s.cur)
	case keyWordRight:
		s.cur = wordRight(s.buf, s.cur)
	case keyHome:
		s.cur = 0
	case keyEnd:
		s.cur = len(s.buf)
	case keyKillStart:
		s.buf = append([]rune(nil), s.buf[s.cur:]...)
		s.cur = 0
	case keyKillEnd:
		s.buf = s.buf[:s.cur]
	case keyKillWord:
		w := wordLeft(s.buf, s.cur)
		s.buf = append(s.buf[:w], s.buf[s.cur:]...)
		s.cur = w
	case keyEsc:
		s.buf, s.cur = nil, 0
	case keyUp:
		s.history(-1)
	case keyDown:
		s.history(1)
	case keyClear:
		s.write([]byte("\x1b[H\x1b[2J"))
		s.shown = false
	case keyCtrlC:
		s.cur = len(s.buf)
		s.redraw()
		s.write([]byte("^C\r\n"))
		s.buf, s.cur, s.more, s.shown, s.atBOL = nil, 0, nil, false, true
		s.prompt = s.basePrompt()
	case keyTab, keyBackTab:
		s.tab(k == keyBackTab)
		return
	}
	s.ver++
	s.redraw()
}

// basePrompt: Prompt ohne „>>“ (nach Abbruch einer mehrzeiligen Eingabe).
func (s *Stream) basePrompt() string {
	p := "PS " + s.cwd + "> "
	if s.info.Computer != "" {
		p = "[" + s.info.Computer + "]: " + p
	}
	return p
}

// finishLater: exit/Strg+D; finish nimmt s.mu selbst.
func (s *Stream) finishLater() {
	s.ready = false
	go s.finish(nil)
}

func (s *Stream) enter() {
	s.cur = len(s.buf)
	s.redraw()
	s.write([]byte("\r\n"))
	line := string(s.buf)
	s.buf, s.cur, s.shown, s.atBOL, s.hidx, s.hsave = nil, 0, false, true, 0, nil
	s.ver++
	if t := strings.TrimSpace(line); t != "" && (len(s.hist) == 0 || s.hist[len(s.hist)-1] != line) {
		s.hist = append(s.hist, line)
	}
	if len(s.more) > 0 {
		all := append(s.more, line)
		if strings.TrimSpace(line) == "" {
			s.ready = false
			s.queue(cmdLine('X', b64(strings.Join(all, "\n"))))
			return
		}
		s.more = all
		s.ready = false
		s.queue(cmdLine('C', b64(strings.Join(all, "\n"))))
		return
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "":
		s.redraw()
		return
	case "exit", "exit-pssession", "logout":
		s.finishLater()
		return
	case "cls", "clear", "clear-host":
		s.write([]byte("\x1b[H\x1b[2J"))
		s.redraw()
		return
	}
	s.more = []string{line}
	s.ready = false
	s.queue(cmdLine('C', b64(line)))
}

// busyKey: ein Befehl läuft. Strg+C bricht ab, Enter gibt die Zeile an
// das laufende Programm (Read-Host, Bestätigungen).
func (s *Stream) busyKey(k keyKind, r rune) {
	switch k {
	case keyCtrlC:
		s.interrupt()
	case keyRune:
		s.busyBuf = append(s.busyBuf, r)
		s.write([]byte(string(r)))
	case keyBack:
		if n := len(s.busyBuf); n > 0 {
			w := width(s.busyBuf[n-1:])
			s.busyBuf = s.busyBuf[:n-1]
			s.write([]byte(strings.Repeat("\b \b", max(1, w))))
		}
	case keyEnter:
		line := string(s.busyBuf)
		s.busyBuf = nil
		s.write([]byte("\r\n"))
		s.atBOL = true
		s.queue([]byte(line + "\r\n"))
	}
}

// interrupt: WinRS kennt kein Strg+C für ein Programm ohne Konsole. Also
// PowerShell beenden und im selben Ordner neu starten.
func (s *Stream) interrupt() {
	s.write([]byte("^C\r\n\x1b[2m" + msgInterrupted + "\x1b[0m\r\n"))
	s.atBOL, s.busyBuf, s.more = true, nil, nil
	old, dir := s.p, s.cwd
	s.p = nil
	s.gen++ // Ausgaben des alten Prozesses verwerfen
	go func() {
		if old != nil {
			old.stop()
		}
		if err := s.startProc(dir); err != nil {
			s.finish(err)
		}
	}()
}

// msgInterrupted: übersetzt die Oberfläche (i18n liegt in internal/i18n,
// das Paket hier bleibt davon frei).
var msgInterrupted = "Befehl abgebrochen · PowerShell neu gestartet (Variablen sind weg, Ordner bleibt)"

// SetInterruptText: Text nach Strg+C (Übersetzung).
func SetInterruptText(t string) { msgInterrupted = t }

func (s *Stream) history(d int) {
	if len(s.hist) == 0 {
		return
	}
	if s.hidx == 0 {
		if d > 0 {
			return
		}
		s.hsave = append([]rune(nil), s.buf...)
	}
	n := s.hidx - d // hidx zählt rückwärts ab dem Ende
	if n < 0 {
		n = 0
	}
	if n > len(s.hist) {
		n = len(s.hist)
	}
	s.hidx = n
	if n == 0 {
		s.buf = s.hsave
	} else {
		s.buf = []rune(s.hist[len(s.hist)-n])
	}
	s.cur = len(s.buf)
}

func isWord(r rune) bool { return r != ' ' && r != '\t' }

func wordLeft(b []rune, i int) int {
	for i > 0 && !isWord(b[i-1]) {
		i--
	}
	for i > 0 && isWord(b[i-1]) {
		i--
	}
	return i
}

func wordRight(b []rune, i int) int {
	for i < len(b) && !isWord(b[i]) {
		i++
	}
	for i < len(b) && isWord(b[i]) {
		i++
	}
	return i
}

// ---------------------------------------------------------------- Tab

func (s *Stream) tab(back bool) {
	if c := s.comp; c != nil && c.ver == s.ver && len(c.matches) > 1 {
		if back {
			c.i = (c.i - 1 + len(c.matches)) % len(c.matches)
		} else {
			c.i = (c.i + 1) % len(c.matches)
		}
		s.applyComp()
		return
	}
	if s.compWait >= 0 {
		return
	}
	s.compWait = s.ver
	pos := len(utf16.Encode(s.buf[:s.cur]))
	s.queue(cmdLine('T', strconv.Itoa(pos)+":"+b64(string(s.buf))))
}

// completion: Antwort auf eine Tab-Anfrage („start,länge␟treffer…“ in UTF-16).
func (s *Stream) completion(v string) {
	want := s.compWait
	s.compWait = -1
	if want != s.ver || !s.ready {
		return // inzwischen weitergetippt
	}
	parts := strings.Split(v, "\x1f")
	idx, n, ok := strings.Cut(parts[0], ",")
	if !ok || len(parts) < 2 {
		s.write([]byte("\a"))
		return
	}
	start16, e1 := strconv.Atoi(idx)
	len16, e2 := strconv.Atoi(n)
	if e1 != nil || e2 != nil {
		return
	}
	start, end := runeIndex(s.buf, start16), runeIndex(s.buf, start16+len16)
	s.comp = &compState{start: start, n: end - start, matches: parts[1:]}
	s.applyComp()
}

func (s *Stream) applyComp() {
	c := s.comp
	m := []rune(c.matches[c.i])
	if c.start > len(s.buf) {
		c.start = len(s.buf)
	}
	end := min(c.start+c.n, len(s.buf))
	nb := append([]rune(nil), s.buf[:c.start]...)
	nb = append(nb, m...)
	nb = append(nb, s.buf[end:]...)
	s.buf, s.cur, c.n = nb, c.start+len(m), len(m)
	s.ver++
	c.ver = s.ver
	s.redraw()
}

// runeIndex: UTF-16-Position → Index in b.
func runeIndex(b []rune, u16 int) int {
	n := 0
	for i, r := range b {
		if n >= u16 {
			return i
		}
		n += utf16.RuneLen(r)
	}
	return len(b)
}
