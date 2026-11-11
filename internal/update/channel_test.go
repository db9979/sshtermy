package update

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Kanal „stabil“: das neueste Release, das alt genug ist.
func TestCheckAged(t *testing.T) {
	at := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	rel := func(tag string, age time.Duration, extra string) string {
		return fmt.Sprintf(`{"tag_name":%q,"published_at":%q,"html_url":"https://github.com/x"%s}`, tag, at.Add(-age).Format(time.RFC3339), extra)
	}
	list := "[" + strings.Join([]string{
		rel("v9.0.5", time.Hour, ""),
		rel("v9.0.4", 23*time.Hour, ""),
		rel("v9.0.3", 30*time.Hour, `,"prerelease":true`),
		rel("v9.0.2", 40*time.Hour, ""),
		rel("v9.0.1", 9*24*time.Hour, ""),
	}, ",") + "]"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			_, _ = w.Write([]byte(rel("v9.0.5", time.Hour, "")))
			return
		}
		_, _ = w.Write([]byte(list))
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	allowedHosts[u.Hostname()] = true
	defer delete(allowedHosts, u.Hostname())
	oldClient, oldBase, oldNow := httpClient, apiBase, now
	httpClient = func() *http.Client { return ts.Client() }
	apiBase = ts.URL
	now = func() time.Time { return at }
	defer func() { httpClient, apiBase, now = oldClient, oldBase, oldNow }()

	for _, c := range []struct {
		wait time.Duration
		want string
	}{
		{0, "9.0.5"},
		{24 * time.Hour, "9.0.2"}, // 9.0.4 zu jung, 9.0.3 ist eine Vorabversion
		{7 * 24 * time.Hour, "9.0.1"},
		{30 * 24 * time.Hour, ""},
	} {
		r, err := CheckAged(context.Background(), c.wait)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if r != nil {
			got = r.Version
		}
		if got != c.want {
			t.Errorf("Wartezeit %v: %q, erwartet %q", c.wait, got, c.want)
		}
	}
}
