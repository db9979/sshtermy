//go:build !windows

package sshx

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jcmturner/gokrb5/v8/client"
	"github.com/jcmturner/gokrb5/v8/config"
	"github.com/jcmturner/gokrb5/v8/credentials"
	"github.com/jcmturner/gokrb5/v8/gssapi"
	"github.com/jcmturner/gokrb5/v8/iana/chksumtype"
	"github.com/jcmturner/gokrb5/v8/iana/keyusage"
	"github.com/jcmturner/gokrb5/v8/messages"
	"github.com/jcmturner/gokrb5/v8/types"
	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/i18n"
)

// krbGSS: GSSAPI-Kerberos ohne C-Bibliothek (gokrb5). Ein AP-REQ ohne
// gegenseitige Prüfung genügt sshd; die Signatur (MIC) läuft über den
// Sitzungsschlüssel des Tickets mit der Folgenummer aus dem Authenticator.
type krbGSS struct {
	key   types.EncryptionKey
	seq   uint64
	first []byte // vorbereitetes AP-REQ (newGSSClient)
	// ticket liefert Ticket und Sitzungsschlüssel für den Dienst (Tests
	// setzen hier ein eigenes ein).
	ticket func(spn string) (messages.Ticket, types.EncryptionKey, string, types.PrincipalName, error)
}

// newGSSClient holt das Dienst-Ticket für spn und baut das erste Token.
func newGSSClient(spn string) (ssh.GSSAPIClient, error) {
	g := &krbGSS{ticket: ccacheTicket}
	if err := g.prepare(spn); err != nil {
		return nil, err
	}
	return g, nil
}

// ccachePath: Credential-Cache von kinit. Nur Datei-Caches gehen ohne
// C-Bibliothek (KCM:, API:, KEYRING: nicht).
func ccachePath() (string, error) {
	p := os.Getenv("KRB5CCNAME")
	if p == "" {
		return "/tmp/krb5cc_" + strconv.Itoa(os.Getuid()), nil
	}
	if rest, ok := strings.CutPrefix(p, "FILE:"); ok {
		return rest, nil
	}
	if i := strings.Index(p, ":"); i > 0 && !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf(i18n.T("Kerberos-Cache %q wird nicht unterstützt, bitte kinit -c FILE:/tmp/krb5cc_%d"), p[:i+1], os.Getuid())
	}
	return p, nil
}

func krbConfig(realm string) *config.Config {
	p := os.Getenv("KRB5_CONFIG")
	if p == "" {
		p = "/etc/krb5.conf"
	}
	if cfg, err := config.Load(p); err == nil {
		if cfg.LibDefaults.DefaultRealm == "" {
			cfg.LibDefaults.DefaultRealm = realm
		}
		return cfg
	}
	cfg := config.New()
	cfg.LibDefaults.DefaultRealm = realm
	cfg.LibDefaults.DNSLookupKDC = true
	cfg.LibDefaults.DNSLookupRealm = true
	return cfg
}

func ccacheTicket(spn string) (messages.Ticket, types.EncryptionKey, string, types.PrincipalName, error) {
	var tkt messages.Ticket
	var key types.EncryptionKey
	p, err := ccachePath()
	if err != nil {
		return tkt, key, "", types.PrincipalName{}, err
	}
	cc, err := credentials.LoadCCache(p)
	if err != nil {
		return tkt, key, "", types.PrincipalName{}, fmt.Errorf(i18n.T("kein Kerberos-Ticket (erst kinit): %w"), err)
	}
	cl, err := client.NewFromCCache(cc, krbConfig(cc.GetClientRealm()), client.DisablePAFXFAST(true))
	if err != nil {
		return tkt, key, "", types.PrincipalName{}, err
	}
	defer cl.Destroy()
	tkt, key, err = cl.GetServiceTicket(spn)
	return tkt, key, cc.GetClientRealm(), cc.GetClientPrincipalName(), err
}

// gssFlags: Integrität (für die MIC), keine Delegation, keine
// gegenseitige Prüfung.
const gssFlags = gssapi.ContextFlagInteg

func (g *krbGSS) InitSecContext(target string, token []byte, isGSSDelegCreds bool) ([]byte, bool, error) {
	if token != nil {
		return nil, false, nil // Antwort des Servers (AP-REP): fertig
	}
	if g.first == nil {
		// x/crypto reicht „host@name“ weiter, der Dienst heißt host/name
		if err := g.prepare(KerberosSPN(strings.TrimPrefix(target, "host@"))); err != nil {
			return nil, false, err
		}
	}
	out := g.first
	g.first = nil
	return out, false, nil
}

func (g *krbGSS) prepare(spn string) error {
	tkt, key, realm, cname, err := g.ticket(spn)
	if err != nil {
		return err
	}
	auth, err := types.NewAuthenticator(realm, cname)
	if err != nil {
		return err
	}
	ck := make([]byte, 24) // RFC 4121 4.1.1: Länge der Kanalbindung, leer, Flags
	binary.LittleEndian.PutUint32(ck[:4], 16)
	binary.LittleEndian.PutUint32(ck[20:], uint32(gssFlags))
	auth.Cksum = types.Checksum{CksumType: chksumtype.GSSAPI, Checksum: ck}
	req, err := messages.NewAPReq(tkt, key, auth)
	if err != nil {
		return err
	}
	b, err := req.Marshal()
	if err != nil {
		return err
	}
	g.key, g.seq, g.first = key, uint64(auth.SeqNumber), gssWrap(b)
	return nil
}

// krb5OID: 1.2.840.113554.1.2.2 (DER).
var krb5OID = []byte{0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x12, 0x01, 0x02, 0x02}

// gssWrap: GSS-API-Rahmen um das AP-REQ (RFC 2743 3.1, Token-ID 01 00).
func gssWrap(apreq []byte) []byte {
	inner := append(append(append([]byte{}, krb5OID...), 0x01, 0x00), apreq...)
	return append(append([]byte{0x60}, derLen(len(inner))...), inner...)
}

func derLen(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	var b []byte
	for v := n; v > 0; v >>= 8 {
		b = append([]byte{byte(v)}, b...)
	}
	return append([]byte{0x80 | byte(len(b))}, b...)
}

func (g *krbGSS) GetMIC(micField []byte) ([]byte, error) {
	if len(g.key.KeyValue) == 0 {
		return nil, errors.New("kerberos: kein Kontext")
	}
	mt := gssapi.MICToken{Flags: 0, SndSeqNum: g.seq, Payload: micField}
	if err := mt.SetChecksum(g.key, keyusage.GSSAPI_INITIATOR_SIGN); err != nil {
		return nil, err
	}
	g.seq++
	return mt.Marshal()
}

func (g *krbGSS) DeleteSecContext() error {
	g.key = types.EncryptionKey{}
	return nil
}
