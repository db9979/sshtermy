package i18n

// Texte für die Container-Ansicht.
var enContainers = map[string]string{
	" · Tipp: Benutzer in die Gruppe docker aufnehmen oder sudo ohne Passwort erlauben": " · Tip: add the user to the docker group or allow sudo without a password",
	"Container":                              "Containers",
	"Container (Docker, Podman, Kubernetes)": "Containers (Docker, Podman, Kubernetes)",
	"Container (Docker, Podman, k8s)":        "Containers (Docker, Podman, k8s)",
	"Container auf %s":                       "Containers on %s",
	"Container gibt es nur auf verbundenen Linux-Servern (SSH)": "Containers are only available on connected Linux servers (SSH)",
	"Container: ":                              "Containers: ",
	"Der Container hat keine Logs":             "The container has no logs",
	"Eingefügt · ⏎ führt aus, Strg+C verwirft": "Inserted · ⏎ runs it, Ctrl+C discards it",
	"Hole Logs von %s …":                       "Fetching logs of %s …",
	"KI wertet Logs aus":                       "AI analyzes the logs",
	"Kein docker, podman oder kubectl auf ":    "No docker, podman or kubectl on ",
	"Keine Container auf ":                     "No containers on ",
	"Keine Container lesbar: ":                 "Cannot read containers: ",
	"Logs laufen · Strg+C beendet":             "Logs running · Ctrl+C stops",
	"Logs live":                                "live logs",
	"Logs von %s (%s)":                         "Logs of %s (%s)",
	"Logs: ":                                   "Logs: ",
	"Name, Image, Namespace":                   "Name, image, namespace",
	"Neustart einfügen":                        "insert restart",
	"Pods startet und stoppt Kubernetes selbst · Strg+R löscht den Pod, der Controller legt ihn neu an": "Kubernetes starts and stops pods itself · Ctrl+R deletes the pod, the controller creates it again",
	"Shell im Container":                          "shell in the container",
	"Start/Stopp einfügen":                        "insert start/stop",
	"Suche Container (docker, podman, kubectl) …": "Looking for containers (docker, podman, kubectl) …",
	"● läuft · ○ gestoppt. Neustart und Start/Stopp werden nur eingefügt, ⏎ im Terminal führt sie aus.": "● running · ○ stopped. Restart and start/stop are only inserted, ⏎ in the terminal runs them.",
}

func init() {
	for k, v := range enContainers {
		en[k] = v
	}
}
