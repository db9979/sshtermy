package changelog

import (
	"testing"

	"github.com/db9979/sshtermy/internal/update"
)

const sample = `# Was ist neu in sshTermy

## v1.0.12 · 05.11.2026
- Changelog in der App
- Neustart nach Update
  ohne Umwege

## v1.0.10 · 04.11.2026
- Maus

## v1.0.9 · 03.11.2026
- Palette
`

func TestParse(t *testing.T) {
	e := Parse(sample)
	if len(e) != 3 || e[0].Version != "1.0.12" || e[0].Date != "05.11.2026" || len(e[0].Items) != 2 {
		t.Fatalf("%+v", e)
	}
	if e[0].Items[1] != "Neustart nach Update ohne Umwege" {
		t.Fatalf("Fortsetzung: %q", e[0].Items[1])
	}
}

func TestSince(t *testing.T) {
	e := Parse(sample)
	got := Since(e, "1.0.9", "1.0.12", update.Newer)
	if len(got) != 2 || got[1].Version != "1.0.10" {
		t.Fatalf("%+v", got)
	}
	if got := Since(e, "1.0.9", "1.0.10", update.Newer); len(got) != 1 {
		t.Fatalf("upto: %+v", got)
	}
	if got := Since(e, "", "1.0.12", update.Newer); len(got) != 3 {
		t.Fatalf("alle: %+v", got)
	}
}

func TestEmbedded(t *testing.T) {
	if len(All()) == 0 {
		t.Fatal("CHANGELOG.md ist leer")
	}
}
