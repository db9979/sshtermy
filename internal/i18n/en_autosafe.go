package i18n

// Absicherung automatischer Aktionen (Sicherheitsprüfung 2026-11-10)
func init() {
	for k, v := range map[string]string{
		"löscht oder überschreibt Systemdateien":                    "deletes or overwrites system files",
		"verschiebt einen Systemordner":                             "moves a system folder",
		"Firewall sperrt dann alles (auch dich)":                    "firewall then blocks everything (you too)",
		"nimmt das Netzwerk weg (Verbindung bricht ab)":             "takes the network down (connection drops)",
		"löscht Kubernetes-Objekte":                                 "deletes Kubernetes objects",
		"löscht Datenbankinhalte":                                   "deletes database contents",
		"löscht Datenträger oder Volumes":                           "deletes disks or volumes",
		"löscht Docker-Volumes":                                     "deletes Docker volumes",
		"löscht am Ziel, was in der Quelle fehlt":                   "deletes at the target what the source lacks",
		"beendet alle Prozesse":                                     "kills all processes",
		"löscht einen Systemordner":                                 "deletes a system folder",
		"sperrt dich aus (Fernzugang-Dienst)":                       "locks you out (remote access service)",
		"entfernt Windows-Funktionen":                               "removes Windows features",
		"löscht Ereignisprotokolle":                                 "clears event logs",
		"alles außer Gefahr, sudo, root und Code in der Zeile":      "everything except danger, sudo, root and inline code",
		"angemeldet als root/Administrator: Änderungen fragen nach": "signed in as root/administrator: changes ask first",
	} {
		en[k] = v
	}
}
