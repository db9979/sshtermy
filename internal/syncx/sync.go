package syncx

import (
	"context"
	"errors"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Run gleicht einmal ab:
//
//  1. Sync-Datei lesen (fehlt sie, wird sie angelegt)
//  2. Konfliktkopien des Cloud-Dienstes mit einarbeiten
//  3. lokal + entfernt pro Eintrag zusammenführen
//  4. nur wenn sich am Inhalt etwas ändert: mit Versionsprüfung zurückschreiben;
//     hat ein anderer Rechner gleichzeitig geschrieben, von vorn (max. 4 Mal)
//
// Das Ergebnis ist der neue Stand für den lokalen Store.
func Run(ctx context.Context, b Backend, k *Key, local *Payload, device string) (*Payload, Stats, error) {
	var st Stats
	for attempt := 0; attempt < 4; attempt++ {
		raw, ver, err := b.Read(ctx)
		var remote *Payload
		switch {
		case errors.Is(err, ErrNotFound):
			remote, ver = &Payload{}, ""
		case err != nil:
			return nil, st, err
		default:
			if remote, err = Decode(k, raw); err != nil {
				return nil, st, err
			}
		}

		now := time.Now().UTC()
		merged := Merge(local, remote, now)

		var retire []string
		if cc, ok := b.(ConflictCopies); ok {
			copies, _ := cc.Copies(ctx)
			for name, data := range copies {
				p, err := Decode(k, data)
				if err != nil {
					continue // fremde oder alte Datei: liegen lassen
				}
				merged = Merge(merged, p, now)
				retire = append(retire, name)
			}
		}

		st.Added, st.Changed, st.Gone = diff(local, merged)
		st.Connections, st.Tunnels, st.Merged = len(merged.Connections), len(merged.Tunnels), len(retire)

		if ver != "" && equalContent(merged, remote) && len(retire) == 0 {
			return merged, st, nil // nichts Neues für die anderen
		}
		if device != "" {
			if merged.Devices == nil {
				merged.Devices = map[string]time.Time{}
			}
			merged.Devices[device] = now
		}
		data, err := Encode(k, merged)
		if err != nil {
			return nil, st, err
		}
		err = b.Write(ctx, data, ver)
		if errors.Is(err, ErrConflict) {
			continue
		}
		if err != nil {
			return nil, st, err
		}
		st.Uploaded = true
		if cc, ok := b.(ConflictCopies); ok {
			for _, n := range retire {
				_ = cc.Retire(n)
			}
		}
		return merged, st, nil
	}
	return nil, st, errors.New(i18n.T("Sync-Datei wird gerade ständig von einem anderen Rechner geändert; später erneut versuchen"))
}
