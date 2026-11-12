package ui

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/relay"
	"github.com/db9979/sshtermy/internal/sharewire"
)

// Zusehen im Browser: Einladung mit Browser-Link, Anklopfen „im Browser“,
// abgeschaltet = Browser-Zuschauer werden abgelehnt.
func TestShareBrowser(t *testing.T) {
	srv, err := relay.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv.Logf, srv.Open = t.Logf, true
	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	fp := relay.CertFP(ts.Certificate().Raw)

	a := dangerApp(t)
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Share.Public = ts.URL + "#fp=" + fp })

	knock := func(s *session, o *shareOut) error {
		inv := o.hs.Invite()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		g, err := relay.JoinShare(ctx, inv.Base, inv.FP, inv.Platz)
		if err != nil {
			t.Fatal(err)
		}
		res := make(chan error, 1)
		go func() {
			_, err := sharewire.Join(ctx, g, inv, sharewire.Hello{Name: "Anna", Rechner: "Firefox · Windows", Version: "Browser", Browser: true}, nil)
			res <- err
		}()
		deadline := time.After(10 * time.Second)
		for {
			select {
			case err := <-res:
				return err
			case m := <-a.events:
				a.shareMsg(m)
				if a.modal != nil && a.modal.kind == mConfirm {
					if !strings.Contains(a.modal.body, "im Browser") || !strings.Contains(a.modal.body, "Firefox") {
						t.Fatalf("Anklopfen ohne Browser-Hinweis: %q", a.modal.body)
					}
					a.modal.onYes()
					a.modal = nil
				}
			case <-deadline:
				t.Fatal("kein Ergebnis beim Anklopfen")
			}
		}
	}

	// angeboten: Link /zusehen/#CODE in Zwischenablage, QR, E-Mail; Warnung beim öffentlichen Vermittler
	s, _ := typedSession(domain.Connection{Alias: "web"}, "$ ", "")
	s.shell = &shareShell{}
	a.sessions = append(a.sessions, s)
	a.shareMsg(a.startShareWith(s, shareOpts{way: "public", knock: true, hidePw: true, browser: true})())
	o := s.share.Load()
	if o == nil || a.modal == nil {
		t.Fatal("nicht geteilt")
	}
	code := o.hs.Invite().Code()
	if want := ts.URL + "/zusehen/#" + code; a.modal.copy != want {
		t.Fatalf("Browser-Link: %q, erwartet %q", a.modal.copy, want)
	}
	all := strings.Join(a.modal.lines, "\n")
	if !strings.Contains(all, "öffentlichen Vermittler") || !strings.Contains(all, "Zertifikat") || !strings.Contains(all, "sshtermy://zusehen/") {
		t.Fatalf("Einladung: %s", all)
	}
	mu, _ := url.Parse(a.modal.mail)
	if body := mu.Query().Get("body"); !strings.Contains(body, "/zusehen/#"+code) || !strings.Contains(body, "Mit sshTermy:") {
		t.Fatalf("E-Mail ohne Browser-Link: %q", body)
	}
	// sshTermy liest den Code auch aus dem Browser-Link
	if inv, err := relay.ParseInvite("Schau: " + a.modal.copy); err != nil || inv.Code() != code || inv.Base != "" {
		t.Fatalf("Browser-Link in sshTermy: %+v %v", inv, err)
	}
	a.modal = nil
	if err := knock(s, o); err != nil {
		t.Fatalf("Browser-Zuschauer nicht zugelassen: %v", err)
	}
	a.stopShare(s)

	// abgeschaltet: kein Browser-Link, Browser-Zuschauer abgelehnt (ohne Rückfrage)
	s2, _ := typedSession(domain.Connection{Alias: "db"}, "$ ", "")
	s2.shell = &shareShell{}
	s2.id = s.id + 1
	a.sessions = append(a.sessions, s2)
	a.shareMsg(a.startShareWith(s2, shareOpts{way: "public", knock: true, hidePw: true})())
	o2 := s2.share.Load()
	if o2 == nil {
		t.Fatalf("nicht geteilt: %+v", a.toasts)
	}
	if a.modal == nil || strings.Contains(a.modal.copy, "/zusehen/#") {
		t.Fatalf("Browser-Link trotz „aus“: %q", a.modal.copy)
	}
	a.modal = nil
	if err := knock(s2, o2); !errors.Is(err, relay.ErrDenied) {
		t.Fatalf("Browser-Zuschauer hätte abgelehnt werden müssen: %v", err)
	}
	a.stopShare(s2)
}
