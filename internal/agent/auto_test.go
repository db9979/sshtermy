package agent

import (
	"errors"
	"strings"
	"testing"
)

func TestParseAutoFile(t *testing.T) {
	out := `{"datei": {"name": "rotate.sh", "inhalt": "#!/usr/bin/env bash\nset -euo pipefail\necho hi\u001b[31m"}, "grund": "Skript anlegen"}`
	st, err := ParseAuto(out, "unix", true)
	if err != nil || st.File == nil {
		t.Fatalf("Datei erwartet: %v %+v", err, st)
	}
	if st.File.Name != "rotate.sh" || strings.Contains(st.File.Content, "\x1b") || !strings.HasSuffix(st.File.Content, "\n") {
		t.Errorf("Inhalt nicht bereinigt: %q", st.File.Content)
	}
	if st.Reason != "Skript anlegen" || len(st.Danger) != 0 {
		t.Errorf("%+v", st)
	}
	// ohne Dateirecht: datei wird nicht ausgeführt, sondern ist unlesbar
	if _, err := ParseAuto(out, "unix", false); !errors.Is(err, ErrUnreadable) {
		t.Errorf("ohne Dateien erwartet ErrUnreadable, bekam %v", err)
	}
}

func TestParseAutoFileDanger(t *testing.T) {
	out := `{"datei": {"name": "clean.sh", "inhalt": "# rm -rf / nur Kommentar\nsudo rm -rf /var/log/app/old\n"}}`
	st, err := ParseAuto(out, "unix", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Danger) != 1 || !st.Elevated {
		t.Errorf("Gefahr im Skript erwartet: %+v", st)
	}
}

func TestCheckFileName(t *testing.T) {
	for _, n := range []string{"../x.sh", "/etc/passwd", "a/b", "", ".hidden", strings.Repeat("a", 70)} {
		if _, err := CheckFile(n, "echo\n"); err == nil {
			t.Errorf("Name %q müsste abgelehnt werden", n)
		}
	}
	if _, err := CheckFile("ok.sh", strings.Repeat("x", 40<<10)); err == nil {
		t.Error("zu große Datei müsste abgelehnt werden")
	}
}

func TestParseAutoDoneLong(t *testing.T) {
	long := strings.Repeat("Logrotation eingerichtet und geprüft. ", 8)
	st, err := ParseAuto(`{"fertig": true, "grund": "`+long+`"}`, "unix", true)
	if err != nil || !st.Done {
		t.Fatal(err)
	}
	if len([]rune(st.Reason)) <= maxReason {
		t.Errorf("Zusammenfassung zu stark gekürzt: %d", len([]rune(st.Reason)))
	}
}

func TestParseAutoCommand(t *testing.T) {
	st, err := ParseAuto(`{"befehl": "rm -rf /tmp/x", "grund": "aufräumen", "plan": ["a"]}`, "unix", true)
	if err != nil || st.Command != "rm -rf /tmp/x" || len(st.Danger) == 0 {
		t.Fatalf("%v %+v", err, st)
	}
}

func TestDoneStepStatus(t *testing.T) {
	c := 2
	cases := map[string]DoneStep{
		"vom Menschen übersprungen": {Skipped: true},
		"Datei angelegt":            {File: "x.sh"},
		"fehlgeschlagen: Exit 1":    {ExitHint: "Exit 1"},
		"Exit 2":                    {Exit: &c},
		"ausgeführt":                {},
	}
	for want, d := range cases {
		if got := d.status(); got != want {
			t.Errorf("%+v: %q statt %q", d, got, want)
		}
	}
}

// Die Abschlusszusammenfassung kommt vollständig an (früher nach 600 Zeichen
// „Server ist…“), Absätze bleiben, im Ziel-Modus galt sogar nur 160.
func TestDoneSummaryComplete(t *testing.T) {
	long := strings.Repeat("Cron-Jobs sind nur Standard (e2scrub, popularity-contest). ", 20) + "Server ist sauber."
	in := `{"fertig": true, "grund": "Absatz eins.\n\n\n` + long + `"}`
	for _, parse := range []func(string) (Step, error){
		func(s string) (Step, error) { return ParseAuto(s, "unix", true) },
		func(s string) (Step, error) { return ParseStep(s, "Ubuntu") },
	} {
		st, err := parse(in)
		if err != nil || !st.Done {
			t.Fatal(err)
		}
		if !strings.HasSuffix(st.Reason, "Server ist sauber.") {
			t.Errorf("Ende fehlt: …%q", tail(st.Reason, 40))
		}
		if !strings.HasPrefix(st.Reason, "Absatz eins.\n\nCron") {
			t.Errorf("Absätze nicht erhalten: %q", short(st.Reason, 40))
		}
	}
}

// Abgeschnittene Abschlussmeldung (Token-Limit): Text retten statt Fehler.
func TestDoneSummarySalvaged(t *testing.T) {
	st, err := ParseAuto(`{"fertig": true, "grund": "Logrotation eingerichtet.\nGeprüft mit logrotate -d, alles in Ord`, "unix", true)
	if err != nil || !st.Done {
		t.Fatalf("abgeschnittene Zusammenfassung nicht gerettet: %v", err)
	}
	if !strings.Contains(st.Reason, "eingerichtet.\nGeprüft") || !strings.HasSuffix(st.Reason, "…") {
		t.Errorf("gerettet: %q", st.Reason)
	}
	// ein abgeschnittener Befehl wird weiterhin nie ergänzt
	if _, err := ParseAuto(`{"befehl": "rm -rf /var/log/old`, "unix", true); err == nil {
		t.Error("abgeschnittener Befehl darf nicht durchgehen")
	}
}

func TestPlanSummaryComplete(t *testing.T) {
	long := strings.Repeat("Erst sichern, dann ändern. ", 40) + "ENDE"
	st, err := ParsePlan(`{"planfertig": {"zusammenfassung": "`+long+`", "schritte": [{"befehl": "df -h", "grund": "Platz prüfen"}]}}`, "unix", true)
	if err != nil || st.Full == nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(st.Full.Summary, "ENDE") {
		t.Errorf("Plan-Zusammenfassung gekürzt: …%q", tail(st.Full.Summary, 30))
	}
}
