package ui

import (
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

// Syslog kommt nur von Geräten an, die als Verbindung gespeichert sind;
// fremde Absender werden gezählt und verworfen, außer der Schalter ist an.
func TestSyslogOnlyKnownSenders(t *testing.T) {
	a := dangerApp(t)
	if _, err := a.st.UpsertConnection(domain.Connection{Alias: "core-sw", Host: "10.0.0.7", Username: "admin", Auth: domain.AuthAgent}); err != nil {
		t.Fatal(err)
	}
	if !a.syslogAccepted(watchItem{addr: "10.0.0.7"}) {
		t.Fatal("bekannter Absender verworfen")
	}
	for range 3 {
		if a.syslogAccepted(watchItem{addr: "10.9.9.9"}) {
			t.Fatal("fremder Absender angenommen")
		}
	}
	if a.lwatch.dropped["10.9.9.9"] != 3 {
		t.Fatalf("nicht gezählt: %v", a.lwatch.dropped)
	}
	a.toggleSyslogAny()
	if !a.syslogAccepted(watchItem{addr: "10.9.9.9"}) {
		t.Fatal("Schalter „alle Absender“ wirkt nicht")
	}
}
