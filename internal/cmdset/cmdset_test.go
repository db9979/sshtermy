package cmdset_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/cmdset"
	_ "github.com/db9979/sshtermy/internal/safety" // trägt die Nur-lesen-Prüfung ein
)

const ownYAML = `id: acme-os
name: ACME OS
detect:
  version: ['(?i)acme os']
commands:
  version: show version
  config: show running-config
syslog:
  steps: [configure terminal, 'logging host {{ip}} port {{ port }}', end]
`

func TestParseFileStrict(t *testing.T) {
	s, err := cmdset.ParseFile([]byte(ownYAML))
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "acme-os" || s.Sum != cmdset.Sum([]byte(ownYAML)) || s.DisplayName() != "ACME OS" {
		t.Fatalf("%+v", s)
	}
	// unbekannte Felder (Tippfehler) werden nicht still übergangen
	if _, err := cmdset.ParseFile([]byte(ownYAML + "comands:\n  version: x\n")); err == nil {
		t.Fatal("unbekanntes Feld angenommen")
	}
	if _, err := cmdset.ParseFile(make([]byte, cmdset.MaxFileSize+1)); err == nil {
		t.Fatal("zu große Datei angenommen")
	}
	if _, err := cmdset.ParseFile([]byte("id: [")); err == nil {
		t.Fatal("kaputtes YAML angenommen")
	}
}

// Eigene Sätze dürfen nur lesende Befehle für die Abfragen enthalten und
// keine kaputten Muster.
func TestParseOwnRejects(t *testing.T) {
	if _, err := cmdset.ParseOwn([]byte(ownYAML)); err != nil {
		t.Fatalf("gültiger Satz abgelehnt: %v", err)
	}
	for name, y := range map[string]string{
		"ändernder Befehl":   strings.Replace(ownYAML, "version: show version", "version: reload", 1),
		"kaputtes Muster":    strings.Replace(ownYAML, "'(?i)acme os'", "'(acme'", 1),
		"eingebaute Kennung": strings.Replace(ownYAML, "id: acme-os", "id: cisco-ios", 1),
		"Grundlage fehlt":    ownYAML + "base: gibt-es-nicht\n",
		"ohne Kennung":       strings.Replace(ownYAML, "id: acme-os\n", "", 1),
	} {
		if _, err := cmdset.ParseOwn([]byte(y)); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
}

func TestSyslogSteps(t *testing.T) {
	got, err := cmdset.SyslogSteps([]string{"logging host {{ip}} port {{ port }}", "end"}, "10.0.0.5", 5514)
	if err != nil || got[0] != "logging host 10.0.0.5 port 5514" || got[1] != "end" {
		t.Fatalf("%v %v", got, err)
	}
	// nichts außer IP und Port gelangt in die Gerätekonfiguration
	for _, ip := range []string{"10.0.0.5; reload", "host.example", ""} {
		if _, err := cmdset.SyslogSteps([]string{"logging {{ip}}"}, ip, 514); err == nil {
			t.Errorf("IP %q angenommen", ip)
		}
	}
	for _, port := range []int{0, -1, 70000} {
		if _, err := cmdset.SyslogSteps([]string{"x"}, "10.0.0.5", port); err == nil {
			t.Errorf("Port %d angenommen", port)
		}
	}
}

func TestSyslogAbort(t *testing.T) {
	for first, want := range map[string]string{"configure terminal": "end", "system-view": "return", "config log syslogd setting": "abort", "show x": ""} {
		s := &cmdset.Set{Syslog: cmdset.Syslog{Steps: []string{first}}}
		got := strings.Join(s.SyslogAbort(), ",")
		if got != want {
			t.Errorf("%q: %q statt %q", first, got, want)
		}
	}
	s := &cmdset.Set{Syslog: cmdset.Syslog{Steps: []string{"configure"}, Abort: []string{"rollback"}}}
	if got := s.SyslogAbort(); len(got) != 1 || got[0] != "rollback" {
		t.Fatalf("eigene Angabe übergangen: %v", got)
	}
}

// Varianten: die erste passende Firmware ändert nur ihre Felder.
func TestResolveVariant(t *testing.T) {
	base := cmdset.Resolve("cisco-ios", "", "15.2(7)E4")
	old := cmdset.Resolve("cisco-ios", "", "12.2(55)SE")
	if base == nil || old == nil {
		t.Fatal("cisco-ios fehlt")
	}
	if strings.Join(base.Syslog.Steps, "|") == strings.Join(old.Syslog.Steps, "|") {
		t.Fatal("Variante IOS 12.x wirkt nicht")
	}
	if old.Commands.Version != base.Commands.Version {
		t.Fatal("Variante hat fremde Felder geändert")
	}
	if cmdset.VariantName("cisco-ios", "", "12.2(55)SE") == "" || cmdset.VariantName("cisco-ios", "", "15.2") != "" {
		t.Fatal("VariantName")
	}
	if cmdset.Resolve("gibt-es-nicht", "", "") != nil {
		t.Fatal("unbekannte Kennung aufgelöst")
	}
}

// Eigene Sätze wirken sofort, Team-Sätze erst nach Freigabe genau dieses
// Inhalts; eine eingebaute Kennung lässt sich nicht überdecken.
func TestInstallApproval(t *testing.T) {
	t.Cleanup(func() { cmdset.Install(cmdset.Loaded{}, nil, nil) })
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	team := "# von: kollege\n" + strings.ReplaceAll(ownYAML, "acme", "team")
	write("acme-os.yaml", ownYAML)
	write("team/team-os.yaml", team)
	write("schatten.yaml", strings.Replace(ownYAML, "id: acme-os", "id: cisco-ios", 1))
	write("kaputt.yaml", "id: [")
	write("notiz.txt", "keine Satzdatei")

	l := cmdset.ReadDir(dir)
	if len(l.Own) != 2 || len(l.Team) != 1 || len(l.Errs) != 1 || l.Team[0].From != "kollege" {
		t.Fatalf("ReadDir: own=%d team=%d errs=%d", len(l.Own), len(l.Team), len(l.Errs))
	}
	state := func(id string) (st cmdset.Status, n int) {
		for _, s := range cmdset.Statuses() {
			if s.Set.ID == id {
				st = s
				n++
			}
		}
		return
	}

	cmdset.Install(l, nil, nil)
	if st, _ := state("acme-os"); !st.Active {
		t.Fatalf("eigener Satz nicht aktiv: %+v", st)
	}
	if st, _ := state("team-os"); st.Active || !st.Pending {
		t.Fatalf("Team-Satz ohne Freigabe aktiv: %+v", st)
	}
	if _, ok := cmdset.Lookup("team-os"); ok {
		t.Fatal("nicht freigegebener Team-Satz ist auffindbar")
	}
	if s, ok := cmdset.Lookup("cisco-ios"); !ok || s.Source == cmdset.Own {
		t.Fatal("eingebauter Satz überdeckt")
	}
	if cmdset.DetectVersion("ACME OS 1.0") == nil || cmdset.DetectVersion("TEAM OS 1.0") != nil {
		t.Fatal("Erkennung folgt nicht der Freigabe")
	}

	// freigegeben: aktiv
	cmdset.Install(l, map[string]string{"team-os": cmdset.Sum([]byte(team))}, nil)
	if st, _ := state("team-os"); !st.Active {
		t.Fatalf("freigegebener Team-Satz nicht aktiv: %+v", st)
	}
	// Inhalt danach geändert: wieder inaktiv und als geändert markiert
	cmdset.Install(l, map[string]string{"team-os": cmdset.Sum([]byte("anderer inhalt"))}, nil)
	if st, _ := state("team-os"); st.Active || !st.Changed {
		t.Fatalf("geänderter Team-Satz: %+v", st)
	}
	// Selbsttest schlägt fehl: Problem wird gemeldet (nur Sätze mit Beispielen)
	write("acme-os.yaml", ownYAML+"samples:\n  - name: Muster\n    version: ACME OS 1.0\n")
	l = cmdset.ReadDir(dir)
	cmdset.Install(l, nil, func(s *cmdset.Set) []string {
		if s.ID == "acme-os" {
			return []string{"Beispiel passt nicht"}
		}
		return nil
	})
	if st, _ := state("acme-os"); !strings.Contains(st.Problem, "Beispiel passt nicht") {
		t.Fatalf("Selbsttest-Problem fehlt: %+v", st)
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	s, err := cmdset.ParseOwn([]byte(ownYAML))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := cmdset.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	back, err := cmdset.ParseOwn(raw)
	if err != nil {
		t.Fatalf("eigene Ausgabe nicht lesbar: %v\n%s", err, raw)
	}
	if back.ID != s.ID || back.Commands.Version != s.Commands.Version || back.Commands.Config != s.Commands.Config || strings.Join(back.Syslog.Steps, "|") != strings.Join(s.Syslog.Steps, "|") {
		t.Fatalf("Inhalt verändert:\n%s", raw)
	}
}
