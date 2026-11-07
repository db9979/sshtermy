package i18n

// Texte zum Team-Ordner (veröffentlichen/abonnieren). Eigene Datei, damit
// parallele Änderungen an en.go nicht kollidieren.
func init() {
	for k, v := range enTeamAbo {
		en[k] = v
	}
}

var enTeamAbo = map[string]string{
	"\nSag den Kollegen den Ort der Datei und das Passwort (getrennt).\nSie übernehmen sie mit Strg+Leertaste → „Team-Paket übernehmen“ und „Abonnieren“.": "\nTell your colleagues where the file is and the password (separately).\nThey import it with Ctrl+Space → “Import team package” and “Subscribe”.",
	" · abonniert":              " · subscribed",
	"%d Verbindungen · in „%s“": "%d connections · in “%s”",
	"Abo":                       "Subscription",
	"Abo beenden":               "End subscription",
	"Abo beenden?":              "End subscription?",
	"Abonnieren":                "Subscribe",
	"Aktuell halten":            "Keep up to date",
	"Automatisch aktuell: Änderungen an diesen Verbindungen schreibt sshTermy in die Datei (Strg+Leertaste → „Team-Ordner“).\n": "Kept up to date: sshTermy writes changes to these connections into the file (Ctrl+Space → “Team folders”).\n",
	"Datei alle 5 min neu lesen und Änderungen des Teams automatisch übernehmen":                                                "re-read the file every 5 min and apply the team's changes automatically",
	"Datei bleibt liegen":        "file stays in place",
	"Jetzt abgleichen":           "Check now",
	"Jetzt veröffentlichen":      "Publish now",
	"Nicht mehr veröffentlichen": "Stop publishing",
	"Pfad zur .sshteam-Datei oder https://…/x.sshteam (Einfügen mit Strg+V)": "path to the .sshteam file or https://…/x.sshteam (paste with Ctrl+V)",
	"Team-Ordner": "Team folders",
	"Team-Ordner (veröffentlicht / abonniert)":                        "Team folders (published / subscribed)",
	"Team-Ordner beenden?":                                            "Stop this team folder?",
	"Team-Ordner „%s“ beendet":                                        "Team folder “%s” stopped",
	"Team-Ordner „%s“ veröffentlicht: %s":                             "Team folder “%s” published: %s",
	"Team-Ordner „%s“: %d neu, %d aktualisiert, %d entfernt":          "Team folder “%s”: %d new, %d updated, %d removed",
	"Team-Ordner „%s“: %s":                                            "Team folder “%s”: %s",
	"Team-Ordner „%s“: keine Änderungen":                              "Team folder “%s”: no changes",
	"Team-Ordner: sshTermy schreibt die Datei bei jeder Änderung neu": "team folder: sshTermy rewrites the file on every change",
	"Verbindungen bleiben":                                            "connections stay",
	"Veröffentlicht":                                                  "Published",
	"einzeln gewählt":                                                 "picked one by one",
	"kein Team-Paket an diesem Ort gefunden":                          "no team package found at this location",
	"z. B. in einen gemeinsamen Ordner (Teams, OneDrive, NAS) oder https://…/x.sshteam (WebDAV)":                                  "e.g. a shared folder (Teams, OneDrive, NAS) or https://…/x.sshteam (WebDAV)",
	"Änderungen aus „%s“ kommen nicht mehr an. Die übernommenen Verbindungen bleiben und gehören ab jetzt dir.":                   "Changes from “%s” no longer arrive. The imported connections stay and are yours from now on.",
	"„%s“ wird nicht mehr aktualisiert. Die Datei %s bleibt liegen; lösche sie dort, wenn das Team sie nicht mehr bekommen soll.": "“%s” is no longer updated. The file %s stays in place; delete it there if the team should no longer get it.",
	"Noch keine Team-Ordner.\n\nVeröffentlichen: Strg+Leertaste → „Mit dem Team teilen“, als Datei einen gemeinsamen Ordner wählen und „Aktuell halten“ einschalten.\n\nAbonnieren: Strg+Leertaste → „Team-Paket übernehmen“ mit „Abonnieren“. sshTermy liest die Datei dann alle 5 Minuten und übernimmt Änderungen.": "No team folders yet.\n\nPublish: Ctrl+Space → “Share with the team”, choose a file in a shared folder and turn on “Keep up to date”.\n\nSubscribe: Ctrl+Space → “Import team package” with “Subscribe”. sshTermy then re-reads the file every 5 minutes and applies changes.",
}
