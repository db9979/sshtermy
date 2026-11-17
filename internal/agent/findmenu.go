package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/db9979/sshtermy/internal/llm"
)

// TaskMenu: „Wo finde ich …?“ in der Befehlspalette.
const TaskMenu = "menu"

const sysFindMenu = "Du hilfst in der Terminal-App sshTermy, einen Menüeintrag zu finden. Du bekommst die Frage des Nutzers und " +
	"eine nummerierte Liste aller Einträge der Befehlspalette (Weg › Eintrag). Wähle den Eintrag, der die Frage am besten " +
	"beantwortet. Antworte genau in zwei Zeilen: „Nr: <Nummer>“ und „Warum: <ein kurzer Satz, Deutsch>“. Passt keiner, " +
	"antworte „Nr: 0“ und sag in „Warum:“, was fehlt. Erfinde keine Einträge." + guard

// MenuPick: Antwort auf „Wo finde ich …?“. Index zählt ab 0, -1 = keiner.
type MenuPick struct {
	Index int
	Why   string
}

// FindMenu sucht zu einer Frage den passenden Eintrag der Befehlspalette.
// Die KI bekommt nur die Frage und die Liste der Einträge, keine Daten aus
// Sitzungen oder Verbindungen.
func (a *Agent) FindMenu(ctx context.Context, question string, entries []string) (MenuPick, error) {
	var b strings.Builder
	for i, e := range entries {
		fmt.Fprintf(&b, "%d. %s\n", i+1, e)
	}
	q := strings.NewReplacer("<<<", "‹‹‹", ">>>", "›››").Replace(question)
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: a.sys(sysFindMenu)},
		{Role: "user", Content: "Einträge:\n<<<\n" + b.String() + ">>>\n\nFrage des Nutzers:\n<<<\n" + q + "\n>>>"},
	}, llm.Opts{MaxTokens: 200, Temperature: 0, NoThink: true, Fast: true, Task: TaskMenu})
	if err != nil {
		return MenuPick{Index: -1}, err
	}
	return ParseMenuPick(stripThink(res.Content), len(entries)), nil
}

// ParseMenuPick liest „Nr: 12“ und „Warum: …“ aus der Antwort.
func ParseMenuPick(s string, n int) MenuPick {
	p := MenuPick{Index: -1}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.NewReplacer("*", "", "`", "").Replace(line))
		low := strings.ToLower(line)
		switch {
		case strings.HasPrefix(low, "nr") || strings.HasPrefix(low, "no") || strings.HasPrefix(low, "nummer"):
			_, v, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			v = strings.Trim(strings.TrimSpace(v), ".#")
			if f := strings.Fields(v); len(f) > 0 {
				if k, err := strconv.Atoi(f[0]); err == nil && k >= 1 && k <= n {
					p.Index = k - 1
				}
			}
		case strings.HasPrefix(low, "warum") || strings.HasPrefix(low, "why"):
			if _, v, ok := strings.Cut(line, ":"); ok {
				p.Why = strings.TrimSpace(v)
			}
		}
	}
	return p
}
