package ui

// Bewegung, Teil 2: Verbindungsaufbau als Karte, Shell blendet ein,
// KI-Antwort mit „denkt nach“-Schimmer und Schreibmaschine, Befehlspalette
// mit leuchtenden Treffern und weich nachrückender Liste. Stufen und Takt
// wie in motion.go.

import (
	"image/color"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
)

const (
	revealDur  = 380 * time.Millisecond // Shell-Zeilen blenden von oben ein
	palRowFade = 140 * time.Millisecond // Palette: eine Zeile blendet ein
	palRowLag  = 14 * time.Millisecond  // … und die nächste etwas später
	palHitGlow = 700 * time.Millisecond // getroffene Buchstaben leuchten nach
)

// ---------------------------------------------------------------- Verbinden

// connectCard: Karte während des Verbindens mit den Schritten, Fortschritt
// und Lichtlauf.
func (a *App) connectCard(s *session, w int) string {
	st := a.s
	now := time.Now()
	lvl := motionLevel()
	bg := st.T.Surface
	on := func(c color.Color, bold bool) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(c).Background(bg).Bold(bold)
	}
	w = max(30, min(w, 58))
	inner := w - 4
	var lines []string
	steps := []string{i18n.T("Verbindung aufbauen"), i18n.T("Host-Schlüssel prüfen"), i18n.T("Anmelden"), i18n.T("Shell öffnen")}
	if s.conn.Protocol.Raw() {
		steps = nil // Telnet/Konsole: keine SSH-Schritte
	}
	spin := "◌"
	if lvl > 0 {
		spin = spinGlyph(now)
	}
	for i, name := range steps {
		var l string
		switch {
		case i < s.connStep:
			took := ""
			if a, b := s.connStepAt[i], s.connStepAt[i+1]; !a.IsZero() && !b.IsZero() {
				took = fmtLatency(b.Sub(a))
			}
			l = on(st.T.OK, true).Render("✓ ") + on(st.T.Text, false).Render(name)
			if took != "" {
				l += on(st.T.Muted, false).Render(strings.Repeat(" ", max(1, 26-utf8.RuneCountInString(name))) + took)
			}
		case i == s.connStep:
			l = on(st.T.Accent, true).Render(spin+" ") + on(st.T.Text, true).Render(name)
		default:
			l = on(st.T.Border, false).Render("○ " + name)
		}
		lines = append(lines, l)
	}
	// Fortschritt: erledigte Schritte + ein Stück vom laufenden
	prog := 0.0
	if n := len(steps); n > 0 {
		part := 0.0
		if t0 := s.connStepAt[min(s.connStep, 3)]; !t0.IsZero() {
			part = 0.85 * easeOut(now.Sub(t0).Seconds()/1.5)
		}
		prog = (float64(s.connStep) + part) / float64(n)
	}
	lines = append(lines, "", a.sweepBar(prog, inner, now, len(steps) == 0))
	if s.connNote != "" {
		lines = append(lines, "", on(st.T.Warn, false).Render(trunc(s.connNote, inner)))
	}
	if len(s.ahead) > 0 { // vorausgetippt (typeahead.go): sichtbar, dass nichts verloren geht
		lbl := i18n.T("Getippt: ")
		txt := string(s.ahead)
		if n := inner - lipgloss.Width(lbl) - 1; utf8.RuneCountInString(txt) > n {
			r := []rune(txt)
			txt = "…" + string(r[len(r)-max(0, n-1):])
		}
		lines = append(lines, "", on(st.T.Muted, false).Render(lbl)+on(st.T.Text, true).Render(txt+"▏"),
			on(st.T.Muted, false).Render(trunc(i18n.T("kommt am Prompt an · Esc bricht ab"), inner)))
	}
	for i, l := range lines {
		lines[i] = l + on(st.T.Text, false).Render(strings.Repeat(" ", max(0, inner-lipgloss.Width(l))))
	}
	title := on(st.T.AI, true).Render(" " + i18n.T("Verbinde mit ") + trunc(s.conn.Alias, inner-14) + " ")
	box := lipgloss.NewStyle().Border(frameBorder).BorderForeground(st.T.Accent).BorderBackground(st.T.Bg).
		Background(bg).Padding(1, 1, 0, 1).Render(strings.Join(lines, "\n"))
	// Titel in die obere Rahmenlinie setzen
	bl := strings.SplitN(box, "\n", 2)
	if len(bl) == 2 {
		tc, _ := frameCorners()
		top := lipgloss.NewStyle().Foreground(st.T.Accent).Background(st.T.Bg).Render(string(tc[0])+"─") + title
		top += lipgloss.NewStyle().Foreground(st.T.Accent).Background(st.T.Bg).Render(strings.Repeat("─", max(0, w-2-lipgloss.Width(title)-2)) + string(tc[1]))
		box = top + "\n" + bl[1]
	}
	sub := st.Muted.Render(s.conn.Target())
	return lipgloss.JoinVertical(lipgloss.Center, box, "", sub)
}

// sweepBar: Fortschritt Akzent → KI, mit einem wandernden Lichtpunkt
// (voll) oder ohne. busy = Fortschritt unbekannt, nur der Lichtlauf.
func (a *App) sweepBar(f float64, w int, now time.Time, busy bool) string {
	st := a.s
	lvl := motionLevel()
	n := int(f * float64(w))
	if busy {
		n = w
	}
	pos := -100.0
	if lvl == 2 {
		period := 1600 * time.Millisecond
		pos = float64(now.UnixNano()%int64(period))/float64(period)*float64(w+16) - 8
	}
	var b strings.Builder
	for i := 0; i < w; i++ {
		c := st.T.Surface2
		if i < n {
			c = mixColor(st.T.Accent, st.T.AI, float64(i)/float64(max(1, w-1)))
			if busy {
				c = mixColor(st.T.Surface2, c, 0.35)
			}
			if d := float64(i) - pos; d > -5 && d < 5 {
				c = mixColor(c, st.T.Text, 0.75*(1-absf(d)/5))
			}
		}
		b.WriteString(lipgloss.NewStyle().Foreground(c).Background(st.T.Surface).Render("━"))
	}
	return b.String()
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// revealLines: kurz nach dem Verbinden erscheinen die Zeilen von oben nach
// unten statt auf einmal.
func revealLines(s *session, body string, rows int, now time.Time) string {
	d := now.Sub(s.revealAt)
	if s.revealAt.IsZero() || d >= revealDur || motionLevel() == 0 {
		return body
	}
	show := int(easeOut(float64(d)/float64(revealDur))*float64(rows)) + 1
	lines := strings.Split(body, "\n")
	for i := show; i < len(lines); i++ {
		lines[i] = ""
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------- KI-Antwort

// shimmer: Text mit einem Lichtpunkt, der durchläuft („denkt nach …“).
func (a *App) shimmer(text string, base, hi color.Color, now time.Time) string {
	if motionLevel() < 2 {
		return lipgloss.NewStyle().Foreground(base).Render(text)
	}
	r := []rune(text)
	period := 1400 * time.Millisecond
	pos := float64(now.UnixNano()%int64(period))/float64(period)*float64(len(r)+8) - 4
	var b strings.Builder
	for i, ch := range r {
		k := 0.0
		if d := float64(i) - pos; d > -3 && d < 3 {
			k = 1 - absf(d)/3
		}
		b.WriteString(lipgloss.NewStyle().Foreground(mixColor(base, hi, k)).Bold(k > 0.5).Render(string(ch)))
	}
	return b.String()
}

// typeOn: Schreibmaschine. Die Antwort kommt in Brocken; gezeigt wird sie
// gleichmäßig, mit Vorsprung abbauen statt springen.
func (q *askState) typeOn(now time.Time) {
	full := utf8.RuneCountInString(q.answer.String())
	if motionLevel() == 0 || q.shown >= full {
		q.shown = full
		return
	}
	step := max(2, (full-q.shown)/5)
	q.shown = min(full, q.shown+step)
}

// visibleAsk: der Teil der Antwort, der schon „getippt“ ist.
func (q *askState) visibleAsk() string {
	ans := q.answer.String()
	if motionLevel() == 0 {
		return agent.VisibleAnswer(ans)
	}
	r := []rune(ans)
	if q.shown < len(r) {
		ans = string(r[:q.shown])
	}
	return agent.VisibleAnswer(ans)
}

// typing: läuft die Schreibmaschine noch?
func (q *askState) typing() bool {
	return motionLevel() > 0 && q.shown < utf8.RuneCountInString(q.answer.String())
}

// ---------------------------------------------------------------- Palette

// palMotion: merkt sich Öffnen, Tippen und Auswahl für die Übergänge.
type palMotion struct {
	query   string
	queryAt time.Time
	sel     int
	selAt   time.Time
	opened  bool
	prevSel int // vorher gewählte Zeile (Nachleuchten, palstyle.go)
	// changed: Label des zuletzt umgeschalteten Werts (leuchtet kurz)
	changed   string
	changedAt time.Time
}

// palRowK: wie weit Zeile i eingeblendet ist (0 … 1).
func (p *palette) rowK(i int, now time.Time) float64 {
	if motionLevel() == 0 {
		return 1
	}
	return easeOut(float64(now.Sub(p.mo.queryAt)-time.Duration(i)*palRowLag) / float64(palRowFade))
}

// track: Änderungen seit dem letzten Bild.
func (p *palette) track(now time.Time) {
	q := p.in.Value()
	if !p.mo.opened || q != p.mo.query {
		p.mo.opened, p.mo.query, p.mo.queryAt = true, q, now
	}
	if p.sel != p.mo.sel {
		p.mo.prevSel, p.mo.sel, p.mo.selAt = p.mo.sel, p.sel, now
	}
}

// animating: braucht die Palette gerade schnelle Bilder?
func (p *palette) animating(now time.Time) bool {
	return now.Sub(p.mo.queryAt) < palHitGlow || now.Sub(p.mo.selAt) < max(selFadeDur, palTrailDur) ||
		now.Sub(p.mo.changedAt) < palGlowDur
}

// fuzzyHits: Positionen (Runen) der Buchstaben von q in s, nacheinander,
// ohne Groß/klein. nil = kein Treffer.
func fuzzyHits(q, s string) []int {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	qr := []rune(q)
	var hits []int
	j := 0
	for i, c := range []rune(strings.ToLower(s)) {
		if j < len(qr) && c == qr[j] {
			hits = append(hits, i)
			j++
			for j < len(qr) && qr[j] == ' ' {
				j++ // Leerzeichen in der Suche überspringen
			}
		}
	}
	if j < len(qr) {
		return nil
	}
	return hits
}

// hitLabel: Beschriftung mit hervorgehobenen Treffern. glow 0 … 1 = wie
// stark der Hintergrund der Treffer noch leuchtet.
func hitLabel(label string, hits []int, base, hit lipgloss.Style, glow color.Color) string {
	if len(hits) == 0 {
		return base.Render(label)
	}
	isHit := map[int]bool{}
	for _, h := range hits {
		isHit[h] = true
	}
	var b strings.Builder
	var run []rune
	runHit := false
	flush := func() {
		if len(run) == 0 {
			return
		}
		if runHit {
			hs := hit
			if glow != nil {
				hs = hs.Background(glow)
			}
			b.WriteString(hs.Render(string(run)))
		} else {
			b.WriteString(base.Render(string(run)))
		}
		run = run[:0]
	}
	for i, c := range []rune(label) {
		if isHit[i] != runHit {
			flush()
			runHit = isHit[i]
		}
		run = append(run, c)
	}
	flush()
	return b.String()
}
