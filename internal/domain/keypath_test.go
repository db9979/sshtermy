package domain

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestPortableKeyPath(t *testing.T) {
	home := filepath.Join(string(filepath.Separator)+"home", "dom")
	if runtime.GOOS == "windows" {
		home = `C:\Users\dom` // ohne Laufwerk ist ein Pfad unter Windows nicht absolut
	}
	in := filepath.Join(home, ".ssh", "id_ed25519")
	if got := PortableKeyPath(in, home); got != "~/.ssh/id_ed25519" {
		t.Fatalf("PortableKeyPath = %q", got)
	}
	other := filepath.Join(string(filepath.Separator)+"opt", "keys", "k")
	if got := PortableKeyPath(other, home); got != other {
		t.Fatalf("fremder Pfad geändert: %q", got)
	}
	if got := PortableKeyPath(home+"x/k", home); got != home+"x/k" {
		t.Fatalf("Präfix ohne Trenner: %q", got)
	}
	if got := ExpandKeyPath("~/.ssh/id", home); got != filepath.Join(home, ".ssh", "id") {
		t.Fatalf("ExpandKeyPath = %q", got)
	}
}

func TestKeyPathCandidatesForeignHome(t *testing.T) {
	home := filepath.Join(string(filepath.Separator)+"Users", "dom")
	cases := map[string][]string{
		`C:\Users\db9\.ssh\id_ed25519`: {`C:\Users\db9\.ssh\id_ed25519`, filepath.Join(home, ".ssh", "id_ed25519")},
		`C:\Users\db9\AppData\Local\NVIDIA Corporation\Sync\config\nvsync.key`: {
			`C:\Users\db9\AppData\Local\NVIDIA Corporation\Sync\config\nvsync.key`,
			filepath.Join(home, "AppData", "Local", "NVIDIA Corporation", "Sync", "config", "nvsync.key"),
			filepath.Join(home, ".ssh", "nvsync.key"),
		},
		"/home/x/keys/k": {"/home/x/keys/k", filepath.Join(home, "keys", "k"), filepath.Join(home, ".ssh", "k")},
	}
	for in, want := range cases {
		got := KeyPathCandidates(in, home)
		if len(got) != len(want) {
			t.Fatalf("%s: %q", in, got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: %q, erwartet %q", in, got, want)
			}
		}
	}
}
