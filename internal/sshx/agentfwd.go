package sshx

import (
	"errors"
	"net"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Agent-Weiterleitung (wie ssh -A): Der Server darf den lokalen SSH-Agent
// über die bestehende Verbindung um Signaturen bitten. Die Keys selbst
// verlassen den Rechner nie. Jede Anfrage öffnet eine eigene Verbindung zum
// Agent, damit ein Neustart des Agents (oder von Pageant) die Weiterleitung
// nicht dauerhaft kaputt macht.

// ErrNoAgent: Weiterleitung gewünscht, aber kein Agent erreichbar.
var ErrNoAgent = errors.New("no ssh agent")

// forwardAgent richtet die Weiterleitung für sess ein (einmal je Verbindung
// den Kanal-Handler, dann je Sitzung die Anfrage an den Server).
func (c *Client) forwardAgent(sess *ssh.Session) error {
	c.fwdOnce.Do(func() {
		conn, err := dialAgent()
		if err != nil {
			c.fwdErr = ErrNoAgent
			return
		}
		_ = conn.Close()
		c.fwdErr = agent.ForwardToAgent(c.Client, dialingAgent{dial: dialAgent})
	})
	if c.fwdErr != nil {
		return c.fwdErr
	}
	return agent.RequestAgentForwarding(sess)
}

// AgentNote: Hinweis für die Statuszeile, wenn die Weiterleitung nicht
// klappte ("" = alles gut oder nicht gewünscht).
func (c *Client) AgentNote() string {
	switch {
	case c.fwdErr == nil && c.fwdAsked == nil:
		return ""
	case errors.Is(c.fwdErr, ErrNoAgent):
		return i18n.T("Agent-Weiterleitung: kein SSH-Agent erreichbar (ssh-agent, Pageant oder Windows-OpenSSH-Agent starten)")
	case c.fwdErr != nil:
		return i18n.T("Agent-Weiterleitung: ") + c.fwdErr.Error()
	default:
		return i18n.T("Agent-Weiterleitung vom Server abgelehnt (AllowAgentForwarding no?)")
	}
}

// dialingAgent: agent.ExtendedAgent, der für jeden Aufruf neu verbindet.
type dialingAgent struct{ dial func() (net.Conn, error) }

func (d dialingAgent) with(f func(agent.ExtendedAgent) error) error {
	conn, err := d.dial()
	if err != nil {
		return err
	}
	defer conn.Close()
	return f(agent.NewClient(conn))
}

func (d dialingAgent) List() (keys []*agent.Key, err error) {
	err = d.with(func(a agent.ExtendedAgent) error { keys, err = a.List(); return err })
	return
}

func (d dialingAgent) Sign(key ssh.PublicKey, data []byte) (sig *ssh.Signature, err error) {
	err = d.with(func(a agent.ExtendedAgent) error { sig, err = a.Sign(key, data); return err })
	return
}

func (d dialingAgent) SignWithFlags(key ssh.PublicKey, data []byte, flags agent.SignatureFlags) (sig *ssh.Signature, err error) {
	err = d.with(func(a agent.ExtendedAgent) error { sig, err = a.SignWithFlags(key, data, flags); return err })
	return
}

// Schlüssel hinzufügen, entfernen, sperren: vom Server aus nicht erlaubt.
var errReadOnly = errors.New("agent: operation not allowed via forwarding")

func (dialingAgent) Add(agent.AddedKey) error       { return errReadOnly }
func (dialingAgent) Remove(ssh.PublicKey) error     { return errReadOnly }
func (dialingAgent) RemoveAll() error               { return errReadOnly }
func (dialingAgent) Lock([]byte) error              { return errReadOnly }
func (dialingAgent) Unlock([]byte) error            { return errReadOnly }
func (dialingAgent) Signers() ([]ssh.Signer, error) { return nil, errReadOnly }
func (dialingAgent) Extension(string, []byte) ([]byte, error) {
	return nil, agent.ErrExtensionUnsupported
}

var _ agent.ExtendedAgent = dialingAgent{}
