package ui

import (
	"context"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/crash"
	"github.com/db9979/sshtermy/internal/i18n"
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
	mPaste  // Einfügeschutz (pasteguard.go)
	mMenu   // Kontextmenü, z. B. zu Links (links.go)
	mInfo   // Ergebnis anzeigen (Ping)
	mInput  // eine Zeile Text (Ordnername)
	mQuick  // Schnellbefehle (quick.go)
	mText   // blätterbarer Text (Config-Diff, Konfiguration)
	mLLM    // KI-Status (llmhealth.go)
	mDanger // Gefahren-Check beim Tippen (dangerguard.go)
	mCrash  // Absturzbericht der letzten Sitzung (crash.go)
	mLayout // Auswahl der Aufteilung (layout.go)
)

type modal struct {
	kind     modalKind
	copy     string // mText: c kopiert diesen Text
	title    string
	body     string
	hk       hostKeyAskMsg
	sec      secretAskMsg
	in       textinput.Model
	onYes    func() tea.Cmd
	onText   func(string) tea.Cmd // nur Texteingabe
	onCancel func() tea.Cmd       // Texteingabe mit Esc verlassen (optional)
	ctx      helpCtx              // nur Hilfe: aktueller Bereich
	helpTab  int                  // nur Hilfe: gewählter Reiter (helpTabs)
	filter   string               // nur Hilfe: getippter Suchtext über alle Reiter
	scroll   int                  // Hilfe/Changelog/Text: Scroll-Position; Schnellbefehle: Auswahl
	quickSID int                  // nur Schnellbefehle: Sitzung
	armed    int                  // nur Schnellbefehle: roter Befehl nach dem ersten ⏎
	lines    []string             // Changelog/Text: fertig gestaltete Zeilen
	styled   bool                 // Text: Zeilen tragen schon Farben (ANSI)

	paste    *pasteCheck // nur Einfügeschutz
	danger   *dangerAsk  // nur Gefahren-Check
	items    []menuItem  // nur Menü
	sel      int         // nur Menü: Auswahl
	at       [2]int      // nur Menü: Bildschirmposition (Mauszeiger)
	anchored bool        // am Mauszeiger statt mittig
	hints    [][2]string // Tastenhinweise (setzt modalView aus modalKeys)

	crash *crash.Report // nur Absturzbericht
	recap bool          // Menü ist der Rückblick des Begleiters (companion.go)
	quit  bool          // Rückfrage beim Beenden (quitask.go)

	guardUntil time.Time // von selbst aufgegangen: bis dahin keine Tasten (modalguard.go)
	twice      bool      // Bestätigen: Gefahr, j zweimal
	yesArmed   bool      // erstes j bei twice gedrückt
}

func hostKeyModal(m hostKeyAskMsg) *modal {
	return &modal{kind: mHostKey, title: i18n.T("Unbekannter Host"), hk: m,
		body: i18n.T("Der Server ") + m.host + i18n.T(" ist noch nicht bekannt.\n\n") +
			i18n.T("Schlüssel: ") + m.typ + i18n.T("\nFingerprint: ") + m.fp + "\n\n" +
			i18n.T("Vergleiche den Fingerprint mit dem Server (ssh-keygen -lf /etc/ssh/ssh_host_*_key.pub).")}
}

func secretModal(m secretAskMsg) *modal {
	in := newInput("", true)
	in.Focus()
	return &modal{kind: mSecret, title: i18n.T("Eingabe nötig"), body: m.label, sec: m, in: in}
}

func confirmModal(title, body string, yes func() tea.Cmd) *modal {
	return &modal{kind: mConfirm, title: title, body: body, onYes: yes}
}

type applyDoneMsg struct{ err error }

func (a *App) aboutModal() *modal {
	b := version.Long() + "\n\n" +
		i18n.T("Sicherheit: AES-256-GCM, Argon2id, DPAPI (Windows), Host-Key-Prüfung (TOFU),\n") +
		i18n.T("KI ohne Werkzeuge: jeder Befehl braucht deine Bestätigung.\n\n") +
		i18n.T("♥ Gefällt dir sshTermy? Spendier einen Kaffee: ") + version.Coffee + "\n\n"
	if a.rel != nil {
		b += i18n.T("↑ Neue Version v") + a.rel.Version + i18n.T(" verfügbar.\n") + firstLines(a.rel.Notes, 6)
		if cmd := update.Managed(); cmd != "" {
			b += "\n\n" + i18n.T("Aktualisieren mit: ") + cmd
		} else if !update.Signed() {
			b += "\n\n" + unsignedNote()
		}
	} else if a.restartPending != "" {
		b += "v" + a.restartPending + i18n.T(" ist installiert.")
	}
	b = strings.TrimRight(b, "\n")
	return &modal{kind: mAbout, title: i18n.T("Über ") + version.Name, body: b}
}

// unsignedNote: Hinweis für Builds ohne eingebauten Update-Schlüssel.
func unsignedNote() string {
	return i18n.T("▲ Dieser Build prüft die Signatur von Updates nicht (selbst gebaut, ohne UPDATE_PUBKEY).\n") +
		i18n.T("  Geprüft wird nur die Prüfsumme; wer das GitHub-Konto übernimmt, könnte ein Update unterschieben.\n") +
		i18n.T("  Sicherer: die offizielle Version installieren (github.com/db9979/sshtermy/releases).")
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
	if m.guarded(time.Now()) {
		return nil // gerade aufgegangen: die Taste galt noch etwas anderem
	}
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
			if m.twice && !m.yesArmed {
				m.yesArmed = true
				m.body += "\n\n" + i18n.T("▲ Gefahr: zum Ausführen nochmal j drücken")
				return nil
			}
			a.modal = nil
			if m.onYes != nil {
				return m.onYes()
			}
		case -1:
			a.modal = nil
			if m.onCancel != nil {
				return m.onCancel()
			}
		}
	case mAbout:
		switch key {
		case "u":
			if a.rel != nil && update.Managed() == "" {
				rel := a.rel
				apply := func() tea.Cmd {
					a.modal = &modal{kind: mBusy, title: "Update", body: i18n.T("Lade v") + rel.Version + i18n.T(", prüfe Signatur und Prüfsumme …")}
					return func() tea.Msg { return applyDoneMsg{update.Apply(context.Background(), rel)} }
				}
				if !update.Signed() {
					a.modal = confirmModal(i18n.T("Update ohne Signaturprüfung?"), unsignedNote()+"\n\n"+
						i18n.T("j = trotzdem installieren · n = abbrechen"), apply)
					return nil
				}
				return apply()
			}
		case "c":
			a.modal = nil
			a.flash(i18n.T("Suche nach Updates …"))
			return a.checkUpdate()
		case "r":
			if a.restartPending != "" {
				return a.askRestart()
			}
		case "w":
			a.modal = a.changelogModal(nil)
		case "b":
			return a.openURL(version.Coffee)
		case "l":
			cmd := a.toggleLang()
			a.modal = a.aboutModal()
			return cmd
		case "esc", "q", "enter":
			a.modal = nil
		}
	case mInput:
		return a.inputKey(k)
	case mPaste:
		return a.pasteKey(key)
	case mDanger:
		return a.dangerKey(k)
	case mMenu:
		return a.menuKey(key)
	case mLLM:
		return a.llmStatusKey(key)
	case mLayout:
		return a.layoutKey(key)
	case mCrash:
		return a.crashKey(key)
	case mInfo:
		if key == "esc" || key == "enter" || key == "q" {
			a.modal = nil
		}
	case mHelp:
		if m.helpKey(key) {
			a.modal = nil
		}
	case mChangelog:
		if key == "b" {
			return a.openURL(version.Coffee)
		}
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
	d := dialog{title: m.title, keys: m.hints, width: dialogInner(w)}
	switch m.kind {
	case mHostKey, mCrash:
		d.icon, d.tone = "▲", s.T.Warn
	case mConfirm:
		d.icon = "?"
	case mAbout:
		d.icon = "◆"
	case mBusy:
		d.icon, d.tone, d.noClose = "◌", s.T.AI, true
	case mInfo:
		d.icon = "i"
	case mInput:
		d.icon = "»"
	case mSecret:
		d.icon = "*"
	}
	body := ansi.Wrap(s.Text.Render(m.body), d.width, "")
	switch m.kind {
	case mInput:
		body += "\n\n" + m.in.View()
	case mSecret:
		body += "\n\n" + m.in.View() + "\n\n" + s.Muted.Render(i18n.T("Wird nicht gespeichert."))
	}
	d.body = body
	if m.kind != mAbout && m.kind != mHostKey && m.kind != mInput && m.kind != mSecret {
		d.width = 0 // kurze Dialoge nur so breit wie nötig
	}
	return s.dialog(d, w)
}

// modalView: Dialoge, die das App-Objekt brauchen (Zonen, Breite), sonst view.
func (a *App) modalView() string {
	switch a.modal.kind {
	case mPaste:
		return a.pasteView(a.modal)
	case mDanger:
		return a.dangerView(a.modal)
	case mMenu:
		return a.menuView(a.modal)
	case mQuick:
		return a.quickModalView(a.modal)
	case mLLM:
		return a.llmStatusView()
	case mLayout:
		return a.layoutView()
	}
	a.modal.hints = a.modalKeys()
	return a.modal.view(a.s, a.w, a.h)
}
