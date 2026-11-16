package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/audit"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/keyinv"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Schlüssel-Inventar (F6 a, Palette „Schlüssel-Inventar“): welche
// öffentlichen Schlüssel stehen in welchen authorized_keys? Lesen ist nur
// lesend. Entfernen fragt je Host, legt eine Sicherungskopie an und
// verweigert den Schlüssel, mit dem sshTermy sich selbst dort anmeldet.

type keyinvHost struct {
	conn domain.Connection
	host *keyinv.Host
	err  string
	done bool
}

type keyinvState struct {
	seq    int
	busy   bool
	title  string
	hosts  []keyinvHost
	keys   []keyinv.Key
	sel    string // gewählter Fingerabdruck
	cancel context.CancelFunc
	// Entfernen: Warteschlange je Host
	rmKey   keyinv.Key
	rmQueue []string // Verbindungs-IDs
	rmDone  []string
}

type keyinvMsg struct {
	seq  int
	i    int
	host *keyinv.Host
	err  error
}

type keyinvRmMsg struct {
	id      string
	removed int
	err     error
}

// keyinvRun: Skript auf einem Host (in Tests austauschbar).
var keyinvRun = func(a *App) func(ctx context.Context, c domain.Connection, script string) (string, error) {
	return a.quietScript
}

// keyinvRemove: Schlüssel auf einem Host entfernen (in Tests austauschbar).
var keyinvRemove = func(a *App) func(ctx context.Context, c domain.Connection, places []keyinv.Place, blob string) (int, error) {
	dial := patchDial(a)
	return func(ctx context.Context, c domain.Connection, places []keyinv.Place, blob string) (int, error) {
		cl, err := dial(ctx, c)
		if err != nil {
			return 0, err
		}
		defer cl.Close()
		stamp := time.Now().Format("20060102-150405")
		total := 0
		for _, p := range places {
			script := keyinv.RemoveScript(p.File, blob, stamp)
			cmd := "sh -c " + sshx.Quote(script)
			var in []byte
			if p.User != c.Username && c.Username != "root" {
				cmd = "LC_ALL=C sudo -n sh -c " + sshx.Quote(script)
				if res, err := cl.ExecInput(ctx, "LC_ALL=C sudo -n true", nil); err != nil || res.Exit != 0 {
					pw := ""
					for _, cand := range []string{c.Password.Reveal(), keychainIf(c, sshx.SecretSudo), keychainIf(c, sshx.SecretPassword)} {
						if cand == "" {
							continue
						}
						if r, err := cl.ExecInput(ctx, "LC_ALL=C sudo -S -p '' true", []byte(cand+"\n")); err == nil && r.Exit == 0 {
							pw = cand
							break
						}
					}
					if pw == "" {
						return total, errors.New(i18n.T("fremde authorized_keys brauchen sudo; kein passendes Passwort gespeichert"))
					}
					cmd, in = "LC_ALL=C sudo -S -p '' sh -c "+sshx.Quote(script), []byte(pw+"\n")
				}
			}
			res, err := cl.ExecInput(ctx, cmd, in)
			if err != nil {
				return total, err
			}
			out := string(res.Output)
			if i := strings.Index(out, "removed="); i >= 0 {
				var n int
				fmt.Sscanf(out[i+8:], "%d", &n)
				total += n
			} else if res.Exit != 0 {
				return total, fmt.Errorf("%s: %s", p.File, strings.TrimSpace(lastLine(out)))
			}
		}
		return total, nil
	}
}

func keychainIf(c domain.Connection, kind string) string {
	if !c.Keychain {
		return ""
	}
	return sshx.KeychainSecret(c.ID, kind)
}

// ownKeys: Fingerabdrücke der Schlüssel dieses Rechners (~/.ssh/*.pub und
// die Schlüssel der Verbindungen) → Name.
func (a *App) ownKeys() (names map[string]string, byConn map[string]string) {
	names, byConn = map[string]string{}, map[string]string{}
	home, _ := os.UserHomeDir()
	if home != "" {
		pubs, _ := filepath.Glob(filepath.Join(home, ".ssh", "*.pub"))
		for _, p := range pubs {
			if b, err := os.ReadFile(p); err == nil {
				if pk, _, _, _, err := ssh.ParseAuthorizedKey(b); err == nil {
					names[ssh.FingerprintSHA256(pk)] = i18n.T("dieser Rechner: ") + filepath.Base(p)
				}
			}
		}
	}
	for _, c := range a.st.Connections() {
		if fp := connKeyFP(c, home); fp != "" {
			byConn[c.ID] = fp
			if _, ok := names[fp]; !ok {
				names[fp] = i18n.T("Schlüssel der Verbindung ") + c.Alias
			}
		}
	}
	return names, byConn
}

// connKeyFP: Fingerabdruck des Schlüssels, mit dem sich die Verbindung
// anmeldet ("" = keiner/unbekannt).
func connKeyFP(c domain.Connection, home string) string {
	if c.Auth != domain.AuthKeyFile {
		return ""
	}
	if c.KeyPEM.IsSet() {
		if s, err := ssh.ParsePrivateKey([]byte(c.KeyPEM.Reveal())); err == nil {
			return ssh.FingerprintSHA256(s.PublicKey())
		}
		return ""
	}
	for _, p := range domain.KeyPathCandidates(c.KeyPath, home) {
		if b, err := os.ReadFile(p + ".pub"); err == nil {
			if pk, _, _, _, err := ssh.ParseAuthorizedKey(b); err == nil {
				return ssh.FingerprintSHA256(pk)
			}
		}
		if b, err := os.ReadFile(p); err == nil {
			if s, err := ssh.ParsePrivateKey(b); err == nil {
				return ssh.FingerprintSHA256(s.PublicKey())
			}
		}
	}
	return ""
}

// openKeyinvHost: F6 a, nur dieser Host.
func (a *App) openKeyinvHost(s *session) tea.Cmd {
	if s == nil || s.netDevice() || s.isWinRM() || s.prof.Family == domain.FamilyWindows || s.conn.Protocol != domain.ProtoSSH && s.conn.Protocol != "" {
		if s != nil {
			s.host.err = i18n.T("Schlüssel-Inventar geht nur bei Servern über SSH")
		}
		return nil
	}
	return a.runKeyinv(s.conn.Alias, []domain.Connection{s.conn})
}

func (a *App) openKeyinvFolder() tea.Cmd {
	return a.folderPicker(i18n.T("Schlüssel-Inventar: Ordner wählen"), vulnScanable,
		i18n.T("Liest die authorized_keys aller Benutzer (mit sudo ohne Passwort, sonst nur die eigenen), nur lesend, ohne Rückfragen."),
		func(folder string) tea.Cmd {
			where := folder
			if where == "" {
				where = i18n.T("alle Verbindungen")
			}
			return a.runKeyinv(where, a.vulnFolderConns(folder))
		})
}

func (a *App) runKeyinv(title string, conns []domain.Connection) tea.Cmd {
	if len(conns) == 0 {
		a.flash(i18n.T("Keine Linux-Server (SSH) im Ordner"))
		return nil
	}
	k := &a.kinv
	if k.cancel != nil {
		k.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	*k = keyinvState{seq: k.seq + 1, busy: true, title: title, cancel: cancel}
	for _, c := range conns {
		k.hosts = append(k.hosts, keyinvHost{conn: c})
	}
	seq, ev, run := k.seq, a.events, keyinvRun(a)
	go func() {
		defer cancel()
		forHosts(ctx, conns, 4, func(ctx context.Context, i int, c domain.Connection) {
			if ctx.Err() != nil {
				ev <- keyinvMsg{seq: seq, i: i, err: ctx.Err()}
				return
			}
			out, err := run(ctx, c, keyinv.Script)
			if err != nil {
				ev <- keyinvMsg{seq: seq, i: i, err: err}
				return
			}
			h := keyinv.Parse(out)
			ev <- keyinvMsg{seq: seq, i: i, host: &h}
		})
	}()
	a.flash(fmt.Sprintf(i18n.T("Schlüssel-Inventar: lese %d Hosts …"), len(conns)))
	return nil
}

func (a *App) keyinvMsgs(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case keyinvMsg:
		k := &a.kinv
		if m.seq != k.seq || m.i >= len(k.hosts) {
			return a.listen(), true
		}
		x := &k.hosts[m.i]
		x.done, x.host = true, m.host
		if m.err != nil {
			x.err = m.err.Error()
		}
		k.busy = false
		for _, y := range k.hosts {
			if !y.done {
				k.busy = true
			}
		}
		if !k.busy {
			a.keyinvMerge()
			a.showKeyinvList()
		}
		return a.listen(), true
	case keyinvRmMsg:
		a.keyinvRmDone(m)
		return nil, true
	}
	return nil, false
}

func (a *App) keyinvMerge() {
	k := &a.kinv
	hosts, names := map[string]keyinv.Host{}, map[string]string{}
	for _, x := range k.hosts {
		if x.host != nil {
			hosts[x.conn.ID] = *x.host
			names[x.conn.ID] = x.conn.Alias
		}
	}
	own, _ := a.ownKeys()
	k.keys = keyinv.Merge(hosts, names, own)
}

// showKeyinvList: Schlüssel als Auswahlliste (Suche nach Name, Host, Typ).
func (a *App) showKeyinvList() {
	k := &a.kinv
	if len(k.hosts) == 0 {
		a.flash(i18n.T("Noch kein Schlüssel-Inventar gelesen"))
		return
	}
	var items []pickItem
	for _, key := range k.keys {
		label := key.Label()
		if label == "" {
			label = i18n.T("(ohne Namen)")
		}
		hint := fmt.Sprintf(i18n.T("%d Hosts"), key.Hosts())
		if w := key.Weak(); w != "" {
			hint = "▲ " + w + " · " + hint
		}
		var hs []string
		for _, p := range key.Places {
			hs = append(hs, p.Host+" "+p.User)
		}
		items = append(items, pickItem{value: key.FP, label: trunc(label, 40) + "  " + shortType(key.Type), hint: hint, search: strings.Join(hs, " ") + " " + key.FP})
	}
	okN, errN, notRoot := 0, 0, 0
	for _, x := range k.hosts {
		switch {
		case x.host != nil:
			okN++
			if !x.host.Root {
				notRoot++
			}
		case x.err != "":
			errN++
		}
	}
	a.pick = newPicker(i18n.T("Schlüssel-Inventar: ")+k.title, i18n.T("Name, Host oder Benutzer suchen …"), items, k.sel, func(it pickItem) tea.Cmd {
		k.sel = it.value
		a.showKeyinvKey(it.value)
		return nil
	})
	help := fmt.Sprintf(i18n.T("%d Schlüssel auf %d Hosts gelesen"), len(k.keys), okN)
	if errN > 0 {
		help += fmt.Sprintf(i18n.T(", %d nicht erreichbar"), errN)
	}
	if notRoot > 0 {
		help += fmt.Sprintf(i18n.T(", %d nur eigener Benutzer (ohne sudo)"), notRoot)
	}
	a.pick.help = help + ". " + i18n.T("⏎ zeigt, wo ein Schlüssel steht; dort entfernt d ihn.")
}

func shortType(t string) string {
	t = strings.TrimPrefix(t, "ssh-")
	t = strings.TrimPrefix(t, "sk-")
	return strings.Split(t, "@")[0]
}

func (a *App) keyinvFind(fp string) (keyinv.Key, bool) {
	for _, k := range a.kinv.keys {
		if k.FP == fp {
			return k, true
		}
	}
	return keyinv.Key{}, false
}

// showKeyinvKey: wo steht der Schlüssel? d entfernt.
func (a *App) showKeyinvKey(fp string) {
	key, ok := a.keyinvFind(fp)
	if !ok {
		return
	}
	var lines []string
	var md strings.Builder
	label := orDefault(key.Label(), i18n.T("(ohne Namen)"))
	lines = append(lines, fmt.Sprintf("%s · %s %d Bit", label, key.Type, key.Bits), key.FP)
	fmt.Fprintf(&md, "# %s\n\n%s %d Bit · %s\n\n", label, key.Type, key.Bits, key.FP)
	if w := key.Weak(); w != "" {
		lines = append(lines, "▲ "+fmt.Sprintf(i18n.T("Veraltet (%s): durch einen ed25519-Schlüssel ersetzen."), w))
	}
	if key.Own != "" {
		lines = append(lines, "i "+i18n.T("Eigener Schlüssel dieses Rechners."))
	}
	lines = append(lines, "", fmt.Sprintf(i18n.T("Steht auf %d Hosts:"), key.Hosts()))
	md.WriteString(i18n.T("| Host | Benutzer | Datei |\n|---|---|---|\n"))
	for _, p := range key.Places {
		lines = append(lines, fmt.Sprintf("  %-22s %-12s %s", trunc(p.Host, 22), trunc(p.User, 12), p.File))
		fmt.Fprintf(&md, "| %s | %s | %s |\n", p.Host, p.User, p.File)
	}
	lines = append(lines, "", i18n.T("d = von allen diesen Hosts entfernen (fragt je Host, mit Sicherungskopie) · ← zurück zur Liste · c kopieren"))
	a.modal = textModal(i18n.T("Schlüssel"), lines)
	a.modal.copy, a.modal.keyinv = md.String(), true
}

func (a *App) keyinvKey(key string) (tea.Cmd, bool) {
	k := &a.kinv
	switch key {
	case "left", "backspace":
		if len(k.rmQueue) == 0 {
			a.modal = nil
			a.showKeyinvList()
			return nil, true
		}
	case "d":
		kk, ok := a.keyinvFind(k.sel)
		if !ok {
			return nil, true
		}
		if !keyinv.ValidBlob(kk.Blob) {
			a.flashErr(i18n.T("Schlüssel lässt sich nicht sicher entfernen"))
			return nil, true
		}
		_, byConn := a.ownKeys()
		seen := map[string]bool{}
		var ids []string
		blocked := 0
		for _, p := range kk.Places {
			if seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			if byConn[p.ID] == kk.FP {
				blocked++
				continue
			}
			ids = append(ids, p.ID)
		}
		if len(ids) == 0 {
			a.flashErr(i18n.T("Darüber meldet sich sshTermy selbst an, nicht entfernt"))
			return nil, true
		}
		body := fmt.Sprintf(i18n.T("„%s“ aus den authorized_keys von %d Hosts entfernen? Du bestätigst danach jeden Host einzeln. Vorher wird je Datei eine Kopie <datei>.sshtermy-<Zeit> angelegt."), orDefault(kk.Label(), kk.FP), len(ids))
		if blocked > 0 {
			body += "\n\n" + fmt.Sprintf(i18n.T("%d Hosts bleiben unverändert: sshTermy meldet sich dort mit genau diesem Schlüssel an."), blocked)
		}
		body += "\n\n" + i18n.T("j = weiter · n = abbrechen")
		m := confirmModal(i18n.T("Schlüssel entfernen?"), body, func() tea.Cmd {
			k.rmKey, k.rmQueue, k.rmDone = kk, ids, nil
			return a.keyinvAskNext()
		})
		m.twice = true
		a.modal = m
		return nil, true
	}
	return nil, false
}

// keyinvAskNext: nächsten Host bestätigen lassen.
func (a *App) keyinvAskNext() tea.Cmd {
	k := &a.kinv
	if len(k.rmQueue) == 0 {
		if len(k.rmDone) > 0 {
			a.flashOK(fmt.Sprintf(i18n.T("Schlüssel entfernt auf: %s"), strings.Join(k.rmDone, ", ")))
		}
		a.keyinvMerge()
		a.showKeyinvKey(k.rmKey.FP)
		if _, ok := a.keyinvFind(k.rmKey.FP); !ok {
			a.modal = nil
			a.showKeyinvList()
		}
		return nil
	}
	id := k.rmQueue[0]
	c, ok := a.st.Connection(id)
	if !ok {
		k.rmQueue = k.rmQueue[1:]
		return a.keyinvAskNext()
	}
	var files []string
	for _, p := range k.rmKey.Places {
		if p.ID == id {
			files = append(files, p.User+": "+p.File)
		}
	}
	body := fmt.Sprintf(i18n.T("%s (%s)\n%s\n\nj = entfernen · n = diesen Host auslassen"), c.Alias, c.Host, strings.Join(files, "\n"))
	m := confirmModal(fmt.Sprintf(i18n.T("Schlüssel auf %s entfernen?"), c.Alias), body, func() tea.Cmd {
		var places []keyinv.Place
		for _, p := range k.rmKey.Places {
			if p.ID == id {
				places = append(places, p)
			}
		}
		rm := keyinvRemove(a)
		blob := k.rmKey.Blob
		a.audit.Add(audit.EvConfig, c.Alias, "authorized_keys: "+k.rmKey.FP+" entfernt")
		a.flash(fmt.Sprintf(i18n.T("entferne auf %s …"), c.Alias))
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			n, err := rm(ctx, c, places, blob)
			return keyinvRmMsg{id: id, removed: n, err: err}
		}
	})
	m.onCancel = func() tea.Cmd {
		k.rmQueue = k.rmQueue[1:]
		return a.keyinvAskNext()
	}
	a.modal = m
	return nil
}

func (a *App) keyinvRmDone(m keyinvRmMsg) {
	k := &a.kinv
	if len(k.rmQueue) > 0 && k.rmQueue[0] == m.id {
		k.rmQueue = k.rmQueue[1:]
	}
	c, _ := a.st.Connection(m.id)
	if m.err != nil {
		a.flashErr(c.Alias + ": " + m.err.Error())
	} else if m.removed > 0 {
		k.rmDone = append(k.rmDone, c.Alias)
		// Inventar nachführen
		for i := range k.hosts {
			if k.hosts[i].conn.ID != m.id || k.hosts[i].host == nil {
				continue
			}
			var keep []keyinv.Entry
			for _, e := range k.hosts[i].host.Entries {
				if e.FP != k.rmKey.FP {
					keep = append(keep, e)
				}
			}
			k.hosts[i].host.Entries = keep
		}
	}
	a.keyinvAskNext()
}
