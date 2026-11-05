package domain

import "testing"

func TestNormPath(t *testing.T) {
	for in, want := range map[string]string{
		"Prod/Web":        "Prod / Web",
		" Prod  /  Web  ": "Prod / Web",
		`Kunde\Switches`:  "Kunde / Switches",
		"Netz / Switches": "Netz / Switches",
		"//a///b/":        "a / b",
		"":                "",
		"Haus   Keller":   "Haus Keller",
	} {
		if got := NormPath(in); got != want {
			t.Errorf("%q → %q, erwartet %q", in, got, want)
		}
	}
	if PathParent("a / b / c") != "a / b" || PathBase("a / b / c") != "c" || PathParent("a") != "" {
		t.Fatal("PathParent/PathBase")
	}
	if !InFolder("a / b", "a") || InFolder("ab", "a") || !InFolder("x", "") {
		t.Fatal("InFolder")
	}
	if p, ok := Reparent("a / b / c", "a / b", "z"); !ok || p != "z / c" {
		t.Fatalf("Reparent: %q", p)
	}
}

func TestBuildTreeOrder(t *testing.T) {
	conns := []Connection{
		{ID: "1", Alias: "zeta", Group: "Prod"},
		{ID: "2", Alias: "alpha", Group: "Prod"},
		{ID: "3", Alias: "web", Group: "Prod / Web"},
		{ID: "4", Alias: "lose"},
		{ID: "5", Alias: "mid", Group: "Prod", Order: 1},
	}
	folders := []Folder{{ID: "f", Path: "Leer"}, {ID: "g", Path: "Prod", Order: 1}, {ID: "h", Path: "Homelab", Collapsed: true}}
	tr := BuildTree(conns, folders)
	if tr.Total != 5 || len(tr.Conns) != 1 {
		t.Fatalf("Wurzel: %+v", tr)
	}
	// Prod hat Order 1 und steht vor den alphabetischen
	if tr.Folders[0].Path != "Prod" || tr.Folders[1].Path != "Homelab" || tr.Folders[2].Path != "Leer" {
		t.Fatalf("Ordner: %s %s %s", tr.Folders[0].Path, tr.Folders[1].Path, tr.Folders[2].Path)
	}
	p := tr.Find("Prod")
	if p.Total != 4 || p.Conns[0].Alias != "mid" || p.Conns[1].Alias != "alpha" || p.Conns[2].Alias != "zeta" {
		t.Fatalf("Prod: %+v", p.Conns)
	}
	if tr.Find("Prod / Web") == nil || !tr.Find("Homelab").Collapsed || tr.Find("Leer").Total != 0 {
		t.Fatal("Find/Collapsed/leer")
	}
}

func TestJumpChainAndCycle(t *testing.T) {
	m := map[string]Connection{
		"a": {ID: "a", Alias: "bastion"},
		"b": {ID: "b", Alias: "jump", JumpHostID: "a"},
		"c": {ID: "c", Alias: "ziel", JumpHostID: "b"},
		"x": {ID: "x", Alias: "x", JumpHostID: "y"},
		"y": {ID: "y", Alias: "y", JumpHostID: "x"},
	}
	get := func(id string) (Connection, bool) { c, ok := m[id]; return c, ok }
	ch, cyc := JumpChain("c", get)
	if cyc || len(ch) != 3 || ch[0].Alias != "bastion" || ch[2].Alias != "ziel" {
		t.Fatalf("Kette: %v %v", ch, cyc)
	}
	if _, cyc := JumpChain("x", get); !cyc {
		t.Fatal("Kreis nicht erkannt")
	}
	if !JumpCreatesCycle("a", "c", get) { // bastion über ziel: ziel geht über bastion
		t.Fatal("a über c wäre ein Kreis")
	}
	if JumpCreatesCycle("c", "a", get) || JumpCreatesCycle("", "c", get) || !JumpCreatesCycle("a", "a", get) {
		t.Fatal("falscher Kreis")
	}
}
