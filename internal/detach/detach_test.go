package detach

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// fakeClient dockt wie „sshtermy attach“ an, nur ohne Terminal.
func fakeClient(t *testing.T, dir, token string) *frameWriter {
	t.Helper()
	network, addr, err := readAddr(dir)
	if err != nil {
		t.Fatal(err)
	}
	nc, err := dial(network, addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })
	fw := &frameWriter{w: nc}
	if err := fw.write(frHello, []byte(token)); err != nil {
		t.Fatal(err)
	}
	if err := fw.write(frResize, sizePayload(120, 40)); err != nil {
		t.Fatal(err)
	}
	return fw
}

func TestRoundTrip(t *testing.T) {
	s, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if fi, err := os.Stat(s.Dir()); err != nil || fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("Ordner muss privat sein: %v %v", fi.Mode(), err)
	}
	tok, _ := os.ReadFile(filepath.Join(s.Dir(), tokenFile))

	// falsches Zugangswort wird abgewiesen, das richtige danach angenommen
	type res struct {
		c          *Conn
		cols, rows int
		err        error
	}
	ch := make(chan res, 1)
	go func() {
		c, cols, rows, err := s.Accept(5 * time.Second)
		ch <- res{c, cols, rows, err}
	}()
	_ = fakeClient(t, s.Dir(), "falsch")
	fw := fakeClient(t, s.Dir(), string(tok))
	r := <-ch
	if r.err != nil || r.cols != 120 || r.rows != 40 {
		t.Fatalf("Accept: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), tokenFile)); !os.IsNotExist(err) {
		t.Error("Zugangswort muss nach dem Andocken weg sein")
	}

	// Tasten und Größe kommen an
	_ = fw.write(frData, []byte("ls\r"))
	_ = fw.write(frResize, sizePayload(90, 30))
	ev, err := r.c.Read()
	if err != nil || string(ev.Input) != "ls\r" {
		t.Fatalf("Tasten: %q %v", ev.Input, err)
	}
	ev, err = r.c.Read()
	if err != nil || ev.Cols != 90 || ev.Rows != 30 {
		t.Fatalf("Größe: %+v %v", ev, err)
	}
	r.c.Bye("tschüss")
	r.c.Bye("zweimal ist egal")
}

func TestAcceptTimeout(t *testing.T) {
	s, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Accept(50 * time.Millisecond); err != ErrTimeout {
		t.Fatalf("erwartet ErrTimeout, bekam %v", err)
	}
	if _, err := os.Stat(s.Dir()); !os.IsNotExist(err) {
		t.Error("Ordner muss nach Zeitüberschreitung weg sein")
	}
}

func TestExpand(t *testing.T) {
	argv := []string{"/opt/ssh termy/sshtermy", "attach", "/tmp/x"}
	got := expand([]string{"--title", "{title}", "-e", "{cmd}"}, "sshTermy · web", argv)
	want := []string{"--title", "sshTermy · web", "-e", "/opt/ssh termy/sshtermy", "attach", "/tmp/x"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expand: %q", got)
	}
	if got := expand([]string{"-x"}, "t", argv); len(got) != 4 || got[1] != argv[0] {
		t.Errorf("ohne {cmd} ans Ende: %q", got)
	}
	if got := shellJoin([]string{"a b", "it's"}); got != `'a b' 'it'\''s'` {
		t.Errorf("shellJoin: %s", got)
	}
}

func TestBigFrameSplit(t *testing.T) {
	p := make([]byte, maxFrame+10)
	var out [][]byte
	fw := &frameWriter{w: writerFunc(func(b []byte) (int, error) { out = append(out, append([]byte(nil), b...)); return len(b), nil })}
	if err := fw.write(frData, p); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || len(out[0]) != 5+maxFrame || len(out[1]) != 5+10 {
		t.Errorf("Aufteilung falsch: %d Teile", len(out))
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(b []byte) (int, error) { return f(b) }
