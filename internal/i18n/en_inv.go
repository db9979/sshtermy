package i18n

// Inventar & Änderungen, Verlaufskurven (ui/inventory.go, ui/trend.go).
func init() {
	for k, v := range enInv {
		en[k] = v
	}
}

var enInv = map[string]string{
	"Inventar & Änderungen (e = CSV)":              "Inventory & changes (e = CSV)",
	"Verlauf 24 h / 7 Tage":                        "History 24 h / 7 days",
	"Platte %s voll in unter einem Tag":            "Disk %s full in under a day",
	"Platte %s voll in ~%d Tagen":                  "Disk %s full in ~%d days",
	"24 h":                                         "24 h",
	"Verlauf %s · w: %s":                           "History %s · w: %s",
	" max %.0f %%":                                 " max %.0f %%",
	"voll in ~%d T.":                               "full in ~%d d",
	"Inventar geht nur bei Linux-Servern über SSH": "Inventory works only for Linux servers over SSH",
	"Inventar & Änderungen: Ordner wählen":         "Inventory & changes: choose folder",
	"Nur lesend, ohne Rückfragen. Der erste Lauf legt den Stand an, jeder weitere zeigt, was sich geändert hat.": "Read-only, no prompts. The first run records the state, every later run shows what changed.",
	"Admin":            "Admin",
	"Cron":             "Cron",
	"Dienst":           "Service",
	"Kernel":           "Kernel",
	"Änderung":         "change",
	"Änderungen":       "changes",
	"Paket-Update":     "package update",
	"Paket-Updates":    "package updates",
	"keine Änderungen": "no changes",
	"Ports":            "ports",
	"Dienste":          "services",
	"Inventar: ":       "Inventory: ",
	"liest Pakete, Benutzer, Ports, Dienste (nur lesend) …":                            "reading packages, users, ports, services (read-only) …",
	"Erster Stand gespeichert. Ab dem nächsten Mal steht hier, was sich geändert hat.": "First state saved. From next time on, this shows what changed.",
	"Keine Änderungen seit ":                     "No changes since ",
	"Keine Änderungen.":                          "No changes.",
	"Seit %s: %s":                                "Since %s: %s",
	"Frühere Änderungen:":                        "Earlier changes:",
	"  … %d weitere (e speichert alles als CSV)": "  … %d more (e saves everything as CSV)",
	"Admins (sudo/wheel)":                        "Admins (sudo/wheel)",
	"Lauschende Ports":                           "Listening ports",
	"wichtigste":                                 "most important",
	"| Host | Änderungen | wichtigste |\n|---|---|---|\n": "| Host | Changes | Most important |\n|---|---|---|\n",
	"erster Stand gespeichert":                            "first state saved",
	"erster Stand":                                        "first state",
	"Einzelheiten: Tab des Hosts öffnen, F6 › i.":         "Details: open the host's tab, F6 › i.",
	"c kopieren · e als CSV speichern · nur lesend":       "c copy · e save as CSV · read-only",
	"Noch kein Inventar zum Speichern":                    "No inventory to save yet",
	"Gespeichert: ":                                       "Saved: ",
	"Inventar":                                            "Inventory",
	"nicht erfasst · i erfasst (nur lesend)":              "not recorded · i records (read-only)",
	"am": "on",
	"Inventar & Änderungen: alle Server im Ordner …": "Inventory & changes: all servers in folder …",
	"was ist neu?":                       "what's new?",
	"Inventar: letztes Ergebnis":         "Inventory: last result",
	"Inventar & Änderungen dieses Hosts": "Inventory & changes of this host",
	"F6 i":                               "F6 i",
}
