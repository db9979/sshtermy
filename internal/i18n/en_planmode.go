package i18n

// Plan-Modus (ui/planmode.go). Eigene Datei gegen Merge-Konflikte in en.go.
func init() {
	for k, v := range enPlanMode {
		en[k] = v
	}
}

var enPlanMode = map[string]string{
	"  erst planen, dann ausführen": "  plan first, then run",
	" (gestrichene Schritte: ":      " (removed steps: ",
	"%d von %d Schritten aktiv":     "%d of %d steps active",
	"AUSFÜHREN ALS":                 "RUN AS",
	"Ausführung":                    "run as",
	"Befehl für Schritt %d":         "Command for step %d",
	"Datei %s (%d Zeilen)":          "File %s (%d lines)",
	"Datei ansehen":                 "view file",
	"Der Plan hat keine Schritte":   "The plan has no steps",
	"Die KI sieht sich mit lesenden Befehlen um und schreibt einen vollständigen Plan: ": "The AI looks around with read-only commands and writes a complete plan: ",
	"Erst planen": "Plan first",
	"Leertaste":   "Space",
	"PRÜFUNG":     "CHECK",
	"Plan fertig: bitte prüfen und freigeben": "Plan ready: please review and approve",
	"Plan ändern":    "Change plan",
	"RISIKEN":        "RISKS",
	"SCHRITTE":       "STEPS",
	"Schritt ändern": "Change step",
	"Schritte, Skripte, Risiken und Prüfung. Du streichst oder änderst Schritte, lässt neu planen und gibst ihn frei.": "steps, scripts, risks and checks. You remove or change steps, have it re-planned and approve it.",
	"Was soll am Plan anders werden? Die KI plant danach neu.":                                                         "What should change in the plan? The AI re-plans afterwards.",
	"erkundet": "exploring",
	"erkundet nur lesend, du gibst den Plan frei": "explores read-only, you approve the plan",
	"freigeben":                   "approve",
	"freigeben und ausführen":     "approve and run",
	"nicht im freigegebenen Plan": "not in the approved plan",
	"Plan: Schritt streichen":     "plan: remove step",
	"Plan: neu planen lassen":     "plan: re-plan",
	"neu planen":                  "re-plan",
	"nicht ausgeführt: beim Planen nur lesende Befehle": "not run: only read-only commands while planning",
	"nur lesende Befehle · %d von höchstens %d":         "read-only commands · %d of at most %d",
	"streichen":                 "remove",
	"verwerfen":                 "discard",
	"von dir geändert":          "changed by you",
	"Änderungswunsch am Plan: ": "Requested change to the plan: ",
	"◇ PLAN":                    "◇ PLAN",
	"◇ Plan":                    "◇ Plan",
	"◇ Plan-Modus":              "◇ Plan mode",
	"◇ Plan-Modus ":             "◇ Plan mode ",
}
