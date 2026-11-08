package ui

// F5: laufende Übertragung mit Balken (Lichtlauf), aktuellem Tempo,
// Restzeit, Datei n von m und einer kleinen Tempo-Kurve aus Braille-Punkten.

import (
	"fmt"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/i18n"
)

// xferSpeed: Tempo über die letzten Sekunden (aus den Ticks alle 200 ms).
type xferSpeed struct {
	at   []time.Time
	done []int64
	hist []float64 // Bytes/s, eine Probe je Tick
}

const xferWindow = 2 * time.Second

func (x *xferSpeed) sample(now time.Time, done int64) {
	x.at = append(x.at, now)
	x.done = append(x.done, done)
	for len(x.at) > 2 && now.Sub(x.at[0]) > xferWindow {
		x.at, x.done = x.at[1:], x.done[1:]
	}
	x.hist = append(x.hist, x.rate())
	if len(x.hist) > 240 {
		x.hist = x.hist[len(x.hist)-240:]
	}
}

// rate: Bytes/s über das Fenster (0 = noch unbekannt).
func (x *xferSpeed) rate() float64 {
	n := len(x.at)
	if n < 2 {
		return 0
	}
	dt := x.at[n-1].Sub(x.at[0]).Seconds()
	if dt <= 0 {
		return 0
	}
	return float64(x.done[n-1]-x.done[0]) / dt
}

// xferStatus: zwei Zeilen unter den Dateilisten.
func (a *App) xferStatus(x *xfer, w int) string {
	st := a.s
	now := time.Now()
	done, total := x.pr.Done.Load(), x.pr.Total.Load()
	pct := 0.0
	if total > 0 {
		pct = min(1, float64(done)/float64(total))
	}
	rate := x.speed.rate()
	if rate == 0 {
		rate = float64(done) / max(now.Sub(x.start).Seconds(), 0.001)
	}
	eta := ""
	if total > 0 && rate > 0 && done < total {
		eta = i18n.T("noch ") + shortDur(time.Duration(float64(total-done)/rate*float64(time.Second)))
	}
	barW := max(10, min(48, w/3))
	bar := a.sweepBar(pct, barW, now, total == 0) // motion2.go
	info := fmt.Sprintf(" %3.0f %%  %s / %s  ", pct*100, humanBytes(done), humanBytes(total))
	line1 := " " + bar + st.Text.Render(info) + st.Accent.Render(humanBytes(int64(rate))+"/s")
	if eta != "" {
		line1 += st.Muted.Render("  " + eta)
	}
	// Zeile 2: Tempo-Kurve, Datei n/m, aktuelle Datei
	graphW := min(24, max(8, w/6))
	curve := graphBlock(scaleTo(x.speed.hist, 1), graphW, 1, st.T.Accent, st.T.AI)
	nf := x.files.Load()
	fileInfo := ""
	if nf > 1 {
		fileInfo = fmt.Sprintf(i18n.T("Datei %d von %d · "), min(nf, x.pr.Files.Load()+1), nf)
	}
	cur := x.pr.Current()
	if cur == "" {
		cur = x.what
	}
	rest := max(10, w-graphW-4-lipgloss.Width(fileInfo))
	line2 := " " + curve + "  " + st.Muted.Render(fileInfo+truncLeft(cur, rest))
	return line1 + "\n" + line2
}
