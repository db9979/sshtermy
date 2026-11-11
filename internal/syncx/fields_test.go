package syncx

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

var updateFields = flag.Bool("update", false, "testdata/payload_fields.txt neu schreiben")

// payloadFields: alle JSON-Felder, die im Sync-Inhalt vorkommen können.
func payloadFields() []string {
	seen := map[reflect.Type]bool{}
	var out []string
	var walk func(t reflect.Type, path string)
	walk = func(t reflect.Type, path string) {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || t == reflect.TypeOf(time.Time{}) {
			return
		}
		if seen[t] {
			return
		}
		seen[t] = true
		defer delete(seen, t)
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			p := path + "." + name
			if f.Anonymous && f.Tag.Get("json") == "" {
				p = path
			} else {
				out = append(out, p)
			}
			walk(f.Type, p)
		}
	}
	walk(reflect.TypeOf(Payload{}), "payload")
	sort.Strings(out)
	return out
}

// Neues Feld im Sync-Inhalt? Dann muss payloadFormat steigen, sonst verliert
// ein älteres sshTermy das Feld still (siehe format.go). Danach:
//
//	go test ./internal/syncx -run TestPayloadFields -update
func TestPayloadFields(t *testing.T) {
	const file = "testdata/payload_fields.txt"
	now := fmt.Sprintf("format %d\n%s\n", payloadFormat, strings.Join(payloadFields(), "\n"))
	if *updateFields {
		_ = os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(file, []byte(now), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	was := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if was == now {
		return
	}
	wasLines := strings.Split(strings.TrimSpace(was), "\n")
	old := map[string]bool{}
	for _, l := range wasLines[1:] {
		old[l] = true
	}
	var added []string
	for _, f := range payloadFields() {
		if !old[f] {
			added = append(added, f)
		}
	}
	if len(added) > 0 && wasLines[0] == fmt.Sprintf("format %d", payloadFormat) {
		t.Fatalf("neue Felder im Sync-Inhalt: %v\npayloadFormat (format.go) erhöhen, dann: go test ./internal/syncx -run TestPayloadFields -update", added)
	}
	t.Fatalf("Feldliste veraltet: go test ./internal/syncx -run TestPayloadFields -update")
}

// Ein Stand von einem neueren sshTermy wird nicht angefasst.
func TestNewerPayloadRefused(t *testing.T) {
	k, err := NewKey([]byte("sync-passwort-1"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Encode(k, &Payload{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(k, raw); err != nil {
		t.Fatal(err)
	}
	raw, err = encodeAs(k, &Payload{}, payloadFormat+1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(k, raw); err != ErrTooNew {
		t.Fatalf("neuerer Stand angenommen: %v", err)
	}
}
