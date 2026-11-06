package ui

// KI-Lernen pro Gerät: Korrigiert der Admin einen Befehl, den die KI
// vorgeschlagen hat, merkt sich sshTermy das im Systemprofil der Verbindung
// (domain.SystemProfile.Learned). Die KI bekommt die Liste bei jeder Anfrage
// mit („so heißt das hier“), und der Ziel-Modus setzt eine bekannte Korrektur
// direkt ein.
//
// Als Korrektur zählt:
//   - ein eingefügter KI-Befehl (Karte, Alt+1…4, Ziel-Modus „i“), den der
//     Admin vor dem Enter verändert hat (nicht bloß ergänzt),
//   - ein KI-Befehl, der fehlgeschlagen ist, wenn der Admin direkt danach
//     selbst einen ähnlichen Befehl tippt,
//   - ein im Ziel-Modus bearbeiteter Vorschlag (e).
//
// Gemerkt wird nur, wenn der korrigierte Befehl danach ohne Fehler läuft.

import (
	"fmt"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
)

// aiPending: zuletzt eingefügter oder ausgeführter KI-Befehl.
type aiPending struct {
	cmd      string
	at       time.Time
	inserted bool // steht in der Eingabezeile, Enter fehlt noch
}

const learnWindow = 10 * time.Minute

// noteAI merkt sich einen KI-Befehl, der eingefügt (inserted) oder
// ausgeführt wurde.
func (s *session) noteAI(cmd string, inserted bool) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" || strings.Contains(cmd, "\n") {
		s.aiPend = nil
		return
	}
	s.aiPend = &aiPending{cmd: cmd, at: time.Now(), inserted: inserted}
}

// correctionFrom: Enter mit typed am Prompt. Liefert den KI-Befehl, den
// typed korrigiert (oder "").
func (s *session) correctionFrom(typed string) string {
	p := s.aiPend
	typed = strings.TrimSpace(typed)
	if p == nil || typed == "" {
		return ""
	}
	if time.Since(p.at) > learnWindow {
		s.aiPend = nil
		return ""
	}
	if typed == p.cmd {
		// unverändert übernommen: läuft jetzt; schlägt er fehl, zählt der
		// nächste eigene Befehl als Korrektur
		p.inserted, p.at = false, time.Now()
		return ""
	}
	s.aiPend = nil
	if p.inserted {
		if strings.HasPrefix(typed, p.cmd) {
			return "" // nur ergänzt (z. B. einen Pfad angehängt)
		}
	} else if s.fail == nil || !(strings.HasPrefix(s.fail.Command, p.cmd) || strings.HasPrefix(p.cmd, s.fail.Command)) {
		return "" // der KI-Befehl hat geklappt: nichts zu korrigieren
	}
	if !similarCmd(p.cmd, typed) {
		return ""
	}
	return p.cmd
}

// similarCmd: sieht b wie eine Korrektur von a aus (und nicht wie etwas
// ganz anderes)? Entweder kaum verschieden („show interface brief“ →
// „show interfaces brief“) oder sie teilen ein bedeutsames Wort, das nicht
// bloß das gemeinsame erste Wort ist („systemctl status nginx“ → „service
// nginx status“, „docker ps“ → „sudo docker ps“).
func similarCmd(a, b string) bool {
	a, b = strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " ")
	if a == "" || b == "" || a == b {
		return false
	}
	if levenshtein(a, b) <= max(len([]rune(a)), len([]rune(b)))/3 {
		return true
	}
	fa, fb := strings.Fields(a), strings.Fields(b)
	sig := func(w string) bool { return len([]rune(w)) >= 3 && !strings.HasPrefix(w, "-") }
	inA := map[string]bool{}
	for i, w := range fa {
		if sig(w) && !(i == 0 && w == fb[0]) {
			inA[w] = true
		}
	}
	for i, w := range fb {
		if inA[w] && !(i == 0 && w == fa[0]) {
			return true
		}
	}
	return false
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// learnCorrection speichert die Korrektur im Systemprofil.
func (a *App) learnCorrection(s *session, from, to string) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" || to == "" || strings.Contains(from+to, "\n") {
		return
	}
	if agent.Redact(from) != from || agent.Redact(to) != to {
		return // nichts mit Passwörtern oder Tokens merken
	}
	if !s.prof.AddLearned(from, to, time.Now()) {
		return
	}
	a.saveProfile(s)
	a.flashOK(fmt.Sprintf(i18n.T("Gemerkt für %s: „%s“ statt „%s“ · die KI schlägt das künftig so vor"),
		s.conn.Alias, trunc(to, 40), trunc(from, 40)))
}

// learnedFix: gibt es für diesen KI-Befehl schon eine Korrektur?
func (s *session) learnedFix(cmd string) (string, bool) {
	cmd = strings.TrimSpace(cmd)
	for i := len(s.prof.Learned) - 1; i >= 0; i-- {
		if c := s.prof.Learned[i]; c.AI == cmd {
			return c.Fixed, true
		}
	}
	return "", false
}
