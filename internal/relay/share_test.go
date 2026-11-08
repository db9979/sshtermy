package relay

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestShareLink(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	l := ShareLink{Base: "https://relay.example.org", ID: strings.Repeat("ab", 16), Key: key, FP: strings.Repeat("cd", 32)}
	got, err := ParseShareLink("  " + l.String() + " ")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != l.String() || !IsShareLink(l.String()) {
		t.Fatalf("%+v", got)
	}
	if _, err := ParseShareLink("https://relay#teilen=" + l.ID); err == nil {
		t.Fatal("ohne Schlüssel")
	}
	if _, err := ParseShareLink("http://relay#teilen=" + l.ID + "&k=" + l.String()[len(l.String())-43:]); err == nil {
		t.Fatal("http")
	}
	f, _ := seal(key, l.ID, FrameData, []byte("hallo"))
	if typ, p, err := open(key, l.ID, f); err != nil || typ != FrameData || string(p) != "hallo" {
		t.Fatal(typ, p, err)
	}
	if _, _, err := open(key, "andere", f); err == nil {
		t.Fatal("andere Kennung darf nicht passen")
	}
}

func TestShareEndToEnd(t *testing.T) {
	dir := t.TempDir()
	srv, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv.Logf = t.Logf
	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	fp := CertFP(ts.Certificate().Raw)
	st, _ := LoadState(dir)
	st.Name, st.URL, st.FP = "Test", ts.URL, fp
	if err := SaveState(dir, st); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	line, _ := Line(dir, CodeClient)
	pl, _ := ParseLine(line)
	cl, _ := NewClient(pl.Base, "", pl.FP)
	if _, err := cl.Pair(ctx, pl.Code, "laptop"); err != nil {
		t.Fatal(err)
	}

	// ohne Kopplung keine Freigabe
	anon, _ := NewClient(ts.URL, "", fp)
	if _, err := anon.StartShare(ctx); err == nil {
		t.Fatal("ohne Schlüssel geteilt")
	}

	host, err := cl.StartShare(ctx)
	if err != nil {
		t.Fatal(err)
	}
	link, err := ParseShareLink(host.Link.String())
	if err != nil {
		t.Fatal(err)
	}
	view, err := WatchShare(ctx, link)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-host.Viewers():
		if n != 1 {
			t.Fatalf("%d Zuschauer", n)
		}
	case <-ctx.Done():
		t.Fatal("keine Meldung über Zuschauer")
	}
	if err := host.Send(FrameSize, []byte("80 24")); err != nil {
		t.Fatal(err)
	}
	if err := host.Send(FrameData, []byte("$ ls\r\n")); err != nil {
		t.Fatal(err)
	}
	if typ, p, err := view.Next(ctx); err != nil || typ != FrameSize || string(p) != "80 24" {
		t.Fatal(typ, string(p), err)
	}
	if typ, p, err := view.Next(ctx); err != nil || typ != FrameData || string(p) != "$ ls\r\n" {
		t.Fatal(typ, string(p), err)
	}

	// falscher Schlüssel: Rahmen unlesbar
	wrong := link
	wrong.Key = bytes.Repeat([]byte{1}, 32)
	v2, err := WatchShare(ctx, wrong)
	if err != nil {
		t.Fatal(err)
	}
	<-host.Viewers()
	_ = host.Send(FrameData, []byte("geheim"))
	if _, _, err := v2.Next(ctx); err == nil || err == io.EOF {
		t.Fatalf("falscher Schlüssel: %v", err)
	}
	_, _, _ = view.Next(ctx) // „geheim“ für den richtigen Zuschauer

	host.Close()
	if _, _, err := view.Next(ctx); err != io.EOF {
		t.Fatalf("Ende: %v", err)
	}
	if _, err := WatchShare(ctx, link); err == nil {
		t.Fatal("beendete Freigabe noch offen")
	}
}
