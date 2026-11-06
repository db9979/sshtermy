package i18n

// enArchive: Texte für das Sync-Archiv (Config-Sicherungen und
// Sitzungsprotokolle zwischen Rechnern). Eigene Datei gegen Merge-Konflikte.
var enArchive = map[string]string{
	"%d Protokolle":  "%d session logs",
	"%d Sicherungen": "%d backups",
	" · von ":        " · from ",
	"%d Tage":        "%d days",
	"Archiv":         "Archive",
	"Archivdatei beschädigt oder verändert": "archive file damaged or modified",
	"Aufbewahren": "Keep",
	"Config-Sicherungen und Sitzungsprotokolle mitnehmen (verschlüsselt, eigener Unterordner)": "take config backups and session logs along (encrypted, own subfolder)",
	"Sync-Archiv: ": "Sync archive: ",
	"Sync-Archiv: neu von anderen Rechnern: ":                    "Sync archive: new from other computers: ",
	"so lange bleiben Einträge im Archiv; lokale Kopien bleiben": "how long entries stay in the archive; local copies are kept",
}

func init() {
	for k, v := range enArchive {
		en[k] = v
	}
}
