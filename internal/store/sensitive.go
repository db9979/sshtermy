package store

import (
	"net/url"
	"reflect"
	"strings"

	"github.com/db9979/sshtermy/internal/domain"
)

// Sensitive liefert alle Werte, die nicht in einen Absturzbericht gehören:
// jedes Feld vom Typ domain.Secret (Passwörter, Keys, Passphrasen,
// API-Keys, Sync-Schlüssel) und die Namen der Ziele (Hosts, Benutzer,
// Aliase, KI- und Sync-Adressen). Gesperrt: nil.
func (s *Store) Sensitive() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil
	}
	var out []string
	collectSecrets(reflect.ValueOf(s.data).Elem(), &out)
	names := []string{s.data.Sync.Folder}
	for _, c := range s.data.Connections {
		names = append(names, c.Host, c.Alias, c.Username)
	}
	for _, raw := range []string{s.data.LLM.BaseURL, s.data.Sync.URL} {
		if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
			names = append(names, u.Hostname())
		}
	}
	for _, n := range names {
		if distinctive(n) {
			out = append(out, n)
		}
	}
	return out
}

// distinctive: Namen wie „main“ oder „test“ kämen auch im Stack vor und
// würden ihn unlesbar machen; „web-01“, „nas.local“ oder „dominik“ nicht.
func distinctive(n string) bool {
	return len(n) >= 5 || (len(n) >= 3 && strings.ContainsAny(n, ".-_:0123456789"))
}

var secretType = reflect.TypeOf(domain.Secret(""))

func collectSecrets(v reflect.Value, out *[]string) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			collectSecrets(v.Elem(), out)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				collectSecrets(v.Field(i), out)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			collectSecrets(v.Index(i), out)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			collectSecrets(v.MapIndex(k), out)
		}
	case reflect.String:
		if v.Type() == secretType && v.String() != "" {
			*out = append(*out, v.String())
		}
	}
}
