package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/llm"
)

// KI-Einstellungen: Endpunkt testen (Strg+T) und Modell aus der Liste des
// Endpunkts wählen (Strg+L). Beides nutzt die Werte, die gerade im Formular
// stehen, auch wenn sie noch nicht gespeichert sind.

type llmTestMsg struct{ res llm.TestResult }

type llmModelsMsg struct {
	models []string
	err    error
}

type modelPick struct {
	all    []string
	filter string
	sel    int
}

type llmUI struct {
	busy   string // läuft gerade: Text für die Statuszeile
	status string // Ergebnis (bereits gestylt)
	models []string
	pick   *modelPick
}

func (p *modelPick) visible() []string {
	f := strings.ToLower(p.filter)
	var out []string
	for _, m := range p.all {
		if f == "" || strings.Contains(strings.ToLower(m), f) {
			out = append(out, m)
		}
	}
	return out
}

// formLLM liest die Formularwerte in eine LLMConfig (ohne zu speichern).
func (a *App) formLLM() domain.LLMConfig {
	f := a.form
	c := a.st.LLM()
	c.BaseURL, c.APIKey, c.Model = f.str("base"), domain.Secret(f.raw("key")), f.str("model")
	c.MaxTokens, c.TimeoutSec = f.num("tokens"), f.num("timeout")
	fmt.Sscanf(f.str("temp"), "%f", &c.Temperature)
	c.SuggestEnabled, c.SuggestDelayMS = f.flag("suggest"), f.num("delay")
	c.SendScreen, c.ContextLines, c.GoalStepsMin = f.flag("screen"), f.num("lines"), f.num("steps")
	c.AllowInsecure = f.flag("insecure")
	return c
}

func testTimeout(c domain.LLMConfig) time.Duration {
	if c.TimeoutSec > 0 && c.TimeoutSec < 30 {
		return time.Duration(c.TimeoutSec) * time.Second
	}
	return 30 * time.Second
}

func (a *App) llmTestCmd() tea.Cmd {
	c := a.formLLM()
	a.llm.busy, a.llm.status = "teste "+c.BaseURL+" …", ""
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout(c))
		defer cancel()
		return llmTestMsg{llm.Test(ctx, c)}
	}
}

func (a *App) llmModelsCmd() tea.Cmd {
	c := a.formLLM()
	a.llm.busy, a.llm.status = "lade Modelle von "+c.BaseURL+" …", ""
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout(c))
		defer cancel()
		m, err := llm.Models(ctx, c)
		return llmModelsMsg{m, err}
	}
}

func (a *App) openModelPick() {
	cur := a.form.str("model")
	p := &modelPick{all: a.llm.models}
	for i, m := range p.all {
		if m == cur {
			p.sel = i
		}
	}
	a.llm.pick = p
}

// llmMsg verarbeitet Ergebnisse der Hintergrund-Abfragen.
func (a *App) llmMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case llmTestMsg:
		a.llm.busy = ""
		if a.scr != scrSettings || a.form == nil {
			return nil, true
		}
		r := m.res
		var parts []string
		if r.ModelsErr == nil {
			a.llm.models = r.Models
			parts = append(parts, a.s.OK.Render(fmt.Sprintf("✓ /models: %d Modelle", len(r.Models))))
		} else {
			parts = append(parts, a.s.Warn.Render("⚠ /models: "+r.ModelsErr.Error()))
		}
		model := a.form.str("model")
		switch {
		case r.ChatErr != nil:
			parts = append(parts, a.s.Danger.Render("✗ Chat: "+r.ChatErr.Error()))
		default:
			parts = append(parts, a.s.OK.Render(fmt.Sprintf("✓ Chat mit %s: %d ms, Antwort „%s“", model, r.Latency.Milliseconds(), trunc(oneLineUI(r.Reply), 30))))
		}
		if r.ModelsErr == nil && model != "" && !contains(r.Models, model) {
			parts = append(parts, a.s.Warn.Render("⚠ „"+model+"“ steht nicht in der Liste des Endpunkts · Strg+L wählen"))
		}
		a.llm.status = strings.Join(parts, "\n")
		return nil, true
	case llmModelsMsg:
		a.llm.busy = ""
		if a.scr != scrSettings || a.form == nil {
			return nil, true
		}
		if m.err != nil {
			a.llm.status = a.s.Danger.Render("✗ Modelle: " + m.err.Error())
			return nil, true
		}
		if len(m.models) == 0 {
			a.llm.status = a.s.Warn.Render("⚠ Der Endpunkt meldet keine Modelle; Namen bitte von Hand eintragen.")
			return nil, true
		}
		a.llm.models, a.llm.status = m.models, ""
		a.openModelPick()
		return nil, true
	}
	return nil, false
}

// settingsKey fängt Strg+T, Strg+L und die Modell-Liste ab, bevor das
// Formular die Taste bekommt.
func (a *App) settingsKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	if p := a.llm.pick; p != nil {
		vis := p.visible()
		switch k.String() {
		case "esc":
			a.llm.pick = nil
		case "up":
			p.sel = max(0, p.sel-1)
		case "down":
			p.sel = min(len(vis)-1, p.sel+1)
		case "enter", "tab":
			if p.sel < len(vis) {
				fl := a.form.get("model")
				fl.in.SetValue(vis[p.sel])
				fl.in.CursorEnd()
				a.llm.status = a.s.OK.Render("Modell „" + vis[p.sel] + "“ gewählt · Strg+T testen · Strg+S speichern")
			}
			a.llm.pick = nil
		case "backspace":
			if r := []rune(p.filter); len(r) > 0 {
				p.filter, p.sel = string(r[:len(r)-1]), 0
			}
		default:
			if k.Text != "" {
				p.filter, p.sel = p.filter+k.Text, 0
			}
		}
		return nil, true
	}
	switch k.String() {
	case "ctrl+t":
		return a.llmTestCmd(), true
	case "ctrl+l":
		if len(a.llm.models) > 0 {
			a.openModelPick()
			return nil, true
		}
		return a.llmModelsCmd(), true
	}
	return nil, false
}

func (a *App) viewLLMExtra(w int) string {
	var b strings.Builder
	if a.llm.busy != "" {
		b.WriteString("\n" + a.s.AI.Render("✦ "+a.llm.busy) + "\n")
	}
	if a.llm.status != "" {
		b.WriteString("\n" + a.llm.status + "\n")
	}
	if p := a.llm.pick; p != nil {
		vis := p.visible()
		if p.sel >= len(vis) {
			p.sel = max(0, len(vis)-1)
		}
		b.WriteString("\n" + a.s.Accent.Render(fmt.Sprintf("Modelle des Endpunkts (%d)", len(p.all))) + "  " +
			a.s.Muted.Render("Filter: ") + a.s.Text.Render(p.filter+"▏") + "\n")
		const rows = 8
		start := 0
		if p.sel >= rows {
			start = p.sel - rows + 1
		}
		cur := a.form.str("model")
		for i := start; i < len(vis) && i < start+rows; i++ {
			mark := "  "
			if vis[i] == cur {
				mark = a.s.OK.Render("● ")
			}
			line := mark + trunc(vis[i], w-6)
			if i == p.sel {
				line = a.s.Selected.Width(w - 2).Render("  " + trunc(vis[i], w-6))
			}
			b.WriteString(line + "\n")
		}
		if len(vis) == 0 {
			b.WriteString(a.s.Muted.Render("  kein Treffer") + "\n")
		}
		b.WriteString(a.s.Muted.Render("↑↓ wählen · ⏎ übernehmen · tippen filtert · Esc schließen") + "\n")
	}
	return b.String()
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func oneLineUI(s string) string { return strings.Join(strings.Fields(s), " ") }
