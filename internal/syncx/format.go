// Package syncx gleicht Verbindungen, Tunnel und Einstellungen zwischen
// mehreren sshTermy-Installationen ab.
//
// # Sicherheitsmodell
//
// Synchronisiert wird nur eine einzige Datei (sshtermy-sync.vault), die schon
// auf dem Rechner verschlüsselt wird. Der Speicherort (Cloud-Ordner, NAS,
// WebDAV) sieht nie Klartext, auch keine Hostnamen.
//
//		Sync-Passwort ──Argon2id──▶ KEK ──AES-256-GCM──▶ Sync-Schlüssel (32 Byte, zufällig)
//		                                                        │
//		                              AES-256-GCM(Sync-Schlüssel) über den Inhalt
//
//	  - Der Sync-Schlüssel ist unabhängig vom lokalen Store-Schlüssel. Die
//	    DPAPI-Bindung bleibt dadurch lokal: jeder Rechner entsperrt seinen
//	    eigenen Store wie bisher und kennt zusätzlich den Sync-Schlüssel.
//	  - Das Sync-Passwort wird nur bei der Einrichtung auf einem Rechner einmal
//	    eingegeben. Danach liegt der Sync-Schlüssel im lokalen, verschlüsselten
//	    Store (nie das Passwort selbst).
//	  - Der Header (KDF-Parameter, Wrap, Schlüssel-ID) ist als AAD mitgeschützt.
//	    Die Schlüssel-ID (SHA-256 über einen festen Kontext und den Schlüssel,
//	    gekürzt) erkennt, ob die Datei mit einem anderen Schlüssel neu angelegt
//	    wurde, ohne etwas über den Schlüssel zu verraten.
package syncx

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/db9979/sshtermy/internal/vault"
)

const (
	FileName      = "sshtermy-sync.vault"
	formatName    = "sshtermy-sync"
	formatVer     = 1
	payloadFormat = 1 // Version des Inhalts (siehe Payload.Format)
	aadWrap       = "sshtermy/v1/sync/wrap"
	aadPayload    = "sshtermy/v1/sync/payload"
	minPassLen    = 8
)

var (
	ErrNotFound    = errors.New("Sync-Datei noch nicht vorhanden")
	ErrConflict    = errors.New("Sync-Datei wurde gleichzeitig geändert")
	ErrWrongPass   = errors.New("Sync-Passwort falsch")
	ErrKeyMismatch = errors.New("die Sync-Datei wurde mit einem anderen Sync-Passwort neu angelegt; bitte im Sync-Assistenten (y) das Sync-Passwort eingeben")
	ErrNotSyncFile = errors.New("die Datei ist keine sshTermy-Sync-Datei")
	ErrTooNew      = errors.New("die Sync-Datei stammt von einer neueren sshTermy-Version; bitte zuerst aktualisieren (F12)")
)

type wrap struct {
	Nonce []byte `json:"nonce"`
	Blob  []byte `json:"blob"`
}

type header struct {
	Format  string          `json:"format"`
	Version int             `json:"version"`
	KeyID   string          `json:"key_id"`
	KDF     vault.KDFParams `json:"kdf"`
	Wrap    wrap            `json:"wrap"`
}

type file struct {
	header
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// Key ist der Sync-Schlüssel samt dem Header, mit dem er per Passwort
// gesichert ist (damit eine fehlende Datei neu geschrieben werden kann).
type Key struct {
	DEK  []byte          `json:"dek"`
	KDF  vault.KDFParams `json:"kdf"`
	Wrap wrap            `json:"wrap"`
	ID   string          `json:"id"`
}

func keyID(dek []byte) string {
	h := sha256.Sum256(append([]byte("sshtermy/v1/sync/key-id\x00"), dek...))
	return hex.EncodeToString(h[:8])
}

// NewKey erzeugt einen neuen Sync-Schlüssel und sichert ihn mit dem Passwort.
func NewKey(pass []byte) (*Key, error) {
	if len(pass) < minPassLen {
		return nil, fmt.Errorf("das Sync-Passwort braucht mindestens %d Zeichen", minPassLen)
	}
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return nil, err
	}
	kdf, err := vault.DefaultKDF()
	if err != nil {
		return nil, err
	}
	kek := kdf.Derive(pass)
	defer vault.Wipe(kek)
	n, ct, err := vault.SealGCM(kek, dek, []byte(aadWrap))
	if err != nil {
		return nil, err
	}
	return &Key{DEK: dek, KDF: kdf, Wrap: wrap{n, ct}, ID: keyID(dek)}, nil
}

// UnlockKey öffnet den Sync-Schlüssel einer vorhandenen Datei mit dem Passwort.
func UnlockKey(raw, pass []byte) (*Key, error) {
	h, err := peek(raw)
	if err != nil {
		return nil, err
	}
	if err := h.KDF.Check(); err != nil {
		return nil, err
	}
	kek := h.KDF.Derive(pass)
	defer vault.Wipe(kek)
	dek, err := vault.OpenGCM(kek, h.Wrap.Nonce, h.Wrap.Blob, []byte(aadWrap))
	if err != nil || len(dek) != 32 || keyID(dek) != h.KeyID {
		return nil, ErrWrongPass
	}
	k := &Key{DEK: dek, KDF: h.KDF, Wrap: h.Wrap, ID: h.KeyID}
	// Inhalt testweise öffnen: erkennt eine manipulierte Datei schon bei der Einrichtung.
	if _, err := Decode(k, raw); err != nil {
		return nil, err
	}
	return k, nil
}

func peek(raw []byte) (header, error) {
	var f file
	if err := json.Unmarshal(raw, &f); err != nil || f.Format != formatName {
		return header{}, ErrNotSyncFile
	}
	if f.Version > formatVer {
		return header{}, ErrTooNew
	}
	return f.header, nil
}

func aad(h header) ([]byte, error) {
	b, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	return append([]byte(aadPayload+"\x00"), b...), nil
}

// Encode verschlüsselt den Inhalt zur Sync-Datei.
func Encode(k *Key, p *Payload) ([]byte, error) {
	p.Format = payloadFormat
	plain, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	defer vault.Wipe(plain)
	h := header{Format: formatName, Version: formatVer, KeyID: k.ID, KDF: k.KDF, Wrap: k.Wrap}
	a, err := aad(h)
	if err != nil {
		return nil, err
	}
	n, ct, err := vault.SealGCM(k.DEK, plain, a)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(file{header: h, Nonce: n, Ciphertext: ct}, "", "  ")
}

// Decode prüft und entschlüsselt eine Sync-Datei.
func Decode(k *Key, raw []byte) (*Payload, error) {
	var f file
	if err := json.Unmarshal(raw, &f); err != nil || f.Format != formatName {
		return nil, ErrNotSyncFile
	}
	if f.Version > formatVer {
		return nil, ErrTooNew
	}
	if f.KeyID != k.ID {
		return nil, ErrKeyMismatch
	}
	a, err := aad(f.header)
	if err != nil {
		return nil, err
	}
	plain, err := vault.OpenGCM(k.DEK, f.Nonce, f.Ciphertext, a)
	if err != nil {
		return nil, errors.New("Sync-Datei beschädigt oder verändert (Prüfsumme passt nicht)")
	}
	defer vault.Wipe(plain)
	p := &Payload{}
	if err := json.Unmarshal(plain, p); err != nil {
		return nil, err
	}
	if p.Format > payloadFormat {
		return nil, ErrTooNew
	}
	return p, nil
}

// Marshal/ParseKey: Ablage des Schlüssels im lokalen Store (domain.SyncConfig.Key).
func (k *Key) Marshal() string {
	b, _ := json.Marshal(k)
	return base64.StdEncoding.EncodeToString(b)
}

func ParseKey(s string) (*Key, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	k := &Key{}
	if err := json.Unmarshal(b, k); err != nil || len(k.DEK) != 32 {
		return nil, errors.New("Sync-Schlüssel im Store unlesbar")
	}
	return k, nil
}
