package debuglog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnOffRotate(t *testing.T) {
	d := t.TempDir()
	if err := Start(d, false); err != nil {
		t.Fatal(err)
	}
	Printf("ssh", "aus: %d", 1)
	if _, err := os.Stat(filepath.Join(d, FileName)); !os.IsNotExist(err) {
		t.Fatal("Datei trotz aus")
	}
	if err := Set(true); err != nil {
		t.Fatal(err)
	}
	defer Set(false)
	Printf("ssh", "zeile\nzwei")
	Err("sync", "hochladen", nil)
	Err("sync", "hochladen", os.ErrPermission)
	s, _ := Read()
	if !strings.Contains(s, "[ssh] zeile ⏎ zwei") || !strings.Contains(s, "[sync] hochladen: permission denied") || strings.Count(s, "\n") != 2 {
		t.Fatalf("%q", s)
	}
	big := strings.Repeat("x", 1<<20)
	for i := 0; i < 6; i++ {
		Printf("t", "%s", big)
	}
	if st, err := os.Stat(filepath.Join(d, FileName+".1")); err != nil || st.Size() > maxSize {
		t.Fatal("keine Rotation", err)
	}
	if err := Clear(); err != nil {
		t.Fatal(err)
	}
	Printf("t", "neu")
	s, _ = Read()
	if !strings.HasSuffix(s, "[t] neu\n") || strings.Contains(s, "xxx") {
		t.Fatalf("nach Clear: %d Bytes", len(s))
	}
}
