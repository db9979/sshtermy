package ui

import (
	"charm.land/lipgloss/v2"
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestQuickPlaceholders(t *testing.T) {
	cmd := "systemctl restart {{dienst}} && journalctl -u {{ dienst }} -n {{zeilen:50}}"
	vars := quickPlaceholders(cmd)
	if !reflect.DeepEqual(vars, []quickVar{{"dienst", ""}, {"zeilen", "50"}}) {
		t.Fatalf("Platzhalter: %#v", vars)
	}
	got := fillQuick(cmd, map[string]string{"dienst": "nginx", "zeilen": "20"})
	if got != "systemctl restart nginx && journalctl -u nginx -n 20" {
		t.Fatalf("eingesetzt: %q", got)
	}
	for _, plain := range []string{"echo ${HOME}", "awk '{print $1}'", "docker ps --format '{{.Names}}'"} {
		if v := quickPlaceholders(plain); len(v) != 0 {
			t.Errorf("%q: kein Platzhalter erwartet, bekam %v", plain, v)
		}
	}
}

func TestBulkChangeApply(t *testing.T) {
	red := "red"
	b := bulkChange{add: []string{"prod", "Linux"}, del: []string{"alt"}, user: "admin", guard: "aus", color: &red}
	c := domain.Connection{Tags: []string{"alt", "linux"}, Username: "root"}
	if !b.apply(&c) {
		t.Fatal("Änderung nicht gemeldet")
	}
	if !reflect.DeepEqual(c.Tags, []string{"linux", "prod"}) || c.Username != "admin" || !c.GuardOff || c.Color != "red" {
		t.Fatalf("nach Änderung: %+v", c)
	}
	if b.apply(&c) {
		t.Fatal("zweites Anwenden darf nichts mehr ändern")
	}
	local := domain.Connection{Protocol: domain.ProtoLocal}
	bulkChange{user: "admin", guard: bulkKeep}.apply(&local)
	if local.Username != "" {
		t.Fatal("Benutzer darf bei lokaler Shell nicht gesetzt werden")
	}
	if !(bulkChange{guard: bulkKeep}).empty() || (bulkChange{guard: "an"}).empty() {
		t.Fatal("empty() falsch")
	}
	if got := splitTags(" #prod, linux ,, "); !reflect.DeepEqual(got, []string{"prod", "linux"}) {
		t.Fatalf("splitTags: %v", got)
	}
}

// Mehrfachauswahl: markieren, Sammeländerung, verschieben, löschen, zurück.
func TestManagerBulkFlow(t *testing.T) {
	g := newGold(t, "de", 120, 36, true)
	a := g.a
	g.keys("/").typ("#nginx").keys("enter", "*")
	ids := a.mgrMarked()
	if len(ids) != 2 {
		t.Fatalf("markiert: %d, erwartet die 2 Verbindungen mit #nginx", len(ids))
	}
	if !strings.Contains(g.screen(), "2 markiert") || !strings.Contains(g.screen(), "✓") {
		t.Fatalf("Markierung fehlt im Bild:\n%s", g.screen())
	}
	g.keys("e")
	if a.scr != scrBulkForm {
		t.Fatalf("e öffnet die Sammeländerung nicht (Ansicht %v)", a.scr)
	}
	g.keys("ctrl+s") // nichts ausgefüllt: bleibt offen
	if a.scr != scrBulkForm || a.form.err == "" {
		t.Fatal("leere Sammeländerung muss abgelehnt werden")
	}
	g.typ("wartung, q4").keys("ctrl+s")
	if a.scr != scrManager || len(a.mgrMarked()) != 0 {
		t.Fatalf("nach dem Speichern: Ansicht %v, markiert %d", a.scr, len(a.mgrMarked()))
	}
	n := 0
	for _, c := range a.st.Connections() {
		has := strings.Contains(strings.Join(c.Tags, ","), "wartung") && strings.Contains(strings.Join(c.Tags, ","), "q4")
		if has != (c.ID == ids[0] || c.ID == ids[1]) {
			t.Errorf("%s: Tags %v", c.Alias, c.Tags)
		}
		if has {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("%d Verbindungen geändert, erwartet 2", n)
	}

	// Leertaste markiert einzeln, Esc hebt auf
	g.keys("esc")
	for i, r := range a.mgrRows() {
		if !r.isFolder() {
			a.mgr.sel = i
			break
		}
	}
	g.keys("space", "space")
	if len(a.mgrMarked()) < 1 {
		t.Fatal("Leertaste markiert nicht")
	}
	g.keys("esc")
	if len(a.mgrMarked()) != 0 {
		t.Fatal("Esc hebt die Auswahl nicht auf")
	}

	// alle mit #wartung löschen und zurückholen
	before := len(a.st.Connections())
	g.keys("/").typ("#wartung").keys("enter", "*", "d")
	if got := len(a.st.Connections()); got != before-2 {
		t.Fatalf("nach dem Löschen %d Verbindungen, erwartet %d", got, before-2)
	}
	g.keys("ctrl+z")
	if got := len(a.st.Connections()); got != before {
		t.Fatalf("nach Rückgängig %d Verbindungen, erwartet %d", got, before)
	}
}

func TestSearchAllTabs(t *testing.T) {
	g := newGold(t, "de", 120, 36, true)
	a := g.a
	g.session("web-01", "eins\nNadel im ersten Tab\n$ ")
	second := g.session("db-01", "zwei\nhier liegt die Nadel auch\nund noch eine Nadel\n$ ")
	a.focusSession(0)
	items := a.searchAllItems("nadel")
	if len(items) != 3 {
		t.Fatalf("%d Treffer, erwartet 3: %+v", len(items), items)
	}
	if !strings.HasPrefix(items[0].value, "1:") {
		t.Fatalf("der aktuelle Tab muss zuerst kommen: %+v", items[0])
	}
	g.keys("alt+s")
	if a.modal == nil || a.modal.kind != mInput {
		t.Fatal("Alt+S öffnet die Suche über alle Tabs nicht")
	}
	g.typ("nadel").keys("enter")
	if a.pick == nil {
		t.Fatal("Trefferliste fehlt")
	}
	a.pick = nil
	a.searchAllJump("nadel", items[2].value) // ältester Treffer im zweiten Tab
	if a.current() != second || !second.search.open || second.search.in.Value() != "nadel" {
		t.Fatalf("Sprung: Tab %v, Suche offen %v", a.current().conn.Alias, second.search.open)
	}
	want := strings.TrimPrefix(items[2].value, "2:")
	if m := second.search.matches[second.search.idx]; want != strconv.Itoa(m.Line) {
		t.Fatalf("Treffer in Zeile %d, erwartet %s", m.Line, want)
	}
	if a.searchAll("gibtesnicht"); a.pick != nil {
		t.Fatal("ohne Treffer darf keine Liste aufgehen")
	}
}

// Tab ziehen: die Reihenfolge folgt der Maus, nur in Zugrichtung.
func TestTabDragReorders(t *testing.T) {
	a := tabsApp(4)
	a.scr = scrTerminal
	view := a.z.scan(a.tabStrip(a.w-lipgloss.Width(a.headerLeft())-1), 0, 0)
	_ = view
	at := func(i int) int { // Spalte in der Mitte von Tab i
		for x := 0; x < a.w; x++ {
			if a.z.tabAt(x, 0) == i && a.z.tabAt(x+2, 0) == i {
				return x + 1
			}
		}
		t.Fatalf("Tab %d nicht gefunden", i)
		return 0
	}
	x0 := at(0)
	a.mouse(tea.MouseClickMsg{X: x0, Y: 0, Button: tea.MouseLeft})
	if a.tabDrag == nil {
		t.Fatal("Klick auf einen Tab beginnt kein Ziehen")
	}
	x2 := at(2)
	a.mouse(tea.MouseMotionMsg{X: x2, Y: 0, Button: tea.MouseLeft})
	if got := aliases(a); got != "host-b,host-c,host-a,host-d" {
		t.Fatalf("nach dem Ziehen: %s", got)
	}
	if a.current().conn.Alias != "host-a" {
		t.Fatalf("aktiver Tab: %s", a.current().conn.Alias)
	}
	a.z.reset()
	a.z.scan(a.tabStrip(a.w-lipgloss.Width(a.headerLeft())-1), 0, 0)
	// Maus bewegt sich nach links über Tab 3 hinweg: kein Sprung nach rechts
	a.tabDrag.x = at(3) + 1
	a.mouse(tea.MouseMotionMsg{X: at(3), Y: 0, Button: tea.MouseLeft})
	if got := aliases(a); got != "host-b,host-c,host-a,host-d" {
		t.Fatalf("gegen die Zugrichtung getauscht: %s", got)
	}
	a.mouse(tea.MouseReleaseMsg{X: at(3), Y: 0, Button: tea.MouseLeft})
	if a.tabDrag != nil {
		t.Fatal("Loslassen beendet das Ziehen nicht")
	}
}

func TestUpdateWaitLabel(t *testing.T) {
	for h, want := range map[int]string{0: "sofort", 24: "stabil: nach 1 Tag", 168: "stabil: nach 7 Tagen", 5: "stabil: nach 5 Stunden"} {
		if got := updateWaitLabel(h); got != want {
			t.Errorf("%d h: %q, erwartet %q", h, got, want)
		}
	}
}

// Notiz, Startbefehl und MAC-Adresse kommen aus dem Formular in die
// Verbindung; gefährliche Startbefehle werden abgelehnt.
func TestConnFormNoteStartMAC(t *testing.T) {
	g := newGold(t, "de", 120, 40, true)
	a := g.a
	var c domain.Connection
	for _, x := range a.st.Connections() {
		if x.Alias == "nas" {
			c = x
		}
	}
	a.openConnForm(c)
	f := a.form
	f.get("note").in.SetValue("  Wartung donnerstags  ")
	f.get("startcmd").in.SetValue("cd /srv/app")
	f.get("mac").in.SetValue("AA-BB-CC-DD-EE-FF")
	got, err := a.connFromForm()
	if err != nil {
		t.Fatal(err)
	}
	if got.Note != "Wartung donnerstags" || got.StartCmd != "cd /srv/app" || got.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("Note %q Start %q MAC %q", got.Note, got.StartCmd, got.MAC)
	}
	f.get("startcmd").in.SetValue("rm -rf /")
	if _, err := a.connFromForm(); err == nil {
		t.Fatal("gefährlicher Startbefehl muss abgelehnt werden")
	}
	f.get("startcmd").in.SetValue("")
	f.get("mac").in.SetValue("keine-mac")
	if _, err := a.connFromForm(); err == nil {
		t.Fatal("ungültige MAC-Adresse muss abgelehnt werden")
	}
}

// Wake-on-LAN: MAC wird nach dem Verbinden gemerkt, Aufwecken schickt das
// Paket an genau diese Adresse.
func TestWakeAndLearnMAC(t *testing.T) {
	g := newGold(t, "de", 120, 36, true)
	a := g.a
	var nas domain.Connection
	for _, x := range a.st.Connections() {
		if x.Alias == "nas" {
			nas = x
		}
	}
	if canWake(nas) {
		t.Fatal("ohne MAC-Adresse darf Aufwecken nicht angeboten werden")
	}
	if cmd := a.wake(nas); cmd != nil {
		t.Fatal("ohne MAC-Adresse darf nichts gesendet werden")
	}
	oldLookup, oldWake := lookupMAC, wakeHost
	defer func() { lookupMAC, wakeHost = oldLookup, oldWake }()
	lookupMAC = func(_ context.Context, host string) string {
		if host != "nas.local" {
			t.Errorf("Lookup für %q", host)
		}
		return "aa:bb:cc:00:11:22"
	}
	s := &session{id: 99, conn: nas}
	a.sessions = append(a.sessions, s)
	cmd := a.learnMAC(s)
	if cmd == nil {
		t.Fatal("learnMAC ohne Kommando")
	}
	a.Update(cmd())
	if got, _ := a.st.Connection(nas.ID); got.MAC != "aa:bb:cc:00:11:22" || s.conn.MAC != got.MAC {
		t.Fatalf("MAC nicht gemerkt: %q / %q", got.MAC, s.conn.MAC)
	}
	nas, _ = a.st.Connection(nas.ID)
	if a.learnMAC(&session{conn: nas}) != nil {
		t.Fatal("bekannte MAC darf nicht erneut gesucht werden")
	}
	sent := ""
	wakeHost = func(mac string) (int, error) { sent = mac; return 2, nil }
	a.Update(a.wake(nas)())
	if sent != "aa:bb:cc:00:11:22" {
		t.Fatalf("Paket an %q", sent)
	}
	// über einen Jump-Host erreicht das Paket den Rechner nicht
	nas.JumpHostID = "x"
	if canWake(nas) {
		t.Fatal("über Jump-Host darf Aufwecken nicht angeboten werden")
	}
}

// Startbefehl: nur für echte Anmeldungen ohne feste Sitzung.
func TestArmStartCmd(t *testing.T) {
	a := &App{}
	for _, c := range []struct {
		conn domain.Connection
		want bool
	}{
		{domain.Connection{StartCmd: "cd /srv"}, true},
		{domain.Connection{}, false},
		{domain.Connection{StartCmd: "cd /srv", Persist: "tmux"}, false},
		{domain.Connection{StartCmd: "cd /srv", Protocol: domain.ProtoLocal}, false},
	} {
		s := &session{conn: c.conn}
		a.armStartCmd(s)
		if s.startPending != c.want {
			t.Errorf("%+v: vorgemerkt %v, erwartet %v", c.conn, s.startPending, c.want)
		}
	}
}
