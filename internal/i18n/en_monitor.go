package i18n

// enMonitor: Texte für Überwachung (Grenzwerte), Befehls-Favoriten aus dem
// Verlauf und KI-Lernen pro Gerät.
var enMonitor = map[string]string{
	" (ersetzt „%s“, am seltensten benutzt)": " (replaces “%s”, used least)",
	" (frei)":                          " (free)",
	" (so hast du es hier korrigiert)": " (as you corrected it here)",
	" (statt „%s“)":                    " (instead of “%s”)",
	" · automatische Prüfung aus":      " · automatic check off",
	" · g ändern":                      " · g to change",
	" · prüft alle %d Min":             " · checks every %d min",
	"%s %d %% (Grenze %d %%)":          "%s %d%% (limit %d%%)",
	"Auslastung über eine Sekunde gemessen · Standard 90": "load measured over one second · default 90",
	"CPU ab %": "CPU from %",
	"gilt für jede Platte einzeln · Standard 90": "applies to each disk separately · default 90",
	"Platten":                  "Disks",
	"+%d weitere":              "+%d more",
	"+%d weitere Platten · F6": "+%d more disks · F6",
	"Gemerkt für %s: „%s“ statt „%s“ · die KI schlägt das künftig so vor": "Learned for %s: “%s” instead of “%s” · the AI will suggest it this way from now on",
	"Grenzwerte für Warnungen":        "Warning limits",
	"KI-Befehl korrigiert":            "AI command corrected",
	"Korrektur wird gemerkt":          "correction is remembered",
	"F9 → a":                          "F9 → a",
	"häufigen Befehl übernehmen":      "adopt a frequent command",
	"Grenzwerte":                      "Limits",
	"Grenzwerte zwischen 1 und 100 %": "Limits between 1 and 100%",
	"Höchstens alle 1440 Minuten":     "At most every 1440 minutes",
	"Korrektur":                       "correction",
	"Korrekturen":                     "corrections",
	"Korrigiert":                      "Corrected",
	"Platte ab %":                     "Disk from %",
	"Prüfen alle (Min)":               "Check every (min)",
	"RAM ab %":                        "RAM from %",
	"Vorschläge aus deinem Verlauf":   "Suggestions from your history",
	"Warnung ab CPU %d %% · RAM %d %% · Platte %d %%":      "Warning from CPU %d%% · RAM %d%% · disk %d%%",
	"Warnung ab CPU %d %% · RAM %d %% · nur auf Abruf (⏎)": "Warning from CPU %d%% · RAM %d%% · on request only (⏎)",
	"a legt ihn auf %s": "a puts it on %s",
	"als Schnellbefehl": "as quick command",
	"belegter Arbeitsspeicher (ohne Cache) · Standard 90": "used memory (without cache) · default 90",
	"nicht vorschlagen": "don't suggest",
	"nur Server, unsichtbar · 0 = aus · Switches nur auf Abruf": "servers only, invisible · 0 = off · switches on request only",
	"Überwachung: Grenzwerte":                                   "Monitoring: limits",
	"„%s“ lief schon %d× · %s, dann a: als Schnellbefehl":       "“%s” has run %d× · %s, then a: make it a quick command",
	"„%s“ liegt jetzt auf %s":                                   "“%s” is now on %s",
}

func init() {
	for k, v := range enMonitor {
		en[k] = v
	}
}
