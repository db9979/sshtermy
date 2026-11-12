package agent

import (
	"fmt"
	"strings"
	"testing"
)

// Lange Ziele: an die KI gehen nur die letzten histLines Befehlszeilen.
func TestWriteHistCapsLines(t *testing.T) {
	var hist []DoneStep
	for i := 1; i <= 300; i++ {
		hist = append(hist, DoneStep{Command: fmt.Sprintf("echo schritt%d", i), Output: "ok"})
	}
	var b strings.Builder
	writeHist(&b, hist)
	s := b.String()
	if !strings.Contains(s, fmt.Sprintf("(%d frühere Schritte ausgelassen)", 300-histLines)) {
		t.Fatalf("Hinweis auf ausgelassene Schritte fehlt:\n%s", s[:200])
	}
	if strings.Contains(s, "echo schritt1 ") || strings.Contains(s, "echo schritt240 ") {
		t.Fatal("alte Schritte stehen noch im Prompt")
	}
	if !strings.Contains(s, "241. echo schritt241") || !strings.Contains(s, "300. echo schritt300") {
		t.Fatal("letzte Schritte fehlen oder Nummern stimmen nicht")
	}
}
