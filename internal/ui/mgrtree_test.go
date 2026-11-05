package ui

import (
	"reflect"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestBuildRowsCollapseAndFilter(t *testing.T) {
	conns := []domain.Connection{
		{ID: "1", Alias: "web1", Host: "10.0.0.1", Group: "Prod / Web", Tags: []string{"linux"}},
		{ID: "2", Alias: "sw1", Host: "10.0.0.2", Group: "Netz", Tags: []string{"aruba"}},
		{ID: "3", Alias: "pi", Host: "pi.lan"},
	}
	tree := domain.BuildTree(conns, []domain.Folder{{Path: "Netz", Collapsed: true}})
	keys := func(rows []mgrRow) []string {
		var out []string
		for _, r := range rows {
			out = append(out, r.key())
		}
		return out
	}
	// Netz zu: sw1 unsichtbar; Prod/Web offen
	if got := keys(buildRows(tree, nil, nil)); !reflect.DeepEqual(got, []string{"f:Netz", "f:Prod", "f:Prod / Web", "c:1", "c:3"}) {
		t.Fatalf("Baum: %v", got)
	}
	// Tag-Filter klappt passende Ordner auf und blendet leere aus
	if got := keys(buildRows(tree, nil, []string{"aruba"})); !reflect.DeepEqual(got, []string{"f:Netz", "c:2"}) {
		t.Fatalf("Tag: %v", got)
	}
	// Suche über den Ordnernamen findet den Inhalt
	if got := keys(buildRows(tree, []string{"prod"}, nil)); !reflect.DeepEqual(got, []string{"f:Prod", "f:Prod / Web", "c:1"}) {
		t.Fatalf("Suche: %v", got)
	}
}

func TestPlaceRel(t *testing.T) {
	l := []string{"a", "b", "c", "d"}
	if got, after := placeRel(l, "a", "c"); !after || !reflect.DeepEqual(got, []string{"b", "c", "a", "d"}) {
		t.Fatalf("nach unten: %v", got)
	}
	if got, after := placeRel(l, "d", "b"); after || !reflect.DeepEqual(got, []string{"a", "d", "b", "c"}) {
		t.Fatalf("nach oben: %v", got)
	}
	if got := insertBefore([]string{"x", "y"}, "n", "y"); !reflect.DeepEqual(got, []string{"x", "n", "y"}) {
		t.Fatalf("einfügen: %v", got)
	}
}
