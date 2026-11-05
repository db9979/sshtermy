// Package vault verschlüsselt den gesamten Datenbestand von sshtermy.
//
// # Sicherheitsmodell (bitte vor Änderungen lesen)
//
// Schlüsselhierarchie (Envelope Encryption):
//
//		Master-Passwort ──Argon2id──▶ KEK_pw ──AES-256-GCM──┐
//		                                                     ├──▶ DEK (32 Byte, zufällig)
//		Windows-Anmeldung ──DPAPI (CryptProtectData)─────────┘        │
//		                                                               ▼
//		                                   AES-256-GCM(DEK) über den gesamten Store
//		                                   (Verbindungen, Passwörter, Keys, API-Keys)
//
//	  - DEK (Data Encryption Key): 256 Bit aus crypto/rand. Er verschlüsselt den
//	    kompletten JSON-Store in einem Stück. Dadurch sind nicht nur Passwörter,
//	    sondern auch Hostnamen, Benutzer und Tunnel-Ziele geschützt (Metadaten).
//	  - AES-256-GCM: authentifizierte Verschlüsselung. Jede Änderung am Chiffrat
//	    oder am Header (KDF-Parameter, Wraps) lässt das Entschlüsseln scheitern,
//	    weil der Header als "Additional Authenticated Data" (AAD) mitgeprüft wird.
//	    Für jedes Speichern wird eine neue 96-Bit-Nonce aus crypto/rand gezogen;
//	    bei zufälligen Nonces ist das bis ~2^32 Speichervorgänge pro DEK sicher.
//	  - Master-Passwort: wird nie gespeichert. Argon2id (RFC 9106, speicherhart,
//	    Standard: t=3, m=64 MiB, p=4) leitet daraus den KEK ab, der den DEK
//	    "einwickelt" (wrap). Falsches Passwort ⇒ GCM-Tag passt nicht ⇒ Fehler,
//	    ohne dass ein Orakel über Teilergebnisse entsteht.
//	  - DPAPI (nur Windows): CryptProtectData bindet den DEK an das Windows-Konto
//	    des Nutzers (Schlüssel aus dem Anmeldegeheimnis, verwaltet von LSASS).
//	    Damit entsperrt sich der Store nach der Windows-Anmeldung ohne Eingabe,
//	    ist aber für andere Konten und auf anderen Rechnern unlesbar. Ein
//	    zusätzlicher Entropy-Wert bindet den Blob an sshtermy, sodass andere
//	    Programme desselben Nutzers ihn nicht "zufällig" per CryptUnprotectData
//	    ohne diesen Wert öffnen können (Defense in Depth, kein Ersatz für ein
//	    Passwort bei Malware im selben Konto).
//	  - Das Master-Passwort bleibt bei aktivem DPAPI als Wiederherstellungs-Weg
//	    bestehen (z. B. Umzug auf einen neuen Rechner oder macOS).
//
// Was der Vault bewusst NICHT verhindert: Schadsoftware, die im selben
// Benutzerkonto läuft, während der Store entsperrt ist, kann den Arbeitsspeicher
// lesen. Deshalb: automatische Sperre nach Leerlauf (Settings.LockAfterMin) und
// Nullen des DEK beim Sperren (best effort, Go hat keinen garantierten
// Speicher-Wipe).
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/argon2"
)

const (
	formatName = "sshtermy-vault"
	formatVer  = 1
	keyLen     = 32 // AES-256
	saltLen    = 16
	// AAD-Kontexte: verhindern, dass ein Chiffrat aus einem Zusammenhang in einem
	// anderen akzeptiert wird (z. B. ein Wrap-Blob als Store-Payload).
	aadWrapPassword = "sshtermy/v1/wrap/password"
	aadPayload      = "sshtermy/v1/payload"
	dpapiEntropy    = "sshtermy/v1/dpapi"
)

var (
	ErrLocked        = errors.New("vault: gesperrt")
	ErrWrongPassword = errors.New("vault: Master-Passwort falsch oder Datei verändert")
	ErrNoUnlockPath  = errors.New("vault: kein Entsperr-Weg verfügbar (Passwort nötig)")
	ErrDPAPI         = errors.New("vault: DPAPI auf diesem System nicht verfügbar")
)

// KDFParams für Argon2id. Werte werden im Header gespeichert, damit sie später
// erhöht werden können, ohne alte Dateien unlesbar zu machen.
type KDFParams struct {
	Alg       string `json:"alg"`
	Salt      []byte `json:"salt"`
	Time      uint32 `json:"time"`
	MemoryKiB uint32 `json:"memory_kib"`
	Threads   uint8  `json:"threads"`
}

// DefaultKDF: OWASP/RFC-9106-nahe Werte, auf ARM64-Laptops ~0,3–0,6 s.
func DefaultKDF() (KDFParams, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return KDFParams{}, err
	}
	return KDFParams{Alg: "argon2id", Salt: salt, Time: 3, MemoryKiB: 64 * 1024, Threads: 4}, nil
}

func (p KDFParams) derive(password []byte) []byte {
	return argon2.IDKey(password, p.Salt, p.Time, p.MemoryKiB, p.Threads, keyLen)
}

// Wrap ist eine eingewickelte Kopie des DEK.
type Wrap struct {
	Kind  string `json:"kind"` // "password" | "dpapi"
	Nonce []byte `json:"nonce,omitempty"`
	Blob  []byte `json:"blob"`
}

// header wird als AAD authentifiziert (alles außer Nonce und Chiffrat).
type header struct {
	Format  string     `json:"format"`
	Version int        `json:"version"`
	KDF     *KDFParams `json:"kdf,omitempty"`
	Wraps   []Wrap     `json:"wraps"`
}

type file struct {
	header
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// Vault hält den entschlüsselten DEK im Speicher, solange er entsperrt ist.
type Vault struct {
	mu   sync.Mutex
	path string
	hdr  header
	dek  []byte // nil = gesperrt
}

// Open liest nur den Header; entschlüsselt wird erst mit Unlock*.
func Open(path string) (*Vault, error) {
	v := &Vault{path: path}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return v, nil // neuer Store, Create aufrufen
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("vault: Datei unlesbar: %w", err)
	}
	if f.Format != formatName || f.Version != formatVer {
		return nil, fmt.Errorf("vault: unbekanntes Format %q v%d", f.Format, f.Version)
	}
	v.hdr = f.header
	return v, nil
}

// Exists: gibt es schon einen Store?
func (v *Vault) Exists() bool { return len(v.hdr.Wraps) > 0 }

// HasDPAPI: wurde der DEK zusätzlich per DPAPI gesichert?
func (v *Vault) HasDPAPI() bool { return v.findWrap("dpapi") != nil }

// Unlocked meldet, ob der DEK im Speicher liegt.
func (v *Vault) Unlocked() bool { v.mu.Lock(); defer v.mu.Unlock(); return v.dek != nil }

func (v *Vault) findWrap(kind string) *Wrap {
	for i := range v.hdr.Wraps {
		if v.hdr.Wraps[i].Kind == kind {
			return &v.hdr.Wraps[i]
		}
	}
	return nil
}

// Create legt einen neuen Store an: neuer DEK, Passwort-Wrap und (wenn
// gewünscht und verfügbar) DPAPI-Wrap.
func (v *Vault) Create(password []byte, useDPAPI bool) error {
	if len(password) < 8 {
		return errors.New("vault: Master-Passwort braucht mindestens 8 Zeichen")
	}
	dek := make([]byte, keyLen)
	if _, err := rand.Read(dek); err != nil {
		return err
	}
	kdf, err := DefaultKDF()
	if err != nil {
		return err
	}
	pwWrap, err := wrapWithPassword(dek, password, kdf)
	if err != nil {
		return err
	}
	hdr := header{Format: formatName, Version: formatVer, KDF: &kdf, Wraps: []Wrap{pwWrap}}
	if useDPAPI && DPAPIAvailable() {
		blob, err := dpapiProtect(dek, []byte(dpapiEntropy))
		if err != nil {
			return fmt.Errorf("vault: DPAPI: %w", err)
		}
		hdr.Wraps = append(hdr.Wraps, Wrap{Kind: "dpapi", Blob: blob})
	}
	v.mu.Lock()
	v.hdr, v.dek = hdr, dek
	v.mu.Unlock()
	return nil
}

// UnlockDPAPI versucht, ohne Eingabe über das Windows-Konto zu entsperren.
func (v *Vault) UnlockDPAPI() error {
	w := v.findWrap("dpapi")
	if w == nil || !DPAPIAvailable() {
		return ErrNoUnlockPath
	}
	dek, err := dpapiUnprotect(w.Blob, []byte(dpapiEntropy))
	if err != nil || len(dek) != keyLen {
		return fmt.Errorf("vault: DPAPI-Entsperren fehlgeschlagen: %v", err)
	}
	v.mu.Lock()
	v.dek = dek
	v.mu.Unlock()
	return nil
}

// UnlockPassword entsperrt mit dem Master-Passwort.
func (v *Vault) UnlockPassword(password []byte) error {
	w := v.findWrap("password")
	if w == nil || v.hdr.KDF == nil {
		return ErrNoUnlockPath
	}
	kek := v.hdr.KDF.derive(password)
	defer wipe(kek)
	dek, err := gcmOpen(kek, w.Nonce, w.Blob, []byte(aadWrapPassword))
	if err != nil || len(dek) != keyLen {
		return ErrWrongPassword
	}
	v.mu.Lock()
	v.dek = dek
	v.mu.Unlock()
	return nil
}

// ChangePassword wickelt denselben DEK mit einem neuen Passwort (neues Salz).
func (v *Vault) ChangePassword(oldPw, newPw []byte) error {
	if err := v.UnlockPassword(oldPw); err != nil {
		return err
	}
	kdf, err := DefaultKDF()
	if err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	w, err := wrapWithPassword(v.dek, newPw, kdf)
	if err != nil {
		return err
	}
	v.hdr.KDF = &kdf
	for i := range v.hdr.Wraps {
		if v.hdr.Wraps[i].Kind == "password" {
			v.hdr.Wraps[i] = w
		}
	}
	return nil // wirksam mit dem nächsten Save
}

// Lock nullt den DEK im Speicher.
func (v *Vault) Lock() {
	v.mu.Lock()
	defer v.mu.Unlock()
	wipe(v.dek)
	v.dek = nil
}

// Load entschlüsselt die Nutzdaten in out.
func (v *Vault) Load(out any) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.dek == nil {
		return ErrLocked
	}
	raw, err := os.ReadFile(v.path)
	if err != nil {
		return err
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	aad, err := payloadAAD(f.header)
	if err != nil {
		return err
	}
	plain, err := gcmOpen(v.dek, f.Nonce, f.Ciphertext, aad)
	if err != nil {
		return ErrWrongPassword
	}
	defer wipe(plain)
	return json.Unmarshal(plain, out)
}

// Save verschlüsselt in und schreibt atomar (temporäre Datei + Rename), damit
// ein Absturz beim Schreiben nie einen halben Store hinterlässt. Rechte 0600.
func (v *Vault) Save(in any) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.dek == nil {
		return ErrLocked
	}
	plain, err := json.Marshal(in)
	if err != nil {
		return err
	}
	defer wipe(plain)
	aad, err := payloadAAD(v.hdr)
	if err != nil {
		return err
	}
	nonce, ct, err := gcmSeal(v.dek, plain, aad)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(file{header: v.hdr, Nonce: nonce, Ciphertext: ct}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(v.path), 0o700); err != nil {
		return err
	}
	tmp := v.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, v.path)
}

func payloadAAD(h header) ([]byte, error) {
	b, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	return append([]byte(aadPayload+"\x00"), b...), nil
}

func wrapWithPassword(dek, password []byte, kdf KDFParams) (Wrap, error) {
	kek := kdf.derive(password)
	defer wipe(kek)
	nonce, ct, err := gcmSeal(kek, dek, []byte(aadWrapPassword))
	if err != nil {
		return Wrap{}, err
	}
	return Wrap{Kind: "password", Nonce: nonce, Blob: ct}, nil
}

// gcmSeal: AES-256-GCM mit frischer Zufalls-Nonce (12 Byte).
func gcmSeal(key, plain, aad []byte) (nonce, ct []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return nonce, aead.Seal(nil, nonce, plain, aad), nil
}

func gcmOpen(key, nonce, ct, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, errors.New("nonce")
	}
	return aead.Open(nil, nonce, ct, aad)
}

// wipe überschreibt einen Puffer (best effort; verhindert keine Kopien, die
// die Laufzeit z. B. beim Wachsen eines Slices angelegt hat).
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
	subtle.ConstantTimeCompare(b, b) // verhindert, dass der Compiler die Schleife wegoptimiert
}

// ---------------------------------------------------------------- Bausteine
//
// Exportierte Grundfunktionen für andere verschlüsselte Formate (z. B. die
// Sync-Datei in internal/syncx). Sie verwenden dieselben Primitive wie der
// lokale Store, damit es im Projekt genau eine Krypto-Implementierung gibt.

// Derive leitet mit Argon2id einen 256-Bit-Schlüssel ab.
func (p KDFParams) Derive(password []byte) []byte { return p.derive(password) }

// Check lehnt KDF-Parameter ab, die zu schwach (Downgrade) oder absurd groß
// (Speicher-DoS durch eine manipulierte Datei) sind.
func (p KDFParams) Check() error {
	if p.Alg != "argon2id" || len(p.Salt) < saltLen || p.Time < 1 || p.Time > 20 ||
		p.MemoryKiB < 19*1024 || p.MemoryKiB > 1024*1024 || p.Threads < 1 {
		return errors.New("vault: ungültige KDF-Parameter")
	}
	return nil
}

// SealGCM verschlüsselt mit AES-256-GCM und frischer Zufalls-Nonce.
func SealGCM(key, plain, aad []byte) (nonce, ct []byte, err error) { return gcmSeal(key, plain, aad) }

// OpenGCM entschlüsselt und prüft Tag und AAD.
func OpenGCM(key, nonce, ct, aad []byte) ([]byte, error) { return gcmOpen(key, nonce, ct, aad) }

// Wipe überschreibt einen Puffer (best effort).
func Wipe(b []byte) { wipe(b) }

// SealBlob verschlüsselt einzelne Daten außerhalb des Stores (z. B. gesicherte
// Switch-Konfigurationen) mit demselben Datenschlüssel. aad bindet das
// Chiffrat an seinen Ablageort, damit Dateien nicht vertauscht werden können.
// Ergebnis: Nonce || Chiffrat.
func (v *Vault) SealBlob(plain, aad []byte) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.dek == nil {
		return nil, ErrLocked
	}
	nonce, ct, err := gcmSeal(v.dek, plain, aad)
	if err != nil {
		return nil, err
	}
	return append(nonce, ct...), nil
}

// OpenBlob entschlüsselt ein Ergebnis von SealBlob.
func (v *Vault) OpenBlob(blob, aad []byte) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.dek == nil {
		return nil, ErrLocked
	}
	const ns = 12 // GCM-Standard-Nonce
	if len(blob) < ns {
		return nil, errors.New("Datei beschädigt")
	}
	return gcmOpen(v.dek, blob[:ns], blob[ns:], aad)
}
