package syncx

import (
	"testing"
)

// Die Sync-Datei liegt in einem fremden Ordner (Cloud, WebDAV, USB-Stick):
// kaputte oder manipulierte Dateien müssen sauber abgelehnt werden.
func FuzzDecode(f *testing.F) {
	k, err := NewKey([]byte("ein langes sync passwort"))
	if err != nil {
		f.Fatal(err)
	}
	good, err := Encode(k, &Payload{})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(good)
	f.Add([]byte(`{"format":"sshtermy-sync","version":99}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		_, _ = Decode(k, raw)
		_, _ = ParseKey(string(raw))
	})
}
