package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/llm"
)

func TestTodayInMaterial(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 15, 0, 0, time.FixedZone("CET", 3600))
	m := material(Context{Host: "econ4", Now: now})
	if !strings.Contains(m, "Heute (Rechner des Admins): 2026-11-10, Dienstag, Zeitzone CET (UTC+01:00)") {
		t.Fatalf("Datum fehlt:\n%s", m)
	}
	if strings.Contains(m, "12:15") {
		t.Fatal("Uhrzeit nicht ins Material, sonst wechselt der Prefix-Cache jede Minute")
	}
	if strings.Contains(material(Context{Host: "x"}), "Heute") {
		t.Fatal("ohne Now keine Datumszeile")
	}
	if !strings.Contains(guard, "ISO 8601") || !strings.Contains(sysAsk, "nie im Landesformat") {
		t.Fatal("Datumsregel muss in allen Prompts stehen")
	}
}

func TestParseCardFix(t *testing.T) {
	failed := `journalctl --since "10.11. 12:00"`
	r, err := parseCardFix(llm.Result{Content: `{"befehl": "journalctl --since \"2026-11-10 12:00\"", "grund": "ISO-Datum"}`}, failed, "")
	if err != nil || r.Command.Command != `journalctl --since "2026-11-10 12:00"` || r.Reason != "ISO-Datum" {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = parseCardFix(llm.Result{Content: `{"befehl": "journalctl  --since \"10.11. 12:00\"", "grund": "x"}`}, failed, "")
	if err != nil || r.Command.Command != "" {
		t.Fatalf("gleicher Befehl ist kein Fix: %+v", r)
	}
	r, err = parseCardFix(llm.Result{Content: `{"keiner": true, "grund": "sudo fehlt"}`}, failed, "")
	if err != nil || r.Command.Command != "" || r.Reason != "sudo fehlt" {
		t.Fatalf("%+v", r)
	}
	if _, err = parseCardFix(llm.Result{Content: "kaputt"}, failed, ""); err == nil {
		t.Fatal("kein JSON = Fehler")
	}
	r, _ = parseCardFix(llm.Result{Content: `{"befehl": "rm -rf /", "grund": "x"}`}, failed, "")
	if len(r.Command.Danger) == 0 {
		t.Fatal("Gefahrenregeln gelten auch für den Fix")
	}
}
