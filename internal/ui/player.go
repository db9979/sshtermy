package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/recording"
	"github.com/db9979/sshtermy/internal/termemu"
)

// Abspieler für Aufnahmen (.cast, asciicast v2/v3): Pause, Tempo, Spulen,
// Suche im Text, Sprungmarken, lange Pausen kürzen. Spielt im eigenen
// Bildschirm mit einem eigenen Terminal-Emulator; nichts geht an einen
// Server.

// playSpeeds: wählbare Tempi.
var playSpeeds = []float64{0.25, 0.5, 1, 2, 4, 8, 16}

// playFrame: Abstand der Bilder beim Abspielen.
const playFrame = 40 * time.Millisecond

// playIdle: auf so viele Sekunden kürzt i lange Pausen.
const playIdle = 2.0

type playerState struct {
	cast    *recording.Cast
	path    string
	term    *termemu.Term
	pos     int     // nächstes Ereignis
	t       float64 // aktuelle Zeit (s)
	playing bool
	speed   int // Index in playSpeeds
	idle    bool
	last    time.Time
	seq     int
	back    screen
	query   string
	note    string
	marker  string // zuletzt passierte Marke
}

type playTickMsg struct{ seq int }

func (a *App) playTick() tea.Cmd {
	seq := a.play.seq
	return tea.Tick(playFrame, func(time.Time) tea.Msg { return playTickMsg{seq: seq} })
}

// openPlayerPicker: .cast-Dateien im Download-Ordner, neueste zuerst.
func (a *App) openPlayerPicker() tea.Cmd {
	dir := exportDir()
	type f struct {
		path string
		mod  time.Time
		size int64
	}
	var fs []f
	matches, _ := filepath.Glob(filepath.Join(dir, "*.cast"))
	for _, p := range matches {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			fs = append(fs, f{p, st.ModTime(), st.Size()})
		}
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].mod.After(fs[j].mod) })
	items := []pickItem{{value: "\x00other", label: i18n.T("Andere Datei …"), hint: i18n.T("Pfad eingeben")}}
	for i, x := range fs {
		if i >= 200 {
			break
		}
		items = append(items, pickItem{value: x.path, label: filepath.Base(x.path), detail: localTime(x.mod), hint: humanSize(x.size)})
	}
	cur := ""
	if len(fs) > 0 {
		cur = fs[0].path
	}
	a.pick = newPicker(i18n.T("Aufnahme abspielen"), i18n.T("Datei suchen …"), items, cur, func(it pickItem) tea.Cmd {
		if it.value == "\x00other" {
			a.modal = inputModal(i18n.T("Aufnahme abspielen"), i18n.T("Pfad zur .cast-Datei:"), filepath.Join(dir, ""), func(p string) tea.Cmd {
				return a.openPlayer(strings.TrimSpace(p))
			})
			return nil
		}
		return a.openPlayer(it.value)
	})
	a.pick.help = fmt.Sprintf(i18n.T("Aufnahmen aus %s (Palette › Aufnahme starten nimmt einen Tab auf)."), dir)
	return nil
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d B", n)
}

// openPlayer lädt eine Aufnahme und spielt sie ab.
func (a *App) openPlayer(path string) tea.Cmd {
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[1:])
		}
	}
	st, err := os.Stat(path)
	if err != nil {
		a.flash(i18n.T("Datei nicht gefunden: ") + path)
		return nil
	}
	if st.Size() > 2*recording.MaxBytes {
		a.flash(i18n.T("Aufnahme zu groß"))
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		a.flash(err.Error())
		return nil
	}
	return a.playCast(path, data)
}

// playCast spielt eine Aufnahme aus dem Speicher ab (name: Anzeige).
func (a *App) playCast(name string, data []byte) tea.Cmd {
	c, err := recording.Load(data)
	if err != nil {
		a.flash(i18n.T("Keine abspielbare Aufnahme: ") + err.Error())
		return nil
	}
	p := &a.play
	if p.term != nil {
		p.term.Close()
	}
	back := a.scr
	if back == scrPlayer {
		back = p.back
	}
	*p = playerState{cast: c, path: name, seq: p.seq + 1, back: back, speed: 2, playing: true, last: time.Now()}
	p.reset()
	a.scr = scrPlayer
	return a.playTick()
}

func (p *playerState) reset() {
	if p.term != nil {
		p.term.Close()
	}
	p.term = termemu.New(max(10, p.cast.Width), max(3, p.cast.Height), io.Discard)
	p.term.MuteReplies(true)
	p.pos, p.t, p.marker = 0, 0, ""
}

// advance: Ereignisse bis Zeitpunkt t ins Terminal schreiben.
func (p *playerState) advance(t float64) {
	ev := p.cast.Events
	for p.pos < len(ev) && ev[p.pos].Time <= t {
		e := ev[p.pos]
		switch e.Kind {
		case "o":
			_, _ = p.term.Write([]byte(e.Data))
		case "r":
			if w, h, ok := strings.Cut(e.Data, "x"); ok {
				cw, _ := strconv.Atoi(w)
				ch, _ := strconv.Atoi(h)
				p.term.Resize(cw, ch)
			}
		case "m":
			p.marker = e.Data
		}
		p.pos++
	}
	p.t = t
}

// seek springt zu t (rückwärts: von vorn neu aufbauen).
func (p *playerState) seek(t float64) {
	t = min(max(0, t), p.cast.Duration())
	if t < p.t {
		p.reset()
	}
	p.advance(t)
	p.last = time.Now()
}

func (a *App) playerMsgs(msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(playTickMsg)
	if !ok {
		return nil, false
	}
	p := &a.play
	if m.seq != p.seq || a.scr != scrPlayer || !p.playing || p.cast == nil {
		return nil, true
	}
	now := time.Now()
	dt := now.Sub(p.last).Seconds() * playSpeeds[p.speed]
	p.last = now
	p.advance(p.t + dt)
	if p.pos >= len(p.cast.Events) {
		p.playing, p.t = false, p.cast.Duration()
		return nil, true
	}
	return a.playTick(), true
}

func (a *App) closePlayer() {
	p := &a.play
	if p.term != nil {
		p.term.Close()
		p.term = nil
	}
	p.seq++
	p.cast = nil
	a.scr = p.back
	if a.scr == scrPlayer || a.scr == scrUnlock {
		a.scr = scrManager
		if a.current() != nil {
			a.scr = scrTerminal
		}
	}
}

func (a *App) updatePlayer(msg tea.Msg) tea.Cmd {
	p := &a.play
	if p.cast == nil {
		a.closePlayer()
		return nil
	}
	if mw, ok := msg.(tea.MouseWheelMsg); ok {
		switch mw.Button {
		case tea.MouseWheelUp:
			p.seek(p.t - 5)
		case tea.MouseWheelDown:
			p.seek(p.t + 5)
		}
		return nil
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	wasPlaying := p.playing
	p.note = ""
	switch k.String() {
	case "esc", "q":
		a.closePlayer()
		return nil
	case "space", " ", "k", "p":
		if p.pos >= len(p.cast.Events) {
			p.seek(0)
		}
		p.playing = !p.playing
		p.last = time.Now()
	case "left", "h":
		p.seek(p.t - 5)
	case "right", "l":
		p.seek(p.t + 5)
	case "shift+left", "H":
		p.seek(p.t - 30)
	case "shift+right", "L":
		p.seek(p.t + 30)
	case "up", "+", "=":
		p.speed = min(len(playSpeeds)-1, p.speed+1)
	case "down", "-":
		p.speed = max(0, p.speed-1)
	case "home", "0":
		p.seek(0)
	case "end":
		p.seek(p.cast.Duration())
		p.playing = false
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		p.seek(p.cast.Duration() * float64(k.String()[0]-'0') / 10)
	case ".":
		// Einzelschritt: nächstes Ereignis
		p.playing = false
		if p.pos < len(p.cast.Events) {
			p.seek(p.cast.Events[p.pos].Time)
		}
	case "i":
		p.idle = !p.idle
		frac := 0.0
		if d := p.cast.Duration(); d > 0 {
			frac = p.t / d
		}
		if p.idle {
			p.cast.IdleLimit(playIdle)
		} else {
			p.cast.IdleLimit(0)
		}
		pos := p.pos
		p.reset()
		if pos > 0 {
			p.advance(p.cast.Events[pos-1].Time)
		} else {
			p.advance(frac * p.cast.Duration())
		}
	case "m":
		if i := p.cast.NextMarker(p.t); i >= 0 {
			p.seek(p.cast.Events[i].Time)
		} else {
			p.note = i18n.T("Keine weitere Marke")
		}
	case "/":
		p.playing = false
		a.modal = inputModal(i18n.T("In der Aufnahme suchen"), i18n.T("Text, der irgendwann auf dem Bildschirm stand:"), p.query, func(q string) tea.Cmd {
			p.query = strings.TrimSpace(q)
			a.playerFind(0)
			return nil
		})
		a.modal.in.Placeholder = i18n.T("Text")
		return nil
	case "n":
		a.playerFind(p.pos)
	case "N":
		a.playerFind(0)
	}
	if p.playing && !wasPlaying {
		return a.playTick()
	}
	return nil
}

// playerFind: nächster Treffer ab Ereignis from; hält dort an.
func (a *App) playerFind(from int) {
	p := &a.play
	if p.query == "" {
		return
	}
	i := p.cast.Search(p.query, from)
	if i < 0 && from > 0 {
		i = p.cast.Search(p.query, 0)
		if i >= 0 {
			p.note = i18n.T("Suche von vorn")
		}
	}
	if i < 0 {
		p.note = fmt.Sprintf(i18n.T("„%s“ kommt nicht vor"), p.query)
		return
	}
	p.playing = false
	p.seek(p.cast.Events[i].Time)
	if p.note == "" {
		p.note = fmt.Sprintf(i18n.T("Treffer bei %s · n nächster"), playClock(p.cast.Events[i].Time))
	}
}

func playClock(s float64) string {
	d := time.Duration(s * float64(time.Second)).Round(time.Second)
	h, m, sec := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%02d:%02d", m, sec)
}

func (a *App) playerKeys() [][2]string {
	p := &a.play
	pl := i18n.T("Pause")
	if !p.playing {
		pl = i18n.T("Abspielen")
	}
	idle := i18n.T("Pausen kürzen")
	if p.idle {
		idle = i18n.T("Originalzeit")
	}
	return [][2]string{{"Space", pl}, {"←→", "5 s"}, {"↑↓", i18n.T("Tempo")}, {"/", i18n.T("suchen")}, {"n", i18n.T("weiter")},
		{"m", i18n.T("Marke")}, {"i", idle}, {"1…9", "10…90 %"}, {"Esc", i18n.T("schließen")}}
}

func (a *App) viewPlayer() string {
	p := &a.play
	st := a.s
	if p.cast == nil || p.term == nil {
		return ""
	}
	w, h := max(20, a.w), max(5, a.h-2)
	var b strings.Builder
	title := orDefault(p.cast.Title, filepath.Base(p.path))
	b.WriteString(st.PanelTitle.Render(trunc("▶ "+title, w)) + "\n")
	tw, th := p.term.Size()
	rows := h - 4
	lines := strings.Split(p.term.Render(0, nil), "\n")
	for i := 0; i < rows; i++ {
		if i < len(lines) && i < th {
			b.WriteString(ansi.Truncate(lines[i], w, "") + "\x1b[0m")
		}
		b.WriteString("\n")
	}
	// Statuszeile + Fortschritt
	icon := "▶"
	if !p.playing {
		icon = "■"
	}
	dur := p.cast.Duration()
	status := fmt.Sprintf("%s %s / %s  ×%s", icon, playClock(p.t), playClock(dur), strconv.FormatFloat(playSpeeds[p.speed], 'f', -1, 64))
	if p.idle {
		status += i18n.T("  · Pausen gekürzt")
	}
	if tw > w || th > rows {
		status += fmt.Sprintf(i18n.T("  · Fenster kleiner als Aufnahme (%d×%d)"), tw, th)
	}
	if p.marker != "" {
		status += "  · " + p.marker
	}
	if p.note != "" {
		status += "  · " + p.note
	}
	b.WriteString(st.Text.Render(trunc(status, w)) + "\n")
	barW := max(10, w-2)
	fill := 0
	if dur > 0 {
		fill = int(float64(barW) * p.t / dur)
	}
	fill = min(fill, barW)
	b.WriteString(st.Accent.Render(strings.Repeat("━", fill)) + st.Muted.Render(strings.Repeat("─", barW-fill)))
	return b.String()
}
