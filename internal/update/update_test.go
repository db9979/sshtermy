package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{{"1.0.10", "1.0.9", true}, {"1.0.9", "1.0.10", false}, {"1.1.0", "dev", true}, {"x", "1.0.0", false}, {"1.0.0", "1.0.0", false}}
	for _, c := range cases {
		if Newer(c.a, c.b) != c.want {
			t.Errorf("Newer(%s,%s)", c.a, c.b)
		}
	}
}

func TestGetRetries(t *testing.T) {
	old := retryWait
	retryWait = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	defer func() { retryWait = old }()
	n := 0
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n < 3 {
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	allowedHosts[u.Hostname()] = true
	defer delete(allowedHosts, u.Hostname())
	oldClient := httpClient
	httpClient = func() *http.Client { c := ts.Client(); return c }
	defer func() { httpClient = oldClient }()
	b, err := get(context.Background(), ts.URL, acceptBin, 100)
	if err != nil || string(b) != "ok" || n != 3 {
		t.Fatalf("%q %v %d", b, err, n)
	}
	n = 0 // erste Adresse kaputt (immer 504), zweite gut
	bad := ts.URL + "/kaputt"
	ts.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != acceptBin {
			t.Errorf("Accept %q", r.Header.Get("Accept"))
		}
		if r.URL.Path == "/kaputt" {
			n++
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	if b, err := download(context.Background(), []string{bad, ts.URL}, 100); err != nil || string(b) != "ok" || n != 4 {
		t.Fatalf("download: %q %v %d", b, err, n)
	}
	n = 0
	if _, err := download(context.Background(), []string{bad}, 100); err == nil || !strings.Contains(err.Error(), "504") || n != 4 {
		t.Fatalf("%v %d", err, n)
	}
}
