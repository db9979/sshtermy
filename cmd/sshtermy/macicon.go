package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// installMacIcon legt icns nach <App>.app/Contents/Resources/sshtermy.icns und
// trägt CFBundleIconFile in Info.plist ein, wenn exe in einer .app liegt
// (install.sh: Contents/Resources/sshtermy). Meldet, ob sich etwas geändert hat.
func installMacIcon(exe string, icns []byte) bool {
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	res := filepath.Dir(exe)
	contents := filepath.Dir(res)
	app := filepath.Dir(contents)
	if filepath.Base(res) != "Resources" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(app, ".app") {
		return false
	}
	changed := false
	dst := filepath.Join(res, "sshtermy.icns")
	if old, err := os.ReadFile(dst); err != nil || !bytes.Equal(old, icns) {
		if os.WriteFile(dst, icns, 0o644) == nil {
			changed = true
		}
	}
	plist := filepath.Join(contents, "Info.plist")
	if b, err := os.ReadFile(plist); err == nil && !bytes.Contains(b, []byte("CFBundleIconFile")) {
		s := strings.Replace(string(b), "</dict>", "\t<key>CFBundleIconFile</key><string>sshtermy</string>\n</dict>", 1)
		if os.WriteFile(plist, []byte(s), 0o644) == nil {
			changed = true
		}
	}
	if changed { // Finder liest das Icon neu, wenn sich die App ändert
		now := time.Now()
		os.Chtimes(app, now, now)
	}
	return changed
}
