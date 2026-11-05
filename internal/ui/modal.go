package ui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/update"
	"github.com/db9979/sshtermy/internal/version"
)

type modalKind int

const (
	mHostKey modalKind = iota
	mSecret
	mConfirm
	mAbout
	mHelp
	mBusy
)

type modal struct {
	kind  modalKind
	title string
	body  string
	hk    hostKeyAskMsg
	sec   secretAskMsg
	in    textinput.Model
	onYes func() tea.Cmd
}

func hostKeyModal(m hostKeyAskMsg) *modal {
	return &modal{kind: mHostKey, title: "Unbekannter Host", hk: m,
		body: "Der Server " + m.host + " ist noch nicht bekannt.\n\n" +
			"Schlüssel: " + m.typ + "\nFingerprint: " + m.fp + "\n\n" +
			"Vergleiche den Fingerprint mit dem Server (ssh-keygen -lf /etc/ssh/ssh_host_*_key.pub).\n" +
			"j = vertrauen und merken · n = abbrechen"}
}

func secretModal(m secretAskMsg) *modal {
	in := newInput("", true)
	in.Focus()
	return &modal{kind: mSecret, title: "Eingabe nötig", body: m.label, sec: m, in: in}
}

func confirmModal(title, body string, yes func() tea.Cmd) *modal {
	return &modal{kind: mConfirm, title: title, body: body + "\n\nj = ja · n = nein", onYes: yes}
}

type applyDoneMsg struct{ err error }

func (a *App) aboutModal() *modal {
	b := version.Long() + "\n\n" +
		"Sicherheit: AES-256-GCM, Argon2id, DPAPI (Windows), Host-Key-Prüfung (TOFU),\n" +
		"KI ohne Werkzeuge: jeder Befehl braucht deine Bestätigung.\n\n"
	if a.rel != nil {
		b += "⬆ Neue Version v" + a.rel.Version + " verfügbar.\n" + firstLines(a.rel.Notes, 6) + "\n\nu = jetzt aktualisieren · "
	} else {
		b += "c = nach Updates suchen · "
	}
	b += "Esc = schließen"
	return &modal{kind: mAbout, title: "Über " + version.Name, body: b}
}

func helpModal() *modal {
	return &modal{kind: mHelp, title: "Tastenkürzel", body: strings.TrimSpace(`
Manager      ⏎ verbinden · n neu · e bearbeiten · d löschen · / suchen · s KI-Einstellungen
KI-Einst.    Strg+T Endpunkt testen · Strg+L Modell aus der Liste wählen
Dateien (F5) Tab Seite · c kopieren (hoch/runter) · n Ordner · d löschen · . versteckte
Terminal     Tab/→ Ghost-Text übernehmen · Alt+1…4 Folgebefehl einfügen
             F2 Ziel-Modus (Agent) · F3 KI fragen · F4 Tunnel · F5 Dateien · F7/F8 Tabs · F10 Manager
Ziel-Modus   ↑↓ Runbook wählen · Strg+S als Runbook speichern · Strg+D Runbook löschen
             ⏎ ausführen · i nur einfügen (Dry-Run) · e bearbeiten · s überspringen
             r neu vorschlagen · x Ziel beenden · Esc zurück ins Terminal
Rote Befehle (löschen, neu starten, Firewall …) brauchen zweimal ⏎.
Palette      Strg+Leertaste: Ziel, Frage, Vorschläge, Tunnel, Verbindungen, Aktionen
             ⏎ ausführen · Tab KI fragen · ↑↓ wählen · Esc schließen
Überall      F1 Hilfe · F12 Über/Update · Strg+Q beenden`) + "\n" + mouseHelp}
}

func firstLines(s string, n int) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	if len(l) > n {
		l = append(l[:n], "…")
	}
	return strings.Join(l, "\n")
}

func (a *App) modalKey(k tea.KeyPressMsg) tea.Cmd {
	m := a.modal
	key := k.String()
	switch m.kind {
	case mHostKey:
		if key == "j" || key == "y" {
			m.hk.reply <- true
			a.modal = nil
		} else if key == "n" || key == "esc" {
			m.hk.reply <- false
			a.modal = nil
		}
		return nil
	case mSecret:
		switch key {
		case "enter":
			m.sec.reply <- m.in.Value()
			a.modal = nil
			return nil
		case "esc":
			m.sec.reply <- "\x00cancel"
			a.modal = nil
			return nil
		}
		var cmd tea.Cmd
		m.in, cmd = m.in.Update(k)
		return cmd
	case mConfirm:
		if key == "j" || key == "y" {
			a.modal = nil
			if m.onYes != nil {
				return m.onYes()
			}
		} else if key == "n" || key == "esc" {
			a.modal = nil
		}
	case mAbout:
		switch key {
		case "u":
			if a.rel != nil {
				rel := a.rel
				a.modal = &modal{kind: mBusy, title: "Update", body: "Lade v" + rel.Version + ", prüfe Signatur und Prüfsumme …"}
				return func() tea.Msg { return applyDoneMsg{update.Apply(context.Background(), rel)} }
			}
		case "c":
			a.modal = nil
			a.flash("Suche nach Updates …")
			return checkUpdate()
		case "esc", "q", "enter":
			a.modal = nil
		}
	case mHelp:
		a.modal = nil
	}
	return nil
}

func (m *modal) view(s Styles, w int) string {
	title := s.Accent.Render(m.title)
	if m.kind == mHostKey {
		title = s.Warn.Render("⚠ " + m.title)
	}
	body := s.Text.Render(m.body)
	if m.kind == mSecret {
		body += "\n\n" + m.in.View() + "\n\n" + s.Muted.Render("⏎ senden · Esc abbrechen (wird nicht gespeichert)")
	}
	return s.Modal.Width(min(90, w-4)).Render(title + "\n\n" + body)
}
