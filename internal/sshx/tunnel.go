package sshx

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

// ActiveTunnel ist ein laufendes Forwarding mit Zählern für die Oberfläche.
type ActiveTunnel struct {
	Spec      domain.Tunnel
	Addr      string // tatsächlich gebundene Adresse (bei Port 0 der gewählte)
	Started   time.Time
	Conns     atomic.Int64 // aktuell offene Verbindungen
	Total     atomic.Int64 // insgesamt
	BytesIn   atomic.Int64
	BytesOut  atomic.Int64
	LastError atomic.Value // string

	ln   net.Listener
	once sync.Once
}

// URL des One-Click-Web-Service (leer, wenn keiner).
func (a *ActiveTunnel) URL() string {
	if !a.Spec.IsWebService() {
		return ""
	}
	return fmt.Sprintf("%s://%s%s", a.Spec.WebScheme, a.Addr, a.Spec.WebPath)
}

func (a *ActiveTunnel) Stop() { a.once.Do(func() { a.ln.Close() }) }

// TunnelManager hält alle Tunnel einer SSH-Verbindung.
type TunnelManager struct {
	c      *Client
	mu     sync.Mutex
	active map[string]*ActiveTunnel
}

func NewTunnelManager(c *Client) *TunnelManager {
	return &TunnelManager{c: c, active: map[string]*ActiveTunnel{}}
}

func (m *TunnelManager) List() []*ActiveTunnel {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*ActiveTunnel, 0, len(m.active))
	for _, a := range m.active {
		out = append(out, a)
	}
	return out
}

func (m *TunnelManager) Get(id string) *ActiveTunnel {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active[id]
}

func (m *TunnelManager) Stop(id string) {
	m.mu.Lock()
	a := m.active[id]
	delete(m.active, id)
	m.mu.Unlock()
	if a != nil {
		a.Stop()
	}
}

func (m *TunnelManager) StopAll() {
	for _, a := range m.List() {
		m.Stop(a.Spec.ID)
	}
}

// Start öffnet den Listener und beginnt mit dem Weiterleiten.
//
// Sicherheit: BindHost ist standardmäßig 127.0.0.1. Ein Tunnel auf 0.0.0.0
// würde den entfernten Dienst für das ganze lokale Netz öffnen; das geht nur,
// wenn es im Profil ausdrücklich so eingetragen ist.
func (m *TunnelManager) Start(t domain.Tunnel) (*ActiveTunnel, error) {
	if old := m.Get(t.ID); old != nil {
		return old, nil
	}
	bind := t.BindHost
	if bind == "" {
		bind = "127.0.0.1"
	}
	laddr := net.JoinHostPort(bind, strconv.Itoa(t.BindPort))

	var (
		ln  net.Listener
		err error
	)
	switch t.Kind {
	case domain.TunnelLocal, domain.TunnelDynamic:
		ln, err = net.Listen("tcp", laddr) // lokal lauschen
	case domain.TunnelRemote:
		// Server lauscht (tcpip-forward). Ob andere als localhost dort binden
		// dürfen, entscheidet GatewayPorts in der sshd_config des Servers.
		ln, err = m.c.Listen("tcp", laddr)
	default:
		return nil, errors.New("unbekannter Tunnel-Typ")
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", t.Spec(), err)
	}
	a := &ActiveTunnel{Spec: t, Addr: ln.Addr().String(), Started: time.Now(), ln: ln}
	a.LastError.Store("")
	m.mu.Lock()
	m.active[t.ID] = a
	m.mu.Unlock()

	go m.serve(a)
	if t.AutoOpen && a.URL() != "" {
		_ = OpenBrowser(a.URL())
	}
	return a, nil
}

func (m *TunnelManager) serve(a *ActiveTunnel) {
	defer m.Stop(a.Spec.ID)
	target := net.JoinHostPort(a.Spec.TargetHost, strconv.Itoa(a.Spec.TargetPort))
	for {
		in, err := a.ln.Accept()
		if err != nil {
			return // Listener geschlossen
		}
		a.Conns.Add(1)
		a.Total.Add(1)
		go func() {
			defer a.Conns.Add(-1)
			defer in.Close()
			switch a.Spec.Kind {
			case domain.TunnelLocal:
				out, err := m.c.Dial("tcp", target) // direct-tcpip über SSH
				if err != nil {
					a.LastError.Store(err.Error())
					return
				}
				pipe(a, in, out)
			case domain.TunnelRemote:
				out, err := net.DialTimeout("tcp", target, 10*time.Second) // lokal verbinden
				if err != nil {
					a.LastError.Store(err.Error())
					return
				}
				pipe(a, in, out)
			case domain.TunnelDynamic:
				if err := serveSOCKS5(in, func(addr string) (net.Conn, error) { return m.c.Dial("tcp", addr) },
					func(c net.Conn) { pipe(a, in, c) }); err != nil {
					a.LastError.Store(err.Error())
				}
			}
		}()
	}
}

// pipe kopiert in beide Richtungen, bis eine Seite schließt.
func pipe(a *ActiveTunnel, x, y net.Conn) {
	defer y.Close()
	done := make(chan struct{}, 2)
	go func() { n, _ := io.Copy(y, x); a.BytesOut.Add(n); closeWrite(y); done <- struct{}{} }()
	go func() { n, _ := io.Copy(x, y); a.BytesIn.Add(n); closeWrite(x); done <- struct{}{} }()
	<-done
	<-done
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
}

// OpenBrowser öffnet eine URL im Standardbrowser des Systems.
func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
