package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sessionlog"
	"github.com/db9979/sshtermy/internal/store"
	"github.com/db9979/sshtermy/internal/syncx"
)

// Sync-Archiv (zuschaltbar im Sync-Assistenten): Config-Sicherungen und
// Sitzungsprotokolle laufen nach dem normalen Abgleich im selben
// Hintergrund-Schritt mit (syncx/archive.go).

// archiveRunning: Protokolle laufender Sitzungen noch nicht hochladen.
const archiveRunning = 24 * time.Hour

// archiveDayOptions: Auswahl der Aufbewahrung im Assistenten.
var archiveDayOptions = []int{30, 90, 365}

func archiveDaysLabel(d int) string { return fmt.Sprintf(i18n.T("%d Tage"), d) }

type archiveStats struct {
	backups, logs, uploaded, dropped int
	err                              error
}

// archiveLocal sammelt die Einträge dieses Rechners.
func archiveLocal(st *store.Store, book *sessionlog.Book, now time.Time) []syncx.LocalItem {
	var out []syncx.LocalItem
	for _, conn := range st.BackupConns() {
		for _, b := range st.Backups(conn) {
			id := b.ID
			out = append(out, syncx.LocalItem{Kind: syncx.KindBackup, ID: conn + "/" + id, Taken: b.Taken,
				Load: func() ([]byte, error) {
					cfg, err := st.LoadBackup(conn, id)
					return []byte(cfg), err
				}})
		}
	}
	if book != nil {
		logs, _, _ := book.List()
		for _, l := range logs {
			if l.Ended.IsZero() && now.Sub(l.Started) < archiveRunning {
				continue // Sitzung läuft (oder lief eben noch)
			}
			out = append(out, syncx.LocalItem{Kind: syncx.KindLog, ID: l.ID, Taken: l.Started,
				Load: func() ([]byte, error) { return json.Marshal(l) }})
		}
	}
	return out
}

// runArchive gleicht das Archiv ab und legt Neues lokal ab (läuft im Hintergrund).
func runArchive(ctx context.Context, b syncx.Backend, k *syncx.Key, st *store.Store, book *sessionlog.Book, deleted map[string]time.Time, days int) archiveStats {
	var as archiveStats
	ar, ok := b.(syncx.Archiver)
	if !ok {
		return as
	}
	now := time.Now().UTC()
	res, err := syncx.SyncArchive(ctx, ar.Archive(), k, archiveLocal(st, book, now), deleted, days, deviceName(), now)
	as.uploaded = res.Uploaded
	if err != nil {
		as.err = err
	}
	for _, it := range res.Got {
		switch it.Kind {
		case syncx.KindBackup:
			conn, id, ok := strings.Cut(it.ID, "/")
			if !ok {
				continue
			}
			if added, err := st.ImportBackup(conn, id, string(it.Data)); err == nil && added {
				as.backups++
			}
		case syncx.KindLog:
			if book == nil {
				continue
			}
			var l sessionlog.Log
			if json.Unmarshal(it.Data, &l) != nil || l.ID != it.ID || book.Has(l.ID) {
				continue
			}
			if l.Device == "" {
				l.Device = it.Device
			}
			if book.Save(&l) == nil {
				as.logs++
			}
		}
	}
	for _, it := range res.Drop {
		switch it.Kind {
		case syncx.KindBackup:
			if conn, id, ok := strings.Cut(it.ID, "/"); ok && st.DeleteBackup(conn, id) == nil {
				as.dropped++
			}
		case syncx.KindLog:
			if book != nil && book.Delete(it.ID) == nil {
				as.dropped++
			}
		}
	}
	return as
}

// archiveNote: Meldung nach dem Abgleich (leer = nichts Neues).
func archiveNote(as archiveStats) string {
	var parts []string
	if as.backups > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T("%d Sicherungen"), as.backups))
	}
	if as.logs > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T("%d Protokolle"), as.logs))
	}
	if len(parts) == 0 {
		return ""
	}
	return i18n.T("Sync-Archiv: neu von anderen Rechnern: ") + strings.Join(parts, ", ")
}
