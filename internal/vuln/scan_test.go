package vuln

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleOut = `os=debian|12|Debian GNU/Linux 12 (bookworm)|
deb=ii |openssh-server|1:9.2p1-2+deb12u2|openssh|1:9.2p1-2+deb12u2
deb=ii |openssh-client|1:9.2p1-2+deb12u2|openssh|1:9.2p1-2+deb12u2
deb=ii |libexpat1|2.5.0-1|expat|2.5.0-1
deb=ii |curl|7.88.1-10+deb12u8|curl|7.88.1-10+deb12u8
deb=ii |bash|5.2.15-2+b7|bash|5.2.15-2
root=1
kernel=6.1.0-26-amd64
pid=812 /usr/sbin/sshd
pid=900 /usr/bin/bash
lis=tcp 0.0.0.0:22 users:(("sshd",pid=812,fd=3))
lis=tcp [::]:22 users:(("sshd",pid=812,fd=4))
own=openssh-server: /usr/sbin/sshd
own=bash: /bin/bash
ctr=abc123|/web|nginx:1.25-alpine|docker
@os=alpine|3.20.3|Alpine Linux v3.20|
@apk=musl|1.2.5-r3|musl
@apk=libcrypto3|3.3.2-r0|openssl
@apk=libssl3|3.3.2-r0|openssl
ctrend=abc123
ctr=def456|/distroless|gcr.io/distroless/base|docker
@osr=ID=debian
@osr=VERSION_ID="12"
@dst=Package: libc6
@dst=Status: install ok installed
@dst=Source: glibc
@dst=Version: 2.36-9+deb12u7
@dst=
ctrend=def456
ctrerr=podman
`

func TestParseInventory(t *testing.T) {
	inv := ParseInventory(sampleOut)
	if inv.OSID != "debian" || inv.Format != FormatDeb || len(inv.Packages) != 5 || !inv.Root {
		t.Fatalf("inv = %+v", inv)
	}
	if got := inv.Ecosystems(); len(got) != 1 || got[0] != "Debian:12" {
		t.Fatalf("eco = %v", got)
	}
	if r := inv.Running["openssh-server"]; len(r) != 1 || r[0] != "sshd :22" {
		t.Fatalf("running = %v", inv.Running)
	}
	if r := inv.Running["bash"]; len(r) != 1 || r[0] != "bash" {
		t.Fatalf("running bash (usrmerge) = %v", inv.Running)
	}
	if len(inv.Containers) != 2 {
		t.Fatalf("containers = %+v", inv.Containers)
	}
	c := inv.Containers[0]
	if c.Name != "web" || c.Inv.Format != FormatAPK || len(c.Inv.Packages) != 3 || c.Inv.Ecosystems()[0] != "Alpine:v3.20" {
		t.Fatalf("container 0 = %+v %+v", c, c.Inv)
	}
	d := inv.Containers[1].Inv
	if d.OSID != "debian" || len(d.Packages) != 1 || d.Packages[0].Source != "glibc" {
		t.Fatalf("distroless = %+v", d)
	}
	if len(inv.Notes) != 1 {
		t.Fatalf("notes = %v", inv.Notes)
	}
}

func TestEcosystems(t *testing.T) {
	for _, c := range []struct{ id, ver, want string }{
		{"ubuntu", "24.04", "Ubuntu:24.04:LTS"},
		{"ubuntu", "24.10", "Ubuntu:24.10"},
		{"ubuntu", "23.04", "Ubuntu:23.04"},
		{"alpine", "3.20.3", "Alpine:v3.20"},
		{"rocky", "9.4", "Rocky Linux:9"},
		{"almalinux", "9.4", "AlmaLinux:9"},
		{"rhel", "9.4", "Red Hat:enterprise_linux:9::baseos"},
		{"opensuse-leap", "15.6", "openSUSE:Leap 15.6"},
		{"sles", "15.6", "SUSE:Linux Enterprise Server 15 SP6"},
	} {
		inv := Inventory{OSID: c.id, OSVersion: c.ver}
		if got := inv.Ecosystems(); len(got) == 0 || got[0] != c.want {
			t.Errorf("%s %s = %v", c.id, c.ver, got)
		}
	}
	if (&Inventory{OSID: "arch"}).Ecosystems() != nil {
		t.Error("arch")
	}
	if srpmName("openssl-3.0.7-27.el9.src.rpm") != "openssl" || srpmName("util-linux-2.37.4-15.el9.src.rpm") != "util-linux" {
		t.Error("srpm")
	}
}

func vulnJSON(id, eco, pkg, fixed, sev string, cves ...string) Vuln {
	v := Vuln{ID: id, Upstream: cves, Details: "Problem in " + pkg + ". Mehr Text."}
	a := Affected{}
	a.Package.Name, a.Package.Ecosystem = pkg, eco
	ev := []Event{{Introduced: "0"}}
	if fixed != "" {
		ev = append(ev, Event{Fixed: fixed})
	}
	a.Ranges = []Range{{Type: "ECOSYSTEM", Events: ev}}
	v.Affected = []Affected{a}
	if sev != "" {
		v.Severity = []Severity{{Type: "CVSS_V3", Score: sev}}
	}
	return v
}

const crit = "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"
const high = "CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:H/I:H/A:H"

// fakeSrc: Einträge je Ökosystem/Paket.
type fakeSrc map[string][]Vuln

func (f fakeSrc) Find(_ context.Context, qs []Query, _ func(string)) ([][]*Vuln, error) {
	out := make([][]*Vuln, len(qs))
	for i, q := range qs {
		for _, v := range f[q.Eco+"/"+q.Name] {
			out[i] = append(out[i], &v)
		}
	}
	return out, nil
}

func (f fakeSrc) Lookup(_ context.Context, base, id string) *Vuln {
	if id == "DEBIAN-CVE-2024-0003" {
		v := vulnJSON(id, "Debian:12", "x", "", high)
		return &v
	}
	return nil
}

func TestScan(t *testing.T) {
	inv := ParseInventory(sampleOut)
	src := fakeSrc{
		"Debian:12/openssh": {vulnJSON("DEBIAN-CVE-2024-6387", "Debian:12", "openssh", "1:9.2p1-2+deb12u3", high, "CVE-2024-6387")},
		"Debian:12/expat":   {vulnJSON("DEBIAN-CVE-2024-45492", "Debian:12", "expat", "2.5.0-1+deb12u1", crit, "CVE-2024-45492")},
		// curl ist schon behoben
		"Debian:12/curl": {vulnJSON("DEBIAN-CVE-2024-9681", "Debian:12", "curl", "7.88.1-10+deb12u8", high, "CVE-2024-9681")},
		// zwei Einträge zur selben CVE (USN + Einzel-CVE) zählen einmal
		"Alpine:v3.20/openssl": {
			vulnJSON("ALPINE-CVE-2024-0003", "Alpine:v3.20", "openssl", "3.3.2-r1", "", "CVE-2024-0003"),
			vulnJSON("ALPINE-CVE-2024-0004", "Alpine:v3.20", "openssl", "", "", "CVE-2024-0004"),
		},
		"Debian:12/glibc": {vulnJSON("DEBIAN-CVE-2024-0005", "Debian:12", "glibc", "2.36-9+deb12u8", crit, "CVE-2024-0005")},
	}
	kev := KEV{"CVE-2024-6387": {CVE: "CVE-2024-6387", Ransomware: "Known"}}
	r, err := Scan(context.Background(), inv, src, kev, Options{Enrich: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 4 {
		for _, f := range r.Findings {
			t.Logf("%+v", f)
		}
		t.Fatalf("findings = %d", len(r.Findings))
	}
	f := r.Findings[0]
	if f.Package != "openssh" || !f.KEV || f.Fixed != "1:9.2p1-2+deb12u3" || len(f.Running) != 1 || len(f.Binaries) != 2 || !f.CVEs[0].Ransomware {
		t.Fatalf("first = %+v", f)
	}
	if got := UpdateCommand(f); got != "sudo apt-get update && sudo apt-get install --only-upgrade openssh-server openssh-client" {
		t.Fatalf("cmd = %q", got)
	}
	var alp *Finding
	for i := range r.Findings {
		if r.Findings[i].Package == "openssl" {
			alp = &r.Findings[i]
		}
	}
	if alp == nil || alp.Container != "web (nginx:1.25-alpine)" || alp.NoFix != 1 || alp.Fixed != "3.3.2-r1" || alp.Score != 8.1 || UpdateCommand(*alp) != "" {
		t.Fatalf("alpine = %+v", alp)
	}
	c := r.Count(nil)
	if c.KEV != 1 || c.Total != 4 || c.Critical != 2 {
		t.Fatalf("counts = %+v", c)
	}
}

func TestOnlineAndOffline(t *testing.T) {
	v := vulnJSON("DEBIAN-CVE-2024-6387", "Debian:12", "openssh", "1:9.2p1-2+deb12u3", high, "CVE-2024-6387")
	other := vulnJSON("DEBIAN-CVE-2024-1", "Debian:13", "openssh", "1:9.7p1-7", high, "CVE-2024-1")
	raw, _ := json.Marshal(v)
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	for _, x := range []Vuln{v, other} {
		w, _ := zw.Create(x.ID + ".json")
		b, _ := json.Marshal(x)
		w.Write(b)
	}
	zw.Close()
	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/querybatch":
			b, _ := io.ReadAll(r.Body)
			sent = string(b)
			w.Write([]byte(`{"results":[{"vulns":[{"id":"DEBIAN-CVE-2024-6387","modified":"2026-01-01T00:00:00Z"}]}]}`))
		case "/Debian/DEBIAN-CVE-2024-6387.json":
			w.Write(raw)
		case "/Debian/all.zip":
			w.Write(zbuf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	qs := []Query{{Eco: "Debian:12", Name: "openssh", Version: "1:9.2p1-2+deb12u2"}}
	on := &Online{HTTP{API: srv.URL, Bucket: srv.URL, Cache: t.TempDir()}}
	res, err := on.Find(context.Background(), qs, func(string) {})
	if err != nil || len(res[0]) != 1 || res[0][0].ID != v.ID {
		t.Fatalf("online = %v %v", res, err)
	}
	if strings.Contains(sent, "host") || !strings.Contains(sent, `"name":"openssh"`) {
		t.Fatalf("gesendet: %s", sent)
	}
	off := &Offline{HTTP: HTTP{Bucket: srv.URL, Cache: t.TempDir()}}
	res, err = off.Find(context.Background(), qs, func(string) {})
	if err != nil || len(res[0]) != 1 || !res[0][0].Check("Debian:12", "openssh", "1:9.2p1-2+deb12u2", FormatDeb).Affected {
		t.Fatalf("offline = %v %v", res, err)
	}
	if len(res[0][0].Affected) != 1 {
		t.Fatalf("offline behält fremde Ökosysteme: %+v", res[0][0].Affected)
	}
	// zweiter Lauf aus der gespeicherten Datenbank, ohne Netz
	srv.Close()
	off2 := &Offline{HTTP: HTTP{Bucket: srv.URL, Cache: off.Cache}}
	if res, err = off2.Find(context.Background(), qs, func(string) {}); err != nil || len(res[0]) != 1 {
		t.Fatalf("offline aus Cache = %v %v", res, err)
	}
}

func TestRanges(t *testing.T) {
	v := Vuln{Affected: []Affected{{Ranges: []Range{{Type: "ECOSYSTEM", Events: []Event{{Introduced: "1.0"}, {Fixed: "1.5"}, {Introduced: "2.0"}, {LastAffected: "2.3"}}}}}}}
	v.Affected[0].Package.Name, v.Affected[0].Package.Ecosystem = "p", "Debian:12"
	for ver, want := range map[string]bool{"0.9": false, "1.0": true, "1.4": true, "1.5": false, "1.9": false, "2.0": true, "2.3": true, "2.4": false} {
		if got := v.Check("Debian:12", "p", ver, FormatDeb).Affected; got != want {
			t.Errorf("%s = %v, want %v", ver, got, want)
		}
	}
	if h := v.Check("Debian:12", "p", "1.2", FormatDeb); h.Fixed != "1.5" {
		t.Errorf("fixed = %q", h.Fixed)
	}
}
