package ui

import (
	"context"
	"strings"
	"unicode"

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
	mChangelog
	mTour
	mPaste // Einfügeschutz (pasteguard.go)
	mMenu  // Kontextmenü, z. B. zu Links (links.go)
	mInfo  // Ergebnis anzeigen (Ping)
	mInput // eine Zeile Text (Ordnername)
	mQuick // Schnellbefehle (quick.go)
	mText  // blätterbarer Text (Config-Diff, Konfiguration)
	mLLM   // KI-Status (llmhealth.go)
)

type modal struct {
	kind     modalKind
	title    string
	body     string
	hk       hostKeyAskMsg
	sec      secretAskMsg
	in       textinput.Model
	onYes    func() tea.Cmd
	onText   func(string) tea.Cmd // nur Texteingabe
	ctx      helpCtx              // nur Hilfe: aktueller Bereich
	scroll   int                  // Hilfe/Changelog/Text: Scroll-Position; Schnellbefehle: Auswahl
	quickSID int                  // nur Schnellbefehle: Sitzung
	armed    int                  // nur Schnellbefehle: roter Befehl nach dem ersten ⏎
	lines    []string             // Changelog/Text: fertig gestaltete Zeilen

	paste    *pasteCheck // nur Einfügeschutz
	items    []menuItem  // nur Menü
	sel      int         // nur Menü: Auswahl
	at       [2]int      // nur Menü: Bildschirmposition (Mauszeiger)
	anchored bool        // am Mauszeiger statt mittig
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
	} else if a.restartPending != "" {
		b += "v" + a.restartPending + " ist installiert.\n\nr = jetzt neu starten · "
	} else {
		b += "c = nach Updates suchen · "
	}
	b += "w = was ist neu · Esc = schließen"
	return &modal{kind: mAbout, title: "Über " + version.Name, body: b}
}

func firstLines(s string, n int) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	if len(l) > n {
		l = append(l[:n], "…")
	}
	return strings.Join(l, "\n")
}

// yesNo: 1 = ja (j/y), -1 = nein (n/Esc), 0 = andere Taste. Groß/klein egal
// (Feststelltaste) und unabhängig davon, ob das Terminal Text mitliefert
// (Windows-Konsole liefert je nach Modus nur Code/BaseCode).
func yesNo(k tea.KeyPressMsg) int {
	if k.Code == tea.KeyEscape {
		return -1
	}
	if k.Mod&^(tea.ModShift|tea.ModCapsLock|tea.ModNumLock) != 0 {
		return 0
	}
	rs := []rune{k.Code, k.ShiftedCode, firstRune(k.Text)}
	if k.Code == 0 {
		rs = append(rs, k.BaseCode) // Windows-VT-Eingabe: Zeichen nur in BaseCode
	}
	// BaseCode sonst nicht: das ist die US-Tastenposition (deutsches „z“ wäre „y“).
	for _, r := range rs {
		switch unicode.ToLower(r) {
		case 'j', 'y':
			return 1
		case 'n':
			return -1
		}
	}
	return 0
}

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

func (a *App) modalKey(k tea.KeyPressMsg) tea.Cmd {
	m := a.modal
	key := k.String()
	switch m.kind {
	case mHostKey:
		switch yesNo(k) {
		case 1:
			m.hk.reply <- true
			a.modal = nil
		case -1:
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
		switch yesNo(k) {
		case 1:
			a.modal = nil
			if m.onYes != nil {
				return m.onYes()
			}
		case -1:
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
		case "r":
			if a.restartPending != "" {
				return a.askRestart()
			}
		case "w":
			a.modal = a.changelogModal(nil)
		case "esc", "q", "enter":
			a.modal = nil
		}
	case mInput:
		return a.inputKey(k)
	case mPaste:
		return a.pasteKey(key)
	case mMenu:
		return a.menuKey(key)
	case mLLM:
		return a.llmStatusKey(key)
	case mInfo:
		if key == "esc" || key == "enter" || key == "q" {
			a.modal = nil
		}
	case mHelp:
		if m.helpKey(key) {
			a.modal = nil
		}
	case mChangelog:
		if m.changelogKey(key, a.h) {
			a.modal = nil
		}
	case mTour:
		return a.tourKey(key)
	case mQuick:
		return a.quickModalKey(key)
	case mText:
		if m.textKey(key) {
			a.modal = nil
		}
	}
	return nil
}

func (m *modal) view(s Styles, w, h int) string {
	if m.kind == mHelp {
		return m.helpView(s, w, h)
	}
	if m.kind == mChangelog {
		return m.changelogView(s, w, h)
	}
	if m.kind == mTour {
		return m.tourView(s, w)
	}
	if m.kind == mText {
		return m.textView(s, w, h)
	}
	title := s.Accent.Render(m.title)
	if m.kind == mHostKey {
		title = s.Warn.Render("⚠ " + m.title)
	}
	body := s.Text.Render(m.body)
	if m.kind == mInput {
		body += "\n\n" + m.in.View() + "\n\n" + s.Muted.Render("⏎ übernehmen · Esc abbrechen")
	}
	if m.kind == mSecret {
		body += "\n\n" + m.in.View() + "\n\n" + s.Muted.Render("⏎ senden · Esc abbrechen (wird nicht gespeichert)")
	}
	return s.Modal.Width(min(90, w-4)).Render(title + "\n\n" + body)
}

// modalView: Dialoge, die das App-Objekt brauchen (Zonen, Breite), sonst view.
func (a *App) modalView() string {
	switch a.modal.kind {
	case mPaste:
		return a.pasteView(a.modal)
	case mMenu:
		return a.menuView(a.modal)
	case mQuick:
		return a.quickModalView(a.modal)
	case mLLM:
		return a.llmStatusView()
	}
	return a.modal.view(a.s, a.w, a.h)
}
