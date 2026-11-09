package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallMacIcon(t *testing.T) {
	d := t.TempDir()
	c := filepath.Join(d, "sshTermy.app", "Contents")
	res := filepath.Join(c, "Resources")
	if err := os.MkdirAll(res, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(res, "sshtermy")
	os.WriteFile(exe, nil, 0o755)
	plist := "<plist version=\"1.0\">\n<dict>\n\t<key>CFBundleName</key><string>sshTermy</string>\n</dict>\n</plist>\n"
	os.WriteFile(filepath.Join(c, "Info.plist"), []byte(plist), 0o644)

	if !installMacIcon(exe, []byte("ICNS")) {
		t.Fatal("erster Aufruf soll das Icon eintragen")
	}
	if b, _ := os.ReadFile(filepath.Join(res, "sshtermy.icns")); string(b) != "ICNS" {
		t.Fatalf("icns = %q", b)
	}
	b, _ := os.ReadFile(filepath.Join(c, "Info.plist"))
	if strings.Count(string(b), "CFBundleIconFile") != 1 || !strings.Contains(string(b), "<string>sshtermy</string>\n</dict>") {
		t.Fatalf("Info.plist:\n%s", b)
	}
	if installMacIcon(exe, []byte("ICNS")) {
		t.Fatal("zweiter Aufruf soll nichts ändern")
	}
	if !installMacIcon(exe, []byte("NEU")) {
		t.Fatal("neues Icon nach Update soll geschrieben werden")
	}

	// außerhalb einer .app: nichts anfassen
	other := filepath.Join(d, "bin", "sshtermy")
	if installMacIcon(other, []byte("ICNS")) {
		t.Fatal("ohne .app darf nichts passieren")
	}
}
