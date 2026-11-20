package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/doctor"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/llm"
)

// Selbstprüfung (internal/doctor) aus der Palette: wie „sshtermy doctor“,
// dazu der KI-Endpunkt (dafür muss der Tresor offen sein). Das Ergebnis
// steht in einem Textfenster, c kopiert es für den Fehlerbericht.

type doctorMsg struct{ text string }

func (a *App) runDoctor() tea.Cmd {
	a.flash(i18n.T("Selbstprüfung läuft …"))
	dir := a.st.Dir
	chain := a.st.LLM().Chain()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		rs := doctor.Run(ctx, doctor.Env{Dir: dir, Extra: []doctor.Check{aiCheck(chain[0])}})
		return doctorMsg{doctor.Format(rs)}
	}
}

// aiCheck: antwortet der eingestellte KI-Endpunkt? (ohne Schlüssel im Text)
func aiCheck(cfg domain.LLMConfig) doctor.Check {
	return func(ctx context.Context) doctor.Result {
		name := i18n.T("KI")
		if cfg.Model == "" && cfg.BaseURL == "" {
			return doctor.Result{Name: name, Status: doctor.Info, Detail: i18n.T("nicht eingerichtet")}
		}
		what := cfg.Model
		if what == "" {
			what = string(cfg.Provider)
		}
		cctx, cancel := context.WithTimeout(ctx, testTimeout(cfg))
		defer cancel()
		if err := llm.Probe(cctx, cfg); err != nil {
			return doctor.Result{Name: name, Status: doctor.Warn, Detail: what + i18n.T(" antwortet nicht: ") + err.Error()}
		}
		return doctor.Result{Name: name, Status: doctor.OK, Detail: what + i18n.T(" antwortet")}
	}
}

func (a *App) doctorDone(m doctorMsg) {
	md := textModal(i18n.T("Selbstprüfung"), strings.Split(strings.TrimRight(m.text, "\n"), "\n"))
	md.copy = m.text
	a.modal = md
}
