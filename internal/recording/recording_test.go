package recording

import (
	"strings"
	"testing"
	"time"
)

func TestRecorderCast(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	clock := func() time.Time { return now }
	r := newAt(100, 30, "web1", clock)
	now = now.Add(500 * time.Millisecond)
	r.Output([]byte("hallo \xc3")) // „ä“ über zwei Blöcke verteilt
	now = now.Add(time.Second)
	r.Output([]byte("\xa4 welt\r\n\x1b[32mok\x1b[0m"))
	r.Resize(120, 40)
	h, ev, err := Parse(r.Cast())
	if err != nil {
		t.Fatal(err)
	}
	if h.Width != 100 || h.Height != 30 || h.Title != "web1" || h.Timestamp != 1_700_000_000 {
		t.Errorf("Header %+v", h)
	}
	if len(ev) != 3 {
		t.Fatalf("Ereignisse %v", ev)
	}
	if ev[0].Data != "hallo " || ev[0].Time != 0.5 || ev[1].Data != "ä welt\r\n\x1b[32mok\x1b[0m" || ev[1].Time != 1.5 {
		t.Errorf("Ausgabe %+v", ev)
	}
	if ev[2].Kind != "r" || ev[2].Data != "120x40" {
		t.Errorf("Resize %v", ev[2])
	}
}

func TestRecorderLimit(t *testing.T) {
	r := New(80, 24, "")
	r.Output([]byte(strings.Repeat("x", MaxBytes)))
	r.Output([]byte("y"))
	if !r.Truncated() || r.Size() != MaxBytes {
		t.Errorf("Grenze: %d %v", r.Size(), r.Truncated())
	}
}

func TestBuilder(t *testing.T) {
	b := NewBuilder(80, 24, "x", time.Unix(5, 0))
	b.Out("$ ")
	b.Type("ls", 100*time.Millisecond)
	b.Wait(time.Second)
	b.Out("a b\r\n")
	_, ev, err := Parse(b.Cast())
	if err != nil || len(ev) != 4 || ev[3].Time != 1.2 || ev[2].Data != "s" {
		t.Fatalf("%v %v", err, ev)
	}
}
