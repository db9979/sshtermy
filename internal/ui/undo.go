package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/store"
)

// Rückgängig statt Rückfrage: Löschen geht sofort, ein Hinweis bietet
// 10 Sekunden lang „Rückgängig“ an (Strg+Z im Manager, Klick auf den
// Hinweis). Erst danach wird endgültig aufgeräumt (z. B. der Eintrag im
// Schlüsselbund des Betriebssystems).

const undoLife = 10 * time.Second

type undoState struct {
	trash    *store.Trash
	toast    string
	until    time.Time
	finalize func() // nach Ablauf (darf nil sein)
	restored func() // nach dem Wiederherstellen (darf nil sein)
}

// offerUndo merkt sich ein Löschen und zeigt den Hinweis.
func (a *App) offerUndo(what string, tr *store.Trash, finalize, restored func()) {
	a.finishUndo()
	text := what + " · " + a.keys.label(actUndo) + i18n.T(" oder Klick: rückgängig")
	now := time.Now()
	a.undo = &undoState{trash: tr, toast: text, until: now.Add(undoLife), finalize: finalize, restored: restored}
	a.toasts = append(a.toasts, toastItem{text: text, kind: toastWarn, born: now, until: now.Add(undoLife), undo: true})
	if len(a.toasts) > maxToasts {
		a.toasts = a.toasts[len(a.toasts)-maxToasts:]
	}
}

// finishUndo macht ein offenes Löschen endgültig.
func (a *App) finishUndo() {
	u := a.undo
	if u == nil {
		return
	}
	a.undo = nil
	a.dropToast(u.toast)
	if u.finalize != nil {
		u.finalize()
	}
}

// expireUndo: nach Ablauf endgültig (aus dem Takt der Toasts).
func (a *App) expireUndo(now time.Time) {
	if a.undo != nil && now.After(a.undo.until) {
		a.finishUndo()
	}
}

// doUndo stellt das zuletzt Gelöschte wieder her.
func (a *App) doUndo() tea.Cmd {
	u := a.undo
	if u == nil {
		a.flash(i18n.T("Nichts rückgängig zu machen"))
		return nil
	}
	a.undo = nil
	a.dropToast(u.toast)
	if err := a.st.Restore(u.trash); err != nil {
		a.flashErr(i18n.T("Rückgängig fehlgeschlagen: ") + err.Error())
		return nil
	}
	if u.restored != nil {
		u.restored()
	}
	a.flashOK(i18n.T("Wiederhergestellt"))
	return nil
}
