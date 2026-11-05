package ui

import (
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/netscan"
)

func TestScanConnection(t *testing.T) {
	c := scanConnection(netscan.Host{IP: "10.0.0.5", Port: 22, Name: "sw-core.lan", Network: true}, "admin", domain.AuthAgent, "", "Switches")
	if c.Alias != "sw-core" || c.Host != "10.0.0.5" || c.Device != domain.DeviceNetwork || c.Group != "Switches" || c.Username != "admin" {
		t.Fatalf("%+v", c)
	}
	c = scanConnection(netscan.Host{IP: "10.0.0.6", Port: 2222}, "root", domain.AuthKeyFile, "~/.ssh/id_ed25519", "")
	if c.Alias != "10.0.0.6" || c.Port != 2222 || c.Device != domain.DeviceAuto || c.KeyPath != "~/.ssh/id_ed25519" {
		t.Fatalf("%+v", c)
	}
	if !ipLess("10.0.0.9", "10.0.0.10") || ipLess("10.0.1.1", "10.0.0.200") {
		t.Fatal("ipLess sortiert falsch")
	}
}
