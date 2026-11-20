package safe

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunWithoutHandlerRepanics(t *testing.T) {
	Reset()
	defer func() {
		if recover() == nil {
			t.Fatal("ohne Handler muss die Panik durchlaufen")
		}
	}()
	Run("x", func() { panic("boom") })
}

func TestRunCatches(t *testing.T) {
	Reset()
	defer Reset()
	var got Caught
	SetHandler(func(c Caught) { got = c })
	if Run("ui.Update", func() { panic("boom") }) {
		t.Fatal("Run meldet ok trotz Panik")
	}
	if got.Where != "ui.Update" || got.Value != "boom" || !strings.Contains(got.Raw(), "panic: boom") {
		t.Fatalf("falsch erfasst: %+v", got)
	}
	if !Run("x", func() {}) {
		t.Fatal("ohne Panik muss Run ok melden")
	}
	if Count() != 1 {
		t.Fatalf("Count = %d", Count())
	}
}

func TestGoCatches(t *testing.T) {
	Reset()
	defer Reset()
	var wg sync.WaitGroup
	wg.Add(1)
	SetHandler(func(c Caught) { wg.Done() })
	Go("bg", func() {
		var m map[string]int
		m["x"] = 1 // nil map: echte Laufzeit-Panik
	})
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Handler nicht aufgerufen")
	}
}

func TestStormRepanics(t *testing.T) {
	Reset()
	defer Reset()
	SetHandler(func(Caught) {})
	for i := 0; i < StormN; i++ {
		Run("x", func() { panic(i) })
	}
	defer func() {
		if recover() == nil {
			t.Fatal("beim Sturm muss die Panik durchlaufen")
		}
	}()
	Run("x", func() { panic("zu viel") })
}

func TestStormForgets(t *testing.T) {
	Reset()
	defer Reset()
	now := time.Now()
	for i := 0; i < StormN; i++ {
		if storm(now) {
			t.Fatal("zu früh Sturm")
		}
	}
	if storm(now.Add(StormWindow + time.Second)) {
		t.Fatal("alte Paniken müssen vergessen werden")
	}
}
