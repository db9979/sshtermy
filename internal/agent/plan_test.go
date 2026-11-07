package agent

import (
	"errors"
	"testing"
)

const planJSON = `{"planfertig": {"zusammenfassung": "Logs komprimieren", "schritte": [
 {"befehl": "du -sh /var/log/app", "grund": "Größe sehen"},
 {"datei": {"name": "rotate.sh", "inhalt": "#!/bin/sh\ngzip -f /var/log/app/*.1\n"}, "grund": "Skript"},
 {"befehl": "bash ~/.sshtermy-agent/x/rotate.sh", "grund": "ausführen"},
 {"befehl": "rm -f /var/log/app/old-*.gz", "grund": "alte Archive weg"}],
 "risiken": ["Logs fehlen danach für Analysen"], "pruefung": ["du -sh /var/log/app"]}}`

func TestParsePlanFull(t *testing.T) {
	st, err := ParsePlan(planJSON, "unix", true)
	if err != nil || st.Full == nil {
		t.Fatalf("Plan erwartet: %v %+v", err, st)
	}
	p := st.Full
	if len(p.Steps) != 4 || p.Steps[1].File == nil || p.Steps[1].File.Name != "rotate.sh" {
		t.Fatalf("Schritte: %+v", p.Steps)
	}
	if len(p.Steps[3].Danger) == 0 {
		t.Error("rm -f muss als Gefahr markiert sein")
	}
	if len(p.Risks) != 1 || len(p.Checks) != 1 || p.Summary != "Logs komprimieren" {
		t.Errorf("%+v", p)
	}
	if p.Steps[1].Label() != "Datei rotate.sh" || p.Steps[0].Label() != "du -sh /var/log/app" {
		t.Error("Label")
	}
	if txt := p.text(); txt == "" {
		t.Error("Plantext leer")
	}
}

func TestParsePlanNoFiles(t *testing.T) {
	if _, err := ParsePlan(planJSON, "unix", false); !errors.Is(err, ErrUnreadable) {
		t.Errorf("Plan mit Datei ohne Dateirecht muss abgelehnt werden: %v", err)
	}
}

func TestParsePlanExplore(t *testing.T) {
	st, err := ParsePlan(`{"befehl": "ls /etc", "grund": "schauen"}`, "unix", true)
	if err != nil || st.Command != "ls /etc" || st.Full != nil {
		t.Fatalf("%v %+v", err, st)
	}
	st, err = ParsePlan(`{"fertig": true, "grund": "schon erledigt"}`, "unix", true)
	if err != nil || st.Full == nil || st.Full.Summary != "schon erledigt" || len(st.Full.Steps) != 0 {
		t.Fatalf("fertig ⇒ leerer Plan: %v %+v", err, st)
	}
}

func TestParsePlanBrokenStep(t *testing.T) {
	bad := `{"planfertig": {"schritte": [{"befehl": "a\nb"}]}}`
	if _, err := ParsePlan(bad, "unix", true); err == nil {
		t.Error("mehrzeiliger Befehl muss den Plan verwerfen")
	}
}
