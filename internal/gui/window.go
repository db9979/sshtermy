//go:build windows || darwin || linux

package gui

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/db9979/sshtermy/internal/termemu"
	"github.com/db9979/sshtermy/internal/winstate"
)

// Cascadia Mono mit Nerd-Font-Symbolen (SIL OFL 1.1, fonts/LICENSE-CascadiaCode.txt):
// Symbole und Rahmen sehen überall gleich aus, ohne dass jemand eine Schrift
// installieren muss.
//
//go:embed fonts/CascadiaMonoNF.ttf
var fontData []byte

// DejaVu Sans Mono (freie Lizenz, fonts/LICENSE-DejaVu.txt) als Ersatz für
// Zeichen, die Cascadia nicht hat (⇧ ⌘ ⚙ ⇅ …).
//
//go:embed fonts/DejaVuSansMono.ttf
var fallbackData []byte

// Noto Emoji (einfarbig, SIL OFL 1.1, fonts/LICENSE-NotoEmoji.txt): Emoji in
// Ausgaben, Commit-Nachrichten, Prompts.
//
//go:embed fonts/NotoEmoji.ttf
var emojiData []byte

// Available: In diesem Build gibt es das Fenster.
func Available() bool { return true }

const (
	defaultFontSize = 14.0
	padding         = 4.0 // Rand in logischen Pixeln
)

// Run zeigt model in einem eigenen Fenster statt im Terminal. Läuft auf der
// Haupt-Goroutine (macOS verlangt das) und kehrt zurück, wenn das Programm
// endet. Ergebnis wie bei tea.Program.Run.
func Run(model tea.Model, opts Options) (tea.Model, error) {
	// Programme in Tabs (lokale Shell) sollen ein vollwertiges Terminal sehen.
	for k, v := range map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"} {
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
	inR, inW := io.Pipe()
	w, err := newWindow(opts, inW)
	if err != nil {
		return nil, err
	}
	// LF wie CR+LF nur dort, wo bubbletea es annimmt: Ohne TTY-Eingabe setzt
	// es mapNl = GOOS != "windows". Unter Windows schickt es CR selbst; mit
	// LNM sprängen Zeilen dort an Spalte 0 und der Aufbau zerfiele.
	if runtime.GOOS != "windows" {
		_, _ = w.term.Write([]byte("\x1b[20h"))
	}
	prog := tea.NewProgram(model,
		tea.WithInput(inR),
		tea.WithOutput(w.output()),
		tea.WithWindowSize(startCols, startRows),
		tea.WithColorProfile(colorprofile.TrueColor),
		tea.WithEnvironment(os.Environ()),
	)
	w.onResize = func(c, r int) { prog.Send(tea.WindowSizeMsg{Width: c, Height: r}) }
	w.onClose = func() {
		if opts.CloseMsg != nil {
			go prog.Send(opts.CloseMsg)
		} else {
			prog.Quit()
		}
	}
	w.onQuit = prog.Quit

	var final tea.Model
	var runErr error
	go func() {
		final, runErr = prog.Run()
		close(w.done)
	}()
	if err := w.run(); err != nil {
		prog.Kill()
		<-w.done
		return final, err
	}
	<-w.done
	return final, runErr
}

const startCols, startRows = 120, 34

// newWindow: Schrift laden, Emulator anlegen. in bekommt Tasten und
// Antworten des Emulators.
func newWindow(opts Options, in io.Writer) (*window, error) {
	in = newQueueWriter(in)
	src, err := text.NewGoTextFaceSource(bytes.NewReader(fontData))
	if err != nil {
		return nil, err
	}
	var fallbacks []*text.GoTextFaceSource
	for _, data := range [][]byte{fallbackData, emojiData} {
		fb, err := text.NewGoTextFaceSource(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		fallbacks = append(fallbacks, fb)
	}
	saved := winstate.Load(opts.StateDir)
	if opts.FontSize <= 0 && saved.Font >= 8 && saved.Font <= 40 {
		opts.FontSize = saved.Font
	}
	if opts.FontSize <= 0 {
		opts.FontSize = defaultFontSize
	}
	if opts.Title == "" {
		opts.Title = "sshTermy"
	}
	w := &window{src: src, fallbacks: fallbacks, fontSize: opts.FontSize, opts: opts, in: in, done: make(chan struct{}), saved: saved}
	w.term = termemu.New(startCols, startRows, in)
	w.images = newImageLayer()
	w.script = newScript(os.Getenv(opts.scriptEnv()))
	w.resize = make(chan [2]int, 1)
	return w, nil
}

// output: Ausgabe des Programms in den Emulator; Bilder (kitty) zeichnet das
// Fenster selbst, pixelgenau.
func (w *window) output() io.Writer {
	return writerFunc(func(p []byte) (int, error) {
		rest := w.images.filter(p, w.term)
		if len(rest) > 0 {
			if _, err := w.term.Write(rest); err != nil {
				return 0, err
			}
		}
		w.dirty.Add(1)
		return len(p), nil
	})
}

// run öffnet das Fenster und kehrt zurück, wenn done zu ist.
func (w *window) run() error {
	go w.resizer()
	w.setFaces(1)
	ebiten.SetWindowTitle(w.opts.Title)
	ebiten.SetWindowSize(int(float64(startCols)*w.cellW+2*padding), int(float64(startRows)*w.cellH+2*padding))
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	w.applySaved() // Vollbild, maximiert oder wie zuletzt (geometry.go)
	ebiten.SetWindowClosingHandled(true)
	// Bild bleibt stehen; Draw zeichnet nur neu, wenn sich etwas geändert hat
	// (Akku, GPU).
	ebiten.SetScreenClearedEveryFrame(false)
	if len(w.opts.Icon) > 0 {
		ebiten.SetWindowIcon(w.opts.Icon)
	}
	return ebiten.RunGameWithOptions(w, &ebiten.RunGameOptions{InitUnfocused: false})
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

type window struct {
	opts Options
	term *termemu.Term
	in   io.Writer
	done chan struct{}

	// Anbindung: bubbletea (Run) oder abgetrennter Tab (RunAttach)
	onResize func(cols, rows int)
	onClose  func() // Fenster wird geschlossen
	onQuit   func() // Skript: beenden

	images   *imageLayer
	imgCache map[*kittyImage]*ebiten.Image

	src       *text.GoTextFaceSource
	fallbacks []*text.GoTextFaceSource // DejaVu, Noto Emoji
	fontSize  float64                  // logische Pixel
	scale     float64                  // Gerätepixel je logischem Pixel
	regular   text.Face                // Cascadia, sonst DejaVu
	bold      text.Face
	ascentF   float64 // Oberlänge laut Schrift
	cellW     float64
	cellH     float64
	ascent    float64
	glyphs    map[glyphKey]*ebiten.Image
	white     *ebiten.Image

	cols, rows int
	resize     chan [2]int
	dirty      atomic.Uint64
	drawn      uint64
	forceDraw  bool
	cells      []uv.Cell

	closeAt time.Time // letzter Schließversuch

	saved     winstate.State // Fensterzustand (geometry.go)
	savedAt   time.Time
	mouseX    int
	mouseY    int
	mouseBtn  uv.MouseButton
	wheelAcc  float64
	lastTitle string

	shotMu sync.Mutex
	script *script
}

type glyphKey struct {
	s            string
	bold, italic bool
	wide         bool
}

// setFaces baut Schrift und Zellmaße für den Skalierungsfaktor neu auf.
func (w *window) setFaces(scale float64) {
	w.scale = scale
	size := w.fontSize * scale
	regular := &text.GoTextFace{Source: w.src, Size: size}
	regular.SetVariation(text.MustParseTag("wght"), 400)
	bold := &text.GoTextFace{Source: w.src, Size: size}
	bold.SetVariation(text.MustParseTag("wght"), 700)
	w.regular, w.bold = regular, bold
	reg, bol := []text.Face{regular}, []text.Face{bold}
	for _, src := range w.fallbacks {
		fb := &text.GoTextFace{Source: src, Size: size}
		reg, bol = append(reg, fb), append(bol, fb)
	}
	if mf, err := text.NewMultiFace(reg...); err == nil {
		w.regular = mf
	}
	if mf, err := text.NewMultiFace(bol...); err == nil {
		w.bold = mf
	}
	m := regular.Metrics()
	w.ascentF = m.HAscent
	w.cellW = math.Round(text.Advance("M", regular))
	w.cellH = math.Ceil(m.HAscent + m.HDescent + m.HLineGap*0.5)
	w.ascent = math.Round(m.HAscent + (w.cellH-m.HAscent-m.HDescent)/2)
	w.glyphs = map[glyphKey]*ebiten.Image{}
	w.forceDraw = true
}

// resizer reicht neue Größen in Reihenfolge an das Programm weiter, ohne den
// Fenster-Thread zu blockieren; Zwischengrößen beim Ziehen fallen weg.
func (w *window) resizer() {
	for {
		select {
		case s := <-w.resize:
			if w.onResize != nil {
				w.onResize(s[0], s[1])
			}
		case <-w.done:
			return
		}
	}
}

// Layout: Bildschirm in Gerätepixeln (scharf auf HiDPI/Retina).
func (w *window) Layout(outsideW, outsideH int) (int, int) {
	panic("LayoutF wird benutzt")
}

func (w *window) LayoutF(outsideW, outsideH float64) (float64, float64) {
	s := ebiten.Monitor().DeviceScaleFactor()
	if s <= 0 {
		s = 1
	}
	if s != w.scale {
		w.setFaces(s)
	}
	pw, ph := math.Ceil(outsideW*s), math.Ceil(outsideH*s)
	pad := padding * s
	cols := max(20, int((pw-2*pad)/w.cellW))
	rows := max(5, int((ph-2*pad)/w.cellH))
	if cols != w.cols || rows != w.rows {
		w.cols, w.rows = cols, rows
		w.term.Resize(cols, rows)
		select {
		case <-w.resize:
		default:
		}
		w.resize <- [2]int{cols, rows}
		w.forceDraw = true
	}
	return pw, ph
}

func (w *window) Update() error {
	select {
	case <-w.done:
		return ebiten.Termination
	default:
	}
	// Das Programm darf das Schließen ablehnen (Rückfrage bei offenen
	// Verbindungen), deshalb jeden Versuch melden, nur nicht doppelt.
	if ebiten.IsWindowBeingClosed() && time.Since(w.closeAt) > 300*time.Millisecond {
		w.closeAt = time.Now()
		w.rememberGeometry(true)
		if w.onClose != nil {
			w.onClose()
		}
	}
	if w.script != nil {
		w.script.step(w)
	}
	w.rememberGeometry(false)
	w.keyboard()
	w.mouse()
	if files := ebiten.DroppedFiles(); files != nil {
		// wie ein Terminal: Pfade als eingefügter Text, sshTermy erkennt
		// sie (ui/drop.go) und bietet Hochladen an
		if paths := droppedPaths(files); len(paths) > 0 {
			w.term.Paste(quotePaths(paths))
		}
	}

	// Zwischenablage: das Programm setzt sie per OSC 52 (tea.SetClipboard)
	if c := w.term.TakeClipboard(); c != "" {
		go func() { _ = clipboard.WriteAll(c) }()
	}
	if w.term.TakeBell() && !ebiten.IsFocused() {
		ebiten.RequestAttention()
	}
	title := w.opts.Title
	if t := strings.TrimSpace(w.term.Title()); t != "" && t != title {
		title = t + " – " + w.opts.Title
	}
	if title != w.lastTitle {
		w.lastTitle = title
		ebiten.SetWindowTitle(title)
	}
	return nil
}

// sendOrdered: w.in ist eine Warteschlange (queue.go), die Reihenfolge
// bleibt, und das Fenster wartet nie auf das Programm.
func (w *window) sendOrdered(b []byte) {
	if len(b) > 0 {
		_, _ = w.in.Write(b)
	}
}

// Sondertasten → Code für encodeKey
var specialKeys = map[ebiten.Key]rune{
	ebiten.KeyEnter: uv.KeyEnter, ebiten.KeyNumpadEnter: uv.KeyEnter,
	ebiten.KeyTab: uv.KeyTab, ebiten.KeyBackspace: uv.KeyBackspace, ebiten.KeyEscape: uv.KeyEscape,
	ebiten.KeyArrowUp: uv.KeyUp, ebiten.KeyArrowDown: uv.KeyDown,
	ebiten.KeyArrowLeft: uv.KeyLeft, ebiten.KeyArrowRight: uv.KeyRight,
	ebiten.KeyHome: uv.KeyHome, ebiten.KeyEnd: uv.KeyEnd,
	ebiten.KeyPageUp: uv.KeyPgUp, ebiten.KeyPageDown: uv.KeyPgDown,
	ebiten.KeyInsert: uv.KeyInsert, ebiten.KeyDelete: uv.KeyDelete,
	ebiten.KeyF1: uv.KeyF1, ebiten.KeyF2: uv.KeyF2, ebiten.KeyF3: uv.KeyF3, ebiten.KeyF4: uv.KeyF4,
	ebiten.KeyF5: uv.KeyF5, ebiten.KeyF6: uv.KeyF6, ebiten.KeyF7: uv.KeyF7, ebiten.KeyF8: uv.KeyF8,
	ebiten.KeyF9: uv.KeyF9, ebiten.KeyF10: uv.KeyF10, ebiten.KeyF11: uv.KeyF11, ebiten.KeyF12: uv.KeyF12,
}

const (
	repeatDelay = 24 // Ticks (60/s) bis zur ersten Wiederholung
	repeatEvery = 2
)

func repeating(k ebiten.Key) bool {
	d := inpututil.KeyPressDuration(k)
	return d == 1 || (d > repeatDelay && (d-repeatDelay)%repeatEvery == 0)
}

func (w *window) keyboard() {
	mac := runtime.GOOS == "darwin"
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl)
	alt := ebiten.IsKeyPressed(ebiten.KeyAlt)
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	cmd := ebiten.IsKeyPressed(ebiten.KeyMeta)
	var mod uv.KeyMod
	if shift {
		mod |= uv.ModShift
	}
	if ctrl {
		mod |= uv.ModCtrl
	}
	if mac {
		// Option bleibt für @ [ ] { } | ~ auf deutscher Tastatur; ⌘ ist Alt.
		if cmd {
			mod |= uv.ModAlt
		}
	} else if alt {
		mod |= uv.ModAlt
	}
	// Windows: AltGr kommt als Strg+Alt an und liefert Text
	chars := modChars(ebiten.AppendInputChars(nil), mod)

	// Fenster-Tasten: Einfügen, Schriftgröße
	shortcut := ctrl
	if mac {
		shortcut = cmd
	}
	for _, k := range inpututil.AppendJustPressedKeys(nil) {
		switch {
		case (mac && cmd && k == ebiten.KeyV) ||
			(!mac && ctrl && k == ebiten.KeyV && (shift || runtime.GOOS == "windows")) ||
			(shift && k == ebiten.KeyInsert):
			w.paste()
			return
		case shortcut && (k == ebiten.KeyEqual || k == ebiten.KeyNumpadAdd || (k == ebiten.KeyBracketRight && keyName(k) == "+")):
			w.zoom(1)
			return
		case shortcut && (k == ebiten.KeyMinus || k == ebiten.KeyNumpadSubtract || keyName(k) == "-"):
			w.zoom(-1)
			return
		case shortcut && (k == ebiten.KeyDigit0 || k == ebiten.KeyNumpad0):
			w.fontSize = defaultFontSize
			w.setFaces(w.scale)
			w.cols, w.rows = 0, 0
			w.rememberFont()
			return
		case k == ebiten.KeyF11 && !ctrl && !shift && !alt && !cmd:
			w.toggleFullscreen()
			return
		}
	}

	var buf []byte
	usedText := false
	for _, k := range inpututil.AppendPressedKeys(nil) {
		if !repeating(k) {
			continue
		}
		if code, ok := specialKeys[k]; ok {
			buf = append(buf, encodeKey(uv.Key{Code: code, Mod: mod})...)
			continue
		}
		if k == ebiten.KeySpace && mod&(uv.ModCtrl|uv.ModAlt) != 0 && len(chars) == 0 {
			buf = append(buf, encodeKey(uv.Key{Code: uv.KeySpace, Mod: mod})...)
			usedText = true
			continue
		}
		// Strg/Alt + Zeichen: Buchstabe nach Tastaturbelegung (QWERTZ!)
		if mod&(uv.ModCtrl|uv.ModAlt) != 0 && len(chars) == 0 {
			if r := keyRune(k); r != 0 {
				buf = append(buf, encodeKey(uv.Key{Code: r, Mod: mod})...)
				usedText = true
			}
		}
	}
	if !usedText {
		for _, r := range chars {
			if r == utf8.RuneError || r < 0x20 || r == 0x7f {
				continue
			}
			buf = append(buf, string(r)...)
		}
	}
	if len(buf) > 0 {
		w.sendOrdered(buf)
	}
}

// modChars wirft das Leerzeichen weg, das Windows bei Strg+Leertaste (und
// Alt+Leertaste) zusätzlich als Text meldet. Sonst ginge statt Strg+Space
// eine nackte Leertaste an die Oberfläche: Der Manager klappt dann den
// Ordner auf und zu, statt die Palette zu öffnen. AltGr-Text (@, €, {)
// bleibt erhalten.
func modChars(chars []rune, mod uv.KeyMod) []rune {
	if mod&(uv.ModCtrl|uv.ModAlt) == 0 {
		return chars
	}
	out := chars[:0]
	for _, r := range chars {
		if r != ' ' {
			out = append(out, r)
		}
	}
	return out
}

func keyName(k ebiten.Key) string { return ebiten.KeyName(k) }

// keyRune: das Zeichen, das die Taste auf der eingestellten Belegung trägt.
func keyRune(k ebiten.Key) rune {
	if n := ebiten.KeyName(k); n != "" {
		r, size := utf8.DecodeRuneInString(n)
		if size == len(n) {
			return r
		}
	}
	switch {
	case k >= ebiten.KeyA && k <= ebiten.KeyZ:
		return 'a' + rune(k-ebiten.KeyA)
	case k >= ebiten.KeyDigit0 && k <= ebiten.KeyDigit9:
		return '0' + rune(k-ebiten.KeyDigit0)
	}
	return 0
}

func (w *window) paste() {
	go func() {
		s, err := clipboard.ReadAll()
		if err == nil && s != "" {
			w.term.Paste(s)
		}
	}()
}

func (w *window) zoom(d float64) {
	w.fontSize = math.Max(8, math.Min(40, w.fontSize+d))
	w.setFaces(w.scale)
	w.cols, w.rows = 0, 0 // nächstes Layout rechnet die Größe neu
	w.rememberFont()
}

func (w *window) cellAt(px, py int) (int, int) {
	pad := padding * w.scale
	x := int((float64(px) - pad) / w.cellW)
	y := int((float64(py) - pad) / w.cellH)
	return max(0, min(x, w.cols-1)), max(0, min(y, w.rows-1))
}

func (w *window) mouse() {
	// Strg + Mausrad: Schriftgröße
	_, wy := ebiten.Wheel()
	if wy != 0 && (ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)) {
		w.zoom(math.Copysign(1, wy))
		return
	}
	level := w.term.MouseTracking()
	if level == termemu.MouseOff {
		return
	}
	var mod uv.KeyMod
	if ebiten.IsKeyPressed(ebiten.KeyShift) {
		mod |= uv.ModShift
	}
	if ebiten.IsKeyPressed(ebiten.KeyAlt) {
		mod |= uv.ModAlt
	}
	if ebiten.IsKeyPressed(ebiten.KeyControl) {
		mod |= uv.ModCtrl
	}
	cx, cy := ebiten.CursorPosition()
	x, y := w.cellAt(cx, cy)
	buttons := []struct {
		e ebiten.MouseButton
		u uv.MouseButton
	}{{ebiten.MouseButtonLeft, uv.MouseLeft}, {ebiten.MouseButtonMiddle, uv.MouseMiddle}, {ebiten.MouseButtonRight, uv.MouseRight}}
	for _, b := range buttons {
		if inpututil.IsMouseButtonJustPressed(b.e) {
			w.mouseBtn = b.u
			w.term.SendMouse(uv.MouseClickEvent{X: x, Y: y, Button: b.u, Mod: mod})
		}
		if inpututil.IsMouseButtonJustReleased(b.e) {
			w.term.SendMouse(uv.MouseReleaseEvent{X: x, Y: y, Button: b.u, Mod: mod})
			if w.mouseBtn == b.u {
				w.mouseBtn = uv.MouseNone
			}
		}
	}
	if x != w.mouseX || y != w.mouseY {
		w.mouseX, w.mouseY = x, y
		if (w.mouseBtn != uv.MouseNone && level >= termemu.MouseDrag) || level == termemu.MouseAny {
			w.term.SendMouse(uv.MouseMotionEvent{X: x, Y: y, Button: w.mouseBtn, Mod: mod})
		}
	}
	// Trackpads liefern Bruchteile: sammeln, bis eine ganze Raste voll ist
	w.wheelAcc += wy
	for w.wheelAcc >= 1 {
		w.term.SendMouse(uv.MouseWheelEvent{X: x, Y: y, Button: uv.MouseWheelUp, Mod: mod})
		w.wheelAcc--
	}
	for w.wheelAcc <= -1 {
		w.term.SendMouse(uv.MouseWheelEvent{X: x, Y: y, Button: uv.MouseWheelDown, Mod: mod})
		w.wheelAcc++
	}
}

func (w *window) Draw(screen *ebiten.Image) {
	gen := w.dirty.Load()
	if gen == w.drawn && !w.forceDraw {
		return
	}
	w.drawn, w.forceDraw = gen, false
	if w.white == nil {
		w.white = ebiten.NewImage(1, 1)
		w.white.Fill(color.White)
	}
	screen.Fill(defaultBg)

	var cw, ch int
	w.cells, cw, ch = w.term.Snapshot(w.cells)
	pad := math.Round(padding * w.scale)
	curX, curY, curVis := w.term.Cursor()

	// erst alle Hintergründe, dann Schrift: breite Zeichen dürfen in die
	// Nachbarzelle ragen
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			c := &w.cells[y*cw+x]
			if c.Width == 0 && c.Content == "" && x > 0 {
				continue
			}
			_, bg, set := cellColors(&c.Style)
			if curVis && x == curX && y == curY {
				bg, set = cursorClr, true
			}
			if !set {
				continue
			}
			width := max(1, c.Width)
			w.rect(screen, pad+float64(x)*w.cellW, pad+float64(y)*w.cellH, float64(width)*w.cellW, w.cellH, bg)
		}
	}
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			c := &w.cells[y*cw+x]
			fg, bg, _ := cellColors(&c.Style)
			if curVis && x == curX && y == curY {
				fg = bg
			}
			px, py := pad+float64(x)*w.cellW, pad+float64(y)*w.cellH
			if w.drawPlaceholder(screen, c, px, py) {
				continue
			}
			if c.Content != "" && c.Content != " " {
				if g := w.glyph(c); g != nil {
					op := &ebiten.DrawImageOptions{}
					op.GeoM.Translate(px, py)
					op.ColorScale.ScaleWithColor(fg)
					screen.DrawImage(g, op)
				}
			}
			width := float64(max(1, c.Width))
			if c.Style.Underline != uv.UnderlineNone {
				uc := fg
				if c.Style.UnderlineColor != nil {
					uc = rgba(c.Style.UnderlineColor, fg)
				}
				w.rect(screen, px, py+w.ascent+math.Max(1, w.scale), width*w.cellW, math.Max(1, w.scale), uc)
			}
			if c.Style.Attrs&uv.AttrStrikethrough != 0 {
				w.rect(screen, px, py+w.cellH/2, width*w.cellW, math.Max(1, w.scale), fg)
			}
		}
	}
	if w.script != nil {
		w.script.afterDraw(screen)
	}
}

func (w *window) rect(dst *ebiten.Image, x, y, width, height float64, c color.RGBA) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(width, height)
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(c)
	dst.DrawImage(w.white, op)
}

// glyph: weißes Zeichen auf durchsichtigem Grund, einmal gezeichnet und dann
// nur noch eingefärbt.
func (w *window) glyph(c *uv.Cell) *ebiten.Image {
	k := glyphKey{s: c.Content, bold: c.Style.Attrs&uv.AttrBold != 0, italic: c.Style.Attrs&uv.AttrItalic != 0, wide: c.Width > 1}
	if g, ok := w.glyphs[k]; ok {
		return g
	}
	if len(w.glyphs) > 8192 {
		w.glyphs = map[glyphKey]*ebiten.Image{}
	}
	cw := int(w.cellW)
	if k.wide {
		cw *= 2
	}
	ch := int(w.cellH)
	var img *ebiten.Image
	if r, size := utf8.DecodeRuneInString(k.s); size == len(k.s) {
		if a := boxGlyph(r, cw, ch); a != nil {
			img = ebiten.NewImageFromImage(a)
		}
	}
	if img == nil {
		img = ebiten.NewImage(cw, ch)
		face := w.regular
		if k.bold {
			face = w.bold
		}
		op := &text.DrawOptions{}
		if k.italic {
			op.GeoM.Skew(-0.2, 0)
			op.GeoM.Translate(w.ascent*0.2, 0)
		}
		op.GeoM.Translate(0, w.ascent-w.ascentF)
		text.Draw(img, k.s, face, op)
	}
	w.glyphs[k] = img
	return img
}

// drawPlaceholder zeichnet an einer kitty-Platzhalterzelle das passende
// Stück des Bildes. false = keine Platzhalterzelle.
func (w *window) drawPlaceholder(dst *ebiten.Image, c *uv.Cell, px, py float64) bool {
	if len(c.Content) < 4 {
		return false
	}
	fg := rgba(c.Style.Fg, defaultFg)
	id, row, col, ok := placeholderCell(c.Content, fg.R, fg.G, fg.B)
	if !ok {
		return false
	}
	ki := w.images.get(id)
	if ki == nil || row >= ki.rows || col >= ki.cols {
		return true // Bild (noch) nicht da: Zelle leer lassen
	}
	if w.imgCache == nil {
		w.imgCache = map[*kittyImage]*ebiten.Image{}
	}
	img, ok := w.imgCache[ki]
	if !ok {
		if len(w.imgCache) > maxImages {
			for k, v := range w.imgCache {
				v.Deallocate()
				delete(w.imgCache, k)
			}
		}
		img = ebiten.NewImageFromImage(ki.img)
		w.imgCache[ki] = img
	}
	b := img.Bounds()
	x0 := b.Min.X + col*b.Dx()/ki.cols
	x1 := b.Min.X + (col+1)*b.Dx()/ki.cols
	y0 := b.Min.Y + row*b.Dy()/ki.rows
	y1 := b.Min.Y + (row+1)*b.Dy()/ki.rows
	if x1 <= x0 || y1 <= y0 {
		return true
	}
	sub := img.SubImage(image.Rect(x0, y0, x1, y1)).(*ebiten.Image)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(w.cellW/float64(x1-x0), w.cellH/float64(y1-y0))
	op.GeoM.Translate(px, py)
	dst.DrawImage(sub, op)
	return true
}
