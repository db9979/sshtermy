package importer

import (
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func byAlias(es []Entry) map[string]Entry {
	m := map[string]Entry{}
	for _, e := range es {
		m[e.Conn.Alias] = e
	}
	return m
}

func TestAnsibleINI(t *testing.T) {
	txt := `
# Inventar
[web]
web1 ansible_host=10.0.0.11 ansible_user=deploy ansible_port=2222
web[01:03].lan

[core]
sw-core ansible_host=10.0.0.2 ansible_password=geheim

[core:vars]
ansible_network_os=arubaoss
ansible_user=manager

[prod:children]
web
`
	es, err := ParseAnsible(txt)
	if err != nil {
		t.Fatal(err)
	}
	m := byAlias(es)
	if len(es) != 5 {
		t.Fatalf("%d Hosts: %v", len(es), m)
	}
	w := m["web1"].Conn
	if w.Host != "10.0.0.11" || w.Port != 2222 || w.Username != "deploy" || w.Group != "web" {
		t.Fatalf("web1 = %+v", w)
	}
	if m["web02.lan"].Conn.Host != "web02.lan" {
		t.Fatal("Bereich nicht aufgelöst")
	}
	sw := m["sw-core"]
	if sw.Conn.Device != domain.DeviceNetwork || sw.Conn.Username != "manager" || sw.Conn.Password != "" {
		t.Fatalf("Switch = %+v", sw.Conn)
	}
	if !strings.Contains(strings.Join(sw.Notes, " "), "Passwort") {
		t.Fatalf("kein Hinweis zum Passwort: %v", sw.Notes)
	}
	if !LooksLikeAnsible(txt, "inventory.ini") {
		t.Fatal("nicht erkannt")
	}
}

func TestAnsibleYAML(t *testing.T) {
	txt := `---
all:
  vars:
    ansible_user: admin
  children:
    db:
      hosts:
        db1:
          ansible_host: 192.168.1.5   # Primär
        db2:
    switches:
      vars:
        ansible_connection: network_cli
      hosts:
        sw1:
          ansible_host: "10.1.1.1"
          ansible_user: manager
`
	if !LooksLikeAnsible(txt, "hosts.yml") {
		t.Fatal("YAML nicht erkannt")
	}
	es, _ := ParseAnsible(txt)
	m := byAlias(es)
	if len(es) != 3 {
		t.Fatalf("%d Hosts: %v", len(es), m)
	}
	if c := m["db1"].Conn; c.Host != "192.168.1.5" || c.Username != "admin" || c.Group != "db" {
		t.Fatalf("db1 = %+v", c)
	}
	if c := m["db2"].Conn; c.Host != "db2" || c.Username != "admin" {
		t.Fatalf("db2 = %+v", c)
	}
	if c := m["sw1"].Conn; c.Host != "10.1.1.1" || c.Username != "manager" || c.Device != domain.DeviceNetwork {
		t.Fatalf("sw1 = %+v", c)
	}
}

func TestCSVNetBoxAndRoundTrip(t *testing.T) {
	netbox := "\ufeffName,Status,Site,Role,Platform,Primary IPv4\n" +
		"core-sw1,Active,Berlin,Core Switch,ArubaOS-CX,10.0.0.1/24\n" +
		"web01,Active,Berlin,Server,Ubuntu,10.0.1.10/24\n" +
		"leer,Active,Berlin,Server,,\n"
	if !LooksLikeCSV(netbox, "netbox_devices.txt") {
		t.Fatal("NetBox-Export nicht erkannt")
	}
	es, err := ParseCSV(netbox)
	if err != nil {
		t.Fatal(err)
	}
	m := byAlias(es)
	if c := m["core-sw1"].Conn; c.Host != "10.0.0.1" || c.Device != domain.DeviceNetwork || c.Group != "Berlin" {
		t.Fatalf("Switch = %+v", c)
	}
	if c := m["web01"].Conn; c.Device != "" || c.Host != "10.0.1.10" {
		t.Fatalf("Server = %+v", c)
	}
	if c := m["leer"].Conn; c.Host != "leer" {
		t.Fatalf("ohne IP: Name als Host erwartet: %+v", c)
	}

	// deutsches Excel: Semikolon
	de := "Name;IP-Adresse;Port;Benutzer;Ordner;Tags\nNAS;192.168.0.9;2222;admin;Zuhause;nas;backup\n"
	es, _ = ParseCSV(de)
	if len(es) != 1 || es[0].Conn.Port != 2222 || es[0].Conn.Group != "Zuhause" || es[0].Conn.Username != "admin" {
		t.Fatalf("Semikolon: %+v", es)
	}

	// Export und wieder einlesen
	conns := []domain.Connection{
		{ID: "a", Alias: "Jump", Host: "jump.example", Port: 22, Username: "j", Group: "DMZ"},
		{ID: "b", Alias: "Intern, alt", Host: "10.0.0.5", Port: 2200, Username: "root", Group: "Prod / Web", Tags: []string{"prod", "web"},
			JumpHostID: "a", Password: "geheim", KeyPath: "~/.ssh/id_ed25519"},
		{ID: "c", Alias: "sw", Host: "10.0.0.2", Port: 23, Protocol: domain.ProtoTelnet, Device: domain.DeviceNetwork},
	}
	out := ExportCSV(conns, func(id string) string {
		if id == "a" {
			return "Jump"
		}
		return ""
	})
	if strings.Contains(string(out), "geheim") {
		t.Fatal("Passwort im Export")
	}
	es, err = ParseCSV(string(out))
	if err != nil || len(es) != 3 {
		t.Fatalf("Rückimport: %v %d", err, len(es))
	}
	m = byAlias(es)
	b := m["Intern, alt"]
	if b.Conn.Host != "10.0.0.5" || b.Conn.Port != 2200 || b.Conn.Group != "Prod / Web" || b.Jump != "Jump" ||
		b.Conn.KeyPath != "~/.ssh/id_ed25519" || b.Conn.Auth != domain.AuthKeyFile || !strings.Contains(strings.Join(b.Conn.Tags, ","), "web") {
		t.Fatalf("Rückimport = %+v jump %q", b.Conn, b.Jump)
	}
	if c := m["sw"].Conn; c.Protocol != domain.ProtoTelnet || c.Port != 23 || c.Device != domain.DeviceNetwork {
		t.Fatalf("Telnet = %+v", c)
	}
}
