package ui

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Die druckbare Kurzreferenz (tools/handout/content.mjs → site/handout/*.pdf)
// muss zu den Tasten passen. Dieser Test merkt sich einen Fingerabdruck der
// Tastenbelegung (actionDefs) und der Hilfe (helpSections). Ändert sich eins
// davon, schlägt er fehl, bis die Kurzreferenz nachgezogen ist:
//
//  1. tools/handout/content.mjs anpassen (README-Tabellen und site/ Tasten auch)
//  2. make handout   (schreibt die PDFs und den neuen Fingerabdruck)
const handoutStamp = "../../tools/handout/keys.sha256"

func handoutFingerprint() string {
	var b strings.Builder
	for _, d := range actionDefs {
		fmt.Fprintf(&b, "%s|%s|%s|%s\n", d.id, d.label, d.area, strings.Join(d.def, ","))
	}
	for _, s := range helpSections {
		fmt.Fprintf(&b, "## %s|%s\n", s.title, s.key)
		for _, r := range s.rows {
			fmt.Fprintf(&b, "%s|%s\n", r[0], r[1])
		}
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(b.String())))
}

func TestHandoutKeysCurrent(t *testing.T) {
	got := handoutFingerprint()
	if os.Getenv("HANDOUT_STAMP") != "" {
		if err := os.WriteFile(handoutStamp, []byte(got+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(handoutStamp)
	if err != nil {
		t.Fatalf("Fingerabdruck fehlt: %v (make handout ausführen)", err)
	}
	if strings.TrimSpace(string(want)) != got {
		t.Fatalf("Tasten oder Hilfe haben sich geändert, die Kurzreferenz ist veraltet.\n" +
			"Bitte tools/handout/content.mjs (und die Tastentabellen in README.md, README.de.md, site/) anpassen,\n" +
			"dann `make handout` ausführen: das schreibt site/handout/*.pdf und tools/handout/keys.sha256 neu.")
	}
}
