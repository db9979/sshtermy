// Package relay ist der „Vermittler“: Server hinter NAT oder Firewall bauen
// selbst eine Verbindung zu einem erreichbaren Vermittler auf (Anker), und
// sshTermy erreicht ihr sshd über diesen Vermittler. Der Vermittler reicht nur
// Bytes durch; SSH läuft Ende-zu-Ende zwischen sshTermy und dem Server, mit
// Host-Key-Prüfung und der normalen Anmeldung am Server.
//
//	sshTermy ──WSS /v1/kanal──► Vermittler ◄──WSS /v1/anker/kanal── Anker ──► sshd (127.0.0.1:22)
//
// Das Protokoll (Version 1, docs/vermittler.md) ist bewusst klein, damit es
// auch andere sprechen können: sshTermy bringt einen eigenen Vermittler und
// Anker mit (sshtermy vermittler / sshtermy anker), und das Panel von
// Speech on DGX Spark kann dieselbe Client-Seite anbieten. Ein Vermittler ist
// immer freiwillig: ohne ihn verbindet sshTermy wie bisher direkt.
package relay

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sharewire"
)

// Wege des Protokolls, relativ zur Basis-URL des Vermittlers.
const (
	PathPair          = "/v1/koppeln"       // POST {code, name} → {key, name, art}
	PathSystems       = "/v1/systeme"       // GET → SystemList
	PathUnlock        = "/v1/entsperren"    // POST {code} → {bis} (nur Vermittler mit Sperre, sonst 404)
	PathChannel       = "/v1/kanal"         // WebSocket ?system=ID: Bytes zum sshd des Systems
	PathAnchorPair    = "/v1/anker/koppeln" // POST {code, name, os, hostkey} → {id, key}
	PathAnchor        = "/v1/anker"         // WebSocket: Steuerleitung des Ankers
	PathAnchorChannel = "/v1/anker/kanal"   // WebSocket ?id=KANAL: ein Kanal des Ankers
	PathCodes         = "/v1/codes"         // POST {art} → {zeile}: Kopplungszeile (nur Geräte mit Verwaltungsrecht)
)

// Arten von Vermittlern (SystemList.Kind, Pair-Antwort).
const (
	KindSSHTermy = "sshtermy" // sshtermy vermittler
	KindSpark    = "spark"    // Speech on DGX Spark (Fernzugang)
)

// System ist ein Server, den der Vermittler kennt.
type System struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OS      string `json:"os,omitempty"`
	Online  bool   `json:"online"`
	HostKey string `json:"hostkey,omitempty"`  // authorized_keys-Format, vom Anker gemeldet
	Folder  string `json:"ordner,omitempty"`   // Ordner im Vermittler (Anzeige)
	User    string `json:"benutzer,omitempty"` // Vorschlag für den Benutzer (Spark: „spark“)
}

// SystemList ist die Antwort auf GET /v1/systeme.
type SystemList struct {
	Name      string   `json:"name"`                    // Anzeigename des Vermittlers
	Kind      string   `json:"art"`                     // KindSSHTermy, KindSpark …
	Locked    bool     `json:"gesperrt,omitempty"`      // erst entsperren (Code), dann gibt es Systeme und Kanäle
	UnlockMin int      `json:"entsperrt_min,omitempty"` // so lange gilt ein Code
	Systems   []System `json:"systeme"`
}

// PairResult ist die Antwort auf POST /v1/koppeln.
type PairResult struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Kind  string `json:"art"`
	Admin bool   `json:"verwalten,omitempty"` // darf weitere Codes holen (PathCodes)
}

// PairLine ist die einmalige Kopplungszeile:
//
//	https://host[:port][/pfad]#termy=CODE[&fp=SHA256]
//
// fp ist der SHA-256 des TLS-Zertifikats (hex), wenn der Vermittler ein
// eigenes (selbst signiertes) Zertifikat hat. Ohne fp gilt die normale
// Prüfung gegen die Zertifizierungsstellen des Systems.
type PairLine struct {
	Base string
	Code string
	FP   string
}

var (
	codeRE = regexp.MustCompile(`^[A-Za-z0-9_\-]{6,64}$`)
	fpRE   = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ParseLine liest eine Kopplungszeile. Leerzeichen, Anführungszeichen und ein
// vorangestelltes „sshtermy … koppeln“ werden toleriert (aus Copy & Paste).
func ParseLine(s string) (PairLine, error) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "https://"); i > 0 {
		s = s[i:]
	} else if i := strings.Index(s, "http://"); i > 0 {
		s = s[i:]
	}
	s = strings.Trim(s, "'\" ")
	base, frag, ok := strings.Cut(s, "#")
	if !ok {
		return PairLine{}, i18n.NewError("Kopplungszeile ohne „#termy=…“")
	}
	v, err := url.ParseQuery(frag)
	if err != nil {
		return PairLine{}, fmt.Errorf(i18n.T("Kopplungszeile: %w"), err)
	}
	p := PairLine{Code: v.Get("termy"), FP: strings.ToLower(strings.ReplaceAll(v.Get("fp"), ":", ""))}
	if !codeRE.MatchString(p.Code) {
		return PairLine{}, i18n.NewError("Kopplungszeile: Code fehlt oder ist ungültig")
	}
	if p.FP != "" && !fpRE.MatchString(p.FP) {
		return PairLine{}, i18n.NewError("Kopplungszeile: Fingerabdruck ungültig")
	}
	if p.Base, err = NormBase(base); err != nil {
		return PairLine{}, err
	}
	return p, nil
}

func (p PairLine) String() string {
	s := p.Base + "#termy=" + p.Code
	if p.FP != "" {
		s += "&fp=" + p.FP
	}
	return s
}

// NormBase prüft und vereinheitlicht eine Basis-URL: nur https, ohne
// Abfrage und Fragment, ohne abschließenden Schrägstrich.
func NormBase(s string) (string, error) { return sharewire.NormBase(s) }
