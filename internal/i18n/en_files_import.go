package i18n

// Eingabe an alle Bereiche, SSH-Zertifikate, trzsz, Import Ansible/CSV
// (ui/syncinput.go, sshx/cert.go, ui/trzsz.go, importer/ansible.go, csv.go).
func init() {
	for k, v := range enFilesImport {
		en[k] = v
	}
}

var enFilesImport = map[string]string{
	" Nichts gefunden. f = Datei laden (PuTTY-.reg-Export, MobaXterm.ini, .mxtsessions, ssh_config, Ansible-Inventar oder CSV, auch NetBox-Export).\n": " Nothing found. f = load file (PuTTY .reg export, MobaXterm.ini, .mxtsessions, ssh_config, Ansible inventory or CSV, also NetBox export).\n",
	" schaltet ab)":                     " turns it off)",
	"%s exportiert (ohne Passwörter): ": "%s exported (without passwords): ",
	". Nirgends ausgeführt; Eingabe an alle abschalten und einzeln bestätigen.":       ". Not run anywhere; turn off typing to all and confirm one by one.",
	"Bei Eingabe an alle geht nur einzeiliges, harmloses Einfügen. Nichts eingefügt.": "While typing to all, only single-line, harmless pastes work. Nothing pasted.",
	"CSV ohne Spalte für Host oder Name (z. B. „host“, „ip“, „name“)":                 "CSV without a column for host or name (e.g. “host”, “ip”, “name”)",
	"Das ist kein SSH-Zertifikat (*-cert.pub)":                                        "This is not an SSH certificate (*-cert.pub)",
	"Datei hochladen (trz)":                                                                    "Upload file (trz)",
	"Datei vom Server wird übertragen (tsz) nach ":                                             "Transferring file from the server (tsz) to ",
	"Eingabe an alle Bereiche: an. Was du tippst, geht an jeden sichtbaren Host.":              "Typing to all panes: on. What you type goes to every visible host.",
	"Eingabe an alle Bereiche: aus":                                                            "Typing to all panes: off",
	"Eingabe an alle geteilten Bereiche":                                                       "Type into all split panes",
	"Eingabe an alle geteilten Bereiche: ausschalten":                                          "Type into all split panes: turn off",
	"Eingabe an alle geteilten Bereiche: einschalten":                                          "Type into all split panes: turn on",
	"Eingabe an alle gibt es nur in der geteilten Ansicht (":                                   "Typing to all only works in split view (",
	"Export fehlgeschlagen: ":                                                                  "Export failed: ",
	"Heruntergeladen: ":                                                                        "Downloaded: ",
	"Hochgeladen: ":                                                                            "Uploaded: ",
	"Keine Verbindungen zum Exportieren":                                                       "No connections to export",
	"Netzwerkgerät (%s)":                                                                       "Network device (%s)",
	"Ordner per trz -d geht nicht; Dateien einzeln mit trz oder Ordner mit F5 (SFTP)":          "Folders via trz -d are not supported; send files one by one with trz or folders with F5 (SFTP)",
	"Passwort aus dem Inventar nicht übernommen, wird beim Verbinden gefragt":                  "Password from the inventory not imported, you will be asked when connecting",
	"Pfad der Datei auf diesem Rechner; mehrere mit ; trennen. Esc bricht ab.":                 "Path of the file on this computer; separate several with ; . Esc cancels.",
	"Pfad: ~/.ssh/config, putty.reg, MobaXterm.ini, Ansible-Inventar, *.csv":                   "Path: ~/.ssh/config, putty.reg, MobaXterm.ini, Ansible inventory, *.csv",
	"Verbindungen als CSV exportieren (ohne Passwörter)":                                       "Export connections as CSV (without passwords)",
	"Verbindungen importieren (ssh-config, PuTTY, MobaXterm, Ansible, CSV)":                    "Import connections (ssh-config, PuTTY, MobaXterm, Ansible, CSV)",
	"Vorschläge gelten je Host, bei Eingabe an alle nicht (":                                   "Suggestions are per host, not while typing to all (",
	"ZMODEM (sz/rz) kann sshTermy nicht; auf dem Server trzsz nutzen (tsz/trz) oder F5 (SFTP)": "sshTermy does not speak ZMODEM (sz/rz); use trzsz on the server (tsz/trz) or F5 (SFTP)",
	"Zertifikat passt nicht zum Key: %w":                                                       "Certificate does not match the key: %w",
	"Zertifikat: %w":                                                                           "Certificate: %w",
	"abgelaufen am %s, bitte neu signieren lassen":                                             "expired on %s, please get it signed again",
	"als CSV exportieren":                                                                      "export as CSV",
	"erst gültig ab %s":                                                                        "not valid before %s",
	"optional, von einer CA signiert · leer = <Keyfile>-cert.pub, falls vorhanden":             "optional, signed by a CA · empty = <keyfile>-cert.pub if present",
	"Übertragung abgebrochen":                                                                  "Transfer cancelled",
	"Übertragung fehlgeschlagen: ":                                                             "Transfer failed: ",
	"◈ alle":                                                                                   "◈ all",
}
