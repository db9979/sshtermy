package ui

import (
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestConnAccent(t *testing.T) {
	cases := []struct {
		c    domain.Connection
		want string
	}{
		{domain.Connection{Group: "Produktion / Web"}, "red"},
		{domain.Connection{Tags: []string{"live"}}, "red"},
		{domain.Connection{Alias: "web-staging"}, "green"},
		{domain.Connection{Tags: []string{"dev"}}, "green"},
		{domain.Connection{Group: "Labor"}, "blue"},
		{domain.Connection{Alias: "nas"}, ""},
		{domain.Connection{Alias: "nas", Color: "purple"}, "purple"},
		{domain.Connection{Group: "prod", Color: accentNone}, ""},
	}
	for _, c := range cases {
		if got := connAccent(c.c); got != c.want {
			t.Errorf("%+v: %q, erwartet %q", c.c, got, c.want)
		}
	}
	for _, d := range accentDefs {
		if accentFromLabel(accentLabel(d.id)) != d.id {
			t.Errorf("Rundreise %q kaputt", d.id)
		}
		if d.id != accentAuto && d.id != accentNone {
			for _, th := range Themes {
				if accentColor(th, d.id) == nil {
					t.Errorf("%s: keine Farbe für %q", th.Name, d.id)
				}
			}
		}
	}
}

func TestDeviceClass(t *testing.T) {
	p := func(sp domain.SystemProfile) domain.Connection { return domain.Connection{Profile: &sp} }
	cases := []struct {
		c    domain.Connection
		want devClass
	}{
		{domain.Connection{}, devUnknown},
		{domain.Connection{Device: domain.DeviceNetwork}, devSwitch},
		{p(domain.SystemProfile{Family: domain.FamilyLinux, OS: "Ubuntu 24.04"}), devLinux},
		{p(domain.SystemProfile{Family: domain.FamilyWindows}), devWindows},
		{p(domain.SystemProfile{Dialect: "powershell"}), devWindows},
		{p(domain.SystemProfile{Dialect: "procurve", Model: "Aruba 2930F"}), devSwitch},
		{p(domain.SystemProfile{Dialect: "cisco-ios", Model: "ISR4331"}), devSwitch}, // „ISR4331“ ist kein eigenes Wort „isr“
		{p(domain.SystemProfile{Dialect: "cisco-ios", Model: "Cisco ISR 4331"}), devRouter},
		{p(domain.SystemProfile{Dialect: "routeros"}), devRouter},
		{p(domain.SystemProfile{Dialect: "fortios"}), devFirewall},
		{p(domain.SystemProfile{Family: domain.FamilyBSD, OS: "pfSense 2.7"}), devFirewall},
		{p(domain.SystemProfile{Family: domain.FamilyLinux, OS: "Kasachstan-Linux"}), devLinux},
	}
	for _, c := range cases {
		if got := deviceClass(c.c); got != c.want {
			t.Errorf("%+v: %q, erwartet %q", c.c.Profile, got, c.want)
		}
	}
	for d := range devIcons {
		if devName(d) == "" {
			t.Errorf("kein Name für %q", d)
		}
	}
}
