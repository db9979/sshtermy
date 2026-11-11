package relay

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestInviteParse(t *testing.T) {
	inv := Invite{Platz: 7, Words: [3]string{"taube", "kompass", "ofen"}}
	for _, s := range []string{"7-taube-kompass-ofen", " 7 Taube Kompass Ofen ", "7·taube·kompass·ofen"} {
		got, err := ParseInvite(s)
		if err != nil || got.Code() != inv.Code() {
			t.Fatalf("%q: %+v %v", s, got, err)
		}
	}
	inv.Base, inv.FP = "https://192.168.1.20:47213", strings.Repeat("ab", 32)
	got, err := ParseInvite("Schau mal: " + inv.Link())
	if err != nil || got != inv {
		t.Fatalf("%+v %v", got, err)
	}
	for _, bad := range []string{"taube-kompass-ofen", "7-taube-kompass", "7-taube-kompass-xyzzy", "0-taube-kompass-ofen", "sshtermy://zusehen/7-taube-kompass-ofen@http://x"} {
		if _, err := ParseInvite(bad); err == nil {
			t.Fatalf("%q sollte nicht gehen", bad)
		}
	}
	seen := map[string]bool{}
	for _, w := range shareWords {
		if seen[w] || w == "" {
			t.Fatalf("Wort doppelt/leer: %q", w)
		}
		seen[w] = true
	}
}

func TestPakeWrongCode(t *testing.T) {
	right := Invite{Platz: 3, Words: [3]string{"taube", "kompass", "ofen"}}
	wrong := right
	wrong.Words[2] = "eis"
	gp, a, _ := StartGuest(wrong)
	b, hk, err := HostPake(right.Words, right.Platz, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gp.Finish(b, hk.Host); err == nil {
		t.Fatal("falscher Code darf nicht passen")
	}
	gp, a, _ = StartGuest(right)
	b, hk, _ = HostPake(right.Words, right.Platz, a)
	gk, err := gp.Finish(b, hk.Host)
	if err != nil || gk.Check != hk.Check || string(gk.Key) != string(hk.Key) {
		t.Fatal("gleicher Code: gleiche Schlüssel und Prüfwörter", err)
	}
}

func openRelay(t *testing.T) (*Server, string, string) {
	srv, err := NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv.Logf = testLogf(t)
	srv.Open = true
	ts := httptest.NewTLSServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts.URL, CertFP(ts.Certificate().Raw)
}

// host: verarbeitet Meldungen, bis want kommt.
func hostWait(t *testing.T, hs *HostSession, want string) HostUI {
	t.Helper()
	tm := time.After(10 * time.Second)
	for {
		select {
		case e := <-hs.Sh.Events():
			if u, ok := hs.Handle(e); ok && u.Kind == want {
				return u
			}
		case <-tm:
			t.Fatalf("keine Meldung %q", want)
		}
	}
}

func TestShare2EndToEnd(t *testing.T) {
	_, base, fp := openRelay(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cl, _ := NewClient(base, "", fp)
	sh, err := cl.StartShare2(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()
	hs := NewHostSession(sh, 2)
	inv := hs.Invite()
	parsed, err := ParseInvite(inv.Link())
	if err != nil {
		t.Fatal(err)
	}

	type res struct {
		w     *Watch2
		check [3]string
		err   error
	}
	join := func(inv Invite, name string) chan res {
		ch := make(chan res, 1)
		go func() {
			g, _, err := Locate(ctx, inv, nil)
			if err != nil {
				ch <- res{err: err}
				return
			}
			var chk [3]string
			w, err := Join(ctx, g, inv, Hello{Name: name, Rechner: "laptop"}, func(c [3]string) { chk = c })
			ch <- res{w, chk, err}
		}()
		return ch
	}
	ch := join(parsed, "Anna")
	u := hostWait(t, hs, "klopft")
	if u.G.Hello.Name != "Anna" || u.G.Check[0] == "" {
		t.Fatalf("%+v", u.G)
	}
	if err := hs.Admit(u.G.V, ShareInfo{Tab: "web02", Von: "Dominik"}); err != nil {
		t.Fatal(err)
	}
	r := <-ch
	if r.err != nil || r.check != u.G.Check || r.w.Info.Tab != "web02" {
		t.Fatalf("%+v", r)
	}
	if err := hs.Frame(0, FrameData, []byte("hallo")); err != nil {
		t.Fatal(err)
	}
	typ, p, err := r.w.Next(ctx)
	if err != nil || typ != FrameData || string(p) != "hallo" {
		t.Fatal(typ, string(p), err)
	}
	// Mitschreiben nur, wenn erlaubt
	_ = r.w.SendCtl(Ctl{T: "tasten", Data: []byte("ls")})
	_ = r.w.SendCtl(Ctl{T: "hand"})
	if u := hostWait(t, hs, "hand"); u.G.Hello.Name != "Anna" {
		t.Fatal(u)
	}
	_ = hs.SetWrite(u.G.V, time.Now().Add(time.Minute))
	if typ, p, err := r.w.Next(ctx); typ != 'c' || !strings.Contains(string(p), "schreiben") || err != nil {
		t.Fatal(typ, string(p), err)
	}
	_ = r.w.SendCtl(Ctl{T: "tasten", Data: []byte("ls")})
	if u := hostWait(t, hs, "tasten"); string(u.Data) != "ls" {
		t.Fatal(u)
	}

	// falscher Code: ein Versuch, dann gesperrt
	bad := parsed
	bad.Words[0] = "zebra"
	if bad.Words == parsed.Words {
		bad.Words[0] = "eis"
	}
	ch2 := join(bad, "Mallory")
	hostWait(t, hs, "falsch")
	if r := <-ch2; r.err == nil {
		t.Fatal("falscher Code kam durch")
	}
	if !hs.Locked() {
		t.Fatal("Einladung muss gesperrt sein")
	}
	time.Sleep(100 * time.Millisecond)
	if r := <-join(parsed, "Ben"); r.err == nil {
		t.Fatal("gesperrte Einladung darf niemanden mehr hereinlassen")
	}
	// neuer Code geht wieder
	inv2 := hs.NewCode()
	ch3 := join(inv2, "Ben")
	u3 := hostWait(t, hs, "klopft")
	hs.Deny(u3.G.V)
	if r := <-ch3; !errors.Is(r.err, ErrDenied) {
		t.Fatal(r.err)
	}

	sh.Close()
	if _, _, err := r.w.Next(ctx); err != io.EOF {
		t.Fatal(err)
	}
}

func TestShare2NeedsKeyWhenClosed(t *testing.T) {
	srv, base, fp := openRelay(t)
	srv.Open = false
	cl, _ := NewClient(base, "", fp)
	if _, err := cl.StartShare2(context.Background(), 10); err == nil {
		t.Fatal("ohne Kopplung darf ein geschlossener Vermittler nicht teilen")
	}
}

func TestLANShare(t *testing.T) {
	if LocalIP() == "" {
		lanIP = func() string { return "127.0.0.1" }
		defer func() { lanIP = LocalIP }()
	}
	l, err := StartLAN()
	if err != nil {
		t.Skip("kein lokales Netz:", err)
	}
	defer l.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sh, err := l.Local.StartShare2(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	sh.Base, sh.FP = l.Base, l.FP
	if sh.Platz < lanPlatzMin {
		t.Fatal(sh.Platz)
	}
	hs := NewHostSession(sh, 1)
	l.Announce(sh.Platz, true)
	if l.Findable() {
		f, err := FindLAN(ctx, sh.Platz, time.Second)
		if err != nil || len(f) == 0 || f[0].FP != l.FP {
			t.Fatalf("%+v %v", f, err)
		}
	}
	// Fremde dürfen beim eingebauten Vermittler nichts anlegen
	other, _ := NewClient(l.Base, "", l.FP)
	if _, err := other.StartShare2(ctx, 5); err == nil {
		t.Fatal("fremde Freigabe angenommen")
	}
	inv := hs.Invite()
	ch := make(chan error, 1)
	go func() {
		g, _, err := Locate(ctx, inv, nil)
		if err == nil {
			_, err = Join(ctx, g, inv, Hello{Name: "Anna"}, nil)
		}
		ch <- err
	}()
	u := hostWait(t, hs, "klopft")
	_ = hs.Admit(u.G.V, ShareInfo{})
	if err := <-ch; err != nil {
		t.Fatal(err)
	}
	if !hs.Locked() {
		t.Fatal("Max 1: nach dem Ersten zu")
	}
}
