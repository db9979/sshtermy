package team

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Team-Ordner: Das Team-Paket liegt an einem festen Ort, den alle erreichen
// (Netzlaufwerk, OneDrive/Teams-Ordner, NAS per https/WebDAV). Wer
// veröffentlicht, schreibt es bei jeder Änderung neu; wer abonniert, liest es
// regelmäßig und übernimmt Änderungen samt gelöschter Verbindungen.

// ErrNotFound: am Ort liegt (noch) kein Team-Paket.
var ErrNotFound = i18n.NewError("kein Team-Paket an diesem Ort gefunden")

// IsURL: Ort ist eine http(s)-Adresse statt eines Dateipfads.
func IsURL(loc string) bool {
	l := strings.ToLower(strings.TrimSpace(loc))
	return strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://")
}

// ShowLoc zeigt einen Ort ohne Passwort aus der Adresse.
func ShowLoc(loc string) string {
	if !IsURL(loc) {
		return loc
	}
	u, err := url.Parse(strings.TrimSpace(loc))
	if err != nil {
		return loc
	}
	if u.User != nil {
		u.User = url.User(u.User.Username())
	}
	return u.String()
}

func request(ctx context.Context, method, loc string, body []byte) (*http.Response, error) {
	u, err := url.Parse(strings.TrimSpace(loc))
	if err != nil {
		return nil, err
	}
	var user, pass string
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
		u.User = nil
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	return http.DefaultClient.Do(req)
}

// ReadLoc liest das Team-Paket von einem Ort.
func ReadLoc(ctx context.Context, loc string) ([]byte, error) {
	if !IsURL(loc) {
		b, err := os.ReadFile(loc)
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return b, err
	}
	resp, err := request(ctx, http.MethodGet, loc, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("GET: HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

// WriteLoc schreibt das Team-Paket (Datei: erst temporär, dann umbenennen,
// damit Leser nie eine halbe Datei sehen).
func WriteLoc(ctx context.Context, loc string, data []byte) error {
	if !IsURL(loc) {
		if err := os.MkdirAll(filepath.Dir(loc), 0o700); err != nil {
			return err
		}
		tmp := filepath.Join(filepath.Dir(loc), "."+filepath.Base(loc)+".tmp")
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			return err
		}
		return os.Rename(tmp, loc)
	}
	resp, err := request(ctx, http.MethodPut, loc, data)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("PUT: HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return nil
}

// Sum: Prüfsumme für „hat sich etwas geändert?“.
func Sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:16]) }

// LinkOptions: Paket-Optionen eines veröffentlichten Team-Ordners.
func LinkOptions(l domain.TeamLink) Options {
	return Options{Users: l.Users, QuickCmds: l.QuickCmds, Runbooks: l.Runbooks, Tunnels: l.Tunnels}
}

// LinkIDs: Verbindungen, die ein veröffentlichter Team-Ordner enthält
// (ganzer Quellordner samt neuer Verbindungen plus einzeln gewählte).
func LinkIDs(d *domain.Data, l domain.TeamLink) []string {
	want := map[string]bool{}
	for _, id := range l.IDs {
		want[id] = true
	}
	var out []string
	for _, c := range d.Connections {
		if want[c.ID] || (l.Folder != "" && domain.InFolder(c.Group, l.Folder)) {
			out = append(out, c.ID)
		}
	}
	return out
}

// BuildLink stellt das Paket eines veröffentlichten Team-Ordners zusammen.
// sum ändert sich nur, wenn sich der Inhalt ändert (nicht mit der Uhrzeit).
func BuildLink(d *domain.Data, l domain.TeamLink, now time.Time) (p *Package, sum string) {
	p, _ = Build(d, LinkIDs(d, l), l.From, LinkOptions(l), now)
	if l.Folder != "" {
		// Ordnerpfad relativ zum Quellordner, sonst stünde beim Kollegen
		// „Team / Kunden / ACME / …“ statt „Team / …“
		parent := domain.PathParent(l.Folder)
		for i := range p.Connections {
			if g := p.Connections[i].Group; parent != "" && domain.InFolder(g, parent) {
				p.Connections[i].Group = strings.TrimPrefix(strings.TrimPrefix(g, parent), domain.PathSep)
			}
		}
	}
	q := *p
	q.Created = time.Time{}
	b, _ := json.Marshal(q)
	return p, Sum(b)
}

// Pull übernimmt ein gelesenes Paket in einen abonnierten Team-Ordner:
// neue und geänderte Verbindungen wie Apply, dazu werden Verbindungen
// entfernt, die früher aus dem Paket kamen und jetzt fehlen. Liefert die
// neue Have-Liste zurück.
func Pull(d *domain.Data, p *Package, l domain.TeamLink, now time.Time) (st Stats, have []string) {
	skip := map[string]bool{}
	for _, id := range l.Skip {
		skip[id] = true
	}
	ids := map[string]bool{}
	for _, c := range p.Connections {
		if !skip[c.ID] && c.ID != "" {
			ids[c.ID] = true
		}
	}
	st = Apply(d, p, ids, l.Into, l.DefaultUser, now)
	var gone []string
	for _, id := range l.Have {
		if !ids[id] && !skip[id] {
			gone = append(gone, id)
		}
	}
	st.Removed = Remove(d, gone, now)
	for id := range ids {
		have = append(have, id)
	}
	sort.Strings(have)
	return st, have
}

// Remove löscht Verbindungen samt ihrer Tunnel und Runbooks und legt
// Grabsteine an, damit der eigene Sync sie auch auf den anderen Rechnern
// entfernt.
func Remove(d *domain.Data, ids []string, now time.Time) int {
	if len(ids) == 0 {
		return 0
	}
	del := map[string]bool{}
	for _, id := range ids {
		del[id] = true
	}
	if d.Deleted == nil {
		d.Deleted = map[string]time.Time{}
	}
	now = now.UTC()
	n := 0
	cs := d.Connections[:0]
	for _, c := range d.Connections {
		if del[c.ID] {
			d.Deleted[c.ID] = now
			delete(d.LocalKeys, c.ID)
			n++
			continue
		}
		cs = append(cs, c)
	}
	d.Connections = cs
	ts := d.Tunnels[:0]
	for _, t := range d.Tunnels {
		if del[t.ConnectionID] {
			d.Deleted[t.ID] = now
			continue
		}
		ts = append(ts, t)
	}
	d.Tunnels = ts
	rs := d.Runbooks[:0]
	for _, r := range d.Runbooks {
		if r.ConnectionID != "" && del[r.ConnectionID] {
			d.Deleted[r.ID] = now
			continue
		}
		rs = append(rs, r)
	}
	d.Runbooks = rs
	return n
}
