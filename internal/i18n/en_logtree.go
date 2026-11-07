package i18n

// Sitzungsprotokolle nach Tagen / nach Host (ui/logtree.go, ui/logs.go).
func init() {
	for k, v := range enLogTree {
		en[k] = v
	}
}

var enLogTree = map[string]string{
	" seit ":                          " since ",
	"%d Hosts":                        "%d hosts",
	"%d Protokolle wirklich löschen?": "Really delete %d logs?",
	"7 Tage":                          "7 days",
	"Alle":                            "All",
	"Bef.":                            "Cmds",
	"Ergebnis":                        "Result",
	"Exportieren geht pro Sitzung: ⏎ klappt die Zeile auf": "Export works per session: ⏎ expands the row",
	"Filter":                           "Filter",
	"Gestern":                          "Yesterday",
	"Gleicher Befehl auf allen Hosts:": "Same command on all hosts:",
	"Heute":                            "Today",
	"Host":                             "Host",
	"Hosts":                            "Hosts",
	"Häufigste Befehle":                "Most used commands",
	"Mit Fehlern":                      "With errors",
	"Sitz.":                            "Sess.",
	"Sitzung %d":                       "Session %d",
	"Tab nach Host":                    "Tab by host",
	"Tab nach Zeit":                    "Tab by time",
	"Zeit":                             "Time",
	"auf/zu":                           "expand",
	"heute":                            "today",
	"keine Befehle":                    "no commands",
	"nach Host":                        "by host",
	"nach Zeit":                        "by time",
	"zuletzt":                          "last",
	"⏎ klappt die Sitzungen auf, dann einzeln ansehen oder exportieren.": "⏎ expands the sessions to view or export them one by one.",
	"◆ KI-Zusammenfassung":            "◆ AI summary",
	"Mo":                              "Mon",
	"Di":                              "Tue",
	"Mi":                              "Wed",
	"Do":                              "Thu",
	"Fr":                              "Fri",
	"Sa":                              "Sat",
	"So":                              "Sun",
	"auf-/zuklappen":                  "expand/collapse",
	"nach Zeit / nach Host":           "by time / by host",
	"Filter: heute · 7 Tage · Fehler": "filter: today · 7 days · errors",
	"Hosts sortieren":                 "sort hosts",
}
