package ui

import (
	"context"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Eigene Felder in der Kopfzeile (Idee aus herdr): die Uhrzeit und/oder die
// erste Zeile eines lokalen Befehls, z. B. VPN-Status, Ticketnummer, freier
// Platz. Der Befehl läuft auf diesem Rechner alle N Sekunden (Standard 30)
// mit 5 s Zeitlimit; er ist rein lokal und kommt nie per Sync von einem
// anderen Rechner. Einstellen über die Palette („Kopfzeile“ suchen).

const (
	headerEveryDefault = 30
	headerEveryMin     = 5
	headerTimeout      = 5 * time.Second
)

type headerOutMsg struct {
	cmd, out string
}

func (a *App) headerCfg() domain.HeaderConfig { return a.st.Settings().Header }

// headerTick (aus onTick): Befehl fällig? Dann im Hintergrund starten.
func (a *App) headerTick(now time.Time) tea.Cmd {
	h := a.headerCfg()
	cmd := strings.TrimSpace(h.Cmd)
	if cmd == "" || a.hdrBusy || a.scr == scrUnlock || a.lock != nil {
		return nil
	}
	every := h.Every
	if every <= 0 {
		every = headerEveryDefault
	}
	every = max(every, headerEveryMin)
	if now.Sub(a.hdrAt) < time.Duration(every)*time.Second && a.hdrFor == cmd {
		return nil
	}
	a.hdrBusy, a.hdrAt = true, now
	return func() tea.Msg { return headerOutMsg{cmd, runHeaderCmd(cmd)} }
}

// runHeaderCmd: erste nicht leere Zeile der Ausgabe, ohne Steuerzeichen.
func runHeaderCmd(cmd string) string {
	ctx, cancel := context.WithTimeout(context.Background(), headerTimeout)
	defer cancel()
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.CommandContext(ctx, "cmd", "/C", cmd)
	} else {
		c = exec.CommandContext(ctx, "sh", "-c", cmd)
	}
	hideConsole(c)
	out, err := c.Output()
	line := ""
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			line = l
			break
		}
	}
	line = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, line)
	if line == "" && err != nil {
		if ctx.Err() != nil {
			return "…"
		}
		return "×"
	}
	return trunc(line, 40)
}

func (a *App) onHeaderOut(m headerOutMsg) {
	a.hdrBusy = false
	a.hdrOut, a.hdrFor = m.out, m.cmd
}

// headerFields: die eigenen Felder als Abzeichen ("" = keine).
func (a *App) headerFields() string {
	h := a.headerCfg()
	st := a.s
	var parts []string
	if strings.TrimSpace(h.Cmd) != "" && a.hdrOut != "" && a.hdrFor == strings.TrimSpace(h.Cmd) {
		parts = append(parts, st.Badge.Foreground(st.T.Text).Render(a.hdrOut))
	}
	if h.Clock {
		parts = append(parts, st.Badge.Foreground(st.T.Text).Render(time.Now().Format("15:04")))
	}
	return strings.Join(parts, "")
}

func (a *App) toggleHeaderClock() tea.Cmd {
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Header.Clock = !s.Header.Clock })
	return nil
}

// askHeaderCmd: Befehl und Abstand einstellen („befehl“ oder „befehl @60“).
func (a *App) askHeaderCmd() tea.Cmd {
	h := a.headerCfg()
	cur := h.Cmd
	if cur != "" && h.Every > 0 && h.Every != headerEveryDefault {
		cur += " @" + strconv.Itoa(h.Every)
	}
	m := inputModal(i18n.T("Kopfzeile: eigener Befehl"),
		i18n.T("Läuft auf diesem Rechner, die erste Zeile der Ausgabe steht rechts in der Kopfzeile. Alle 30 s; anders mit „@Sekunden“ am Ende. Leer = aus. Beispiele:")+
			"\n  nmcli -t -f NAME connection show --active | head -1\n  curl -s ifconfig.me @300\n  powershell -c \"(Get-VpnConnection).ConnectionStatus\"",
		cur, func(v string) tea.Cmd {
			cmd, every := parseHeaderCmd(v)
			_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Header.Cmd, s.Header.Every = cmd, every })
			a.hdrOut, a.hdrFor, a.hdrAt = "", "", time.Time{}
			if cmd == "" {
				a.flash(i18n.T("Kopfzeile: eigener Befehl aus"))
			} else {
				a.flashOK(i18n.T("Kopfzeile: eigener Befehl gespeichert"))
			}
			return nil
		})
	a.modal = m
	return m.in.Focus()
}

// parseHeaderCmd: „befehl @60“ → befehl, 60.
func parseHeaderCmd(v string) (string, int) {
	v = strings.TrimSpace(v)
	if i := strings.LastIndex(v, " @"); i > 0 {
		if n, err := strconv.Atoi(strings.TrimSpace(v[i+2:])); err == nil && n > 0 {
			return strings.TrimSpace(v[:i]), max(n, headerEveryMin)
		}
	}
	return v, 0
}
