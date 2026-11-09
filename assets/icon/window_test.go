package icon

import "testing"

func TestWindow(t *testing.T) {
	ims := Window()
	if len(ims) != 4 {
		t.Fatalf("%d Bilder, erwartet 4", len(ims))
	}
	for i, want := range []int{16, 32, 48, 256} {
		if b := ims[i].Bounds(); b.Dx() != want || b.Dy() != want {
			t.Errorf("Bild %d: %v, erwartet %d px", i, b, want)
		}
	}
}
