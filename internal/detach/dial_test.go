package detach

import (
	"io"
	"testing"
	"time"
)

// Dial ist die Fensterseite für das eigene Fenster (internal/gui).
func TestDialClient(t *testing.T) {
	s, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
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
	cl, err := Dial(s.Dir(), 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	r := <-ch
	if r.err != nil || r.cols != 100 || r.rows != 30 {
		t.Fatalf("Accept: %+v", r)
	}
	if _, err := cl.Write([]byte("ls\r")); err != nil {
		t.Fatal(err)
	}
	if ev, err := r.c.Read(); err != nil || string(ev.Input) != "ls\r" {
		t.Fatalf("Taste: %+v %v", ev, err)
	}
	_ = cl.Resize(90, 20)
	if ev, err := r.c.Read(); err != nil || ev.Cols != 90 || ev.Rows != 20 {
		t.Fatalf("Größe: %+v %v", ev, err)
	}
	_, _ = r.c.Write([]byte("hallo"))
	_ = r.c.Title("web1")
	r.c.Bye("zurück")
	want := []Message{{Output: []byte("hallo")}, {Title: "web1"}, {Bye: true, Text: "zurück"}}
	for _, w := range want {
		m, err := cl.Next()
		if err != nil || string(m.Output) != string(w.Output) || m.Title != w.Title || m.Bye != w.Bye || m.Text != w.Text {
			t.Fatalf("Next: %+v %v, erwartet %+v", m, err, w)
		}
	}
	r.c.Close()
	if _, err := cl.Next(); err != io.EOF {
		t.Fatalf("nach Close: %v", err)
	}
	cl.Close()
}
