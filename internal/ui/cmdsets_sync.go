package ui

// Eigene Befehlssätze im Sync. Hinaus gehen die eigenen Dateien
// (befehlssaetze/*.yaml). Was von anderen Rechnern kommt, landet in
// befehlssaetze/sync und wirkt erst, wenn man es übernimmt (f) – dieselbe
// Regel wie bei Team-Sätzen, denn wer den Sync-Ordner beschreiben kann, soll
// sshTermy nicht unbemerkt neue Befehle oder Erkennungen unterschieben.
// Löschen wird per Grabstein abgeglichen; gelöschte Dateien wandern nach
// befehlssaetze/.geloescht statt zu verschwinden.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/cmdset"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/syncx"
)

const cmdsetRejected = ".abgelehnt" // in befehlssaetze/sync: verworfene Prüfsummen

func cmdsetSum(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// notifySync: Befehlssatz-Dateien geändert; wie eine Store-Änderung behandeln.
func (a *App) notifySync() {
	if a.events == nil {
		return
	}
	select {
	case a.events <- syncChangedMsg{}:
	default:
	}
}

// cmdsetsEdited: eigene Dateien geändert (Formular, Import, Datei): neu laden
// und abgleichen.
func (a *App) cmdsetsEdited() {
	a.loadCmdSets()
	a.notifySync()
}

// cmdsetTombstone merkt das Löschen eines eigenen Satzes für den Sync.
func (a *App) cmdsetTombstone(id string) {
	now := time.Now().UTC()
	_ = a.st.Update(func(d *domain.Data) {
		if d.Deleted == nil {
			d.Deleted = map[string]time.Time{}
		}
		d.Deleted[syncx.CmdSetTombstone(id)] = now
	})
}

// ownCmdSetFiles: eigene Dateien mit Änderungszeit (Kennung → Datei).
func ownCmdSetFiles(dir string) map[string]syncx.CmdSet {
	out := map[string]syncx.CmdSet{}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), cmdset.Ext) {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Size() > cmdset.MaxFileSize {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		s, err := cmdset.ParseFile(raw)
		if err != nil || s.ID == "" || cmdset.IsBuiltin(s.ID) {
			continue
		}
		out[s.ID] = syncx.CmdSet{ID: s.ID, YAML: string(raw), UpdatedAt: info.ModTime().UTC().Truncate(time.Second)}
	}
	return out
}

// syncCmdSets: was dieser Rechner in den Sync gibt.
func (a *App) syncCmdSets() []syncx.CmdSet {
	var out []syncx.CmdSet
	for _, c := range ownCmdSetFiles(a.cmdsetDir()) {
		out = append(out, c)
	}
	return out
}

// applySyncCmdSets legt Sätze anderer Rechner zur Übernahme bereit und
// räumt gelöschte weg.
func (a *App) applySyncCmdSets(p *syncx.Payload) {
	if p == nil {
		return
	}
	dir := a.cmdsetDir()
	sdir := filepath.Join(dir, cmdset.SyncDir)
	own := ownCmdSetFiles(dir)
	rejected := map[string]bool{}
	if b, err := os.ReadFile(filepath.Join(sdir, cmdsetRejected)); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			rejected[strings.TrimSpace(l)] = true
		}
	}
	changed, offered, removed := false, 0, 0
	// Auf einem anderen Rechner gelöscht: beiseitelegen, nicht vernichten.
	for id, c := range own {
		if t, gone := p.Deleted[syncx.CmdSetTombstone(id)]; gone && !t.Before(c.UpdatedAt) {
			src := filepath.Join(dir, id+cmdset.Ext)
			dst := filepath.Join(dir, ".geloescht", id+"-"+time.Now().Format("20060102-150405")+cmdset.Ext)
			if os.MkdirAll(filepath.Dir(dst), 0o700) == nil && os.Rename(src, dst) == nil {
				delete(own, id)
				changed = true
				removed++
			}
		}
	}
	want := map[string]bool{}
	for _, rs := range p.CmdSets {
		if len(rs.YAML) > cmdset.MaxFileSize || cmdset.IsBuiltin(rs.ID) {
			continue
		}
		s, err := cmdset.ParseFile([]byte(rs.YAML))
		if err != nil || s.ID != rs.ID {
			continue
		}
		if o, ok := own[rs.ID]; ok && (o.YAML == rs.YAML || !o.UpdatedAt.Before(rs.UpdatedAt)) {
			continue // gleich oder hier neuer (geht beim nächsten Abgleich hinaus)
		}
		if rejected[cmdsetSum(rs.YAML)] {
			continue
		}
		want[rs.ID] = true
		path := filepath.Join(sdir, rs.ID+cmdset.Ext)
		if old, err := os.ReadFile(path); err == nil && string(old) == rs.YAML {
			continue
		}
		if writeCmdSet(path, []byte(rs.YAML)) == nil {
			_ = os.Chtimes(path, rs.UpdatedAt, rs.UpdatedAt)
			changed = true
			offered++
		}
	}
	// Angebote, die sich erledigt haben (übernommen, gelöscht, hier neuer).
	if ents, err := os.ReadDir(sdir); err == nil {
		for _, e := range ents {
			n := e.Name()
			if e.IsDir() || !strings.HasSuffix(n, cmdset.Ext) || want[strings.TrimSuffix(n, cmdset.Ext)] {
				continue
			}
			if os.Remove(filepath.Join(sdir, n)) == nil {
				changed = true
			}
		}
	}
	if !changed {
		return
	}
	a.loadCmdSets()
	if offered > 0 {
		a.pushToast(toastWarn, fmt.Sprintf(i18n.T("%d Befehlssätze von anderen Rechnern warten auf Übernahme (Einstellungen › Befehlssätze, f)"), offered))
	}
	if removed > 0 {
		a.flash(fmt.Sprintf(i18n.T("Sync: %d eigene Befehlssätze anderswo gelöscht (beiseitegelegt in %s)"), removed, filepath.Join(dir, ".geloescht")))
	}
}

// askAdoptSyncCmdSet: Satz vom Sync ansehen (Diff zum eigenen) und übernehmen.
func (a *App) askAdoptSyncCmdSet(s *cmdset.Set) {
	if err := s.Validate(); err != nil {
		a.cmds.err = err.Error()
		return
	}
	ownPath := filepath.Join(a.cmdsetDir(), s.ID+cmdset.Ext)
	var lines []string
	if old, err := os.ReadFile(ownPath); err == nil {
		lines = append(lines, i18n.T("Änderungen gegenüber dem Satz auf diesem Rechner:"), "")
		lines = append(lines, a.diffLines(old, s.Raw)...)
	} else {
		lines = append(lines, i18n.T("Neuer Befehlssatz von einem anderen Rechner:"), "")
		lines = append(lines, strings.Split(strings.TrimRight(string(s.Raw), "\n"), "\n")...)
	}
	body := strings.Join(lines, "\n") + "\n\n" +
		i18n.T("Übernehmen? Danach ist er hier ein eigener Satz: Erkennung, Befehle, KI-Hinweis und zusätzliche Gefahrenregeln gelten. Lesebefehle prüft sshTermy trotzdem, Syslog-Schritte zeigt es jedes Mal.")
	src, raw := s.File, s.Raw
	a.modal = confirmModal(fmt.Sprintf(i18n.T("Befehlssatz „%s“ übernehmen"), s.Name), body, func() tea.Cmd {
		var mt time.Time
		if info, err := os.Stat(src); err == nil {
			mt = info.ModTime()
		}
		if err := writeCmdSet(ownPath, raw); err != nil {
			a.cmds.err = err.Error()
			return nil
		}
		if !mt.IsZero() {
			_ = os.Chtimes(ownPath, mt, mt) // Stand des anderen Rechners, kein neuer
		}
		_ = os.Remove(src)
		a.loadCmdSets()
		a.selectCmdSet(s.ID)
		a.flashOK(fmt.Sprintf(i18n.T("Befehlssatz „%s“ übernommen"), s.Name))
		return nil
	})
}

// rejectSyncCmdSet: Angebot verwerfen; derselbe Inhalt kommt nicht wieder.
func (a *App) rejectSyncCmdSet(s *cmdset.Set) {
	a.modal = confirmModal(i18n.T("Befehlssatz verwerfen"), fmt.Sprintf(i18n.T("Angebot „%s“ (%s) vom Sync verwerfen? Derselbe Stand wird nicht noch einmal angeboten; der Satz auf den anderen Rechnern bleibt."), s.Name, s.ID), func() tea.Cmd {
		path := filepath.Join(a.cmdsetDir(), cmdset.SyncDir, cmdsetRejected)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = f.WriteString(cmdsetSum(string(s.Raw)) + "\n")
			_ = f.Close()
		}
		_ = os.Remove(s.File)
		a.loadCmdSets()
		a.cmds.sel = max(0, a.cmds.sel-1)
		return nil
	})
}
