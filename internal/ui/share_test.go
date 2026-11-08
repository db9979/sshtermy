package ui

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/relay"
)

func TestShareAndWatch(t *testing.T) {
	dir := t.TempDir()
	srv, err := relay.NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv.Logf = t.Logf
	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	fp := relay.CertFP(ts.Certificate().Raw)
	st, _ := relay.LoadState(dir)
	st.Name, st.URL, st.FP = "Test", ts.URL, fp
	_ = relay.SaveState(dir, st)
	line, _ := relay.Line(dir, relay.CodeClient)
	pl, _ := relay.ParseLine(line)
	cl, _ := relay.NewClient(pl.Base, "", pl.FP)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pr, err := cl.Pair(ctx, pl.Code, "laptop")
	if err != nil {
		t.Fatal(err)
	}

	a := dangerApp(t)
	r, err := a.st.UpsertRelay(domain.Relay{Name: "Test", Kind: relay.KindSSHTermy, Base: pl.Base, FP: pl.FP, Key: domain.Secret(pr.Key)})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := typedSession(domain.Connection{Alias: "web"}, "$ ", "uptime")
	a.sessions = append(a.sessions, s)

	// Gastgeber: Freigabe anlegen
	a.shareMsg(a.startShareVia(s, r.ID)())
	o := s.share.Load()
	if o == nil || !strings.Contains(o.link, "#teilen=") || a.modal == nil {
		t.Fatalf("nicht geteilt: %+v", o)
	}

	// Zuschauer: eigener Tab über den Link
	out := &syncBuf{}
	ws, done, err := dialWatch(99, o.link, out, a.events)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	// Meldung über den Zuschauer → Bildschirm wird geschickt
	deadline := time.After(10 * time.Second)
	for o.viewers == 0 {
		select {
		case m := <-a.events:
			a.shareMsg(m)
		case <-deadline:
			t.Fatal("kein Zuschauer gemeldet")
		}
	}
	if !out.has("uptime") {
		t.Fatalf("Bildschirm fehlt: %q", out.String())
	}
	// laufende Ausgabe kommt an, Tasten des Zuschauers nicht beim Gastgeber
	_, _ = outWriter{s, a.events}.Write([]byte("\r\n load average: 0.42\r\n"))
	if !out.has("load average: 0.42") {
		t.Fatalf("Ausgabe fehlt: %q", out.String())
	}
	if n, _ := ws.Write([]byte("rm -rf /\r")); n != 9 {
		t.Fatal("Zuschauer-Tasten sollen still verworfen werden")
	}
	// Größe kommt als eigene Meldung
	gotSize := false
	for !gotSize {
		select {
		case m := <-a.events:
			if w, ok := m.(watchSizeMsg); ok && w.sid == 99 && w.cols == 100 && w.rows == 20 {
				gotSize = true
			}
		case <-time.After(5 * time.Second):
			t.Fatal("keine Größe")
		}
	}
	// Gastgeber beendet: Zuschauer-Tab endet ohne Fehler
	a.stopShare(s)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Ende mit Fehler: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Zuschauer merkt das Ende nicht")
	}
	if relay.IsShareLink("hallo") || !relay.IsShareLink(o.link) {
		t.Fatal("IsShareLink")
	}
	if !strings.Contains(watchConn(o.link).WatchLink, "#teilen=") {
		t.Fatal("watchConn")
	}
}
