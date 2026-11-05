package ui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"net/url"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/llm"
)

// KI-Status: ein kleiner Punkt zeigt, ob der eingestellte Endpunkt erreichbar
// ist und das Modell antwortet. Grün = in Ordnung, Gelb = langsam, Rot =
// Fehler, Grau = noch unbekannt oder nicht eingerichtet.
//
// Die Prüfung ist leicht: Jede echte Anfrage (Ghost-Text, Frage, Ziel-Modus)
// zählt mit. Nur wenn fünf Minuten lang nichts lief, schickt sshTermy selbst
// eine Chat-Anfrage mit einem einzigen Token. Nach einem Fehler wird öfter
// nachgesehen (30 s, 1, 2, dann 5 min), damit Grün schnell zurückkommt.

const (
	llmProbeEvery = 5 * time.Minute
	llmSlow       = 2 * time.Second // Reaktionszeit für ein Token
)

var llmRetry = []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute}

type llmProbeMsg struct{}

type llmHealthState struct {
	probing bool
	next    time.Time // nächste Prüfung frühestens
	fails   int       // Fehler in Folge (für den Abstand)
	sig     string    // Endpunkt+Modell der letzten Prüfung
	seen    uint64    // zuletzt verarbeitete Meldung
}

type llmLevel int

const (
	lhUnknown llmLevel = iota
	lhOff
	lhOK
	lhSlow
	lhError
)

// llmHealth fasst den Stand für die aktuelle Einstellung zusammen.
type llmHealth struct {
	level   llmLevel
	last    llm.Observation // letzte Meldung zu diesem Endpunkt/Modell
	latency time.Duration   // letzte gemessene Reaktionszeit (0 = keine)
	cfg     domain.LLMConfig
}

func llmSig(c domain.LLMConfig) string { return llm.BaseURL(c) + "|" + c.Model }

func (a *App) llmHealth() llmHealth {
	cfg := a.st.LLM()
	h := llmHealth{cfg: cfg}
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Model) == "" {
		h.level = lhOff
		return h
	}
	last, ok, _ := llm.LastHealth()
	match := func(o llm.Observation) bool {
		return !o.At.IsZero() && o.BaseURL == llm.BaseURL(cfg) && o.Model == cfg.Model
	}
	if match(ok) {
		h.latency = ok.Latency
	}
	if !match(last) {
		return h
	}
	h.last = last
	switch {
	case last.Err != nil:
		h.level = lhError
	case h.latency >= llmSlow:
		h.level = lhSlow
	default:
		h.level = lhOK
	}
	return h
}

func (a *App) llmColor(l llmLevel) color.Color {
	switch l {
	case lhOK:
		return a.s.T.OK
	case lhSlow:
		return a.s.T.Warn
	case lhError:
		return a.s.T.Danger
	}
	return a.s.T.Muted
}

func (h llmHealth) dot(probing bool) string {
	switch {
	case h.level == lhOff:
		return "○"
	case probing && h.level == lhUnknown:
		return "◌"
	}
	return "●"
}

func fmtLatency(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d ms", max(1, d.Milliseconds()))
	}
	return fmt.Sprintf("%.1f s", d.Seconds())
}

// llmBadgeText: kurzer Zustand hinter dem Modellnamen.
func (h llmHealth) badgeText() string {
	switch h.level {
	case lhError:
		return i18n.T("Fehler")
	case lhOK, lhSlow:
		if h.latency > 0 {
			return fmtLatency(h.latency)
		}
	}
	return ""
}

// llmChip: kompakte Anzeige für die Kopfzeile (außerhalb des Terminals).
func (a *App) llmChip() string {
	h := a.llmHealth()
	txt := a.s.Header.Foreground(a.llmColor(h.level)).Render(" "+h.dot(a.lh.probing)) +
		a.s.Header.Foreground(a.s.T.AI).Render(" "+i18n.T("KI"))
	switch {
	case h.level == lhOff:
		txt += a.s.Header.Foreground(a.s.T.Muted).Render(i18n.T(" aus"))
	case h.badgeText() != "":
		txt += a.s.Header.Foreground(a.llmColor(h.level)).Render(" " + h.badgeText())
	}
	txt += a.s.Header.Render(" ")
	return a.z.click(txt, func(tea.Mouse, bool) tea.Cmd { return a.openLLMStatus() })
}

// llmTick plant die Hintergrundprüfung (einmal pro Sekunde aufgerufen).
func (a *App) llmTick() tea.Cmd {
	if a.scr == scrUnlock || a.intro != nil {
		return nil
	}
	cfg := a.st.LLM()
	if cfg.BaseURL == "" || cfg.Model == "" {
		return nil
	}
	now := time.Now()
	last, _, n := llm.LastHealth()
	if n != a.lh.seen { // neue Meldung, auch von echten Anfragen
		a.lh.seen = n
		if last.BaseURL == llm.BaseURL(cfg) && last.Model == cfg.Model {
			d := llmProbeEvery
			if last.Err == nil {
				a.lh.fails = 0
			} else {
				if a.lh.fails < len(llmRetry) {
					d = llmRetry[a.lh.fails]
				}
				a.lh.fails++
			}
			a.lh.next = last.At.Add(d)
		}
	}
	if sig := llmSig(cfg); sig != a.lh.sig { // neu eingestellt: gleich prüfen
		a.lh.sig, a.lh.next, a.lh.fails = sig, now, 0
	}
	if a.lh.probing || now.Before(a.lh.next) {
		return nil
	}
	return a.llmProbeCmd()
}

func (a *App) llmProbeCmd() tea.Cmd {
	if a.lh.probing {
		return nil
	}
	cfg := a.st.LLM()
	a.lh.probing = true
	a.lh.next = time.Now().Add(llmProbeEvery) // wird von der Meldung neu gesetzt
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout(cfg))
		defer cancel()
		_ = llm.Probe(ctx, cfg)
		return llmProbeMsg{}
	}
}

// ---------------------------------------------------------------- Dialog

func (a *App) openLLMStatus() tea.Cmd {
	a.modal = &modal{kind: mLLM, title: i18n.T("KI-Status")}
	return nil
}

func (a *App) llmStatusKey(key string) tea.Cmd {
	switch key {
	case "p", "t":
		return a.llmProbeCmd()
	case "e":
		a.modal = nil
		a.returnToTerminal = a.scr == scrTerminal
		return a.openSettings()
	case "esc", "q", "enter":
		a.modal = nil
	}
	return nil
}

func (a *App) llmStatusView() string {
	s := a.s
	h := a.llmHealth()
	c := s.Text.Foreground(a.llmColor(h.level))
	var head string
	switch {
	case h.level == lhOff:
		head = c.Render(i18n.T("○ nicht eingerichtet")) + s.Muted.Render(i18n.T(" · Base-URL und Modell fehlen"))
	case a.lh.probing && h.level == lhUnknown:
		head = c.Render(i18n.T("◌ wird geprüft …"))
	case h.level == lhUnknown:
		head = c.Render(i18n.T("● noch nicht geprüft"))
	case h.level == lhOK:
		head = c.Render(i18n.T("● erreichbar, das Modell antwortet"))
	case h.level == lhSlow:
		head = c.Render(i18n.T("● erreichbar, aber langsam")) + s.Muted.Render(i18n.T(" · über ")+fmtLatency(llmSlow)+i18n.T(" für ein Token"))
	case h.level == lhError:
		head = c.Render(i18n.T("● Fehler: das Modell antwortet nicht"))
	}
	if a.lh.probing && h.level != lhUnknown {
		head += s.AI.Render(i18n.T("  ◌ prüfe …"))
	}
	row := func(k, v string) string { return s.Muted.Render(fmt.Sprintf("%-13s", k)) + v + "\n" }
	var b strings.Builder
	b.WriteString(head + "\n\n")
	b.WriteString(row(i18n.T("Endpunkt"), s.Text.Render(llm.BaseURL(h.cfg))))
	b.WriteString(row(i18n.T("Modell"), s.Text.Render(h.cfg.Model)))
	if h.latency > 0 && h.level != lhError {
		b.WriteString(row(i18n.T("Reaktion"), c.Render(fmtLatency(h.latency))+s.Muted.Render(i18n.T(" (ein Token, ab ")+fmtLatency(llmSlow)+i18n.T(" gelb)"))))
	}
	if !h.last.At.IsZero() {
		what := i18n.T("Anfrage")
		if h.last.Probe {
			what = i18n.T("Prüfung")
		}
		res := s.OK.Render("ok")
		if h.last.Err != nil {
			res = s.Danger.Render(i18n.T("fehlgeschlagen"))
		}
		b.WriteString(row(i18n.T("Zuletzt"), s.Text.Render(what+" "+ago(h.last.At)+" · ")+res))
	}
	if h.last.Err != nil {
		w := min(90, a.w-4) - 6 - 13 // Rahmen, Innenabstand, Spalte links
		b.WriteString(row(i18n.T("Meldung"), s.Danger.Render(trunc(llmErrText(h.last.Err), max(20, w)))))
		if hint := llmErrHint(h.last.Err.Error()); hint != "" {
			b.WriteString("\n" + s.Warn.Render(i18n.T("Was tun: ")) + s.Text.Render(hint) + "\n")
		}
	}
	if h.level != lhOff && !a.lh.probing && !a.lh.next.IsZero() {
		if d := time.Until(a.lh.next); d > 0 {
			b.WriteString("\n" + s.Muted.Render(i18n.T("Nächste Prüfung in ")+fmtWait(d)+i18n.T(", nur falls bis dahin keine echte Anfrage lief.")) + "\n")
		}
	}
	return s.Modal.Width(min(90, a.w-4)).Render(s.Accent.Render(i18n.T("✦ KI-Status")) + "\n\n" + strings.TrimRight(b.String(), "\n"))
}

func fmtWait(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d s", int(d.Seconds())+1)
	}
	return fmt.Sprintf(i18n.T("%d min"), int(d.Minutes()+0.5))
}

// llmErrText: Fehlermeldung ohne das „Post "http://…": “ davor, die URL
// steht schon in der Zeile Endpunkt.
func llmErrText(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		err = ue.Err
	}
	return oneLineUI(err.Error())
}

// llmErrHint übersetzt typische Fehler in einen nächsten Schritt.
func llmErrHint(e string) string {
	l := strings.ToLower(e)
	has := func(xs ...string) bool {
		for _, x := range xs {
			if strings.Contains(l, x) {
				return true
			}
		}
		return false
	}
	switch {
	case has("connection refused", "verbindung verweigert", "actively refused"):
		return i18n.T("Unter dieser Adresse lauscht nichts. Läuft der Server, und stimmt der Port?")
	case has("no such host", "server misbehaving", "unbekannter host"):
		return i18n.T("Der Rechnername ist nicht auflösbar. Adresse prüfen oder die IP eintragen.")
	case has("deadline exceeded", "timeout", "i/o timeout"):
		return i18n.T("Keine Antwort innerhalb des Timeouts. Ist der Server überlastet, lädt das Modell noch, oder blockt eine Firewall?")
	case has("network is unreachable", "no route to host"):
		return i18n.T("Das Netz zum Server fehlt. VPN oder WLAN prüfen.")
	case has("401", "403", "api-key", "unauthorized"):
		return i18n.T("Der API-Key fehlt oder ist falsch. In den Einstellungen (e) eintragen.")
	case has("model") && has("not found", "does not exist", "unknown", "not exist"):
		return i18n.T("Dieses Modell kennt der Server nicht. In den Einstellungen (e) mit Strg+L aus der Liste wählen.")
	case has("404"):
		return i18n.T("Der Pfad stimmt nicht. Die Base-URL endet meist auf /v1.")
	case has("x509", "certificate", "tls"):
		return i18n.T("Das TLS-Zertifikat wird nicht akzeptiert. Adresse (http/https) und Zertifikat prüfen.")
	case has("http 5", "502", "503", "500"):
		return i18n.T("Der Server meldet einen internen Fehler. Logs des LLM-Servers ansehen.")
	case has("unverschlüsselt", "unencrypted", "http://"):
		return i18n.T("Der API-Key würde unverschlüsselt gesendet. https verwenden oder in den Einstellungen bewusst erlauben.")
	case has("modellname fehlt", "model name missing", "base-url", "base url"):
		return i18n.T("Die Einstellungen sind unvollständig. Mit e öffnen.")
	}
	return ""
}
