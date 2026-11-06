package ui

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/llm"
)

// KI-Anbieter: Der Hauptendpunkt ist frei wählbar (Spark bzw. jeder
// OpenAI-kompatible Server, Ollama, OpenAI, Claude). Bis zu drei
// Ersatz-Anbieter springen der Reihe nach ein, wenn er nicht antwortet. Das
// Formular für die Ersatz-Anbieter ist ein eigenes, damit die Einstellungen
// auch in kleinen Fenstern auf den Schirm passen.

const maxFallbacks = 3

// labelOff: Auswahl „kein Ersatz“ in einem Platz.
const labelOff = "aus"

func providerLabels(withOff bool) []string {
	var out []string
	if withOff {
		out = append(out, labelOff)
	}
	// Für den Ersatz zuerst die typischen Dienste, der Spark steht oben schon.
	order := domain.Providers
	if withOff {
		order = []domain.LLMProvider{domain.ProviderOllama, domain.ProviderOpenAI, domain.ProviderAnthropic, domain.ProviderCompat}
	}
	for _, p := range order {
		out = append(out, p.Label())
	}
	return out
}

func providerFromLabel(l string) domain.LLMProvider {
	for _, p := range domain.Providers {
		if p.Label() == l {
			return p
		}
	}
	return domain.ProviderCompat
}

func fbKey(i int, name string) string { return fmt.Sprintf("fb%d_%s", i, name) }

// fallbackSummary: „Ollama → Claude“ für die Zeile im Hauptformular.
func (a *App) fallbackSummary(c domain.LLMConfig) string {
	var names []string
	for _, e := range c.Chain()[1:] {
		names = append(names, i18n.T(e.Provider.Label()))
	}
	if len(names) == 0 {
		return a.s.Muted.Render(i18n.T("keine · ⏎ einrichten"))
	}
	return strings.Join(names, " → ")
}

// settingsToFallbacks: Hauptformular speichern, dann zu den Ersatz-Anbietern.
func (a *App) settingsToFallbacks() tea.Cmd {
	c := a.formLLM()
	if err := checkLLM(c); err != nil {
		a.form.err = err.Error()
		return nil
	}
	if err := a.st.SetLLM(c); err != nil {
		a.form.err = err.Error()
		return nil
	}
	back := a.returnToTerminal
	cmd := a.openFallbackForm()
	a.returnToTerminal = back
	a.flash(i18n.T("Gespeichert (verschlüsselt)"))
	return cmd
}

func (a *App) openFallbackForm() tea.Cmd {
	c := a.st.LLM()
	f := &form{title: fmt.Sprintf(i18n.T("Ersatz-Anbieter · springen der Reihe nach ein, wenn %s nicht antwortet"), c.Model+" @ "+hostOf(c.BaseURL))}
	for i := 1; i <= maxFallbacks; i++ {
		var b domain.LLMBackup
		if i <= len(c.Fallbacks) {
			b = c.Fallbacks[i-1]
		}
		cur := labelOff
		if b.Model != "" || b.BaseURL != "" {
			cur = b.Provider.Label()
		}
		f.choice(fbKey(i, "prov"), fmt.Sprintf(i18n.T("Ersatz %d"), i), providerLabels(true), cur, i18n.T("←/→ wählt den Anbieter · aus = kein Ersatz"))
		i := i
		off := func(f *form) bool { return f.sel(fbKey(i, "prov")) == labelOff }
		f.text(fbKey(i, "base"), i18n.T("  Base-URL"), b.BaseURL, i18n.T("leer = Standardadresse des Anbieters")).hidden = off
		f.secret(fbKey(i, "key"), i18n.T("  API-Key"), b.APIKey.Reveal(), i18n.T("verschlüsselt gespeichert; für OpenAI und Claude nötig")).hidden = func(f *form) bool {
			return off(f) || f.sel(fbKey(i, "prov")) == domain.ProviderOllama.Label()
		}
		f.text(fbKey(i, "model"), i18n.T("  Modell"), b.Model, i18n.T("Strg+L: Liste vom Anbieter · Strg+T: testen")).hidden = off
	}
	a.form, a.scr = f, scrSettings
	a.llm = llmUI{fallback: true}
	a.llm.rememberProviders(f)
	return f.setFocus(0)
}

// formFallbacks liest die Plätze des Ersatz-Formulars.
func (a *App) formFallbacks() []domain.LLMBackup {
	f := a.form
	var out []domain.LLMBackup
	for i := 1; i <= maxFallbacks; i++ {
		l := f.sel(fbKey(i, "prov"))
		if l == labelOff || l == "" {
			continue
		}
		p := providerFromLabel(l)
		b := domain.LLMBackup{Provider: p, BaseURL: f.str(fbKey(i, "base")), APIKey: domain.Secret(strings.TrimSpace(f.raw(fbKey(i, "key")))), Model: f.str(fbKey(i, "model"))}
		if p == domain.ProviderOllama {
			b.APIKey = "" // Feld ist bei Ollama ausgeblendet
		}
		if b.BaseURL == "" {
			b.BaseURL = p.DefaultBaseURL()
		}
		out = append(out, b)
	}
	return out
}

func (a *App) saveFallbacks() error {
	c := a.formLLM()
	for i, e := range c.Chain()[1:] {
		if err := llm.Check(e); err != nil {
			return fmt.Errorf(i18n.T("Ersatz %d (%s): %v"), i+1, i18n.T(e.Provider.Label()), err)
		}
	}
	if len(c.Chain())-1 != len(c.Fallbacks) {
		return errors.New(i18n.T("Ersatz ohne Modell: bitte ein Modell eintragen oder auf „aus“ stellen"))
	}
	return a.st.SetLLM(c)
}

// focusSlot: welcher Ersatz-Platz gerade den Fokus hat (0 = Hauptformular).
func (a *App) focusSlot() int {
	if !a.llm.fallback || a.form == nil {
		return 0
	}
	key := a.form.fields[a.form.focus].key
	var i int
	if _, err := fmt.Sscanf(key, "fb%d_", &i); err == nil {
		return i
	}
	return 1
}

// modelKey: das Modell-Feld, auf das Strg+L und Strg+T wirken.
func (a *App) modelKey() string {
	if i := a.focusSlot(); i > 0 {
		return fbKey(i, "model")
	}
	return "model"
}

// focusedLLM: die Einstellung, die Strg+T/Strg+L gerade meinen (im
// Ersatz-Formular der Platz unter dem Cursor). ok = false bei „aus“.
func (a *App) focusedLLM() (domain.LLMConfig, bool) {
	c := a.formLLM()
	i := a.focusSlot()
	if i == 0 {
		c.Fallbacks = nil
		return c, true
	}
	f := a.form
	l := f.sel(fbKey(i, "prov"))
	if l == labelOff {
		return c, false
	}
	p := providerFromLabel(l)
	c.Fallbacks = nil
	c.Provider, c.BaseURL, c.APIKey, c.Model = p, f.str(fbKey(i, "base")), domain.Secret(strings.TrimSpace(f.raw(fbKey(i, "key")))), f.str(fbKey(i, "model"))
	if p == domain.ProviderOllama {
		c.APIKey = ""
	}
	if c.BaseURL == "" {
		c.BaseURL = p.DefaultBaseURL()
	}
	c.AllowInsecure = false
	return c, true
}

// providerFields: Anbieter-Feld → (Adresse, Modell) im aktuellen Formular.
func (a *App) providerFields() map[string][2]string {
	if a.llm.fallback {
		m := map[string][2]string{}
		for i := 1; i <= maxFallbacks; i++ {
			m[fbKey(i, "prov")] = [2]string{fbKey(i, "base"), fbKey(i, "model")}
		}
		return m
	}
	return map[string][2]string{"prov": {"base", "model"}}
}

func (u *llmUI) rememberProviders(f *form) {
	u.prev = map[string]string{}
	for _, fl := range f.fields {
		if fl.kind == fChoice && (fl.key == "prov" || strings.HasSuffix(fl.key, "_prov")) {
			u.prev[fl.key] = f.sel(fl.key)
		}
	}
}

// isDefaultValue: leer oder die Vorbelegung irgendeines Anbieters, also
// nichts, was der Nutzer selbst eingetragen hat.
func isDefaultValue(v string, model bool) bool {
	if v == "" {
		return true
	}
	for _, p := range domain.Providers {
		d := p.DefaultBaseURL()
		if model {
			d = p.DefaultModel()
		}
		if d != "" && strings.TrimRight(v, "/") == d {
			return true
		}
	}
	return false
}

// applyProviderDefaults: Wechselt der Anbieter, werden Adresse und Modell
// vorbelegt. Was beim vorigen Anbieter stand, wird gemerkt und beim
// Zurückblättern wieder eingesetzt (die eigene Spark-Adresse geht beim
// Durchklicken mit ←/→ nicht verloren). Die Modell-Liste gilt dann nicht mehr.
func (a *App) applyProviderDefaults() {
	f := a.form
	if a.llm.saved == nil {
		a.llm.saved = map[string][2]string{}
	}
	for pk, fs := range a.providerFields() {
		cur, old := f.sel(pk), a.llm.prev[pk]
		if cur == old {
			continue
		}
		a.llm.prev[pk] = cur
		base, model := f.get(fs[0]), f.get(fs[1])
		a.llm.saved[pk+"|"+old] = [2]string{base.in.Value(), model.in.Value()}
		a.llm.models, a.llm.modelsFor, a.llm.status = nil, "", ""
		if cur == labelOff {
			continue
		}
		p := providerFromLabel(cur)
		set := func(fl *field, val string) {
			fl.in.SetValue(val)
			fl.in.CursorEnd()
		}
		if v, ok := a.llm.saved[pk+"|"+cur]; ok {
			set(base, v[0])
			set(model, v[1])
		} else {
			// Cloud-Dienste haben genau eine Adresse; sonst nur Vorbelegungen ersetzen.
			if p.DefaultBaseURL() != "" && (p.NeedsKey() || isDefaultValue(strings.TrimSpace(base.in.Value()), false)) {
				set(base, p.DefaultBaseURL())
			}
			if p.DefaultModel() != "" && (p.NeedsKey() || isDefaultValue(strings.TrimSpace(model.in.Value()), true)) {
				set(model, p.DefaultModel())
			}
		}
		if p.NeedsKey() {
			a.llm.status = a.s.Muted.Render(fmt.Sprintf(i18n.T("%s braucht einen API-Key. Strg+L lädt danach die Modelle."), i18n.T(p.Label())))
		}
	}
}

// hostOf: nur Rechner und Port einer Base-URL (für kurze Überschriften).
func hostOf(base string) string {
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		return u.Host
	}
	return base
}
