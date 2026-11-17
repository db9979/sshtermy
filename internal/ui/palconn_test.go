package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/store"
)

func TestPalConnScore(t *testing.T) {
	web := domain.Connection{Alias: "web-01", Host: "10.0.0.5", Group: "Produktion / Web", Tags: []string{"nginx"}}
	db := domain.Connection{Alias: "db-01", Host: "web-backend.local", Group: "Produktion / DB"}
	cases := []struct {
		q    string
		c    domain.Connection
		want bool
	}{
		{"web", web, true}, {"wb1", web, true}, {"prod web", web, true}, {"#nginx", web, true},
		{"#ngi", web, true}, {"10.0.0", web, true}, {"db", web, false}, {"#db", db, false}, {"prod xyz", web, false},
	}
	for _, c := range cases {
		if got := palConnScore(c.c, c.q) > 0; got != c.want {
			t.Errorf("%q auf %s: %v, erwartet %v", c.q, c.c.Alias, got, c.want)
		}
	}
	if palConnScore(web, "web") <= palConnScore(db, "web") {
		t.Error("Treffer im Namen muss vor Treffer im Host stehen")
	}
	if palConnScore(web, "web") <= palConnScore(web, "wb1") {
		t.Error("zusammenhängender Treffer muss vor unscharfem stehen")
	}
}

// Viele Server: ohne Text nur die zuletzt benutzten plus „alle im
// Manager“, mit Text höchstens palMaxHits Treffer plus „weitere“.
func TestPalConnItemsLimited(t *testing.T) {
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	var cs []domain.Connection
	for i := 0; i < 40; i++ {
		cs = append(cs, domain.Connection{Alias: fmt.Sprintf("web-%02d", i), Host: fmt.Sprintf("10.0.0.%d", i), Port: 22, Username: "u", Auth: domain.AuthPassword})
	}
	cs, err = st.AddConnections(cs)
	if err != nil {
		t.Fatal(err)
	}
	st.Touch(cs[30].ID)
	time.Sleep(5 * time.Millisecond)
	st.Touch(cs[12].ID)
	a, err := New(st)
	if err != nil {
		t.Fatal(err)
	}

	items := a.palConnItems("")
	if len(items) != 3 || items[0].label != "web-12" || items[1].label != "web-30" || !items[2].more {
		t.Fatalf("ohne Text: %v", labels(items))
	}
	items = a.palConnItems("web")
	if len(items) != palMaxHits+1 || !items[palMaxHits].more || !strings.Contains(items[palMaxHits].label, "32") {
		t.Fatalf("mit Text: %v", labels(items))
	}
	if items[0].label != "web-12" {
		t.Fatalf("bei gleicher Güte zuletzt benutzte zuerst: %v", labels(items))
	}
	items = a.palConnItems("web-07")
	if len(items) != 1 || items[0].label != "web-07" {
		t.Fatalf("genauer Name: %v", labels(items))
	}
}

func labels(items []palItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.label)
	}
	return out
}

// Die Palette zeigt bei ihren Verbindungen denselben Online-Punkt wie der
// Manager und lässt genau diese auch außerhalb des Managers prüfen.
func TestPalConnReachDot(t *testing.T) {
	a := manyConnsApp(t, 40)
	a.scr = scrTerminal
	if cmd := a.reachTick(); cmd != nil || a.reach.running {
		t.Fatal("ohne Manager und ohne Palette darf nichts geprüft werden")
	}
	a.openPalette()
	ids := a.palReachIDs()
	if len(ids) != palRecent {
		t.Fatalf("Palette zeigt %d Verbindungen zum Prüfen, erwartet %d", len(ids), palRecent)
	}
	if cmd := a.reachTick(); cmd == nil || !a.reach.running {
		t.Fatal("offene Palette prüft ihre Verbindungen nicht")
	}
	a.reach.running = false

	items := a.palConnItems("")
	up, down := items[0], items[1]
	var upID, downID string
	for _, c := range a.st.Connections() {
		switch c.Alias {
		case up.label:
			upID = c.ID
		case down.label:
			downID = c.ID
		}
	}
	if !ids[upID] || !ids[downID] {
		t.Fatal("geprüfte Verbindungen passen nicht zur Liste")
	}
	if up.dot != "○" {
		t.Fatalf("ohne Ergebnis %q, erwartet ○", up.dot)
	}
	a.reach.res = map[string]reachResult{upID: {ok: true, at: time.Now()}, downID: {err: "timeout", at: time.Now()}}
	items = a.palConnItems("")
	if items[0].dot != "●" || items[0].dotColor == a.s.T.Danger || items[0].dotColor == a.s.T.Muted {
		t.Fatalf("erreichbar: %q", items[0].dot)
	}
	if d := items[1].dot; (d != "●" && d != "×") || items[1].dotColor != a.s.T.Danger {
		t.Fatalf("nicht erreichbar: %q", d)
	}
	a.scr = scrManager
	if v := a.View().Content; !strings.Contains(v, "●") {
		t.Fatal("Punkt erscheint nicht in der Palette")
	}

	a.pal.in.SetValue("srv-003")
	if ids := a.palReachIDs(); len(ids) != 1 {
		t.Fatalf("mit Text: %d Treffer zum Prüfen", len(ids))
	}
}
