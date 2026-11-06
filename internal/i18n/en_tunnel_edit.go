package i18n

// Tunnel im Manager bearbeiten/löschen (ui/manager.go). Eigene Datei,
// damit parallele Änderungen an en.go nicht kollidieren.
func init() {
	for k, v := range enTunnelEdit {
		en[k] = v
	}
}

var enTunnelEdit = map[string]string{
	" · Klick = bearbeiten/löschen": " · click = edit/delete",
	"Tunnel gelöscht: ":             "Tunnel deleted: ",
}
