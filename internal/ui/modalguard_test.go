package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Ein Host-Key-Dialog, der während eines anderen Dialogs kommt, wartet und
// nimmt danach eine Sekunde lang keine Tasten an.
func TestAsyncModalQueuedAndGuarded(t *testing.T) {
	a := newPaletteTestApp(t)
	a.modal = textModal("x", []string{"y"})
	r := make(chan bool, 1)
	a.Update(hostKeyAskMsg{host: "h:22", fp: "SHA256:x", typ: "ssh-ed25519", reply: r})
	if a.modal.kind == mHostKey || len(a.asyncModals) != 1 {
		t.Fatal("Host-Key-Dialog hat den offenen Dialog ersetzt")
	}
	a.modal = nil
	a.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if a.modal == nil || a.modal.kind != mHostKey {
		t.Fatalf("wartender Dialog nicht gezeigt: %+v", a.modal)
	}
	a.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	select {
	case <-r:
		t.Fatal("j direkt nach dem Aufgehen hat den Host-Key angenommen")
	default:
	}
}
