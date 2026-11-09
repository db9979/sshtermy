package sysprof

import (
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestFromBanner(t *testing.T) {
	for banner, want := range map[string]string{
		"SSH-2.0-Mocana SSH 6.3":                   "procurve",
		"SSH-2.0-ROSSSH":                           "routeros",
		"SSH-2.0-Cisco-1.25":                       "cisco-ios",
		"SSH-2.0-OpenSSH_9.6p1 Ubuntu-3ubuntu13.5": "posix",
		"SSH-2.0-OpenSSH_for_Windows_9.5":          "",
		"SSH-2.0-OpenSSH_8.9":                      "",
	} {
		if got := FromBanner(banner).Dialect; got != want {
			t.Errorf("%s: Dialekt %q, erwartet %q", banner, got, want)
		}
	}
	if FromBanner("SSH-2.0-OpenSSH_for_Windows_9.5").Family != domain.FamilyWindows {
		t.Error("Windows nicht erkannt")
	}
}

func TestFromProbe(t *testing.T) {
	p := FromProbe("Ubuntu 24.04.1 LTS\nLinux 6.8.0-45-generic x86_64\n/bin/bash\napt-get\nsystemd\n")
	if p.Family != domain.FamilyLinux || p.PkgMgr != "apt-get" || p.Init != "systemd" || p.OS != "Ubuntu 24.04.1 LTS" {
		t.Fatalf("%+v", p)
	}
	m := FromProbe("macOS 15.1\nDarwin 24.1.0 arm64\n/bin/zsh\nbrew\nlaunchd\n")
	if m.Family != domain.FamilyMacOS || m.Dialect != "macos" {
		t.Fatalf("%+v", m)
	}
	if !FromProbe("").Empty() {
		t.Fatal("leere Ausgabe muss leeres Profil geben")
	}
}

const procurveVersion = `switch# show version
Image stamp:    /ws/swbuildm/rel_ukiah_qaoff/code/build/bom(swbuildm_rel_ukiah_qaoff_rel_ukiah)
                Jan 13 2023 11:43:29
                WC.16.10.0023
                425
Boot Image:     Primary

Build Options:  None
switch# `

const procurveLogin = `HPE Networking

 Aruba JL256A 2930F-48G-PoE+-4SFP+ Switch
 Software revision WC.16.10.0023

 (C) Copyright 2023 Hewlett Packard Enterprise Development LP

Press any key to continue`

const cxVersion = `-----------------------------------------------------------------------------
ArubaOS-CX
(c) Copyright 2017-2024 Hewlett Packard Enterprise Development LP
-----------------------------------------------------------------------------
Version      : FL.10.13.1000
Build Date   : 2024-01-01 00:00:00 UTC
Product Name : JL658A 6300M 24SFP+ 4SFP56 Swch
cx01# `

func TestFromScreen(t *testing.T) {
	p := FromScreen(procurveLogin)
	if p.Dialect != "procurve" || p.Version != "WC.16.10.0023" || !strings.Contains(p.Model, "2930F") {
		t.Fatalf("ProCurve-Login: %+v", p)
	}
	if FromScreen(procurveVersion).Dialect != "procurve" {
		t.Fatal("ProCurve show version nicht erkannt")
	}
	cx := FromScreen(cxVersion)
	if cx.Dialect != "arubaos-cx" || cx.Version != "FL.10.13.1000" || !strings.Contains(cx.Model, "6300M") {
		t.Fatalf("CX: %+v", cx)
	}
	if r := FromScreen("[admin@MikroTik] > "); r.Dialect != "routeros" {
		t.Fatalf("RouterOS-Prompt: %+v", r)
	}
	if ps := FromScreen("PS C:\\Users\\admin> "); ps.Dialect != "powershell" || ps.Family != domain.FamilyWindows {
		t.Fatalf("PowerShell: %+v", ps)
	}
	if !FromScreen("user@host:~$ ls\nfoo bar\nuser@host:~$ ").Empty() {
		t.Fatal("normale Linux-Shell darf nichts behaupten")
	}
}

func TestRejection(t *testing.T) {
	cisco := "sw1#show ip int br\n                ^\n% Invalid input detected at '^' marker.\n\nsw1#"
	if r := Rejection(OutputAfter(cisco, "show ip int br")); !strings.HasPrefix(r, "% Invalid input") {
		t.Fatalf("Cisco-Fehler: %q", r)
	}
	pc := "switch# show ip interface brief\nInvalid input: interface\nswitch# "
	if r := Rejection(OutputAfter(pc, "show ip interface brief")); r != "Invalid input: interface" {
		t.Fatalf("ProCurve-Fehler: %q", r)
	}
	if r := Rejection("bash: htop: command not found"); r == "" {
		t.Fatal("command not found nicht erkannt")
	}
	if r := Rejection("total 0\ndrwxr-xr-x 2 root root 40 ."); r != "" {
		t.Fatalf("Fehlalarm: %q", r)
	}
	// Ältere Fehlermeldung über dem Befehl zählt nicht.
	old := "sw# foo\n% Invalid input\nsw# show vlans\nVLAN 1\nsw# "
	if r := Rejection(OutputAfter(old, "show vlans")); r != "" {
		t.Fatalf("alte Meldung gezählt: %q", r)
	}
}

func TestMerge(t *testing.T) {
	// Banner sagt nur „Netzwerkgerät“, das Terminal dann ArubaOS-CX.
	old := domain.SystemProfile{Family: domain.FamilyNetwork, Dialect: "network", Banner: "SSH-2.0-OpenSSH_8.9", Notes: "Core"}
	p, ch := Merge(old, FromScreen(cxVersion))
	if !ch || p.Dialect != "arubaos-cx" || p.OS != "ArubaOS-CX" || p.Notes != "Core" || p.Banner == "" {
		t.Fatalf("%+v", p)
	}
	// Ein allgemeiner Befund ersetzt keinen spezifischen.
	p2, _ := Merge(p, domain.SystemProfile{Family: domain.FamilyNetwork, Dialect: "network"})
	if p2.Dialect != "arubaos-cx" {
		t.Fatalf("spezifischer Dialekt verloren: %+v", p2)
	}
	// Von Hand gesetzt: bleibt.
	man := domain.SystemProfile{OS: "Mein Switch", Dialect: "procurve", Manual: true}
	p3, _ := Merge(man, FromScreen(cxVersion))
	if p3.Dialect != "procurve" || p3.OS != "Mein Switch" {
		t.Fatalf("manuelle Angabe überschrieben: %+v", p3)
	}
	// Nichts Neues: unverändert.
	if _, ch := Merge(p, domain.SystemProfile{}); ch {
		t.Fatal("leeres Profil darf nichts ändern")
	}
}

func TestProfilePromptAndFailed(t *testing.T) {
	p := domain.SystemProfile{Family: domain.FamilyNetwork, OS: "ArubaOS-Switch (ProCurve)", Dialect: "procurve", Notes: "nie reload"}
	for i := 0; i < domain.MaxFailed+3; i++ {
		p.AddFailed(FailedEntry("show ip int br "+strings.Repeat("x", i), "% Invalid input"))
	}
	if len(p.Failed) != domain.MaxFailed {
		t.Fatalf("Failed = %d", len(p.Failed))
	}
	if p.AddFailed(p.Failed[len(p.Failed)-1]) {
		t.Fatal("Dublette gemerkt")
	}
	pr := p.Prompt()
	for _, w := range []string{"Netzwerkgerät", "write memory", "nie reload", "abgelehnt"} {
		if !strings.Contains(pr, w) {
			t.Errorf("Prompt ohne %q:\n%s", w, pr)
		}
	}
	if s := p.Summary(); !strings.Contains(s, "ProCurve") {
		t.Errorf("Summary: %s", s)
	}
}

// ArubaOS-CX beantwortet die Abfrage mit einer Zeile Fehlermeldung.
func TestProbeRejected(t *testing.T) {
	if m := ProbeRejected("Command not supported.\r\n"); m != "Command not supported." {
		t.Fatalf("ProbeRejected = %q", m)
	}
	if !FromProbe("Command not supported.\r\n").Empty() {
		t.Fatal("Fehlermeldung als Systemprofil gelesen")
	}
	for _, ok := range []string{"", "Ubuntu 24.04.1 LTS\nLinux 6.8.0 x86_64\n/bin/bash\napt-get\nsystemd\nde_DE.UTF-8\n", "\n\n\n\n\n\n"} {
		if m := ProbeRejected(ok); m != "" {
			t.Fatalf("ProbeRejected(%q) = %q", ok, m)
		}
	}
}

// Früher gespeichertes Falschprofil („Command not supported.“ als Linux)
// wird überschrieben, sobald das Gerät als Netzwerkgerät erkannt ist.
func TestMergeServerToNetworkResets(t *testing.T) {
	old := domain.SystemProfile{Family: domain.FamilyLinux, OS: "Command not supported.", Dialect: "posix"}
	p, changed := Merge(old, domain.SystemProfile{Family: domain.FamilyNetwork, Dialect: "network"})
	if !changed || p.Family != domain.FamilyNetwork || p.OS != "" || p.Dialect != "network" {
		t.Fatalf("Merge = %+v (changed=%v)", p, changed)
	}
	man := old
	man.Manual = true
	if p, _ := Merge(man, domain.SystemProfile{Family: domain.FamilyNetwork, Dialect: "network"}); p.OS != old.OS {
		t.Fatalf("eigene Angabe überschrieben: %+v", p)
	}
}

// UniFi-Switch: Die Abfrage läuft (BusyBox-Shell), Zeile 7 verrät das Gerät.
func TestFromProbeUniFi(t *testing.T) {
	out := "\nLinux 3.6.5 mips\n/bin/sh\n\ninit\n\nUSW-24-PoE|USW.mt7621.v6.6.55.15189.230712.1033\n"
	p := FromProbe(out)
	if p.Family != domain.FamilyNetwork || p.Dialect != "unifi" || p.Model != "USW-24-PoE" || p.Version != "6.6.55.15189" {
		t.Fatalf("%+v", p)
	}
	if ProbeRejected(out) != "" {
		t.Fatal("UniFi-Abfrage als Ablehnung gelesen")
	}
	old := FromProbe("\nLinux 3.6.5 mips\n/bin/sh\n\ninit\n\n|US.bcm5334x.v4.3.13.11253.200430.1420\n")
	if old.Dialect != "unifi" || old.Version != "4.3.13.11253" {
		t.Fatalf("ältere Firmware: %+v", old)
	}
	// UniFi-AP und Gateway bleiben Linux
	for _, o := range []string{"\nLinux 4.4 mips\n/bin/sh\n\ninit\n\nUAP-AC-Lite|BZ.qca956x.v6.5.62.14789\n", "\nLinux 4.9 mips64\n/bin/sh\n\ninit\n\nUSG-3P|UGW3.v4.4.57.5578372\n"} {
		if q := FromProbe(o); q.Dialect != "posix" {
			t.Fatalf("%q: %+v", o, q)
		}
	}
	// Server ohne board.info: Zeile 7 ist nur „|“
	if s := FromProbe("Ubuntu 24.04.1 LTS\nLinux 6.8.0 x86_64\n/bin/bash\napt-get\nsystemd\nde_DE.UTF-8\n|\n"); s.Dialect != "posix" {
		t.Fatalf("%+v", s)
	}
	if d, ok := domain.DialectByID("unifi"); !ok || domain.FamilyOf(d.ID) != domain.FamilyNetwork {
		t.Fatal("Befehlssatz UniFi fehlt in der Auswahl")
	}
}

func TestFromScreenUniFi(t *testing.T) {
	banner := `  ___ ___      .-. _____ _
 | | | |____ _(_)/ __(_)| |  Welcome to UniFi USW-Lite-8-PoE!
 |___|_||_|_||_|_|_| |_|
      https://www.ui.com

USW-Lite-8-PoE.6.6.55# info

Model:       USW-Lite-8-PoE
Version:     6.6.55.15189
MAC Address: 74:ac:b9:00:11:22
Hostname:    sw-flur
Uptime:      266460 seconds
`
	p := FromScreen(banner)
	if p.Dialect != "unifi" || p.Model != "USW-Lite-8-PoE" || p.Version != "6.6.55.15189" {
		t.Fatalf("%+v", p)
	}
	if q := FromScreen("(UBNT) >enable\n(UBNT) #"); q.Dialect != "unifi" {
		t.Fatalf("FASTPATH-CLI: %+v", q)
	}
	if q := FromScreen("Welcome to UniFi USG-3P!\n"); q.Dialect == "unifi" {
		t.Fatal("Gateway als Switch erkannt")
	}
}
