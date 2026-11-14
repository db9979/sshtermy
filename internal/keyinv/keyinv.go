// Package keyinv: Schlüssel-Inventar. Welche öffentlichen SSH-Schlüssel
// stehen in welchen authorized_keys? Lesen geht nur lesend; Entfernen nur
// nach Bestätigung je Host, mit Sicherungskopie der Datei.
package keyinv

import (
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Script liest alle authorized_keys (mit root bzw. sudo -n die aller
// Benutzer, sonst nur die eigenen). Ausgabe:
//
//	root=1
//	key=<benutzer>|<datei>|<zeile>
const Script = `export LC_ALL=C
P=""
if [ "$(id -u)" = 0 ]; then echo "root=1"; elif sudo -n true 2>/dev/null; then P="sudo -n"; echo "root=1"; fi
emit() { u="$1"; f="$2"; $P cat "$f" 2>/dev/null | while IFS= read -r l || [ -n "$l" ]; do
  case "$l" in ''|'#'*) continue;; esac; printf 'key=%s|%s|%s\n' "$u" "$f" "$l"; done; }
if [ -n "$P" ] || [ "$(id -u)" = 0 ]; then
  getent passwd 2>/dev/null >/dev/null && PW="getent passwd" || PW="cat /etc/passwd"
  $PW | while IFS=: read -r u x uid gid g h sh; do
    [ -n "$h" ] || continue
    for f in "$h/.ssh/authorized_keys" "$h/.ssh/authorized_keys2"; do
      $P test -f "$f" 2>/dev/null && emit "$u" "$f"
    done
  done
  for f in /etc/ssh/authorized_keys/* ; do [ -f "$f" ] && emit "$(basename "$f")" "$f"; done
else
  for f in "$HOME/.ssh/authorized_keys" "$HOME/.ssh/authorized_keys2"; do [ -f "$f" ] && emit "$(id -un)" "$f"; done
fi
exit 0
`

// Entry: ein Schlüssel in einer Datei.
type Entry struct {
	User    string `json:"user"`
	File    string `json:"file"`
	Type    string `json:"type"`
	Bits    int    `json:"bits,omitempty"`
	FP      string `json:"fp"`   // SHA256:…
	Blob    string `json:"blob"` // base64 wie in der Datei (zum Entfernen)
	Comment string `json:"comment,omitempty"`
	Options string `json:"options,omitempty"`
}

// Host: Ergebnis eines Hosts.
type Host struct {
	Root    bool    `json:"root,omitempty"`
	Entries []Entry `json:"entries"`
	Bad     int     `json:"bad,omitempty"` // unlesbare Zeilen
}

// Parse liest die Ausgabe von Script.
func Parse(out string) Host {
	var h Host
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimRight(ln, "\r")
		switch {
		case ln == "root=1":
			h.Root = true
		case strings.HasPrefix(ln, "key="):
			parts := strings.SplitN(ln[4:], "|", 3)
			if len(parts) != 3 {
				continue
			}
			e, ok := ParseLine(parts[2])
			if !ok {
				h.Bad++
				continue
			}
			e.User, e.File = parts[0], parts[1]
			h.Entries = append(h.Entries, e)
		}
	}
	return h
}

// ParseLine: eine Zeile aus authorized_keys.
func ParseLine(line string) (Entry, bool) {
	pk, comment, opts, _, err := ssh.ParseAuthorizedKey([]byte(line))
	if err != nil {
		return Entry{}, false
	}
	e := Entry{Type: pk.Type(), FP: ssh.FingerprintSHA256(pk), Comment: comment, Options: strings.Join(opts, ",")}
	e.Blob = base64.StdEncoding.EncodeToString(pk.Marshal())
	e.Bits = bits(pk)
	return e, true
}

func bits(pk ssh.PublicKey) int {
	if cpk, ok := pk.(ssh.CryptoPublicKey); ok {
		if r, ok := cpk.CryptoPublicKey().(*rsa.PublicKey); ok {
			return r.N.BitLen()
		}
	}
	switch pk.Type() {
	case ssh.KeyAlgoED25519, ssh.KeyAlgoSKED25519:
		return 256
	case ssh.KeyAlgoECDSA256, ssh.KeyAlgoSKECDSA256:
		return 256
	case ssh.KeyAlgoECDSA384:
		return 384
	case ssh.KeyAlgoECDSA521:
		return 521
	case "ssh-dss": // nur zum Erkennen alter Schlüssel
		return 1024
	}
	return 0
}

// Weak: alter oder zu kurzer Schlüssel (Grund, "" = in Ordnung).
func (e Entry) Weak() string {
	switch {
	case e.Type == "ssh-dss":
		return "DSA"
	case e.Type == ssh.KeyAlgoRSA && e.Bits > 0 && e.Bits < 2048:
		return fmt.Sprintf("RSA %d", e.Bits)
	}
	return ""
}

// Place: wo ein Schlüssel steht.
type Place struct {
	Host string `json:"host"` // Verbindung (Alias)
	ID   string `json:"id"`   // Verbindungs-ID
	User string `json:"user"`
	File string `json:"file"`
}

// Key: ein Schlüssel über alle Hosts.
type Key struct {
	FP       string   `json:"fp"`
	Type     string   `json:"type"`
	Bits     int      `json:"bits,omitempty"`
	Blob     string   `json:"blob"`
	Comments []string `json:"comments,omitempty"`
	Places   []Place  `json:"places"`
	Own      string   `json:"own,omitempty"` // eigener Schlüssel dieses Rechners (Datei/Name)
}

// Hosts: Zahl verschiedener Hosts.
func (k Key) Hosts() int {
	seen := map[string]bool{}
	for _, p := range k.Places {
		seen[p.ID] = true
	}
	return len(seen)
}

// Weak wie Entry.Weak.
func (k Key) Weak() string { return Entry{Type: k.Type, Bits: k.Bits}.Weak() }

// Label: Kommentar oder „ohne Namen“.
func (k Key) Label() string {
	if k.Own != "" {
		return k.Own
	}
	if len(k.Comments) > 0 {
		return strings.Join(k.Comments, ", ")
	}
	return ""
}

// Merge fasst die Hosts zu Schlüsseln zusammen. own: Fingerabdruck →
// Name der eigenen Schlüssel dieses Rechners.
func Merge(hosts map[string]Host, names map[string]string, own map[string]string) []Key {
	idx := map[string]*Key{}
	var ids []string
	for id := range hosts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		for _, e := range hosts[id].Entries {
			k := idx[e.FP]
			if k == nil {
				k = &Key{FP: e.FP, Type: e.Type, Bits: e.Bits, Blob: e.Blob, Own: own[e.FP]}
				idx[e.FP] = k
			}
			if e.Comment != "" && !contains(k.Comments, e.Comment) {
				k.Comments = append(k.Comments, e.Comment)
			}
			k.Places = append(k.Places, Place{Host: names[id], ID: id, User: e.User, File: e.File})
		}
	}
	out := make([]Key, 0, len(idx))
	for _, k := range idx {
		out = append(out, *k)
	}
	sort.SliceStable(out, func(i, j int) bool {
		wi, wj := out[i].Weak() != "", out[j].Weak() != ""
		if wi != wj {
			return wi
		}
		if out[i].Hosts() != out[j].Hosts() {
			return out[i].Hosts() > out[j].Hosts()
		}
		return out[i].FP < out[j].FP
	})
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// RemoveScript: entfernt den Schlüssel (blob) aus der Datei. Legt vorher
// eine Kopie <datei>.sshtermy-<stamp> an und behält Rechte und Besitzer
// (Inhalt wird zurückgeschrieben, nicht ersetzt). Gibt „removed=N“ aus.
func RemoveScript(file, blob, stamp string) string {
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
	return fmt.Sprintf(`set -e
f=%s; b=%s
[ -f "$f" ] || { echo "removed=0"; exit 0; }
n=$(grep -cF -- "$b" "$f" || true)
[ "$n" -gt 0 ] || { echo "removed=0"; exit 0; }
cp -p "$f" "$f.sshtermy-%s"
grep -vF -- "$b" "$f.sshtermy-%s" > "$f.sshtermy-tmp" || true
cat "$f.sshtermy-tmp" > "$f"
rm -f "$f.sshtermy-tmp"
echo "removed=$n"
`, q(file), q(blob), stamp, stamp)
}

// ValidBlob: nur base64 (schützt das Skript).
func ValidBlob(b string) bool {
	if len(b) < 20 {
		return false
	}
	for _, r := range b {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '/' || r == '=') {
			return false
		}
	}
	return true
}
