//go:build windows

package sshx

import (
	"errors"

	"github.com/alexbrainman/sspi"
	"github.com/alexbrainman/sspi/kerberos"
	"golang.org/x/crypto/ssh"
)

type sspiGSS struct {
	cred  *sspi.Credentials
	ctx   *kerberos.ClientContext
	first []byte
	more  bool
}

// newGSSClient baut mit dem Ticket der Windows-Anmeldung das erste Token
// für spn; ohne Domänen-Konto/Ticket kommt hier schon der Fehler.
func newGSSClient(spn string) (ssh.GSSAPIClient, error) {
	cred, err := kerberos.AcquireCurrentUserCredentials()
	if err != nil {
		return nil, err
	}
	flags := uint32(sspi.ISC_REQ_MUTUAL_AUTH | sspi.ISC_REQ_INTEGRITY)
	ctx, done, out, err := kerberos.NewClientContextWithFlags(cred, spn, flags)
	if err != nil {
		_ = cred.Release()
		return nil, err
	}
	return &sspiGSS{cred: cred, ctx: ctx, first: out, more: !done}, nil
}

func (g *sspiGSS) InitSecContext(target string, token []byte, isGSSDelegCreds bool) ([]byte, bool, error) {
	if g.first != nil {
		out := g.first
		g.first = nil
		return out, g.more, nil
	}
	if g.ctx == nil {
		return nil, false, errors.New("kerberos: kein Kontext")
	}
	done, out, err := g.ctx.Update(token)
	if err != nil {
		return nil, false, err
	}
	return out, !done, nil
}

func (g *sspiGSS) GetMIC(micField []byte) ([]byte, error) {
	if g.ctx == nil {
		return nil, errors.New("kerberos: kein Kontext")
	}
	return g.ctx.MakeSignature(micField, 0, 0)
}

func (g *sspiGSS) DeleteSecContext() error {
	if g.ctx != nil {
		_ = g.ctx.Release()
		g.ctx = nil
	}
	if g.cred != nil {
		_ = g.cred.Release()
		g.cred = nil
	}
	return nil
}
