package ui

// Überwachung light: Grenzwerte für CPU, RAM und Platte (Host-Übersicht F6).
// Server fragt sshTermy im eingestellten Abstand still nach (hostpanel.go
// monitorTick); liegt eine Kennzahl auf oder über ihrer Schwelle, erscheint
// einmal ein gelber Toast, die Plakette in der Sitzungsleiste wird gelb und
// die Host-Übersicht zeigt die Warnung. Netzwerkgeräte werden nur auf Abruf
// geprüft, weil sshTermy dort sichtbar am Prompt tippt.

import (
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

func (a *App) openMonitorForm() tea.Cmd {
	m := a.st.Settings().Monitor
	c, r, d := m.Limits()
	every := int(m.Every().Minutes())
	f := &form{title: i18n.T("Überwachung: Grenzwerte")}
	f.number("cpu", i18n.T("CPU ab %"), c, i18n.T("Auslastung über eine Sekunde gemessen · Standard 90"))
	f.number("ram", i18n.T("RAM ab %"), r, i18n.T("belegter Arbeitsspeicher (ohne Cache) · Standard 90"))
	f.number("disk", i18n.T("Platte ab %"), d, i18n.T("gilt für jede Platte einzeln · Standard 90"))
	f.number("every", i18n.T("Prüfen alle (Min)"), every, i18n.T("nur Server, unsichtbar · 0 = aus · Switches nur auf Abruf"))
	if every == 0 {
		f.get("every").in.SetValue("0")
	}
	a.returnToTerminal = a.scr == scrTerminal
	a.form, a.scr = f, scrMonitorForm
	return f.setFocus(0)
}

func (a *App) saveMonitor() error {
	f := a.form
	var m domain.MonitorConfig
	for _, x := range []struct {
		key string
		dst *int
	}{{"cpu", &m.CPU}, {"ram", &m.RAM}, {"disk", &m.Disk}} {
		v := f.num(x.key)
		if v < 1 || v > 100 {
			return errors.New(i18n.T("Grenzwerte zwischen 1 und 100 %"))
		}
		if v != domain.DefaultMonitorLimit {
			*x.dst = v
		}
	}
	switch e := f.num("every"); {
	case e <= 0:
		m.EveryMin = -1
	case e > 24*60:
		return errors.New(i18n.T("Höchstens alle 1440 Minuten"))
	case e != domain.DefaultMonitorEvery:
		m.EveryMin = e
	}
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Monitor = m }); err != nil {
		return err
	}
	// sofort neu bewerten
	for _, s := range a.sessions {
		if s.host.srv != nil {
			s.host.alerts = s.host.srv.Alerts(a.monLimits())
		} else if s.host.sw != nil {
			s.host.alerts = s.host.sw.Alerts(a.monLimits())
		}
	}
	return nil
}
