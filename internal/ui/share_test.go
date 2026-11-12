package ui

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/relay"
)

type shareShell struct {
	mu  sync.Mutex
	got strings.Builder
}

func (f *shareShell) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.got.Write(p)
}
func (f *shareShell) Resize(int, int) error { return nil }
func (f *shareShell) Close() error          { return nil }
func (f *shareShell) String() string        { f.mu.Lock(); defer f.mu.Unlock(); return f.got.String() }

// pumpUntil verarbeitet Meldungen, bis ok() stimmt.
func pumpUntil(t *testing.T, a *App, what string, ok func() bool) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for !ok() {
		select {
		case m := <-a.events:
			a.shareMsg(m)
		case <-deadline:
			t.Fatal("wartet vergeblich auf: " + what)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestShareAndWatchV2(t *testing.T) {
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
	s, _ := typedSession(domain.Connection{Alias: "web"}, "$ ", "cat geheim-von-vorher")
	sh := &shareShell{}
	s.shell = sh
	a.sessions = append(a.sessions, s)

	// Dialog statt Fehlermeldung
	a.startShare(s)
	if a.scr != scrShareForm || a.form == nil || !strings.Contains(a.form.title, "web") {
		t.Fatalf("kein Teilen-Dialog: %v", a.scr)
	}
	a.form, a.scr = nil, scrTerminal

	// Gastgeber: Freigabe über den Vermittler, Standard-Vorgaben
	a.shareMsg(a.startShareWith(s, shareOpts{way: "relay:" + r.ID, knock: true, hidePw: true})())
	o := s.share.Load()
	if o == nil || a.modal == nil || !strings.Contains(a.modal.copy, "sshtermy://zusehen/") {
		t.Fatalf("nicht geteilt / keine Einladung: %+v", o)
	}
	// m: fertige E-Mail im Mailprogramm
	mails := make(chan string, 1)
	defer func(f func(string) error) { openMail = f }(openMail)
	openMail = func(u string) error { mails <- u; return nil }
	if a.modal.mail == "" {
		t.Fatal("Einladung ohne E-Mail")
	}
	a.modal.textKey("m")
	var mailed string
	select {
	case mailed = <-mails:
	case <-time.After(5 * time.Second):
	}
	mu, err := url.Parse(mailed)
	if err != nil || mu.Scheme != "mailto" || strings.Contains(mailed, "+") ||
		!strings.Contains(mu.Query().Get("body"), o.hs.Invite().Code()) || !strings.Contains(mu.Query().Get("subject"), "web") {
		t.Fatalf("E-Mail: %q", mailed)
	}
	a.modal = nil
	if !strings.Contains(a.shareChip(s), "LIVE") {
		t.Fatal("LIVE-Anzeige fehlt")
	}

	// Zuschauer mit Link
	out := &syncBuf{}
	ws, done, err := a.dialWatch(99, o.hs.Invite().Link(), out, a.events)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	pumpUntil(t, a, "Anklopfen", func() bool { return a.modal != nil && a.modal.kind == mConfirm })
	if !out.has("Prüfwörter") || !strings.Contains(a.modal.body, strings.ToUpper(o.hs.Guests()[0].Check[0])) {
		t.Fatalf("Prüfwörter fehlen: %q / %q", out.String(), a.modal.body)
	}
	a.modal.onYes()
	a.modal = nil
	pumpUntil(t, a, "zugelassen", func() bool { return len(o.guestNames()) == 1 })
	time.Sleep(200 * time.Millisecond)
	if strings.Contains(out.String(), "geheim-von-vorher") {
		t.Fatal("„ab jetzt“: alter Bildschirm darf nicht zum Zuschauer")
	}
	// laufende Ausgabe kommt an
	_, _ = outWriter{s, a.events}.Write([]byte("\r\n load average: 0.42\r\n"))
	if !out.has("load average: 0.42") {
		t.Fatalf("Ausgabe fehlt: %q", out.String())
	}
	// Passwortabfrage: Pause, was dann kommt, sieht niemand
	_, _ = outWriter{s, a.events}.Write([]byte("[sudo] password for dominik: "))
	if !out.has("Passwort ein") {
		t.Fatalf("keine Pause: %q", out.String())
	}
	_, _ = outWriter{s, a.events}.Write([]byte("hunter2-echo"))
	_, _ = outWriter{s, a.events}.Write([]byte("\r\nweiter-nach-pause\r\n$ "))
	if !out.has("weiter-nach-pause") {
		t.Fatalf("kein Weiter: %q", out.String())
	}
	if strings.Contains(out.String(), "hunter2") {
		t.Fatal("Ausgabe während der Passwortabfrage ging hinaus")
	}
	// Tasten des Zuschauers gehen nirgendwohin, bis Mitschreiben erlaubt ist
	_, _ = ws.Write([]byte("rm -rf /\r"))
	time.Sleep(200 * time.Millisecond)
	pumpUntil(t, a, "nichts", func() bool { return true })
	if sh.String() != "" {
		t.Fatalf("Zuschauer-Tasten kamen an: %q", sh.String())
	}
	g := o.hs.Guests()[0]
	_ = o.hs.SetWrite(g.V, time.Now().Add(time.Minute))
	pumpUntil(t, a, "Schreibrecht beim Zuschauer", func() bool { return ws.(*watch2Stream).writeTo.Load() > 0 })
	_, _ = ws.Write([]byte("uptime\r"))
	pumpUntil(t, a, "⏎-Rückfrage (produktiv)", func() bool { return a.modal != nil && strings.Contains(a.modal.title, "⏎") })
	if sh.String() != "uptime" {
		t.Fatalf("vor der Bestätigung: %q", sh.String())
	}
	a.modal.onYes()
	if sh.String() != "uptime\r" {
		t.Fatalf("nach der Bestätigung: %q", sh.String())
	}
	a.modal = nil

	// Größe kommt als eigene Meldung
	pumpUntil(t, a, "Größe", func() bool { return s.share.Load() != nil })
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
	if !strings.Contains(watchConn("7-taube-kompass-ofen").WatchLink, "taube") {
		t.Fatal("watchConn")
	}
	_ = tea.Quit
}

func TestShareFormWays(t *testing.T) {
	a := dangerApp(t)
	s, _ := typedSession(domain.Connection{Alias: "db01"}, "$ ", "")
	a.sessions = append(a.sessions, s)
	a.openShareForm(s)
	fl := a.form.get("way")
	if len(fl.options) != 2 || a.shareForm.ways[fl.options[0]] != "lan" || a.shareForm.ways[fl.options[1]] != "setup" {
		t.Fatalf("Wege: %v", fl.options)
	}
	if !a.form.flag("knock") || !a.form.flag("hidepw") || a.form.flag("screen") || a.form.sel("max") != maxLabel(1) {
		t.Fatal("sichere Vorgaben fehlen")
	}
	if a.form.get("prod").label == "" {
		t.Fatal("Hinweis „produktiv“ fehlt")
	}
}

func TestQRLines(t *testing.T) {
	q := qrLines("sshtermy://zusehen/7-taube-kompass-ofen@192.168.1.20:47213?fp=" + strings.Repeat("ab", 32))
	if len(q) < 10 || len(q) > 40 {
		t.Fatalf("QR: %d Zeilen", len(q))
	}
}
