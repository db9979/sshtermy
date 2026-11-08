package hostinfo

import (
	"strings"
	"testing"
	"time"
)

func feed(p *LiveParser, text string) (out []LiveSample) {
	for _, l := range strings.Split(text, "\n") {
		if s, ok := p.Line(l); ok {
			out = append(out, s)
		}
	}
	return out
}

func TestLiveParserAndRates(t *testing.T) {
	var p LiveParser
	ss := feed(&p, `c cpu  1000 0 500 8000 500 0 0 0 0 0
m MemTotal: 1000000
m MemAvailable: 250000
n 1000 2000
d 100 200
e
c cpu  1300 0 600 8500 600 0 0 0 0 0
m MemTotal: 1000000
m MemAvailable: 400000
n 3001000 2002000
d 2148 200
p 30 postgres: writer
p 10 nginx
e`)
	if len(ss) != 2 {
		t.Fatalf("Proben: %d", len(ss))
	}
	ss[1].At = ss[0].At.Add(2 * time.Second)
	r, ok := Rates(ss[0], ss[1])
	if !ok {
		t.Fatal("keine Rate")
	}
	// 1000 Ticks insgesamt, davon 400 belegt (idle 500 + iowait 100)
	if r.CPU < 0.399 || r.CPU > 0.401 {
		t.Errorf("CPU %.3f", r.CPU)
	}
	if r.Mem < 0.599 || r.Mem > 0.601 {
		t.Errorf("RAM %.3f", r.Mem)
	}
	if r.NetRx != 1.5e6 || r.NetTx != 1e6 {
		t.Errorf("Netz %v %v", r.NetRx, r.NetTx)
	}
	if r.Disk != 2048*512/2 {
		t.Errorf("Platte %v", r.Disk)
	}
	if len(r.TopProcs) != 2 || r.TopProcs[0].Name != "postgres: writer" || r.TopProcs[0].Pct != 0.03 {
		t.Errorf("Prozesse %+v", r.TopProcs)
	}
	if _, ok := Rates(ss[1], ss[0]); ok {
		t.Error("rückwärts darf keine Rate ergeben")
	}
}

func TestLiveNoProc(t *testing.T) {
	var p LiveParser
	feed(&p, "nolive")
	if !p.NoLive {
		t.Error("nolive nicht erkannt")
	}
}

func TestRate(t *testing.T) {
	for v, want := range map[float64]string{12: "12 B/s", 4600: "5 kB/s", 3.2e6: "3.2 MB/s", 1.5e9: "1.5 GB/s"} {
		if got := Rate(v); got != want {
			t.Errorf("Rate(%v) = %q, erwartet %q", v, got, want)
		}
	}
}
