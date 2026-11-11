package relay

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// State ist alles, was der eigene Vermittler dauerhaft weiß (state.json im
// Datenordner, nur für den Besitzer lesbar). Schlüssel stehen nur als
// SHA-256 darin, nie im Klartext.
type State struct {
	Name    string           `json:"name"`
	URL     string           `json:"url,omitempty"` // öffentliche Basis-URL (für Kopplungszeilen)
	FP      string           `json:"fp,omitempty"`  // Pin des eigenen Zertifikats ("" = CA-Zertifikat/Proxy)
	Clients map[string]*Peer `json:"clients"`       // sshTermy-Geräte
	Anchors map[string]*Peer `json:"anker"`         // Server mit Anker
}

// Peer ist ein gekoppeltes Gerät (sshTermy) oder ein Server (Anker).
type Peer struct {
	Name    string    `json:"name"`
	OS      string    `json:"os,omitempty"`
	HostKey string    `json:"hostkey,omitempty"`
	Folder  string    `json:"ordner,omitempty"`
	KeySHA  string    `json:"key_sha"`
	Since   time.Time `json:"seit"`
}

const (
	stateFile = "state.json"
	codesDir  = "codes"
	// CodeTTL: so lange gilt ein Kopplungscode, und nur einmal.
	CodeTTL = 10 * time.Minute
	// MaxPeers: Geräte und Server je höchstens.
	MaxPeers = 200
)

// Arten von Kopplungscodes.
const (
	CodeClient = "client"
	CodeAnchor = "anker"
)

func newState() *State {
	return &State{Clients: map[string]*Peer{}, Anchors: map[string]*Peer{}}
}

// LoadState liest den Zustand (fehlt er, ist er leer).
func LoadState(dir string) (*State, error) {
	b, err := os.ReadFile(filepath.Join(dir, stateFile))
	if errors.Is(err, os.ErrNotExist) {
		return newState(), nil
	}
	if err != nil {
		return nil, err
	}
	s := newState()
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	if s.Clients == nil {
		s.Clients = map[string]*Peer{}
	}
	if s.Anchors == nil {
		s.Anchors = map[string]*Peer{}
	}
	return s, nil
}

// SaveState schreibt atomar (temporäre Datei, dann umbenennen).
func SaveState(dir string, s *State) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, stateFile), b)
}

func writeAtomic(path string, b []byte) error {
	tmp := path + ".neu"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// stateStamp: Zeit und Größe von state.json. Die Größe zählt mit, weil
// Windows zwei schnell aufeinanderfolgende Schreibvorgänge mit derselben
// Zeit versehen kann; sonst bliebe ein gerade entferntes Gerät zugelassen.
type stateStamp struct {
	mod  time.Time
	size int64
}

func stateMtime(dir string) stateStamp {
	fi, err := os.Stat(filepath.Join(dir, stateFile))
	if err != nil {
		return stateStamp{}
	}
	return stateStamp{fi.ModTime(), fi.Size()}
}

// newID: kurze zufällige Kennung mit Präfix ("c_", "a_", "k_").
func newID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// newKey: 32 zufällige Bytes; zurück kommen der Schlüssel und sein SHA-256.
func newKey() (key, sha string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	key = "st_" + base64.RawURLEncoding.EncodeToString(b)
	return key, keySHA(key)
}

func keySHA(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// byKey sucht den Eintrag zu einem Schlüssel (Vergleich in konstanter Zeit).
func byKey(m map[string]*Peer, key string) (string, *Peer) {
	if key == "" {
		return "", nil
	}
	want := keySHA(key)
	var id string
	var hit *Peer
	for k, p := range m {
		if hmac.Equal([]byte(p.KeySHA), []byte(want)) {
			id, hit = k, p
		}
	}
	return id, hit
}

// ---------------------------------------------------------------- Kopplungscodes

// Codes liegen als eigene Dateien im Datenordner (codes/<sha>.json), damit
// `sshtermy vermittler code` sie anlegen kann, während der Vermittler läuft.
type codeFile struct {
	Kind  string    `json:"art"`
	Until time.Time `json:"bis"`
}

const codeAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ" // ohne 0/O und 1/I

// NewCode legt einen Einmal-Code an (CodeTTL gültig).
func NewCode(dir, kind string) (string, error) {
	if kind != CodeClient && kind != CodeAnchor {
		return "", errors.New("bad kind")
	}
	cd := filepath.Join(dir, codesDir)
	if err := os.MkdirAll(cd, 0o700); err != nil {
		return "", err
	}
	cleanCodes(dir, time.Now())
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	var sb strings.Builder
	for _, x := range b {
		sb.WriteByte(codeAlphabet[int(x)%len(codeAlphabet)])
	}
	code := sb.String()
	j, _ := json.Marshal(codeFile{Kind: kind, Until: time.Now().Add(CodeTTL)})
	return code, writeAtomic(filepath.Join(cd, keySHA(normCode(code))+".json"), j)
}

func normCode(c string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(c), "-", ""))
}

// takeCode löst einen Code ein: gültig, richtige Art, danach gelöscht.
func takeCode(dir, code, kind string, now time.Time) bool {
	c := normCode(code)
	if len(c) < 6 || len(c) > 64 {
		return false
	}
	p := filepath.Join(dir, codesDir, keySHA(c)+".json")
	b, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	if os.Remove(p) != nil { // nur einmal, auch bei zwei gleichzeitigen Versuchen
		return false
	}
	var f codeFile
	return json.Unmarshal(b, &f) == nil && f.Kind == kind && now.Before(f.Until)
}

func cleanCodes(dir string, now time.Time) {
	cd := filepath.Join(dir, codesDir)
	es, _ := os.ReadDir(cd)
	for _, e := range es {
		p := filepath.Join(cd, e.Name())
		b, err := os.ReadFile(p)
		var f codeFile
		if err != nil || json.Unmarshal(b, &f) != nil || now.After(f.Until) {
			_ = os.Remove(p)
		}
	}
}

// ---------------------------------------------------------------- Verwaltung (CLI)

// PeerRow ist eine Zeile für `sshtermy vermittler liste`.
type PeerRow struct {
	ID, Kind, Name, OS string
	Since              time.Time
}

// ListPeers: Geräte und Server, nach Art und Name sortiert.
func ListPeers(dir string) ([]PeerRow, error) {
	s, err := LoadState(dir)
	if err != nil {
		return nil, err
	}
	var out []PeerRow
	for id, p := range s.Clients {
		out = append(out, PeerRow{id, CodeClient, p.Name, p.OS, p.Since})
	}
	for id, p := range s.Anchors {
		out = append(out, PeerRow{id, CodeAnchor, p.Name, p.OS, p.Since})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind > out[j].Kind
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// RemovePeer entfernt ein Gerät oder einen Server; der laufende Vermittler
// trennt ihn binnen weniger Sekunden.
func RemovePeer(dir, id string) error {
	s, err := LoadState(dir)
	if err != nil {
		return err
	}
	if _, ok := s.Clients[id]; ok {
		delete(s.Clients, id)
	} else if _, ok := s.Anchors[id]; ok {
		delete(s.Anchors, id)
	} else {
		return i18n.NewError("Kein Gerät und kein Server mit dieser Kennung")
	}
	return SaveState(dir, s)
}

// SetFolder ordnet einen Server einem Ordner zu (Anzeige in sshTermy).
func SetFolder(dir, id, folder string) error {
	s, err := LoadState(dir)
	if err != nil {
		return err
	}
	p, ok := s.Anchors[id]
	if !ok {
		return i18n.NewError("Kein Server mit dieser Kennung")
	}
	p.Folder = clean(folder, 120)
	return SaveState(dir, s)
}
