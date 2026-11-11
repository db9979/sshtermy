package agent

import (
	"context"
	"strings"
	"testing"
	"time"
)

func testWins() []Window {
	return []Window{
		{N: 1, Name: "core-sw1", Ctx: Context{Host: "core-sw1", OS: "ArubaOS-CX", Net: true}},
		{N: 2, Name: "web02", Ctx: Context{Host: "web02", OS: "Ubuntu 24.04", Screen: "password=geheim123\nens18 UP 169.254.12.7/16"}},
		{N: 3, Name: "db01", Prod: true, Ctx: Context{Host: "db01", OS: "Debian 12"}},
	}
}

func TestParseMulti(t *testing.T) {
	ans, err := ParseMulti(`{"antwort":"VLAN falsch.","befund":[{"fenster":1,"text":"VLAN 1"},{"fenster":7,"text":"x"}],
	 "schritte":[
	  {"fenster":1,"befehl":"no vlan 20","grund":"weg"},
	  {"fenster":2,"befehl":"ip -br a show ens18","merke":{"name":"web02-ip","muster":"inet (\\S+)"}},
	  {"fenster":3,"befehl":"grep {{web02-ip}} /etc/postgresql/16/main/pg_hba.conf"},
	  {"fenster":3,"befehl":"echo {{gibts-nicht}}"},
	  {"fenster":0,"system":"lb01","befehl":"cat /etc/haproxy/haproxy.cfg"}],
	 "fehlt":[{"system":"lb01","grund":"Load-Balancer"}]}`, testWins())
	if err != nil {
		t.Fatal(err)
	}
	if len(ans.Findings) != 1 || len(ans.Steps) != 5 || len(ans.Missing) != 1 {
		t.Fatalf("%+v", ans)
	}
	if len(ans.Steps[0].Danger) == 0 || ans.Steps[0].ReadOnly {
		t.Fatal("Switch-Regel „no vlan“ nicht nach dem System von Fenster 1 bewertet")
	}
	if !ans.Steps[1].ReadOnly || ans.Steps[1].Save != "web02-ip" {
		t.Fatalf("Schritt 2: %+v", ans.Steps[1])
	}
	if len(ans.Steps[2].Needs) != 0 || len(ans.Steps[3].Needs) != 1 {
		t.Fatalf("Platzhalter: %v / %v", ans.Steps[2].Needs, ans.Steps[3].Needs)
	}
	if ans.Steps[4].Win != 0 || ans.Steps[4].System != "lb01" || ans.Steps[4].ReadOnly {
		t.Fatalf("fehlendes System: %+v", ans.Steps[4])
	}
	for _, bad := range []string{
		`{"schritte":[{"fenster":5,"befehl":"ls"}]}`,                                            // Fenster gibt es nicht
		`{"schritte":[{"fenster":2,"befehl":"ls\nrm -rf /"}]}`,                                  // mehrzeilig
		`{"schritte":[{"fenster":2,"befehl":"ls","merke":{"name":"X Y","muster":"a"}}]}`,        // Name
		`{"schritte":[{"fenster":2,"befehl":"ls","merke":{"name":"ok","muster":"(unclosed"}}]}`, // Muster
		`kein json`,
	} {
		if _, err := ParseMulti(bad, testWins()); err == nil {
			t.Errorf("angenommen: %s", bad)
		}
	}
}

func TestExtractAndFill(t *testing.T) {
	st := MultiStep{Pattern: `(\d+\.\d+\.\d+\.\d+)/\d+`}
	v, ok := st.Extract("ens18 UP 10.20.0.15/24 fe80::1/64")
	if !ok || v != "10.20.0.15" {
		t.Fatalf("%q %v", v, ok)
	}
	if _, ok := (MultiStep{Pattern: `x=(.*)`}).Extract("x=1; rm -rf /"); ok {
		t.Fatal("Wert mit ; übernommen")
	}
	got := FillPlaceholders("grep {{ip}} f && echo {{rest}}", map[string]string{"ip": "10.0.0.1"})
	if got != "grep 10.0.0.1 f && echo {{rest}}" {
		t.Fatal(got)
	}
}

func TestMultiAskMaterial(t *testing.T) {
	a, seen := fakeSeen(t, `{"antwort":"ok","schritte":[{"fenster":2,"befehl":"ip -br a"}]}`)
	wins := testWins()
	wins[0].Ctx.Now = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	ans, err := a.MultiAsk(context.Background(), "web02 in Betrieb", "Warum keine IP?", wins, nil, false)
	if err != nil || len(ans.Steps) != 1 {
		t.Fatalf("%+v %v", ans, err)
	}
	all := strings.Join(*seen, "\n")
	for _, want := range []string{"Fenster 1: core-sw1", "Fenster 3: db01 · produktiv", "Ziel des Vorgangs: web02 in Betrieb", "Frage: Warum keine IP?"} {
		if !strings.Contains(all, want) {
			t.Errorf("Anfrage ohne %q", want)
		}
	}
	if strings.Contains(all, "geheim123") {
		t.Fatal("Passwort ging an die KI")
	}
}

func TestCrossHint(t *testing.T) {
	a, _ := fakeSeen(t, `{"fenster":3,"text":"pg_hba.conf noch nicht geladen.","befehl":"sudo systemctl reload postgresql"}`)
	h, still, err := a.Cross(context.Background(), 2, Failure{Command: "psql -h db01", Output: "FATAL: no pg_hba.conf entry"}, "", testWins())
	if err != nil || still || h.Win != 3 || h.Command == "" {
		t.Fatalf("%+v %v %v", h, still, err)
	}
	// Hinweis auf das eigene Fenster ist keiner
	a, _ = fakeSeen(t, `{"fenster":2,"text":"Tippfehler"}`)
	if _, still, _ := a.Cross(context.Background(), 2, Failure{Command: "x"}, "", testWins()); !still {
		t.Fatal("Hinweis aufs selbe Fenster gemeldet")
	}
	a, _ = fakeSeen(t, `{"still":true}`)
	if _, still, _ := a.Cross(context.Background(), 2, Failure{Command: "x"}, "", testWins()); !still {
		t.Fatal("still nicht erkannt")
	}
}

func TestRecapMulti(t *testing.T) {
	a, _ := fakeSeen(t, `{"zusammenfassung":["VLAN 20 auf 1/1/12"],"zurueck":[{"fenster":1,"befehl":"vlan access 1"},{"fenster":9,"befehl":"x"}],"offen":["Monitoring"]}`)
	r, err := a.RecapMulti(context.Background(), "Ziel", []MultiEvent{{At: time.Now(), Win: 1, Command: "vlan access 20"}}, testWins())
	if err != nil || len(r.Points) != 1 || len(r.Undo) != 1 || len(r.Open) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
}
