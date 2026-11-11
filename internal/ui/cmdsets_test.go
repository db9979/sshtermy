package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/cmdset"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
	"github.com/db9979/sshtermy/internal/syncx"
	"github.com/db9979/sshtermy/internal/team"
)

// Eigenen Satz anlegen, Team-Satz erst nach Freigabe aktiv, Gefahrenregel
// eines Team-Satzes wirkt erst danach, eingebaute Kennungen bleiben gesperrt.
func TestCmdSetsFlow(t *testing.T) {
	g := newGold(t, i18n.DE, 110, 34, true)
	t.Cleanup(func() { cmdset.Install(cmdset.Loaded{}, nil, nil) })
	a := g.a
	a.openCmdSets()
	a.selectCmdSet("cisco-ios")
	g.keys("n")
	if a.scr != scrCmdSetForm {
		t.Fatalf("Formular nicht offen: %v", a.scr)
	}
	g.keys("ctrl+s")
	if a.scr != scrCmdSets {
		t.Fatalf("nach dem Speichern nicht zurück in der Liste (Fehler: %q)", a.form.err)
	}
	s := cmdset.Resolve("cisco-ios-eigen", "", "")
	if s == nil || s.Source != cmdset.Own || s.Commands.Version == "" || s.EffFamily() != cmdset.FamilyNetwork {
		t.Fatalf("eigener Satz fehlt oder erbt nicht: %+v", s)
	}
	if _, err := os.Stat(filepath.Join(a.cmdsetDir(), "cisco-ios-eigen.yaml")); err != nil {
		t.Fatal(err)
	}

	// Team-Satz mit eingebauter Kennung wird verworfen, eigener wartet.
	evil := "id: cisco-ios\nname: Fake\n"
	good := "id: acme-sw\nname: ACME\ndanger:\n  - rx: '^\\s*acme-wipe\\b'\n    reason: löscht alles\n"
	if n := a.storeTeamCmdSets("Kollege", []team.CmdSet{{ID: "cisco-ios", YAML: evil}, {ID: "acme-sw", YAML: good}}); n != 1 {
		t.Fatalf("gespeichert: %d, erwartet 1", n)
	}
	if b, _ := cmdset.Lookup("cisco-ios"); b.Source != cmdset.Builtin {
		t.Fatal("eingebauter Satz überschrieben")
	}
	if _, ok := cmdset.Lookup("acme-sw"); ok {
		t.Fatal("Team-Satz ohne Freigabe aktiv")
	}
	if len(safety.Check("acme-wipe", "")) != 0 {
		t.Fatal("Gefahrenregel eines nicht freigegebenen Team-Satzes wirkt")
	}
	a.selectCmdSet("acme-sw")
	g.keys("f")
	if a.modal == nil || !strings.Contains(a.modal.body, "acme-wipe") {
		t.Fatal("Freigabe zeigt den Inhalt nicht")
	}
	a.modal.onYes()
	a.modal = nil
	if s, ok := cmdset.Lookup("acme-sw"); !ok || s.From != "Kollege" {
		t.Fatalf("Team-Satz nach Freigabe nicht aktiv: %+v", s)
	}
	if len(safety.Check("acme-wipe", "")) == 0 {
		t.Fatal("Gefahrenregel nach Freigabe wirkt nicht")
	}

	// Geändert vom Team: wieder gesperrt bis zur nächsten Freigabe.
	a.storeTeamCmdSets("Kollege", []team.CmdSet{{ID: "acme-sw", YAML: good + "hint: neu\n"}})
	if _, ok := cmdset.Lookup("acme-sw"); ok {
		t.Fatal("geänderter Team-Satz ohne neue Freigabe aktiv")
	}
}

// Ohne family gilt die Familie der Grundlage, auch für die Nur-lesen-Prüfung.
func TestCmdSetInheritsFamily(t *testing.T) {
	s, err := cmdset.ParseOwn([]byte("id: mein-linux\nname: Mein Linux\nbase: posix\ncommands:\n  system: uptime\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cmdset.ParseOwn([]byte("id: mein-linux\nname: Mein Linux\nbase: posix\ncommands:\n  system: rm -rf /tmp/x\n")); err == nil {
		t.Fatal("schreibender Befehl angenommen")
	}
	t.Cleanup(func() { cmdset.Install(cmdset.Loaded{}, nil, nil) })
	cmdset.Install(cmdset.Loaded{Own: []*cmdset.Set{s}}, nil, nil)
	if f := cmdset.Resolve("mein-linux", "", "").EffFamily(); f != cmdset.FamilyLinux {
		t.Fatalf("Familie %q, erwartet linux", f)
	}
	if l, _ := cmdset.Lookup("mein-linux"); l.EffFamily() != cmdset.FamilyLinux {
		t.Fatalf("Familie ohne Auflösung %q", l.EffFamily())
	}
}

// Sync: eigene Sätze gehen hinaus, ankommende wirken erst nach Übernahme,
// Löschen kommt per Grabstein an und legt die Datei beiseite.
func TestCmdSetsSync(t *testing.T) {
	g := newGold(t, i18n.DE, 110, 34, true)
	t.Cleanup(func() { cmdset.Install(cmdset.Loaded{}, nil, nil) })
	a := g.a
	dir := a.cmdsetDir()
	mine := "id: mein-sw\nname: Mein Switch\nbase: cisco-ios\n"
	if err := writeCmdSet(filepath.Join(dir, "mein-sw.yaml"), []byte(mine)); err != nil {
		t.Fatal(err)
	}
	out := a.syncCmdSets()
	if len(out) != 1 || out[0].ID != "mein-sw" || out[0].YAML != mine {
		t.Fatalf("hinaus: %+v", out)
	}

	later := time.Now().Add(time.Hour).UTC()
	other := "id: fremd-sw\nname: Fremd\nbase: cisco-ios\ndanger:\n  - rx: '^\\s*fremd-wipe\\b'\n    reason: x\n"
	a.applySyncCmdSets(&syncx.Payload{CmdSets: []syncx.CmdSet{
		{ID: "fremd-sw", YAML: other, UpdatedAt: later},
		{ID: "cisco-ios", YAML: "id: cisco-ios\nname: Fake\n", UpdatedAt: later},
		{ID: "mein-sw", YAML: mine + "hint: neu\n", UpdatedAt: later},
	}})
	if _, ok := cmdset.Lookup("fremd-sw"); ok {
		t.Fatal("Satz vom Sync ohne Übernahme aktiv")
	}
	if len(safety.Check("fremd-wipe", "")) != 0 {
		t.Fatal("Gefahrenregel eines nicht übernommenen Satzes wirkt")
	}
	if b, _ := cmdset.Lookup("cisco-ios"); b.Source != cmdset.Builtin {
		t.Fatal("eingebauter Satz überschrieben")
	}
	if s, _ := cmdset.Lookup("mein-sw"); s.Hint != "" {
		t.Fatal("eigener Satz ohne Übernahme geändert")
	}
	a.openCmdSets()
	a.selectCmdSet("fremd-sw")
	st, _ := a.cmdsetSelected()
	if st.Set.Source != cmdset.Synced || !st.Pending {
		t.Fatalf("Angebot fehlt: %+v", st)
	}
	g.keys("f")
	if a.modal == nil {
		t.Fatal("keine Rückfrage")
	}
	a.modal.onYes()
	a.modal = nil
	if s, ok := cmdset.Lookup("fremd-sw"); !ok || s.Source != cmdset.Own {
		t.Fatal("nach Übernahme kein eigener Satz")
	}
	if len(safety.Check("fremd-wipe", "")) == 0 {
		t.Fatal("Gefahrenregel nach Übernahme wirkt nicht")
	}

	// Anderswo gelöscht: Datei beiseite, Angebot weg.
	a.applySyncCmdSets(&syncx.Payload{Deleted: map[string]time.Time{syncx.CmdSetTombstone("fremd-sw"): later.Add(time.Hour)}})
	if _, ok := cmdset.Lookup("fremd-sw"); ok {
		t.Fatal("gelöschter Satz noch aktiv")
	}
	if m, _ := filepath.Glob(filepath.Join(dir, ".geloescht", "fremd-sw-*.yaml")); len(m) != 1 {
		t.Fatalf("nicht beiseitegelegt: %v", m)
	}
	// Angebot für mein-sw verschwindet, wenn es nicht mehr im Sync steht.
	if _, err := os.Stat(filepath.Join(dir, cmdset.SyncDir, "mein-sw.yaml")); err == nil {
		t.Fatal("erledigtes Angebot liegt noch da")
	}
}
