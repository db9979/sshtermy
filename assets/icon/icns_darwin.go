// Package icon enthält das App-Icon für die macOS-App (sshTermy.app).
// Quelle und Neuerzeugung: tools/icon (make icon).
package icon

import _ "embed"

// ICNS ist das Mac-Icon, das sshTermy beim Start in seine .app legt.
//
//go:embed sshtermy.icns
var ICNS []byte
