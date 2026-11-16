package vuln

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const nvdSample = `{"resultsPerPage":2,"startIndex":0,"totalResults":3,"vulnerabilities":[
{"cve":{"id":"CVE-2024-21762","vulnStatus":"Analyzed","cisaExploitAdd":"2024-02-09",
 "descriptions":[{"lang":"en","value":"A out-of-bounds write in Fortinet FortiOS allows remote code execution."}],
 "metrics":{"cvssMetricV31":[{"type":"Primary","cvssData":{"baseScore":9.8}}]},
 "configurations":[{"nodes":[{"operator":"OR","negate":false,"cpeMatch":[
  {"vulnerable":true,"criteria":"cpe:2.3:o:fortinet:fortios:*:*:*:*:*:*:*:*","versionStartIncluding":"7.2.0","versionEndExcluding":"7.2.7"},
  {"vulnerable":true,"criteria":"cpe:2.3:o:fortinet:fortios:*:*:*:*:*:*:*:*","versionStartIncluding":"7.4.0","versionEndExcluding":"7.4.3"}]}]}]}},
{"cve":{"id":"CVE-2023-0001","vulnStatus":"Analyzed",
 "descriptions":[{"lang":"en","value":"Only 6.x."}],
 "metrics":{"cvssMetricV31":[{"type":"Primary","cvssData":{"baseScore":5.3}}]},
 "configurations":[{"nodes":[{"operator":"OR","cpeMatch":[
  {"vulnerable":true,"criteria":"cpe:2.3:o:fortinet:fortios:6.4.1:*:*:*:*:*:*:*"}]}]}]}}
]}`

const nvdSample2 = `{"resultsPerPage":2,"startIndex":2,"totalResults":3,"vulnerabilities":[
{"cve":{"id":"CVE-2023-0002","vulnStatus":"Analyzed",
 "descriptions":[{"lang":"en","value":"Exact 7.2.5 on some models."}],
 "metrics":{"cvssMetricV2":[{"cvssData":{"baseScore":7.5}}]},
 "configurations":[{"operator":"AND","nodes":[
  {"operator":"OR","cpeMatch":[{"vulnerable":true,"criteria":"cpe:2.3:o:fortinet:fortios:7.2.5:*:*:*:*:*:*:*"}]},
  {"operator":"OR","cpeMatch":[{"vulnerable":false,"criteria":"cpe:2.3:h:fortinet:fortigate-60f:-:*:*:*:*:*:*:*"}]}]}]}}
]}`

func TestNVDFirmware(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Query().Get("virtualMatchString") != "cpe:2.3:o:fortinet:fortios" {
			t.Errorf("Anfrage: %s", r.URL.RawQuery)
		}
		if r.Header.Get("apiKey") != "k" {
			t.Error("Schlüssel fehlt")
		}
		if r.URL.Query().Get("startIndex") == "0" {
			w.Write([]byte(nvdSample))
			return
		}
		w.Write([]byte(nvdSample2))
	}))
	defer srv.Close()
	n := &NVD{HTTP: HTTP{Cache: t.TempDir()}, URL: srv.URL, Key: "k", Wait: time.Millisecond}
	cves, stale, err := n.Product(context.Background(), "cpe:2.3:o:fortinet:fortios", nil)
	if err != nil || stale || len(cves) != 3 {
		t.Fatalf("%v %v %d", err, stale, len(cves))
	}
	r := FirmwareReportFor("cpe:2.3:o:fortinet:fortios", "v7.2.5,build1517 (GA)", "", cves, KEV{})
	if r.Version != "7.2.5" || len(r.Hits) != 2 {
		t.Fatalf("%+v", r)
	}
	if r.Hits[0].ID != "CVE-2024-21762" || !r.Hits[0].KEV || r.Hits[0].FixedIn != "7.2.7" || r.Hits[0].HW {
		t.Fatalf("%+v", r.Hits[0])
	}
	if !r.Hits[1].HW || r.Hits[1].Score != 7.5 {
		t.Fatalf("%+v", r.Hits[1])
	}
	if r.MinFixed != "7.2.7" || r.Counts().KEV != 1 || r.Counts().Critical != 1 {
		t.Fatalf("%+v %+v", r.MinFixed, r.Counts())
	}
	// zweiter Aufruf aus dem Cache
	c0 := atomic.LoadInt32(&calls)
	if _, _, err := n.Product(context.Background(), "cpe:2.3:o:fortinet:fortios", nil); err != nil || atomic.LoadInt32(&calls) != c0 {
		t.Fatal("Cache nicht benutzt")
	}
	// fixe Version: nichts
	if r := FirmwareReportFor("x", "7.2.7", "", cves, nil); len(r.Hits) != 0 {
		t.Fatalf("%+v", r.Hits)
	}
	raw, _ := json.Marshal(r)
	if len(raw) == 0 {
		t.Fatal()
	}
}

func TestNormFirmware(t *testing.T) {
	for _, c := range []struct{ raw, pat, want string }{
		{"WC.16.11.0012", "", "16.11.0012"},
		{"15.2(7)E8", "", "15.2(7)e8"},
		{"7.11.2 (stable)", "", "7.11.2"},
		{"21.4R3-S5.4", `(?i)(\d+\.\d+[a-z]\d+(?:-s\d+)?)`, "21.4r3-s5"},
	} {
		if got := NormFirmware(c.raw, c.pat); got != c.want {
			t.Errorf("%q → %q, will %q", c.raw, got, c.want)
		}
	}
	cves := []NVDCVE{{ID: "CVE-1", Score: 8, Matches: []NVDMatch{{Version: "15.2(7)e8"}}},
		{ID: "CVE-2", Score: 8, Matches: []NVDMatch{{Version: "21.4r3-s5"}}},
		{ID: "CVE-3", Score: 8, Matches: []NVDMatch{{EndExcl: "15.2(7)e10"}}}}
	if h := CheckFirmware(cves, "15.2(7)e8", nil); len(h) != 2 || h[0].FixedIn+h[1].FixedIn != "15.2(7)e10" {
		t.Fatalf("%+v", h)
	}
	if h := CheckFirmware(cves, "21.4r3-s5", nil); len(h) != 1 || h[0].ID != "CVE-2" {
		t.Fatalf("%+v", h)
	}
	if f := splitCPE(`cpe:2.3:o:cisco:ios:15.2\(7\)e8:*`); f[5] != "15.2(7)e8" {
		t.Fatal(f)
	}
}
