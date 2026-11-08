package ui

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/broadcast"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/sshx"
)

// fakeSwitch antwortet wie ein ProCurve über die Shell.
type fakeSwitch struct {
	name string
	out  map[string]string
}

func (e fakeSwitch) NetDevice() bool { return true }
func (e fakeSwitch) Learned() bool   { return false }
func (e fakeSwitch) Close() error    { return nil }
func (e fakeSwitch) Exec(context.Context, string) (sshx.RunResult, error) {
	return sshx.RunResult{}, nil
}
func (e fakeSwitch) RunInShell(_ context.Context, cmds []string) (sshx.RunResult, error) {
	var b strings.Builder
	for _, c := range cmds {
		b.WriteString(e.name + "# " + c + "\n")
		if o, ok := e.out[c]; ok {
			b.WriteString(o)
		} else if c != "no page" && c != "terminal length 0" {
			b.WriteString("Invalid input: " + c + "\n")
		}
	}
	b.WriteString(e.name + "# ")
	return sshx.RunResult{Output: []byte(b.String()), Exit: -1}, nil
}

var fakeLab = map[string]fakeSwitch{
	"core": {"core", map[string]string{
		"show mac-address": "  001122-aabbcc 48 20\n  001122-000001 48 20\n  001122-000002 48 20\n",
		"show arp":         "  10.0.20.15  001122-aabbcc  dynamic 48\n",
		"show lldp info remote-device": "  LocalPort | ChassisId PortId PortDescr SysName\n" +
			"  --------- + --------- ------ --------- -------\n  48        | 941882000002 25 25 sw-etage2\n",
		"show running-config": "hostname \"core\"\nntp server 10.0.0.1\ntelnet-server\n",
	}},
	"etage2": {"sw-etage2", map[string]string{
		"show mac-address": "  001122-aabbcc 17 20\n  001122-000001 25 20\n",
		"show lldp info remote-device": "  LocalPort | ChassisId PortId PortDescr SysName\n" +
			"  --------- + --------- ------ --------- -------\n  25        | 941882000001 48 48 core\n  3 | 001122334455 eth0 eth0 ap-03\n",
		"show running-config": "hostname \"sw-etage2\"\nntp server 10.0.0.1\nvlan 20\n",
	}},
}

func netTestApp(t *testing.T) *App {
	a := newPaletteTestApp(t)
	a.pal = nil
	for _, alias := range []string{"core", "etage2"} {
		if _, err := a.st.UpsertConnection(domain.Connection{Alias: alias, Host: alias, Username: "admin", Auth: domain.AuthAgent,
			Device: domain.DeviceNetwork, Group: "Netz"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.st.UpsertConnection(domain.Connection{Alias: "web", Host: "web", Username: "root", Auth: domain.AuthAgent, Device: domain.DeviceServer, Group: "Netz"}); err != nil {
		t.Fatal(err)
	}
	dial := func(*App) broadcast.Dialer {
		return func(_ context.Context, c domain.Connection) (broadcast.Executor, error) {
			if c.Alias == "web" {
				t.Errorf("Server darf nicht abgefragt werden")
			}
			return fakeLab[c.Alias], nil
		}
	}
	oldN, oldS := netDialer, schedDialer
	t.Cleanup(func() { netDialer, schedDialer = oldN, oldS })
	netDialer, schedDialer = dial, dial
	return a
}

func TestNetFind(t *testing.T) {
	a := netTestApp(t)
	if len(a.netDevices("")) != 2 {
		t.Fatalf("%d Switches", len(a.netDevices("")))
	}
	cmd := a.netFind("10.0.20.15")
	if cmd == nil {
		t.Fatal("keine Abfrage")
	}
	a.netMsg(cmd())
	if a.modal == nil {
		t.Fatal("kein Ergebnis")
	}
	text := strings.Join(a.modal.lines, "\n")
	if !strings.Contains(text, "→ etage2  Port 17  VLAN 20") || !strings.Contains(text, "Uplink zu etage2") {
		t.Fatal(text)
	}
	// zweite Suche kommt aus dem Speicher
	a.modal = nil
	if cmd := a.netFind("00:11:22:00:00:01"); cmd != nil {
		t.Fatal("sollte den Speicher nutzen")
	}
	if a.modal == nil {
		t.Fatal("kein Ergebnis aus dem Speicher")
	}
	if a.netFind("hallo") != nil {
		t.Fatal("Unsinn darf nichts abfragen")
	}
}

func TestNetTopology(t *testing.T) {
	a := netTestApp(t)
	t.Setenv("HOME", t.TempDir())
	cmd := a.netTopology()
	a.netMsg(cmd())
	text := strings.Join(a.modal.lines, "\n")
	if !strings.Contains(text, "etage2") || !strings.Contains(text, "ap-03") || !strings.Contains(text, "Mermaid") {
		t.Fatal(text)
	}
	var path string
	for _, l := range a.modal.lines {
		if i := strings.Index(l, "sshtermy-topologie-"); i > 0 {
			path = strings.TrimSpace(l[strings.Index(l, ": ")+2:])
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "graph LR") {
		t.Fatalf("%s: %v", path, err)
	}
}

func TestScheduleBackupWithRules(t *testing.T) {
	a := netTestApp(t)
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.ConfigRules = []string{"+^ntp server", "-telnet-server"} })
	a.openScheduleForm(domain.Schedule{At: "03:00"})
	f := a.form
	setChoice(t, f, "kind", "Config-Sicherung (Switches)")
	setChoice(t, f, "folder", "Netz")
	if err := a.saveScheduleForm(); err != nil {
		t.Fatal(err)
	}
	sc := a.schedules()[0]
	if sc.Kind != domain.ScheduleBackup || sc.Name == "" || len(a.schedConns(sc)) != 2 {
		t.Fatalf("%+v", sc)
	}
	a.scheduleMsg(a.startSchedule(sc)())
	sc, _ = a.scheduleByID(sc.ID)
	if sc.LastOK != 1 || sc.LastFail != 1 || !strings.Contains(sc.LastText, "verboten: telnet-server") || !strings.Contains(sc.LastText, "erste Sicherung") {
		t.Fatalf("%d/%d\n%s", sc.LastOK, sc.LastFail, sc.LastText)
	}
	core, _ := a.connByAlias("core")
	bs := a.st.Backups(core.ID)
	if len(bs) != 1 {
		t.Fatalf("%d Sicherungen", len(bs))
	}
	cfg, _ := a.st.LoadBackup(core.ID, bs[0].ID)
	if !strings.HasPrefix(cfg, "hostname \"core\"") || strings.Contains(cfg, "core#") {
		t.Fatalf("%q", cfg)
	}
	// zweiter Lauf: unverändert, nichts Neues gespeichert
	a.scheduleMsg(a.startSchedule(sc)())
	sc, _ = a.scheduleByID(sc.ID)
	if len(a.st.Backups(core.ID)) != 1 || !strings.Contains(sc.LastText, "unverändert") {
		t.Fatalf("%s", sc.LastText)
	}
}

func TestNetScopePicker(t *testing.T) {
	a := netTestApp(t)
	t.Setenv("HOME", t.TempDir())
	// zweite Infrastruktur in eigenem Ordner: erst fragen, welches Netz
	if _, err := a.st.UpsertConnection(domain.Connection{Alias: "kunde-sw", Host: "kunde-sw", Username: "admin", Auth: domain.AuthAgent,
		Device: domain.DeviceNetwork, Group: "Kunde B / Halle"}); err != nil {
		t.Fatal(err)
	}
	if cmd := a.netTopology(); cmd == nil || a.pick == nil {
		t.Fatal("keine Auswahl des Netzes")
	}
	var labels []string
	for _, it := range a.pick.items {
		labels = append(labels, it.label+" "+it.detail)
	}
	if got := strings.Join(labels, "|"); got != "Alle Netzwerke 3 Switches|Kunde B 1 Switches|Kunde B / Halle 1 Switches|Netz 2 Switches" {
		t.Fatal(got)
	}
	cmd := a.pick.onPick(a.pick.items[3]) // nur „Netz“: kunde-sw wird nicht gefragt
	a.pick = nil
	a.netMsg(cmd())
	if a.netData.scope != "Netz" || len(a.netData.devs) != 2 || len(a.netData.fails) != 0 || !strings.Contains(a.modal.title, "· Netz") {
		t.Fatalf("%+v %q", a.netData, a.modal.title)
	}
	// gleiche Wahl ist vorausgewählt, Speicher gilt nur für dasselbe Netz
	a.netTopology()
	if a.pick == nil || a.pick.current != "Netz" {
		t.Fatal("letzte Wahl nicht vorausgewählt")
	}
	if cmd := a.pick.onPick(pickItem{value: "Netz"}); cmd != nil {
		t.Fatal("sollte den Speicher nutzen")
	}
	if cmd := a.pick.onPick(pickItem{value: ""}); cmd == nil {
		t.Fatal("anderes Netz muss neu abfragen")
	}
}
