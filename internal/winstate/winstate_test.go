package winstate

import "testing"

func TestDefaultFullAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if m := Load(dir).Mode(); m != StartFull {
		t.Fatalf("Standard %q, erwartet Vollbild", m)
	}
	if err := Save(dir, State{Font: 16, W: 800, H: 600, Max: true}); err != nil {
		t.Fatal(err)
	}
	if err := SetStart(dir, StartLast); err != nil {
		t.Fatal(err)
	}
	s := Load(dir)
	if s.Mode() != StartLast || s.Font != 16 || s.W != 800 || !s.Max {
		t.Fatalf("falsch gelesen: %+v", s)
	}
}
