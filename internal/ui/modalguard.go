package ui

import "time"

// Dialoge, die von selbst aufgehen (Host-Key, Passwort, MCP-Rückfrage),
// dürfen keinen Tastendruck übernehmen, der eigentlich dem Terminal galt:
// sonst bestätigte das „j“ aus „journalctl“ einen fremden Befehl oder das
// halb getippte Kommando landete als Passwort beim Server. Eine Sekunde nach
// dem Aufgehen nimmt der Dialog deshalb keine Tasten an. Ist schon ein
// Dialog offen, wartet der neue, statt ihn zu ersetzen.

const modalGuard = time.Second

// showAsync zeigt einen Dialog, den niemand gerade angefordert hat.
func (a *App) showAsync(m *modal) {
	if a.modal != nil {
		a.asyncModals = append(a.asyncModals, m)
		return
	}
	m.guardUntil = time.Now().Add(modalGuard)
	a.modal = m
}

// popAsyncModal: nächsten wartenden Dialog zeigen, sobald keiner offen ist.
func (a *App) popAsyncModal() {
	if a.modal != nil || len(a.asyncModals) == 0 {
		return
	}
	m := a.asyncModals[0]
	a.asyncModals = a.asyncModals[1:]
	m.guardUntil = time.Now().Add(modalGuard)
	a.modal = m
}

// modalGuarded: gerade erst aufgegangen, Taste verwerfen?
func (m *modal) guarded(now time.Time) bool { return now.Before(m.guardUntil) }
