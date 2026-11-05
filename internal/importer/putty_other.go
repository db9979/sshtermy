//go:build !windows

package importer

// Ohne Windows gibt es keine PuTTY-Registry; ein .reg-Export lässt sich
// über „Datei importieren“ einlesen.
func scanPuTTY() Result {
	return Result{Source: SrcPuTTY, Path: `HKCU\` + puttyKey, Err: ErrNotFound}
}
