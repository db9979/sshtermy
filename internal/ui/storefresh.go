package ui

import (
	"sync/atomic"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/update"
)

// Zwei sshTermy auf demselben Tresor (zweites Fenster, zweiter Start): der
// Store lädt neu, sobald das andere Programm gespeichert hat (store.Fresh).
// Hier wird das im Sekundentakt angestoßen und einmal pro Minute gemeldet,
// damit niemand sich wundert, warum eine Verbindung plötzlich da ist.

const freshToastEvery = time.Minute

type storeFresh struct {
	reloaded atomic.Bool // OnReload kann aus jedem Läufer kommen
	toastAt  time.Time

	saveErr   atomic.Pointer[error] // letzter Fehler beim Speichern (OnSaveError)
	saveErrAt time.Time

	ticks     int  // Sekunden seit dem Start (für updateTick)
	confirmed bool // Update als funktionierend gemeldet
}

// confirmAfter: läuft die Oberfläche so viele Sekunden, gilt ein frisch
// installiertes Update als funktionierend (update/guard.go).
const confirmAfter = 20

// Variablen für Tests: die echten fassen Dateien neben der Programmdatei an.
var (
	confirmUpdate    = update.Confirm
	takeRollbackNote = update.TakeRollbackNote
)

// updateTick: beim ersten Takt melden, falls ein Update zurückgenommen
// wurde; nach confirmAfter Sekunden das laufende Update bestätigen.
func (a *App) updateTick() {
	f := &a.fresh
	f.ticks++
	if f.ticks == 1 {
		if v := takeRollbackNote(); v != "" {
			a.pushToast(toastWarn, i18n.F("Das Update auf v%s ist zweimal nicht sauber gestartet und wurde zurückgenommen. Es wird nicht erneut angeboten; die nächste Version kommt wie gewohnt.", v))
		}
	}
	if !f.confirmed && f.ticks >= confirmAfter {
		f.confirmed = true
		go confirmUpdate()
	}
}

func (a *App) hookFresh() {
	a.st.OnReload = func() { a.fresh.reloaded.Store(true) }
	a.st.OnSaveError = func(err error) { a.fresh.saveErr.Store(&err) }
}

// saveErrTick meldet einen Fehler beim Speichern, auch wenn die Stelle, die
// gespeichert hat, ihn verworfen hat. Höchstens einmal pro halbe Minute.
func (a *App) saveErrTick() {
	p := a.fresh.saveErr.Swap(nil)
	if p == nil || time.Since(a.fresh.saveErrAt) < 30*time.Second {
		return
	}
	a.fresh.saveErrAt = time.Now()
	a.pushToast(toastErr, i18n.F("Speichern fehlgeschlagen: %s. Die Änderung gilt nur, bis sshTermy beendet wird.", (*p).Error()))
}

func (a *App) freshTick() {
	a.saveErrTick()
	a.updateTick()
	a.st.Fresh()
	if !a.fresh.reloaded.Swap(false) {
		return
	}
	if time.Since(a.fresh.toastAt) < freshToastEvery {
		return
	}
	a.fresh.toastAt = time.Now()
	a.pushToast(toastInfo, i18n.T("Ein anderes sshTermy hat gespeichert: Verbindungen und Einstellungen wurden neu geladen."))
}
