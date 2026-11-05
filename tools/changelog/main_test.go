package main

import "testing"

func TestNewsSection(t *testing.T) {
	body := "<!-- claude -->\n## Zusammenfassung\n- intern umgebaut\n\n## Was ist neu\n<!-- Hinweis -->\n- Changelog in der App\n* Neustart nach Update\n\n## Test\n- go test\n"
	items, found := newsSection(body)
	if !found || len(items) != 2 || items[0] != "Changelog in der App" || items[1] != "Neustart nach Update" {
		t.Fatalf("%v %v", found, items)
	}
	items, found = newsSection("## Was ist neu\n- intern\n")
	if !found || len(items) != 0 {
		t.Fatalf("intern: %v %v", found, items)
	}
	if _, found = newsSection("## Summary\n- x\n"); found {
		t.Fatal("ohne Abschnitt")
	}
}
