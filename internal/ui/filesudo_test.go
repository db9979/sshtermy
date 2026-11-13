package ui

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/db9979/sshtermy/internal/sshx"
)

// Ohne SSH-Server (lokale Seite) oder ohne Rechtefehler gibt es keine
// sudo-Rückfrage, der Fehler bleibt wie bisher.
func TestOfferSudoOnlyRemote(t *testing.T) {
	a, s, _, _ := filesApp(t)
	denied := &fs.PathError{Op: "open", Path: "/opt/x", Err: fs.ErrPermission}
	if a.offerSudo(s, sshx.LocalFS{}, "x", denied, nil) || a.modal != nil {
		t.Fatal("lokaler Rechtefehler darf nicht nach sudo fragen")
	}
	if a.offerSudo(s, &sshx.RemoteFS{}, "x", errors.New("disk full"), nil) || a.modal != nil {
		t.Fatal("anderer Fehler darf nicht nach sudo fragen")
	}
	if got := errShort(denied); got != "permission denied" {
		t.Fatalf("errShort = %q", got)
	}
}
