package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Eingabe-Protokoll zur Fehlersuche (z. B. Tasten, die unter Windows nicht
// ankommen). Nur mit SSHTERMY_DEBUG=1; Datei debug-input.log im
// sshTermy-Ordner. Getippte Zeichen werden in Passwort-Dialogen und beim
// Entsperren nicht protokolliert.
func openInputLog(dir string) *os.File {
	if os.Getenv("SSHTERMY_DEBUG") == "" || dir == "" {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, "debug-input.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil
	}
	fmt.Fprintf(f, "---- Start %s\n", time.Now().Format(time.RFC3339))
	return f
}

func (a *App) logInput(msg tea.Msg) {
	if a.inputLog == nil {
		return
	}
	var line string
	secret := a.scr == scrUnlock || (a.modal != nil && a.modal.kind == mSecret)
	switch m := msg.(type) {
	case tea.KeyPressMsg:
		if secret && m.Text != "" {
			line = "key [verborgen]"
		} else {
			line = fmt.Sprintf("key %q code=%U base=%U shifted=%U text=%q mod=%d", m.String(), m.Code, m.BaseCode, m.ShiftedCode, m.Text, m.Mod)
		}
	case tea.MouseClickMsg:
		line = fmt.Sprintf("click %+v", m.Mouse())
	case tea.MouseReleaseMsg:
		line = fmt.Sprintf("release %+v", m.Mouse())
	case tea.PasteMsg:
		line = fmt.Sprintf("paste len=%d", len(m.Content))
	case hostKeyAskMsg:
		line = "hostkey-frage " + m.host
	case secretAskMsg:
		line = "secret-frage"
	default:
		return
	}
	modal := "-"
	if a.modal != nil {
		modal = fmt.Sprint(a.modal.kind)
	}
	fmt.Fprintf(a.inputLog, "%s scr=%d modal=%s %s\n", time.Now().Format("15:04:05.000"), a.scr, modal, line)
}
