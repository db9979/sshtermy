package sharewire

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"strconv"
	"strings"

	"filippo.io/cpace"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Einladungscode zum Zusehen (Teilen v2)
//
// Ein Code wie „7-taube-kompass-ofen“ besteht aus dem Platz (Nummer der
// Freigabe beim Vermittler oder im lokalen Netz) und drei Wörtern. Die Wörter
// sind kein Schlüssel: Gastgeber und Zuschauer leiten daraus mit CPace (einem
// PAKE) einen gemeinsamen Schlüssel ab. Wer mitliest, lernt daraus nichts; wer
// rät, hat genau einen Versuch, danach sperrt der Gastgeber die Einladung.
//
// Der Link sshtermy://zusehen/CODE@host:port?fp=… trägt zusätzlich, wo die
// Freigabe liegt. Der gesprochene Code allein reicht im lokalen Netz und bei
// Vermittlern, die der Zuschauer kennt.

var shareWords = [256]string{
	"abend", "achse", "adler", "affe", "ahorn", "akte", "alarm", "alpen", "ampel", "anker", "apfel", "arena", "arm", "atlas", "auge", "auto",
	"bach", "bad", "bahn", "ball", "band", "bank", "bart", "bau", "baum", "becher", "beere", "berg", "besen", "bett", "biene", "bild",
	"birne", "blatt", "blitz", "blume", "boden", "bogen", "boot", "brief", "brot", "buch", "burg", "busch", "butter", "dach", "damm", "dampf",
	"decke", "deich", "delfin", "dose", "drache", "draht", "duft", "dune", "eber", "echo", "efeu", "eiche", "eimer", "eis", "elch", "engel",
	"ente", "erbse", "erde", "esel", "eule", "falke", "farbe", "feder", "feld", "fels", "fenster", "fest", "feuer", "film", "finger", "fisch",
	"flagge", "flasche", "fleck", "flocke", "floh", "fluss", "flut", "form", "frosch", "fuchs", "funke", "gabel", "gans", "garten", "geige", "geist",
	"gipfel", "glas", "glocke", "gold", "gras", "gurke", "hafen", "hagel", "hahn", "haken", "halle", "hammer", "hand", "harfe", "hase", "haus",
	"hecke", "heft", "held", "helm", "hemd", "herd", "herz", "heu", "himmel", "hirsch", "hof", "honig", "horn", "hose", "huhn", "hund",
	"hut", "igel", "insel", "jacke", "jaguar", "joker", "kabel", "kaffee", "kahn", "kakao", "kamel", "kamm", "kanal", "kanne", "karte", "kasse",
	"katze", "kegel", "kerze", "kette", "kino", "kirsche", "kissen", "kiste", "klee", "knopf", "koala", "koch", "koffer", "komet", "kompass", "korb",
	"kran", "krebs", "kreis", "krone", "kuchen", "kugel", "lager", "lampe", "land", "laub", "lava", "leiter", "licht", "linde", "loch", "luchs",
	"luft", "lupe", "magnet", "mais", "mantel", "maus", "meer", "melone", "messer", "milch", "minze", "mond", "motor", "nadel", "nebel", "nest",
	"netz", "nudel", "ofen", "ohr", "olive", "onkel", "orgel", "otter", "paket", "palme", "panda", "papier", "pfanne", "pferd", "pilz", "pinsel",
	"pirat", "planet", "pony", "post", "puma", "quelle", "rabe", "radio", "rakete", "regen", "reh", "riese", "ring", "rose", "sack", "salz",
	"sand", "schaf", "schal", "schiff", "segel", "sessel", "sofa", "sonne", "spiegel", "stern", "stiefel", "strand", "tafel", "tanne", "taube", "teich",
	"tiger", "tisch", "tomate", "turm", "uhr", "vogel", "wagen", "wal", "wald", "welle", "wolke", "zebra", "zelt", "ziege", "zucker", "zwerg",
}

var wordIndex = func() map[string]int {
	m := make(map[string]int, len(shareWords))
	for i, w := range shareWords {
		m[w] = i
	}
	return m
}()

// Invite: Platz + drei Wörter, optional mit Ort (aus dem Link).
type Invite struct {
	Platz int
	Words [3]string
	Base  string // https://host:port[/pfad], "" = suchen
	FP    string
}

// Code: „7-taube-kompass-ofen“.
func (i Invite) Code() string {
	return strconv.Itoa(i.Platz) + "-" + strings.Join(i.Words[:], "-")
}

// Link: sshtermy://zusehen/CODE@host:port[/pfad]?fp=…
func (i Invite) Link() string {
	s := "sshtermy://zusehen/" + i.Code()
	if i.Base != "" {
		s += "@" + strings.TrimPrefix(i.Base, "https://")
		if i.FP != "" {
			s += "?fp=" + i.FP
		}
	}
	return s
}

// password: das Geheimnis für CPace (nur die Wörter, kleingeschrieben).
func (i Invite) password() string { return strings.Join(i.Words[:], "-") }

func RandN(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(err)
	}
	return int(v.Int64())
}

// NewWords: drei zufällige Wörter (24 Bit, ein Rateversuch).
func NewWords() [3]string {
	var w [3]string
	for i := range w {
		w[i] = shareWords[RandN(len(shareWords))]
	}
	return w
}

// IsInvite: sieht s nach Code oder Link zum Zusehen aus?
func IsInvite(s string) bool {
	_, err := ParseInvite(s)
	return err == nil
}

// ParseInvite liest „7-taube-kompass-ofen“, „7 taube kompass ofen“ oder den Link.
func ParseInvite(s string) (Invite, error) {
	s = strings.Trim(strings.TrimSpace(s), "'\"<>")
	var inv Invite
	if i := strings.Index(s, "/zusehen/#"); i >= 0 && strings.Contains(s[:i], "https://") {
		// Browser-Link (https://host:port/zusehen/#CODE): sshTermy nimmt nur
		// den Code und sucht die Freigabe selbst (Fingerabdruck steht nicht drin).
		s = s[i+len("/zusehen/#"):]
	} else if i := strings.Index(s, "sshtermy://zusehen/"); i >= 0 {
		u, err := url.Parse(s[i:])
		if err != nil {
			return inv, i18n.NewError("Link zum Zusehen ungültig")
		}
		rest := strings.TrimPrefix(u.Path, "/")
		code, where, _ := strings.Cut(rest, "@")
		s = code
		if where != "" {
			if strings.Contains(where, "://") {
				return inv, i18n.NewError("Link zum Zusehen ungültig")
			}
			b, err := NormBase("https://" + where)
			if err != nil {
				return inv, err
			}
			inv.Base = b
			inv.FP = strings.ToLower(u.Query().Get("fp"))
			if inv.FP != "" && !IsFP(inv.FP) {
				return inv, i18n.NewError("Link zum Zusehen: Fingerabdruck ungültig")
			}
		}
	}
	f := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == '-' || r == ' ' || r == '·' || r == '_' || r == ',' })
	if len(f) != 4 {
		return inv, i18n.NewError("Code zum Zusehen: Zahl und drei Wörter, z. B. 7-taube-kompass-ofen")
	}
	n, err := strconv.Atoi(f[0])
	if err != nil || n < 1 || n > 9999 {
		return inv, i18n.NewError("Code zum Zusehen: beginnt mit einer Zahl")
	}
	inv.Platz = n
	for k := 0; k < 3; k++ {
		w := f[k+1]
		if _, ok := wordIndex[w]; !ok {
			return inv, fmt.Errorf(i18n.T("Code zum Zusehen: „%s“ ist kein Codewort"), w)
		}
		inv.Words[k] = w
	}
	return inv, nil
}

// ---------------------------------------------------------------- PAKE

func pakeCtx(platz int) *cpace.ContextInfo {
	return cpace.NewContextInfo("zuschauer", "gastgeber", []byte("sshtermy-teilen-v2 platz="+strconv.Itoa(platz)))
}

// PakeKeys: was beide Seiten aus dem gemeinsamen Geheimnis ableiten.
type PakeKeys struct {
	Key      []byte    // 32 Byte: verschlüsselt die Steuernachrichten zwischen beiden
	Host     []byte    // Bestätigung des Gastgebers
	Guest    []byte    // Bestätigung des Zuschauers
	Check    [3]string // Prüfwörter (beide Seiten zeigen sie an)
	ctlLabel string
}

func derive(secret []byte, platz int) PakeKeys {
	read := func(info string, n int) []byte {
		b := make([]byte, n)
		_, _ = io.ReadFull(hkdf.New(sha256.New, secret, nil, []byte("sshtermy-teilen "+info)), b)
		return b
	}
	k := PakeKeys{Key: read("schluessel", chacha20poly1305.KeySize), Host: read("gastgeber", 32), Guest: read("zuschauer", 32),
		ctlLabel: "k" + strconv.Itoa(platz)}
	c := read("pruefwoerter", 3)
	for i := range k.Check {
		k.Check[i] = shareWords[c[i]]
	}
	return k
}

// GuestPake: Zuschauer beginnt.
type GuestPake struct {
	inv Invite
	st  *cpace.State
}

// StartGuest: erste Nachricht des Zuschauers.
func StartGuest(inv Invite) (*GuestPake, []byte, error) {
	msg, st, err := cpace.Start(inv.password(), pakeCtx(inv.Platz))
	if err != nil {
		return nil, nil, err
	}
	return &GuestPake{inv: inv, st: st}, msg, nil
}

// Finish: Antwort des Gastgebers prüfen (Code richtig?).
func (g *GuestPake) Finish(msgB, hostTag []byte) (PakeKeys, error) {
	sec, err := g.st.Finish(msgB)
	if err != nil {
		return PakeKeys{}, ErrWrongCode
	}
	k := derive(sec, g.inv.Platz)
	if !hmac.Equal(k.Host, hostTag) {
		return PakeKeys{}, ErrWrongCode
	}
	return k, nil
}

var ErrWrongCode = i18n.NewError("Code passt nicht (vertippt oder abgelaufen?)")

// HostPake: Antwort des Gastgebers auf die erste Nachricht.
func HostPake(words [3]string, platz int, msgA []byte) (msgB []byte, k PakeKeys, err error) {
	inv := Invite{Platz: platz, Words: words}
	msgB, sec, err := cpace.Exchange(inv.password(), pakeCtx(platz), msgA)
	if err != nil {
		return nil, PakeKeys{}, err
	}
	return msgB, derive(sec, platz), nil
}

// SealCtl / OpenCtl: Steuernachrichten zwischen Gastgeber und einem Zuschauer.
func (k PakeKeys) SealCtl(p []byte) ([]byte, error) { return Seal(k.Key, k.ctlLabel, 'c', p) }

func (k PakeKeys) OpenCtl(b []byte) ([]byte, error) {
	_, p, err := Open(k.Key, k.ctlLabel, b)
	return p, err
}

// ---------------------------------------------------------------- Nachrichten

// Erstes Byte jeder Nachricht zwischen Gastgeber und Zuschauer (v2).
const (
	MsgPakeA = 'A' // Zuschauer → Gastgeber: CPace-Start
	MsgPakeB = 'B' // Gastgeber → Zuschauer: CPace-Antwort + Bestätigung
	MsgHello = 'C' // Zuschauer → Gastgeber: Bestätigung + Hallo (verschlüsselt)
	MsgCtl   = 'K' // beide Richtungen: Steuernachricht (verschlüsselt mit PAKE-Schlüssel)
	MsgFrame = 'S' // Gastgeber → Zuschauer: Bildschirm (verschlüsselt mit Freigabe-Schlüssel)
	MsgNo    = 'N' // Gastgeber → Zuschauer: abgelehnt / Code falsch (Klartext-Grund)
)

// Rahmenarten v2 (zusätzlich zu FrameData, FrameSize, FrameEnd).
const (
	FramePause  = 'p' // Gastgeber gibt gerade ein Passwort ein
	FrameResume = 'q' // weiter (danach kommt der Bildschirm)
	FrameInfo   = 'i' // JSON ShareInfo
)

// SealFrame / OpenFrame: Bildschirmrahmen mit dem Freigabe-Schlüssel.
func SealFrame(key []byte, id string, typ byte, p []byte) ([]byte, error) {
	f, err := Seal(key, id, typ, p)
	if err != nil {
		return nil, err
	}
	return append([]byte{MsgFrame}, f...), nil
}

func OpenFrame(key []byte, id string, b []byte) (byte, []byte, error) {
	if len(b) < 1 || b[0] != MsgFrame {
		return 0, nil, errors.New("kein Rahmen")
	}
	return Open(key, id, b[1:])
}

// PakeB: Antwort des Gastgebers zusammensetzen / lesen.
func PakeB(msgB, tag []byte) []byte {
	out := make([]byte, 0, 3+len(msgB)+len(tag))
	out = append(out, MsgPakeB)
	out = binary.BigEndian.AppendUint16(out, uint16(len(msgB)))
	out = append(out, msgB...)
	return append(out, tag...)
}

func SplitPakeB(b []byte) (msgB, tag []byte, ok bool) {
	if len(b) < 3 || b[0] != MsgPakeB {
		return nil, nil, false
	}
	n := int(binary.BigEndian.Uint16(b[1:3]))
	if len(b) < 3+n+32 {
		return nil, nil, false
	}
	return b[3 : 3+n], b[3+n : 3+n+32], true
}
