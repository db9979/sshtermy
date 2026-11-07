package i18n

// Meldungen aus verdeckten Tabs, OSC 52, Bildschirmsperre, Agent-Weiterleitung
// (ui/notify.go, ui/lock.go, sshx/agentfwd.go).
func init() {
	for k, v := range enNotifyLock {
		en[k] = v
	}
}

var enNotifyLock = map[string]string{
	" im Hintergrund weiter.": " in the background.",
	"Agent weiterreichen":     "Forward agent",
	"Agent-Weiterleitung vom Server abgelehnt (AllowAgentForwarding no?)": "Agent forwarding refused by the server (AllowAgentForwarding no?)",
	"Agent-Weiterleitung: ": "Agent forwarding: ",
	"Agent-Weiterleitung: kein SSH-Agent erreichbar (ssh-agent, Pageant oder Windows-OpenSSH-Agent starten)": "Agent forwarding: no SSH agent reachable (start ssh-agent, Pageant or the Windows OpenSSH agent)",
	"Automatisch sperren bei Leerlauf": "Lock automatically when idle",
	"Befehl ":                          "Command ",
	"Bildschirm gesperrt.":             "Screen locked.",
	"Desktop-Benachrichtigung, wenn sshTermy im Hintergrund ist":    "Desktop notification while sshTermy is in the background",
	"Desktop-Benachrichtigungen sind an":                            "Desktop notifications are on",
	"Falsches Passwort":                                             "Wrong password",
	"Feld leeren":                                                   "clear field",
	"Glocke (BEL) im Terminal":                                      "Bell (BEL) in the terminal",
	"Jetzt sperren":                                                 "Lock now",
	"Meldung, wenn lange Befehle im Hintergrund fertig sind":        "Notify when long commands finish in the background",
	"Server darf in die Zwischenablage kopieren (OSC 52)":           "Server may copy to the clipboard (OSC 52)",
	"Sitzung läuft":                                                 "session keeps running",
	"Sitzungen laufen":                                              "sessions keep running",
	"So sieht eine Meldung aus, wenn ein langer Befehl fertig ist.": "This is how it looks when a long command has finished.",
	"Vom Server kopiert: ":                                          "Copied from server: ",
	"entsperren":                                                    "unlock",
	"gesperrt":                                                      "locked",
	"nach %d min":                                                   "after %d min",
	"wie ssh -A: git, ssh, scp auf dem Server mit deinen lokalen Keys · nur bei vertrauten Servern": "like ssh -A: git, ssh, scp on the server with your local keys · trusted servers only",
	"⠿ Passwort wird geprüft …": "⠿ Checking password …",
}
