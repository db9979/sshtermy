package ui

import (
	"context"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/push"
)

// Handy-Meldungen (ntfy, Gotify, Webhook)
//
// Was als Desktop-Meldung kommt (langer Befehl fertig, Zeitplan, Logwache,
// Farbregel, Agent oder Programm wartet auf Eingabe, Gegenprüfung mit
// Zweifel), geht zusätzlich aufs Handy, wenn niemand am Rechner ist: Fenster
// im Hintergrund, gesperrt oder pushIdle lang keine Taste. Der Text läuft
// durch denselben Geheimnisfilter wie bei der KI; gesperrt steht wie bei der
// Desktop-Meldung kein Befehl darin.

// pushIdle: so lange ohne Tastendruck gilt man als weg vom Rechner.
const pushIdle = 5 * time.Minute

// pushSame: dieselbe Meldung nicht öfter als einmal in dieser Zeit.
const pushSame = time.Minute

type pushErrMsg struct{ err error }

var pushRecent = struct {
	sync.Mutex
	at map[string]time.Time
}{at: map[string]time.Time{}}

// pushSendFn: in Tests austauschbar.
var pushSendFn = push.Send

func (a *App) pushCfg() push.Config {
	p := a.st.Settings().Push
	return push.Config{Kind: p.Kind, URL: p.URL, Token: string(p.Token)}
}

func (a *App) pushOn() bool { return a.st.Settings().Push.Kind != "" }

// desktopAway: Desktop-Meldung jetzt sinnvoll?
func (a *App) desktopAway() bool { return (a.blurred || a.lock != nil) && a.desktopNotifyOn() }

// pushAway: Handy-Meldung jetzt sinnvoll?
func (a *App) pushAway() bool {
	if !a.pushOn() {
		return false
	}
	idle := !a.lastKey.IsZero() && time.Since(a.lastKey) > pushIdle
	return a.blurred || a.lock != nil || idle
}

// notifyAway: soll eine Meldung nach außen (Desktop oder Handy)?
func (a *App) notifyAway() bool { return a.desktopAway() || a.pushAway() }

// pushSend: Meldung ans Handy, im Hintergrund.
func (a *App) pushSend(title, body string) {
	if !a.pushAway() {
		return
	}
	cfg := a.pushCfg()
	title, body = trunc(agent.Redact(title), 120), trunc(agent.Redact(body), 400)
	key := title + "\x00" + body
	pushRecent.Lock()
	if time.Since(pushRecent.at[key]) < pushSame {
		pushRecent.Unlock()
		return
	}
	pushRecent.at[key] = time.Now()
	pushRecent.Unlock()
	urgent := strings.ContainsAny(title+body, "×◆") || containsAny(title+body, i18n.T("fehlgeschlagen"), i18n.T("Befund"), i18n.T("Zweifel"), i18n.T("wartet"))
	ev := a.events
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := pushSendFn(ctx, cfg, push.Message{Title: title, Body: body, Urgent: urgent}); err != nil && ev != nil {
			select {
			case ev <- pushErrMsg{err}:
			default:
			}
		}
	}()
}

func (a *App) pushErr(err error) {
	if time.Since(a.pushErrAt) < 10*time.Minute {
		return // nicht bei jeder Meldung erneut
	}
	a.pushErrAt = time.Now()
	a.pushToast(toastWarn, i18n.T("Handy-Meldung ging nicht raus: ")+err.Error())
}

func pushKindLabel(k string) string {
	switch k {
	case push.Ntfy:
		return "ntfy"
	case push.Gotify:
		return "Gotify"
	case push.Webhook:
		return "Webhook"
	}
	return i18n.T("aus")
}

// openPushSetup: Art wählen, dann Adresse und Token, dann Probemeldung.
func (a *App) openPushSetup() tea.Cmd {
	cur := a.st.Settings().Push
	var items []menuItem
	for _, k := range []string{push.Ntfy, push.Gotify, push.Webhook, ""} {
		hint := map[string]string{
			push.Ntfy:    i18n.T("App ntfy, eigener Server oder ntfy.sh"),
			push.Gotify:  i18n.T("eigener Gotify-Server"),
			push.Webhook: i18n.T("JSON an eine Adresse (Slack, Mattermost, Home Assistant …)"),
			"":           i18n.T("keine Handy-Meldungen"),
		}[k]
		label := pushKindLabel(k)
		if k == cur.Kind {
			label = "● " + label
		}
		items = append(items, menuItem{label: label, hint: hint, run: func() tea.Cmd {
			if k == "" {
				_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Push = domain.PushConfig{} })
				a.flash(i18n.T("Handy-Meldungen aus"))
				return nil
			}
			return a.askPushURL(k, cur)
		}})
	}
	return a.showMenu(i18n.T("Handy-Meldungen, wenn du nicht am Rechner bist"), items)
}

func (a *App) askPushURL(kind string, cur domain.PushConfig) tea.Cmd {
	val := ""
	if cur.Kind == kind {
		val = cur.URL
	} else if kind == push.Ntfy {
		val = push.RandomNtfyURL() // zufälliges Thema: errät niemand
	}
	body := map[string]string{
		push.Ntfy:    i18n.T("Adresse mit Thema, z. B. https://ntfy.sh/ein-langes-zufälliges-thema (das Thema wirkt wie ein Passwort). In der ntfy-App dasselbe Thema abonnieren."),
		push.Gotify:  i18n.T("Adresse des Gotify-Servers, z. B. https://gotify.example.org"),
		push.Webhook: i18n.T("Volle Adresse, an die sshTermy JSON schickt (title, message, text, urgent)."),
	}[kind]
	m := inputModal(i18n.T("Handy-Meldungen: ")+pushKindLabel(kind), body, val, func(u string) tea.Cmd {
		cfg := push.Config{Kind: kind, URL: u, Token: "x"}
		if err := cfg.Check(); err != nil {
			a.flashErr(err.Error())
			return a.askPushURL(kind, domain.PushConfig{Kind: kind, URL: u})
		}
		return a.askPushToken(kind, u, cur)
	})
	m.in.Placeholder = "https://"
	a.modal = m
	return m.in.Focus()
}

func (a *App) askPushToken(kind, u string, cur domain.PushConfig) tea.Cmd {
	body := i18n.T("Zugangstoken, falls dein ntfy-Server eins verlangt. Leer = ohne.")
	switch kind {
	case push.Gotify:
		body = i18n.T("App-Token aus Gotify (Apps › Anlegen).")
	case push.Webhook:
		body = i18n.T("Optional: wird als „Authorization: Bearer …“ mitgeschickt. Leer = ohne.")
	}
	if cur.Kind == kind && cur.Token.IsSet() {
		body += "\n" + i18n.T("Leer lassen behält das gespeicherte Token.")
	}
	m := inputModal(i18n.T("Handy-Meldungen: Token"), body, "", func(tok string) tea.Cmd {
		t := domain.Secret(tok)
		if tok == "" && cur.Kind == kind {
			t = cur.Token
		}
		cfg := push.Config{Kind: kind, URL: u, Token: string(t)}
		if err := cfg.Check(); err != nil {
			a.flashErr(err.Error())
			return nil
		}
		_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Push = domain.PushConfig{Kind: kind, URL: strings.TrimSpace(u), Token: t} })
		return a.pushTest()
	})
	m.in.EchoMode = textinput.EchoPassword
	m.in.Placeholder = ""
	a.modal = m
	return m.in.Focus()
}

type pushTestMsg struct{ err error }

// pushTest: Probemeldung, gleich nach dem Einrichten.
func (a *App) pushTest() tea.Cmd {
	if !a.pushOn() {
		a.flash(i18n.T("Handy-Meldungen sind aus"))
		return nil
	}
	cfg := a.pushCfg()
	a.flash(i18n.T("Probemeldung wird geschickt …"))
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return pushTestMsg{pushSendFn(ctx, cfg, push.Message{Title: "sshTermy", Body: i18n.T("So sieht eine Meldung aus, wenn ein langer Befehl fertig ist und du nicht am Rechner bist.")})}
	}
}

func (a *App) pushMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case pushTestMsg:
		if m.err != nil {
			a.flashErr(i18n.T("Probemeldung ging nicht raus: ") + m.err.Error())
		} else {
			a.flashOK(i18n.T("Probemeldung verschickt, schau aufs Handy"))
		}
		return nil, true
	case pushErrMsg:
		a.pushErr(m.err)
		return nil, true
	}
	return nil, false
}
