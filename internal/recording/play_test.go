package recording

import (
	"testing"
	"time"
)

func TestLoadV2(t *testing.T) {
	b := NewBuilder(100, 30, "x", time.Now())
	b.Out("\x1b[1mhal")
	b.Wait(time.Second)
	b.Out("lo welt\r\n")
	b.Marker("Schritt 2")
	b.Wait(30 * time.Second)
	b.Out("ende")
	c, err := Load(b.Cast())
	if err != nil {
		t.Fatal(err)
	}
	if c.Width != 100 || c.Height != 30 || len(c.Events) != 4 || c.Duration() != 31 {
		t.Fatalf("%+v", c)
	}
	if i := c.Search("HALLO", 0); i != 1 {
		t.Fatalf("über Ereignisgrenze: %d", i)
	}
	if i := c.Search("ende", 2); i != 3 {
		t.Fatalf("ende: %d", i)
	}
	if c.Search("fehlt", 0) != -1 {
		t.Fatal("falscher Treffer")
	}
	if i := c.NextMarker(0.5); i != 2 {
		t.Fatalf("marke %d", i)
	}
	c.IdleLimit(2)
	if c.Duration() != 3 {
		t.Fatalf("kürzen: %v", c.Duration())
	}
	c.IdleLimit(0)
	if c.Duration() != 31 || c.Index(1) != 3 || c.Index(0) != 1 {
		t.Fatalf("zurück: %v %d", c.Duration(), c.Index(1))
	}
}

func TestLoadV3(t *testing.T) {
	data := `{"version":3,"term":{"cols":90,"rows":20},"idle_time_limit":1}
# Kommentar
[0.5,"o","a"]
[3.0,"o","b"]
[0.25,"x","0"]
`
	c, err := Load([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if c.Width != 90 || len(c.Events) != 3 || c.Events[1].Time != 1.5 || c.Duration() != 1.75 {
		t.Fatalf("%+v", c.Events)
	}
	if _, err := Load([]byte(`{"version":1}`)); err == nil {
		t.Fatal("v1 angenommen")
	}
}
