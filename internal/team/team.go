// Package team packt Verbindungen, Tunnel, Runbooks und Schnellbefehle in
// eine verschlüsselte Datei, die Kollegen übernehmen können.
//
// # Was mitgeht und was nicht
//
// Nie im Paket: Passwörter, eingebettete Keys, Passphrasen, Schlüsseldatei-
// Pfade außerhalb von ~/…, abgelehnte KI-Vorschläge, Nutzungszähler und
// alles Lokale (Sync, KI-Einstellungen mit API-Keys). Der Empfänger meldet
// sich mit seinen eigenen Zugangsdaten an; sshTermy fragt beim ersten
// Verbinden danach.
//
// # Verschlüsselung
//
//	Team-Passwort ──Argon2id──▶ Schlüssel ──AES-256-GCM──▶ Inhalt
//
// Der Kopf (Format, Version, KDF-Parameter) ist als AAD mitgeschützt. Im
// Klartext steht nichts über den Inhalt, auch nicht der Absender.
package team

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/vault"
)

const (
	Ext        = ".sshteam"
	formatName = "sshtermy-team"
	formatVer  = 1
	aadPrefix  = "sshtermy/v1/team\x00"
	MinPassLen = 8
)

var (
	ErrNotTeamFile = i18n.NewError("die Datei ist kein sshTermy-Team-Paket")
	ErrWrongPass   = i18n.NewError("Team-Passwort falsch oder Datei verändert")
	ErrTooNew      = i18n.NewError("das Team-Paket stammt von einer neueren sshTermy-Version; bitte zuerst aktualisieren (F12)")
)

// Package ist der entschlüsselte Inhalt.
type Package struct {
	From        string              `json:"from,omitempty"`
	Note        string              `json:"note,omitempty"`
	Created     time.Time           `json:"created"`
	Connections []domain.Connection `json:"connections"`
	Tunnels     []domain.Tunnel     `json:"tunnels,omitempty"`
	Runbooks    []domain.Runbook    `json:"runbooks,omitempty"`
	CmdSets     []CmdSet            `json:"cmdsets,omitempty"` // eigene Befehlssätze der Verbindungen
}

type header struct {
	Format  string          `json:"format"`
	Version int             `json:"version"`
	KDF     vault.KDFParams `json:"kdf"`
}

type file struct {
	header
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

func aad(h header) []byte {
	b, _ := json.Marshal(h)
	return append([]byte(aadPrefix), b...)
}

// Seal verschlüsselt das Paket mit dem Team-Passwort.
func Seal(p *Package, pass []byte) ([]byte, error) {
	if len([]rune(string(pass))) < MinPassLen {
		return nil, fmt.Errorf(i18n.T("das Team-Passwort braucht mindestens %d Zeichen"), MinPassLen)
	}
	plain, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	defer vault.Wipe(plain)
	kdf, err := vault.DefaultKDF()
	if err != nil {
		return nil, err
	}
	key := kdf.Derive(pass)
	defer vault.Wipe(key)
	h := header{Format: formatName, Version: formatVer, KDF: kdf}
	n, ct, err := vault.SealGCM(key, plain, aad(h))
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(file{header: h, Nonce: n, Ciphertext: ct}, "", "  ")
}

// IsTeamFile: sieht der Inhalt wie ein Team-Paket aus (ohne Passwort)?
func IsTeamFile(raw []byte) bool {
	var f file
	return json.Unmarshal(raw, &f) == nil && f.Format == formatName
}

// Open prüft und entschlüsselt ein Team-Paket.
func Open(raw, pass []byte) (*Package, error) {
	var f file
	if err := json.Unmarshal(raw, &f); err != nil || f.Format != formatName {
		return nil, ErrNotTeamFile
	}
	if f.Version > formatVer {
		return nil, ErrTooNew
	}
	if err := f.KDF.Check(); err != nil {
		return nil, err
	}
	key := f.KDF.Derive(pass)
	defer vault.Wipe(key)
	plain, err := vault.OpenGCM(key, f.Nonce, f.Ciphertext, aad(f.header))
	if err != nil {
		return nil, ErrWrongPass
	}
	defer vault.Wipe(plain)
	p := &Package{}
	if err := json.Unmarshal(plain, p); err != nil {
		return nil, ErrNotTeamFile
	}
	for _, c := range p.Connections { // Schutz, falls ein fremdes Programm Geheimnisse hineinschreibt
		if c.Password.IsSet() || c.KeyPEM.IsSet() || c.KeyPassphrase.IsSet() {
			return nil, errors.New(i18n.T("das Team-Paket enthält Passwörter oder Keys; aus Sicherheitsgründen nicht übernommen"))
		}
	}
	return p, nil
}

// Options: was außer den Verbindungen mitgeht.
type Options struct {
	Runbooks  bool // Runbooks dieser Verbindungen und allgemeine
	QuickCmds bool // eigene Schnellbefehle
	Users     bool // Benutzernamen (aus = jeder trägt seinen ein)
	Tunnels   bool
}

// Build stellt das Paket aus den gewählten Verbindungen zusammen. Jump-Hosts
// der gewählten Verbindungen kommen automatisch mit, sonst ginge die
// Verbindung beim Empfänger nicht. jumps meldet, welche dazukamen.
func Build(d *domain.Data, ids []string, from string, o Options, now time.Time) (p *Package, jumps []string) {
	byID := map[string]domain.Connection{}
	for _, c := range d.Connections {
		byID[c.ID] = c
	}
	want := map[string]bool{}
	for _, id := range ids {
		if _, ok := byID[id]; ok {
			want[id] = true
		}
	}
	// Jump-Hosts (auch verkettet) ergänzen
	for changed := true; changed; {
		changed = false
		for id := range want {
			j := byID[id].JumpHostID
			if _, ok := byID[j]; ok && !want[j] {
				want[j] = true
				jumps = append(jumps, j)
				changed = true
			}
		}
	}
	p = &Package{From: strings.TrimSpace(from), Created: now.UTC()}
	for _, c := range d.Connections {
		if !want[c.ID] {
			continue
		}
		p.Connections = append(p.Connections, Strip(c, o))
	}
	if o.Tunnels {
		for _, t := range d.Tunnels {
			if want[t.ConnectionID] {
				p.Tunnels = append(p.Tunnels, t)
			}
		}
	}
	if o.Runbooks {
		for _, r := range d.Runbooks {
			if r.ConnectionID == "" || want[r.ConnectionID] {
				r.Uses, r.LastUsed = 0, time.Time{}
				p.Runbooks = append(p.Runbooks, r)
			}
		}
	}
	p.CmdSets = cmdSetsFor(p.Connections)
	sort.Strings(jumps)
	return p, jumps
}

// Strip entfernt alles Persönliche aus einer Verbindung.
func Strip(c domain.Connection, o Options) domain.Connection {
	c.Password, c.KeyPEM, c.KeyPassphrase = "", "", ""
	if !strings.HasPrefix(c.KeyPath, "~") {
		c.KeyPath = "" // fremder Rechnerpfad sagt dem Kollegen nichts
	}
	if !o.Users {
		c.Username = ""
	}
	if !o.QuickCmds {
		c.QuickCmds = nil
	}
	if !o.Tunnels {
		c.TunnelIDs = nil
	}
	if c.Profile != nil {
		pr := *c.Profile
		pr.Failed = nil // abgelehnte Vorschläge sind persönlich
		c.Profile = &pr
	}
	c.LastUsed = time.Time{}
	c.Note, c.StartCmd = "", "" // eigene Notiz und Startbefehl bleiben hier
	return c
}

// safeIncoming: eine neue Verbindung aus einem fremden Paket darf auf diesem
// Rechner nichts ausführen und nichts weiterreichen, bevor man sie selbst
// eingestellt hat. Sonst könnte jeder mit Schreibrecht auf eine abonnierte
// Team-Datei hier Befehle starten (cmd:-Passwortquelle, lokale Shell) oder
// den eigenen SSH-Agent an einen fremden Server hängen.
func safeIncoming(c domain.Connection) domain.Connection {
	c.ForwardAgent, c.ForwardX11 = false, false
	c.GuardOff = false // Schutz abschalten geht nur hier im Formular
	c.LocalShell, c.LocalDir = "", ""
	c.StartCmd = "" // würde nach dem Anmelden von selbst getippt
	for _, src := range []*string{&c.PassSource, &c.OTPSource} {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(*src)), "cmd:") {
			*src = ""
		}
	}
	if !strings.HasPrefix(c.KeyPath, "~") {
		c.KeyPath = ""
	}
	if !strings.HasPrefix(c.CertPath, "~") {
		c.CertPath = ""
	}
	return c
}

// Stats: was die Übernahme geändert hat.
type Stats struct {
	Added, Updated   int
	Tunnels          int
	Runbooks         int
	QuickCmds        int // Verbindungen mit eigenen Schnellbefehlen
	NeedUser, NeedPw int // ohne Benutzername / mit Passwort-Anmeldung (fragt beim Verbinden)
	Removed          int // Team-Ordner: beim Absender entfernt, hier gelöscht
	// KeptGuard: Verbindungen, deren Schutz (Produktiv, Netzwerkgerät) das
	// Paket wegnehmen wollte; hier bleibt er.
	KeptGuard int
	// Review: Runbooks und Schnellbefehle, die neu oder geändert ankamen und
	// vor dem ersten Lauf angesehen werden wollen.
	Review int
	// AddrChanges: neue Adresse für Verbindungen mit eigenen Zugangsdaten,
	// noch nicht übernommen (erst nach Rückfrage, AcceptAddr).
	AddrChanges []AddrChange
}

// AddrChange: das Paket will Host/Port/Jump-Host einer Verbindung ändern.
type AddrChange struct {
	ID, Alias, Old, New string
	Host                string
	Port                int
	Jump                string
}

func addrOf(host string, port int) string {
	if port == 0 || port == 22 {
		return host
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// holdsSecrets: hat die Verbindung hier etwas, das an eine fremde Adresse
// nicht gehen soll?
func holdsSecrets(c domain.Connection) bool {
	return c.Password.IsSet() || c.KeyPEM.IsSet() || c.KeyPassphrase.IsSet() || c.Keychain || c.ForwardAgent || c.PassSource != ""
}

// AcceptAddr übernimmt die bestätigten Adressänderungen.
func AcceptAddr(d *domain.Data, ch []AddrChange, now time.Time) {
	for _, a := range ch {
		for i := range d.Connections {
			if d.Connections[i].ID == a.ID {
				c := &d.Connections[i]
				c.Host, c.Port, c.JumpHostID, c.UpdatedAt = a.Host, a.Port, a.Jump, now.UTC()
			}
		}
	}
}

// Known: welche Verbindungen des Pakets es hier schon gibt (gleiche ID, also
// schon einmal übernommen).
func Known(d *domain.Data, p *Package) map[string]bool {
	have := map[string]bool{}
	for _, c := range d.Connections {
		have[c.ID] = true
	}
	out := map[string]bool{}
	for _, c := range p.Connections {
		if have[c.ID] {
			out[c.ID] = true
		}
	}
	return out
}

// Apply übernimmt die gewählten Verbindungen (ids; nil = alle) in den
// lokalen Stand. Schon übernommene werden aktualisiert, eigene Zugangsdaten
// (Anmeldung, Passwort, Key, Benutzername) und gelernte Systemprofile bleiben.
// folder (optional) wird jedem Ordner vorangestellt, z. B. „Team“; user
// (optional) gilt für alle Verbindungen ohne Benutzernamen.
func Apply(d *domain.Data, p *Package, ids map[string]bool, folder, user string, now time.Time) Stats {
	var st Stats
	now = now.UTC()
	folder = domain.NormPath(folder)
	idx := map[string]int{}
	for i, c := range d.Connections {
		idx[c.ID] = i
	}
	nextOrder := func(group string) float64 {
		m := 0.0
		for _, c := range d.Connections {
			if c.Group == group && c.Order > m {
				m = c.Order
			}
		}
		return m + 1
	}
	taken := map[string]bool{}
	for _, in := range p.Connections {
		if ids != nil && !ids[in.ID] {
			continue
		}
		if in.ID == "" || in.Password.IsSet() || in.KeyPEM.IsSet() || in.KeyPassphrase.IsSet() {
			continue
		}
		taken[in.ID] = true
		if in.Username == "" {
			in.Username = strings.TrimSpace(user)
		}
		in.Group = domain.NormPath(in.Group)
		if folder != "" {
			in.Group = domain.PathJoin(folder, in.Group)
		}
		delete(d.Deleted, in.ID)
		if in.QuickCmds != nil {
			st.QuickCmds++
		}
		if i, ok := idx[in.ID]; ok {
			cur := d.Connections[i]
			if in.Port == 0 {
				in.Port = cur.Port
			}
			moved := cur.Host != in.Host || cur.Port != in.Port || cur.JumpHostID != in.JumpHostID
			if moved && holdsSecrets(cur) {
				// Mit gespeichertem Passwort, Key oder Agent-Weiterleitung nicht
				// still umbiegen: sonst ginge das Passwort an eine Adresse, die
				// jemand anderes in das Paket geschrieben hat. Die Oberfläche fragt.
				st.AddrChanges = append(st.AddrChanges, AddrChange{ID: cur.ID, Alias: in.Alias,
					Old: addrOf(cur.Host, cur.Port), New: addrOf(in.Host, in.Port), Host: in.Host, Port: in.Port, Jump: in.JumpHostID})
				in.Host, in.Port, in.JumpHostID = cur.Host, cur.Port, cur.JumpHostID
			}
			wasProd, wasNet := domain.LooksProduction(cur), cur.Device == domain.DeviceNetwork
			cur.Alias, cur.Host, cur.Port = in.Alias, in.Host, in.Port
			if cur.Group != in.Group {
				cur.Group, cur.Order = in.Group, nextOrder(in.Group)
			}
			cur.Tags, cur.JumpHostID, cur.Device = in.Tags, in.JumpHostID, in.Device
			cur.TunnelIDs = mergeIDs(cur.TunnelIDs, in.TunnelIDs)
			// Schutz nimmt ein Paket nie weg: sonst schaltete ein geänderter Tag
			// oder Gerätetyp Gefahren-Check und Agent-Bremse still ab.
			if wasNet && cur.Device != domain.DeviceNetwork {
				cur.Device = domain.DeviceNetwork
				st.KeptGuard++
			}
			if wasProd && !domain.LooksProduction(cur) {
				cur.Tags = append(append([]string(nil), cur.Tags...), "prod")
				st.KeptGuard++
			}
			if in.QuickCmds != nil {
				if len(in.QuickCmds) > 0 && !sameStrings(cur.QuickCmds, in.QuickCmds) {
					cur.QuickTeamChanged = true
					st.Review++
				}
				cur.QuickCmds = in.QuickCmds
			}
			if cur.Username == "" {
				cur.Username = in.Username
			}
			if cur.Profile == nil && in.Profile != nil {
				cur.Profile = in.Profile
			} else if cur.Profile != nil && in.Profile != nil && in.Profile.Notes != "" {
				pr := *cur.Profile
				pr.Notes = in.Profile.Notes // Team-Hinweise („nie reload“) gelten für alle
				cur.Profile = &pr
			}
			cur.UpdatedAt = now
			d.Connections[i] = cur
			st.Updated++
			continue
		}
		c := safeIncoming(in)
		c.QuickTeamChanged = len(c.QuickCmds) > 0
		if c.QuickTeamChanged {
			st.Review++
		}
		if c.Port == 0 {
			c.Port = 22
		}
		if c.Auth == domain.AuthKeyFile && c.KeyPath == "" {
			c.Auth = domain.AuthAuto // Agent und ~/.ssh/id_*, sonst Passwort-Abfrage
		}
		if c.Auth == "" {
			c.Auth = domain.AuthAuto
		}
		c.Order = nextOrder(c.Group)
		c.CreatedAt, c.UpdatedAt, c.LastUsed = now, now, time.Time{}
		d.Connections = append(d.Connections, c)
		idx[c.ID] = len(d.Connections) - 1
		st.Added++
	}
	for _, c := range d.Connections {
		if taken[c.ID] {
			if c.Username == "" {
				st.NeedUser++
			}
			if c.Auth == domain.AuthPassword && !c.Password.IsSet() {
				st.NeedPw++
			}
		}
	}
	tidx := map[string]int{}
	for i, t := range d.Tunnels {
		tidx[t.ID] = i
	}
	for _, t := range p.Tunnels {
		if !taken[t.ConnectionID] || t.ID == "" {
			continue
		}
		t.UpdatedAt = now
		delete(d.Deleted, t.ID)
		// Tunnel aus dem Team starten nie von selbst: ein Remote-Tunnel oder
		// eine Bindung an alle Adressen öffnete sonst still einen Weg in dein
		// Netz. Ein Autostart, den du hier gesetzt hast, bleibt, solange das
		// Paket Art, Bindung und Ziel nicht ändert.
		t.AutoStart, t.AutoOpen = false, false
		if i, ok := tidx[t.ID]; ok {
			cur := d.Tunnels[i]
			if cur.Kind == t.Kind && cur.BindHost == t.BindHost && cur.BindPort == t.BindPort &&
				cur.TargetHost == t.TargetHost && cur.TargetPort == t.TargetPort {
				t.AutoStart, t.AutoOpen = cur.AutoStart, cur.AutoOpen
			}
			d.Tunnels[i] = t
		} else {
			d.Tunnels = append(d.Tunnels, t)
		}
		st.Tunnels++
	}
	ridx := map[string]int{}
	for i, r := range d.Runbooks {
		ridx[r.ID] = i
	}
	for _, r := range p.Runbooks {
		if r.ID == "" || (r.ConnectionID != "" && !taken[r.ConnectionID]) {
			continue
		}
		r.UpdatedAt = now
		delete(d.Deleted, r.ID)
		// Neu oder geändert: erst ansehen, bevor ein Zeitplan es ausführt.
		r.TeamChanged = true
		if i, ok := ridx[r.ID]; ok {
			cur := d.Runbooks[i]
			r.Uses, r.LastUsed, r.CreatedAt = cur.Uses, cur.LastUsed, cur.CreatedAt
			if sameRunbook(cur, r) {
				r.TeamChanged = cur.TeamChanged
			}
			if r.TeamChanged && !cur.TeamChanged {
				st.Review++
			}
			d.Runbooks[i] = r
		} else {
			st.Review++
			if r.CreatedAt.IsZero() {
				r.CreatedAt = now
			}
			d.Runbooks = append(d.Runbooks, r)
		}
		st.Runbooks++
	}
	return st
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sameRunbook: gleiche Befehle und Skripte (Name und Ziel zählen nicht).
func sameRunbook(a, b domain.Runbook) bool {
	if !sameStrings(a.Steps, b.Steps) || len(a.Files) != len(b.Files) || a.Goal != b.Goal {
		return false
	}
	for i := range a.Files {
		if a.Files[i] != b.Files[i] {
			return false
		}
	}
	return true
}

func mergeIDs(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range append(append([]string(nil), a...), b...) {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// Passphrase erzeugt ein gut diktierbares Team-Passwort, z. B.
// „k7mq-x2pf-9tzc-hw4r“ (ohne leicht verwechselbare Zeichen, ~80 Bit).
func Passphrase() string {
	const abc = "abcdefghjkmnpqrstuvwxyz23456789"
	var b strings.Builder
	for i := 0; i < 16; i++ {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(abc))))
		b.WriteByte(abc[n.Int64()])
	}
	return b.String()
}
