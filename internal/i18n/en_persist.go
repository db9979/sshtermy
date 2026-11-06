package i18n

// Feste Sitzung (ui/persist.go). Eigene Datei, damit parallele Änderungen
// an en.go nicht kollidieren.
func init() {
	for k, v := range enPersist {
		en[k] = v
	}
}

var enPersist = map[string]string{
	" · zuschauen":                           " · watching",
	" (nur zuschauen)":                       " (view only)",
	": welche Sitzung?":                      ": which session?",
	"Befehl zum Mitschauen kopieren":         "Copy command to watch along",
	"Esc = normale Shell":                    "Esc = plain shell",
	"Feste Sitzung":                          "Persistent session",
	"Feste Sitzung in zweitem Tab zuschauen": "Watch persistent session in a second tab",
	"Feste Sitzung: ":                        "Persistent session: ",
	"Feste Sitzung: %s fehlt auf dem Server (z. B. sudo apt install %s)":                 "Persistent session: %s is missing on the server (e.g. sudo apt install %s)",
	"Feste Sitzung: Server ließ sich nicht abfragen (":                                   "Persistent session: could not query the server (",
	"Feste Sitzung: nur zuschauen geht nur mit tmux":                                     "Persistent session: view only needs tmux",
	"Feste Sitzung: weder tmux noch screen auf dem Server (z. B. sudo apt install tmux)": "Persistent session: neither tmux nor screen on the server (e.g. sudo apt install tmux)",
	"Feste Sitzung: „%s“ läuft noch nicht, zum Zuschauen muss sie schon offen sein":      "Persistent session: “%s” is not running yet, it must be open to watch it",
	"Neue Sitzung „%s“ anlegen":                                                          "Create new session “%s”",
	"Normale Shell (ohne ":                                                               "Plain shell (without ",
	"Nur zuschauen":                                                                      "View only",
	"Sitzung suchen":                                                                     "Search sessions",
	"Sitzungsname":                                                                       "Session name",
	"beim Verbinden immer in dieselbe tmux-/screen-Sitzung, auch von anderen Rechnern":   "always land in the same tmux/screen session on connect, even from other computers",
	"frei":               "detached",
	"für andere Rechner": "for other computers",
	"gerade angehängt":   "attached",
	"leer = „sshtermy“ · gleicher Name auf zwei Rechnern = derselbe Bildschirm": "empty = “sshtermy” · same name on two computers = same screen",
	"nur tmux: mitlesen, was in der Sitzung passiert, ohne mitzutippen":         "tmux only: follow what happens in the session without typing",
}
