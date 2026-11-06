package i18n

// PuTTY-Keys (sshx/ppk.go) und Tabs nach dem Update (ui/restore.go).
// Eigene Datei, damit parallele Änderungen an en.go nicht kollidieren.
func init() {
	for k, v := range enPPK {
		en[k] = v
	}
}

var enPPK = map[string]string{
	"PuTTY-Key unlesbar (%s)": "PuTTY key unreadable (%s)",
	"Zeilenzahl":              "line count",
	"Base64":                  "Base64",
	"nur Version 2 und 3":     "only version 2 and 3",
	"unvollständig":           "incomplete",
	"PuTTY-Key: Verschlüsselung %s wird nicht unterstützt":                                     "PuTTY key: encryption %s is not supported",
	"PuTTY-Key unlesbar (Argon2-Angaben fehlen)":                                               "PuTTY key unreadable (Argon2 parameters missing)",
	"PuTTY-Key: Verfahren %s wird nicht unterstützt, bitte in PuTTYgen mit Argon2id speichern": "PuTTY key: %s is not supported, please save it with Argon2id in PuTTYgen",
	"PuTTY-Key unlesbar (Länge)":                                                               "PuTTY key unreadable (length)",
	"PuTTY-Key beschädigt (Prüfsumme falsch)":                                                  "PuTTY key damaged (checksum wrong)",
	"PuTTY-Key unlesbar (Inhalt)":                                                              "PuTTY key unreadable (content)",
	"PuTTY-Key: Typ %s wird nicht unterstützt (bitte ed25519, RSA oder ECDSA)":                 "PuTTY key: type %s is not supported (please use ed25519, RSA or ECDSA)",
	"Das ist kein privater SSH-Key (OpenSSH-, PEM- oder PuTTY-Format)":                         "This is not a private SSH key (OpenSSH, PEM or PuTTY format)",
	"Sitzung":                             "session",
	"Sitzungen":                           "sessions",
	" von vor dem Update wieder geöffnet": " from before the update reopened",
	"  Die Tabs öffnen sich danach wieder, tmux/screen wird wieder angehängt.\n\n": "  The tabs reopen afterwards, tmux/screen is reattached.\n\n",
	"  (laufende Befehle, Tunnel und Dateiübertragungen brechen ab).\n":            "  (running commands, tunnels and file transfers are aborted).\n",
}
