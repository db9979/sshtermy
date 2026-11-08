package ui

import "testing"

func TestMasterForm(t *testing.T) {
	a := newPaletteTestApp(t)
	it, ok := hasLabel(a.palMenuItems(), "Master-Passwort ändern")
	if !ok {
		t.Fatal("Eintrag fehlt")
	}
	a.pal = nil
	it.run()
	if a.scr != scrMasterForm || a.form == nil {
		t.Fatal("Formular nicht offen")
	}
	set := func(k, v string) { a.form.get(k).in.SetValue(v) }
	set("old", "correct horse")
	set("new", "battery staple")
	set("again", "battery stapel")
	if err := a.saveMasterForm(); err == nil {
		t.Fatal("ungleiche Wiederholung angenommen")
	}
	set("again", "battery staple")
	set("old", "falsch!!")
	if err := a.saveMasterForm(); err == nil {
		t.Fatal("falsches altes Passwort angenommen")
	}
	set("old", "correct horse")
	if err := a.saveMasterForm(); err != nil {
		t.Fatal(err)
	}
	if err := a.st.V.CheckPassword([]byte("battery staple")); err != nil {
		t.Fatal("neues Passwort gilt nicht")
	}
}
